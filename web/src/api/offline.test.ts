import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, NetworkError } from './client';
import { reportConnection } from './connection';
import { inventory } from './index';

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

beforeEach(() => reportConnection(true));
afterEach(() => vi.unstubAllGlobals());

describe('работа без связи в API', () => {
  it('расход несёт номер операции и время', async () => {
    const fetchMock = vi.fn(async () => json(201, {}));
    vi.stubGlobal('fetch', fetchMock);
    await inventory.consume({ product_id: 1, qty: 5, client_op_id: 'abcdefgh', occurred_at: 123 });
    const init = (fetchMock.mock.calls[0] as unknown as [string, RequestInit])[1];
    expect(JSON.parse(init.body as string)).toEqual({ product_id: 1, qty: 5, client_op_id: 'abcdefgh', occurred_at: 123 });
  });

  it('409 нехватки отдаёт available', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(409, { error: 'Не хватает: есть 399 мл', available: 399 })));
    const err = await inventory.consume({ product_id: 1, qty: 500 }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).available).toBe(399);
  });

  it('другой 409 без available', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(409, { error: 'номер занят', field: 'client_op_id' })));
    const err = (await inventory.consume({ product_id: 1, qty: 5 }).catch((e: unknown) => e)) as ApiError;
    expect(err.available).toBeUndefined();
  });

  it('без связи изменения (кроме расхода) не ждут сети, расход идёт', async () => {
    const fetchMock = vi.fn(async () => json(200, { ok: 1 }));
    vi.stubGlobal('fetch', fetchMock);
    reportConnection(false);
    await expect(inventory.createStore({ name: 'a', url: '' })).rejects.toBeInstanceOf(NetworkError);
    expect(fetchMock).not.toHaveBeenCalled();
    await inventory.consume({ product_id: 1, qty: 5 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
