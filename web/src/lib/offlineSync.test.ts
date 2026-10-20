import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, NetworkError } from '../api/client';
import { memoryStorage } from './testStorage';
import { makeApplyOp, makeConsumeOp, OfflineQueue, shortageMessage } from './offlineQueue';
import { flushQueue, isFlushing, MAX_SERVER_FAILS, SERVER_FAIL_TEXT, type SendApi } from './offlineSync';

const milk = { id: 7, name: 'Шампунь', unit: 'ml' as const };
const ok = { movements: [], product: {}, opened_unit_ids: [], finished_unit_ids: [], cost: null };

function setup(...qtys: number[]) {
  const q = new OfflineQueue(1, memoryStorage());
  qtys.forEach((n, i) => q.add(makeConsumeOp(milk, n, undefined, `op-0000000${i + 1}`, 1000 + i)));
  const consume = vi.fn();
  const applyRecipe = vi.fn();
  const api = { consume, applyRecipe } as unknown as SendApi & { consume: typeof consume; applyRecipe: typeof applyRecipe };
  return { q, api, consume, applyRecipe };
}

describe('отправка очереди', () => {
  let t: ReturnType<typeof setup>;
  beforeEach(() => {
    t = setup(10, 20, 30);
  });

  it('отправляет по порядку, с номером и временем, и очищает очередь', async () => {
    t.consume.mockResolvedValue(ok);
    const r = await flushQueue(t.q, t.api);
    expect(r).toMatchObject({ sent: 3, failed: 0, stopped: null, joined: false });
    expect(t.consume.mock.calls.map((c) => c[0])).toEqual([
      { product_id: 7, qty: 10, client_op_id: 'op-00000001', occurred_at: 1000 },
      { product_id: 7, qty: 20, client_op_id: 'op-00000002', occurred_at: 1001 },
      { product_id: 7, qty: 30, client_op_id: 'op-00000003', occurred_at: 1002 },
    ]);
    expect(t.q.size).toBe(0);
  });

  it('нехватка: запись остаётся с пометкой, остальные уходят дальше', async () => {
    t.consume.mockRejectedValueOnce(new ApiError(409, 'Не хватает: есть 5 мл', undefined, undefined, 5)).mockResolvedValue(ok);
    const r = await flushQueue(t.q, t.api);
    expect(r).toMatchObject({ sent: 2, failed: 1 });
    expect(t.q.list()).toHaveLength(1);
    expect(t.q.list()[0]).toMatchObject({ op_id: 'op-00000001', status: 'error', available: 5, error: shortageMessage(5, 'ml') });
  });

  it('другой 409 (номер занят) — обычная ошибка записи, а не нехватка', async () => {
    t.consume.mockRejectedValueOnce(new ApiError(409, 'Этот номер операции уже использован для другого действия', 'client_op_id')).mockResolvedValue(ok);
    await flushQueue(t.q, t.api);
    const op = t.q.get('op-00000001')!;
    expect(op.error).toBe('Этот номер операции уже использован для другого действия');
    expect(op.available).toBeUndefined();
  });

  it('404 и 400 — ошибка с текстом сервера, дальше идём', async () => {
    t.consume.mockRejectedValueOnce(new ApiError(404, 'Позиция не найдена')).mockRejectedValueOnce(new ApiError(400, 'Количество — от 1 до 100 000', 'qty')).mockResolvedValue(ok);
    const r = await flushQueue(t.q, t.api);
    expect(r).toMatchObject({ sent: 1, failed: 2 });
    expect(t.q.list().map((o) => o.error)).toEqual(['Позиция не найдена', 'Количество — от 1 до 100 000']);
  });

  it('401 останавливает отправку, очередь цела и ждёт', async () => {
    t.consume.mockResolvedValueOnce(ok).mockRejectedValueOnce(new ApiError(401, 'Войдите'));
    const r = await flushQueue(t.q, t.api);
    expect(r).toMatchObject({ sent: 1, stopped: 'auth' });
    expect(t.q.list().map((o) => [o.op_id, o.status])).toEqual([
      ['op-00000002', 'pending'],
      ['op-00000003', 'pending'],
    ]);
    expect(t.consume).toHaveBeenCalledTimes(2);
  });

  it('пропавшая связь и сбой сервера останавливают отправку без пометок', async () => {
    t.consume.mockRejectedValueOnce(new NetworkError('timeout'));
    expect(await flushQueue(t.q, t.api)).toMatchObject({ sent: 0, stopped: 'network' });
    t.consume.mockReset().mockRejectedValueOnce(new ApiError(500, 'Внутренняя ошибка'));
    expect(await flushQueue(t.q, t.api)).toMatchObject({ stopped: 'network' });
    expect(t.q.list().every((o) => o.status === 'pending')).toBe(true);
    expect(t.q.size).toBe(3);
  });

  it('параллельный запуск присоединяется к идущей отправке', async () => {
    let release!: () => void;
    t.consume.mockImplementation(() => new Promise((res) => (release = () => res(ok))));
    const first = flushQueue(t.q, t.api);
    expect(isFlushing(t.q)).toBe(true);
    const second = flushQueue(t.q, t.api);
    await Promise.resolve();
    release();
    await vi.waitFor(() => expect(t.consume).toHaveBeenCalledTimes(2));
    release();
    await vi.waitFor(() => expect(t.consume).toHaveBeenCalledTimes(3));
    release();
    const [a, b] = await Promise.all([first, second]);
    expect(a.joined).toBe(false);
    expect(b.joined).toBe(true);
    expect(b.sent).toBe(3);
    expect(t.consume).toHaveBeenCalledTimes(3);
    expect(isFlushing(t.q)).toBe(false);
  });

  it('записи с ошибкой автоматически не повторяются, «Отправить сейчас» повторяет', async () => {
    t.q.markError('op-00000001', 'x');
    t.consume.mockResolvedValue(ok);
    await flushQueue(t.q, t.api);
    expect(t.consume).toHaveBeenCalledTimes(2);
    expect(t.q.size).toBe(1);
    await flushQueue(t.q, t.api, { retryErrors: true });
    expect(t.consume).toHaveBeenCalledTimes(3);
    expect(t.q.size).toBe(0);
  });

  it('запись, удалённая во время отправки, не уходит; onSent вызывается до удаления', async () => {
    const seen: string[] = [];
    t.consume.mockImplementation(async () => {
      t.q.remove('op-00000002');
      return ok;
    });
    const r = await flushQueue(t.q, t.api, { onSent: (op) => seen.push(`${op.op_id}:${t.q.get(op.op_id) ? 'в очереди' : 'нет'}`) });
    expect(r.sent).toBe(2);
    expect(t.consume.mock.calls.map((c) => c[0].client_op_id)).toEqual(['op-00000001', 'op-00000003']);
    expect(seen[0]).toBe('op-00000001:в очереди');
  });

  it('шаблон: строки, номер; частичная нехватка считается принятой записью', async () => {
    const q = new OfflineQueue(2, memoryStorage());
    const recipe = { id: 3, name: 'Мойка', items: [{ product_id: 7, qty: 30, product: milk }] } as never;
    q.add(makeApplyOp(recipe, [{ product_id: 7, qty: 30 }], 'op-00000009', 555));
    const applyRecipe = vi.fn().mockResolvedValue({ run_id: 1, movements: [], products: [], shortages: [{ product_id: 7, requested: 30, consumed: 10 }], cost: null });
    const r = await flushQueue(q, { consume: vi.fn(), applyRecipe });
    expect(applyRecipe).toHaveBeenCalledWith(3, [{ product_id: 7, qty: 30 }], { client_op_id: 'op-00000009', occurred_at: 555 });
    expect(r).toMatchObject({ sent: 1, partial: 1 });
    expect(q.size).toBe(0);
  });

  it('очередь другого пользователя не трогается', async () => {
    const other = new OfflineQueue(2, memoryStorage());
    other.add(makeConsumeOp(milk, 5));
    t.consume.mockResolvedValue(ok);
    await flushQueue(t.q, t.api);
    expect(other.size).toBe(1);
  });

  it('сессия другого пользователя: ничего не отправляется', async () => {
    t.consume.mockResolvedValue(ok);
    const r = await flushQueue(t.q, t.api, { verify: async () => 'auth' });
    expect(r).toMatchObject({ sent: 0, stopped: 'auth' });
    expect(t.consume).not.toHaveBeenCalled();
    expect(t.q.size).toBe(3);
    const r2 = await flushQueue(t.q, t.api, { verify: async () => 'ok' });
    expect(r2.sent).toBe(3);
  });

  it('сбой сервера 3 раза подряд на одной записи: ошибка, очередь идёт дальше', async () => {
    t.consume.mockImplementation(async (b: { client_op_id: string }) => {
      if (b.client_op_id === 'op-00000001') throw new ApiError(500, 'Внутренняя ошибка');
      return ok;
    });
    for (let i = 1; i < MAX_SERVER_FAILS; i++) {
      expect(await flushQueue(t.q, t.api)).toMatchObject({ sent: 0, stopped: 'network' });
      expect(t.q.get('op-00000001')).toMatchObject({ status: 'pending', fails: i });
    }
    const r = await flushQueue(t.q, t.api);
    expect(r).toMatchObject({ sent: 2, failed: 1, stopped: null });
    expect(t.q.get('op-00000001')).toMatchObject({ status: 'error', error: SERVER_FAIL_TEXT });
  });

  it('429 не считается неудачей записи', async () => {
    t.consume.mockRejectedValue(new ApiError(429, 'Слишком часто', undefined, 30));
    for (let i = 0; i < 5; i++) await flushQueue(t.q, t.api);
    expect(t.q.get('op-00000001')).toMatchObject({ status: 'pending' });
    expect(t.q.get('op-00000001')!.fails).toBeUndefined();
  });
});
