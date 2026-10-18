import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, NetworkError, request } from './client';
import { isOnline, reportConnection } from './connection';
import { api } from './index';

type Call = { url: string; init: RequestInit };

function stubFetch(respond: (call: Call) => Response | Promise<Response>) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit) => {
    const call = { url, init };
    calls.push(call);
    return respond(call);
  }));
  return calls;
}

const json = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } });

const header = (call: Call, name: string) => new Headers(call.init.headers).get(name);

beforeEach(() => reportConnection(true));
afterEach(() => vi.unstubAllGlobals());

describe('заголовки', () => {
  it('GET без X-Requested-With', async () => {
    const calls = stubFetch(() => json(200, { user: { id: 1, login: 'a', is_owner: true }, settings: { step_small: 10, step_large: 50 } }));
    const s = await api.me();
    expect(s.user.login).toBe('a');
    expect(calls[0]!.url).toBe('/api/auth/me');
    expect(calls[0]!.init.method).toBe('GET');
    expect(header(calls[0]!, 'X-Requested-With')).toBeNull();
    expect(calls[0]!.init.body).toBeUndefined();
  });

  it('POST с X-Requested-With: garage и JSON-телом', async () => {
    const calls = stubFetch(() => json(200, { user: {}, settings: {} }));
    await api.login('ringino', 'secret123');
    const c = calls[0]!;
    expect(c.init.method).toBe('POST');
    expect(header(c, 'X-Requested-With')).toBe('garage');
    expect(header(c, 'Content-Type')).toBe('application/json');
    expect(JSON.parse(c.init.body as string)).toEqual({ login: 'ringino', password: 'secret123' });
  });

  it('PUT и DELETE тоже несут заголовок', async () => {
    const calls = stubFetch((c) => (c.init.method === 'DELETE' ? new Response(null, { status: 204 }) : json(200, { step_small: 5, step_large: 25 })));
    await expect(api.saveSettings({ step_small: 5, step_large: 25 })).resolves.toEqual({ step_small: 5, step_large: 25 });
    await expect(api.revokeInvite(7)).resolves.toBeUndefined();
    expect(calls.map((c) => [c.init.method, c.url, header(c, 'X-Requested-With')])).toEqual([
      ['PUT', '/api/auth/settings', 'garage'],
      ['DELETE', '/api/invites/7', 'garage'],
    ]);
  });

  it('смена пароля отправляет поле new, токен приглашения экранируется', async () => {
    const calls = stubFetch(() => new Response(null, { status: 204 }));
    await api.changePassword('old', 'newpass12');
    expect(JSON.parse(calls[0]!.init.body as string)).toEqual({ current: 'old', new: 'newpass12' });
    const calls2 = stubFetch(() => json(200, { expires_at: 1 }));
    await api.checkInvite('a/b?c');
    expect(calls2[0]!.url).toBe('/api/invites/by-token/a%2Fb%3Fc');
  });
});

describe('ошибки API', () => {
  it('разбирает error и field', async () => {
    stubFetch(() => json(403, { error: 'Текущий пароль неверный', field: 'current' }));
    const err = await api.changePassword('x', 'yyyyyyyy').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 403, message: 'Текущий пароль неверный', field: 'current' });
    expect(isOnline()).toBe(true);
  });

  it('401 — ApiError, а не «нет связи»', async () => {
    stubFetch(() => json(401, { error: 'Нужно войти' }));
    const err = await api.me().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(401);
  });

  it('429 — retry_after из тела', async () => {
    stubFetch(() => json(429, { error: 'Слишком много попыток. Повторите через 12 минут', retry_after: 720 }, { 'Retry-After': '720' }));
    const err = (await api.login('a', 'b').catch((e: unknown) => e)) as ApiError;
    expect(err.status).toBe(429);
    expect(err.retryAfter).toBe(720);
  });

  it('429 — retry_after из заголовка, если в теле нет', async () => {
    stubFetch(() => json(429, { error: 'Слишком много попыток' }, { 'Retry-After': '90' }));
    const err = (await api.login('a', 'b').catch((e: unknown) => e)) as ApiError;
    expect(err.retryAfter).toBe(90);
  });

  it('тело не JSON — текст по коду', async () => {
    stubFetch(() => new Response('oops', { status: 400 }));
    const err = (await request('/api/x').catch((e: unknown) => e)) as ApiError;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.message).toBe('Сервер ответил 400');
    expect(err.field).toBeUndefined();
  });

  it('500 с телом ошибки garage — ApiError', async () => {
    stubFetch(() => json(500, { error: 'Внутренняя ошибка' }));
    await expect(request('/api/x')).rejects.toBeInstanceOf(ApiError);
  });
});

describe('нет связи', () => {
  it('fetch упал — NetworkError и флаг связи', async () => {
    stubFetch(() => Promise.reject(new TypeError('Failed to fetch')));
    await expect(api.me()).rejects.toBeInstanceOf(NetworkError);
    expect(isOnline()).toBe(false);
  });

  it.each([502, 503, 504])('%i от nginx — NetworkError', async (status) => {
    stubFetch(() => new Response('<html>Bad Gateway</html>', { status }));
    await expect(api.me()).rejects.toBeInstanceOf(NetworkError);
  });

  it('вид: 502/503 и упавший fetch — не дошло, 504 — ответ не пришёл', async () => {
    const kind = async (respond: () => Response | Promise<Response>) => {
      stubFetch(respond);
      return (await api.logout().catch((e: unknown) => e)) as NetworkError;
    };
    expect((await kind(() => new Response('', { status: 502 }))).kind).toBe('offline');
    expect((await kind(() => new Response('', { status: 503 }))).maybeSaved).toBe(false);
    expect((await kind(() => Promise.reject(new TypeError('Failed to fetch')))).kind).toBe('offline');
    const gw = await kind(() => new Response('', { status: 504 }));
    expect(gw.kind).toBe('timeout');
    expect(gw.maybeSaved).toBe(true);
  });

  it('500 без JSON-тела (прокси) — NetworkError', async () => {
    stubFetch(() => new Response('', { status: 500 }));
    await expect(api.me()).rejects.toBeInstanceOf(NetworkError);
  });

  it('удачный ответ возвращает связь', async () => {
    reportConnection(false);
    stubFetch(() => json(200, { status: 'ok', version: 'dev' }));
    await api.health();
    expect(isOnline()).toBe(true);
  });

  it('отмена вызывающим — не «нет связи»', async () => {
    stubFetch((c) => new Promise((_, reject) => {
      c.init.signal!.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
    }));
    const ctrl = new AbortController();
    const p = api.me(ctrl.signal);
    ctrl.abort();
    const err = await p.catch((e: unknown) => e);
    expect(err).not.toBeInstanceOf(NetworkError);
    expect(isOnline()).toBe(true);
  });
});

describe('ответы', () => {
  it('204 — undefined', async () => {
    stubFetch(() => new Response(null, { status: 204 }));
    await expect(api.logout()).resolves.toBeUndefined();
  });

  it('список приглашений разворачивается из {invites}', async () => {
    stubFetch(() => json(200, { invites: [{ id: 1, created_at: 1, expires_at: 2, status: 'active' }] }));
    await expect(api.invites()).resolves.toEqual([{ id: 1, created_at: 1, expires_at: 2, status: 'active' }]);
  });
});
