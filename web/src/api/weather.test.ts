import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { reportConnection } from './connection';
import { weather } from './index';

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

describe('weather', () => {
  it('get — GET /api/weather', async () => {
    const body = { enabled: false, location: null, fetched_at: null, stale: false, days: [], today_hours: [], best: { wash: null, coat: null } };
    const calls = stubFetch(() => json(200, body));
    expect(await weather.get()).toEqual(body);
    expect(calls[0]?.url).toBe('/api/weather');
    expect(header(calls[0]!, 'X-Requested-With')).toBeNull();
  });

  it('502 — ApiError с текстом сервера', async () => {
    stubFetch(() => json(502, { error: 'Прогноз сейчас недоступен' }));
    await expect(weather.get()).rejects.toMatchObject({ name: 'ApiError', status: 502, message: 'Прогноз сейчас недоступен' });
  });

  it('places — запрос кодируется, ответ — массив', async () => {
    const places = [{ name: 'Донецк', admin1: 'Донецкая область', country: 'Украина', lat: 48, lon: 37.8 }];
    const calls = stubFetch(() => json(200, { places }));
    expect(await weather.places('Дон ец&')).toEqual(places);
    expect(calls[0]?.url).toBe(`/api/weather/places?q=${encodeURIComponent('Дон ец&')}`);
  });

  it('places 429 — ApiError со статусом и ожиданием', async () => {
    stubFetch(() => json(429, { error: 'Слишком часто', retry_after: 5 }));
    await expect(weather.places('Дон')).rejects.toMatchObject({ name: 'ApiError', status: 429, retryAfter: 5 });
  });

  it('setLocation — PUT с телом и X-Requested-With', async () => {
    const loc = { name: 'Донецк', lat: 48.02, lon: 37.8, custom: true };
    const calls = stubFetch(() => json(200, loc));
    expect(await weather.setLocation({ name: 'Донецк', lat: 48.02, lon: 37.8 })).toEqual(loc);
    expect(calls[0]?.init.method).toBe('PUT');
    expect(calls[0]?.url).toBe('/api/weather/location');
    expect(JSON.parse(String(calls[0]?.init.body))).toEqual({ name: 'Донецк', lat: 48.02, lon: 37.8 });
    expect(header(calls[0]!, 'X-Requested-With')).toBe('garage');
  });

  it('resetLocation — DELETE, 204', async () => {
    const calls = stubFetch(() => new Response(null, { status: 204 }));
    expect(await weather.resetLocation()).toBeUndefined();
    expect(calls[0]?.init.method).toBe('DELETE');
    expect(header(calls[0]!, 'X-Requested-With')).toBe('garage');
  });
});
