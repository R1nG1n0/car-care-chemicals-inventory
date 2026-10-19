import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { reportConnection } from './connection';
import { telegram } from './index';

type Call = { url: string; init: RequestInit };

function stubFetch(respond: () => Response) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    return respond();
  }));
  return calls;
}

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const header = (call: Call, name: string) => new Headers(call.init.headers).get(name);

beforeEach(() => reportConnection(true));
afterEach(() => vi.unstubAllGlobals());

describe('telegram', () => {
  it('status — GET без X-Requested-With', async () => {
    const body = { enabled: true, bot_username: 'b', linked: false, telegram_name: null, linked_at: null };
    const calls = stubFetch(() => json(200, body));
    expect(await telegram.status()).toEqual(body);
    expect(calls[0]?.url).toBe('/api/telegram');
    expect(header(calls[0]!, 'X-Requested-With')).toBeNull();
  });

  it('link — POST с X-Requested-With, возвращает url и срок', async () => {
    const calls = stubFetch(() => json(200, { url: 'https://t.me/b?start=x', expires_at: 1234 }));
    expect(await telegram.link()).toEqual({ url: 'https://t.me/b?start=x', expires_at: 1234 });
    expect(calls[0]?.url).toBe('/api/telegram/link');
    expect(calls[0]?.init.method).toBe('POST');
    expect(header(calls[0]!, 'X-Requested-With')).toBe('garage');
  });

  it('link при выключенном боте — 503 с текстом сервера, не «нет связи»', async () => {
    stubFetch(() => json(503, { error: 'Бот не настроен' }));
    await expect(telegram.link()).rejects.toMatchObject({ name: 'ApiError', status: 503, message: 'Бот не настроен' });
  });

  it('503 без JSON (nginx) остаётся «нет связи»', async () => {
    stubFetch(() => new Response('<html>503</html>', { status: 503 }));
    await expect(telegram.status()).rejects.toMatchObject({ name: 'NetworkError' });
  });

  it('unlink — DELETE, ответ 204', async () => {
    const calls = stubFetch(() => new Response(null, { status: 204 }));
    expect(await telegram.unlink()).toBeUndefined();
    expect(calls[0]?.init.method).toBe('DELETE');
    expect(header(calls[0]!, 'X-Requested-With')).toBe('garage');
  });
});
