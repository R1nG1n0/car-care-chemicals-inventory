import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { reportConnection } from './connection';
import { notes, rags } from './index';

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
const bodyOf = (c: Call) => JSON.parse(String(c.init.body)) as unknown;

beforeEach(() => reportConnection(true));
afterEach(() => vi.unstubAllGlobals());

const note = { id: 1, title: 'Т', body: '', product_ids: [], recipe_ids: [], created_at: 1, updated_at: 1 };

describe('notes', () => {
  it('list — GET /api/notes с фильтрами', async () => {
    const calls = stubFetch(() => json(200, { notes: [note] }));
    expect(await notes.list()).toEqual([note]);
    expect(calls[0]!.url).toBe('/api/notes');
    await notes.list({ q: 'мойка & салон', product_id: 5, recipe_id: 7 });
    const u = new URL(calls[1]!.url, 'http://x');
    expect(u.pathname).toBe('/api/notes');
    expect(u.searchParams.get('q')).toBe('мойка & салон');
    expect(u.searchParams.get('product_id')).toBe('5');
    expect(u.searchParams.get('recipe_id')).toBe('7');
  });
  it('get, create, update, remove', async () => {
    const calls = stubFetch(() => json(200, note));
    await notes.get(3);
    expect([calls[0]!.url, calls[0]!.init.method ?? 'GET']).toEqual(['/api/notes/3', 'GET']);
    await notes.create({ title: 'Т', body: 'x', product_ids: [1] });
    expect([calls[1]!.url, calls[1]!.init.method]).toEqual(['/api/notes', 'POST']);
    expect(bodyOf(calls[1]!)).toEqual({ title: 'Т', body: 'x', product_ids: [1] });
    await notes.update(3, { recipe_ids: [] });
    expect([calls[2]!.url, calls[2]!.init.method]).toEqual(['/api/notes/3', 'PATCH']);
    expect(bodyOf(calls[2]!)).toEqual({ recipe_ids: [] });
  });
  it('remove — DELETE, 204', async () => {
    const calls = stubFetch(() => new Response(null, { status: 204 }));
    await notes.remove(3);
    expect([calls[0]!.url, calls[0]!.init.method]).toEqual(['/api/notes/3', 'DELETE']);
  });
});

describe('rags', () => {
  const rag = { id: 2, product_id: 5, label: '№ 1', number: 1, washes: 0, wear_limit: 100, level: 'ok', status: 'active', retire_reason: null, created_at: 1, retired_at: null, last_washed_at: null };
  it('list — активные или все (?all=1)', async () => {
    const calls = stubFetch(() => json(200, { rags: [rag] }));
    expect(await rags.list(5)).toEqual([rag]);
    expect(calls[0]!.url).toBe('/api/products/5/rags');
    await rags.list(5, true);
    expect(calls[1]!.url).toBe('/api/products/5/rags?all=1');
  });
  it('start — POST /api/products/{id}/rags с номером операции', async () => {
    const calls = stubFetch(() => json(201, { rag, product: {}, movement_ids: [9] }));
    const r = await rags.start(5, { client_op_id: 'abcdefgh1234' });
    expect(r.movement_ids).toEqual([9]);
    expect([calls[0]!.url, calls[0]!.init.method]).toEqual(['/api/products/5/rags', 'POST']);
    expect(bodyOf(calls[0]!)).toEqual({ client_op_id: 'abcdefgh1234' });
  });
  it('wash и unwash — ids в теле, ответ — список тряпок', async () => {
    const calls = stubFetch(() => json(200, { rags: [rag] }));
    expect(await rags.wash([2, 3])).toEqual([rag]);
    expect(await rags.unwash([2, 3])).toEqual([rag]);
    expect(calls.map((c) => c.url)).toEqual(['/api/rags/wash', '/api/rags/unwash']);
    expect(bodyOf(calls[0]!)).toEqual({ ids: [2, 3] });
  });
  it('unstart — POST /api/rags/{id}/unstart, ответ с позицией', async () => {
    const calls = stubFetch(() => json(200, { product: { id: 5 } }));
    expect(await rags.unstart(2)).toEqual({ product: { id: 5 } });
    expect([calls[0]!.url, calls[0]!.init.method]).toEqual(['/api/rags/2/unstart', 'POST']);
  });
  it('unstart 409 — ApiError', async () => {
    stubFetch(() => json(409, { error: 'Тряпку уже стирали' }));
    await expect(rags.unstart(2)).rejects.toMatchObject({ name: 'ApiError', status: 409 });
  });
  it('rename — PATCH, retire — POST с причиной', async () => {
    const calls = stubFetch(() => json(200, rag));
    await rags.rename(2, 'Красная');
    expect([calls[0]!.url, calls[0]!.init.method]).toEqual(['/api/rags/2', 'PATCH']);
    expect(bodyOf(calls[0]!)).toEqual({ label: 'Красная' });
    await rags.retire(2, 'worn');
    expect([calls[1]!.url, calls[1]!.init.method]).toEqual(['/api/rags/2/retire', 'POST']);
    expect(bodyOf(calls[1]!)).toEqual({ reason: 'worn' });
  });
});
