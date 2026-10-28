import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { reportConnection } from './connection';
import { inventory } from './index';

type Call = { url: string; init: RequestInit };

function stubFetch(body: unknown, status = 200) {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, init });
      return new Response(status === 204 ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
    }),
  );
  return calls;
}

beforeEach(() => reportConnection(true));
afterEach(() => vi.unstubAllGlobals());

describe('назначения и оценка (ФТ-15)', () => {
  it('purposes: GET с архивом и без', async () => {
    const calls = stubFetch({ purposes: [{ id: 1, name: 'ЛКП', sort: 1, archived: false }] });
    expect(await inventory.purposes(true)).toEqual([{ id: 1, name: 'ЛКП', sort: 1, archived: false }]);
    await inventory.purposes();
    expect(calls[0]?.url).toBe('/api/purposes?archived=1');
    expect(calls[1]?.url).toBe('/api/purposes');
  });
  it('createPurpose, updatePurpose, reorderPurposes', async () => {
    const calls = stubFetch({ id: 4, name: 'Кожа', sort: 4, archived: false });
    await inventory.createPurpose({ name: 'Кожа' });
    await inventory.updatePurpose(4, { archived: true });
    expect(calls[0]).toMatchObject({ url: '/api/purposes' });
    expect(calls[0]?.init.method).toBe('POST');
    expect(JSON.parse(String(calls[0]?.init.body))).toEqual({ name: 'Кожа' });
    expect(calls[1]?.url).toBe('/api/purposes/4');
    expect(calls[1]?.init.method).toBe('PATCH');
    expect(JSON.parse(String(calls[1]?.init.body))).toEqual({ archived: true });
    const c2 = stubFetch(undefined, 204);
    await inventory.reorderPurposes([3, 1, 2]);
    expect(c2[0]?.url).toBe('/api/purposes/reorder');
    expect(JSON.parse(String(c2[0]?.init.body))).toEqual({ ids: [3, 1, 2] });
  });
  it('setRating: PATCH /rating, rating: null снимает оценку', async () => {
    const calls = stubFetch({ id: 7, rating: null, do_not_buy: true });
    await inventory.setRating(7, { rating: null, do_not_buy: true });
    expect(calls[0]?.url).toBe('/api/products/7/rating');
    expect(calls[0]?.init.method).toBe('PATCH');
    expect(JSON.parse(String(calls[0]?.init.body))).toEqual({ rating: null, do_not_buy: true });
  });
  it('analogs: список из analogs (как отдаёт сервер)', async () => {
    const calls = stubFetch({ analogs: [{ id: 2 }] });
    expect(await inventory.analogs(7)).toEqual([{ id: 2 }]);
    expect(calls[0]?.url).toBe('/api/products/7/analogs');
  });
  it('products: фильтр purpose_id уходит в запрос', async () => {
    const calls = stubFetch({ products: [] });
    await inventory.products({ purpose_id: 3, q: 'кожа' });
    expect(calls[0]?.url).toContain('purpose_id=3');
  });
});
