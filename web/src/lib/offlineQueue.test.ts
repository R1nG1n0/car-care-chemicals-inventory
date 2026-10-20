import { memoryStorage } from './testStorage';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { makeApplyOp, makeConsumeOp, newOpId, OfflineQueue, opProductIds, QueueSaveError, setSpaceFreer, opQtyText, parseQueue, queueFor, resetQueuesForTest, shortageMessage } from './offlineQueue';

const milk = { id: 7, name: 'Шампунь', unit: 'ml' as const };

describe('номер операции', () => {
  it('подходит под договор сервера: 8–64 символа [A-Za-z0-9_-]', () => {
    for (let i = 0; i < 20; i++) expect(newOpId()).toMatch(/^[A-Za-z0-9_-]{8,64}$/u);
  });
  it('без randomUUID тоже годится и не повторяется', () => {
    vi.stubGlobal('crypto', {});
    const a = newOpId();
    const b = newOpId();
    vi.unstubAllGlobals();
    expect(a).toMatch(/^[A-Za-z0-9_-]{8,64}$/u);
    expect(a).not.toBe(b);
  });
});

describe('очередь', () => {
  let st: ReturnType<typeof memoryStorage>;
  beforeEach(() => {
    st = memoryStorage();
    resetQueuesForTest();
  });

  it('добавление сохраняет порядок и переживает «перезапуск»', () => {
    const q = new OfflineQueue(1, st);
    q.add(makeConsumeOp(milk, 40, undefined, 'op-00000001', 100));
    q.add(makeConsumeOp(milk, 10, 5, 'op-00000002', 101));
    const again = new OfflineQueue(1, st);
    expect(again.list().map((o) => o.op_id)).toEqual(['op-00000001', 'op-00000002']);
    expect(again.list()[1]).toMatchObject({ unit_id: 5, qty: 10, status: 'pending' });
  });

  it('list возвращает тот же массив, пока ничего не менялось', () => {
    const q = new OfflineQueue(1, st);
    q.add(makeConsumeOp(milk, 40));
    expect(q.list()).toBe(q.list());
  });

  it('удаление (отмена) и подписка', () => {
    const q = new OfflineQueue(1, st);
    const fn = vi.fn();
    q.subscribe(fn);
    q.add(makeConsumeOp(milk, 40, undefined, 'op-00000001'));
    expect(q.remove('op-00000001')).toBe(true);
    expect(q.remove('op-00000001')).toBe(false);
    expect(q.size).toBe(0);
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it('ошибка, правка количества возвращает запись в ожидание', () => {
    const q = new OfflineQueue(1, st);
    q.add(makeConsumeOp(milk, 40, undefined, 'op-00000001'));
    expect(q.setQty('op-00000001', 5)).toBe(false);
    expect(q.get('op-00000001')).toMatchObject({ qty: 40, status: 'pending' });
    q.markError('op-00000001', shortageMessage(25, 'ml'), 25);
    expect(q.get('op-00000001')).toMatchObject({ status: 'error', available: 25, error: shortageMessage(25, 'ml') });
    expect(q.pending()).toHaveLength(0);
    q.setQty('op-00000001', 25);
    const op = q.get('op-00000001')!;
    expect(op).toMatchObject({ status: 'pending', qty: 25 });
    expect(op.error).toBeUndefined();
    expect(op.available).toBeUndefined();
  });

  it('строка шаблона правится по позиции', () => {
    const q = new OfflineQueue(1, st);
    const recipe = { id: 3, name: 'Мойка', items: [{ product_id: 7, qty: 30, product: { ...milk } }] } as never;
    q.add(makeApplyOp(recipe, [{ product_id: 7, qty: 30 }], 'op-00000009'));
    expect(q.setQty('op-00000009', 20, 7)).toBe(false); // ждущую правит только удаление
    q.markError('op-00000009', 'x');
    expect(q.setQty('op-00000009', 20, 7)).toBe(true);
    const op = q.get('op-00000009')!;
    expect(op.kind === 'apply' && op.items[0]!.qty).toBe(20);
    expect(opQtyText(op)).toContain('Шампунь');
    expect([...opProductIds(q.list())]).toEqual([7]);
  });

  it('retryErrors возвращает все ошибочные записи в ожидание', () => {
    const q = new OfflineQueue(1, st);
    q.add(makeConsumeOp(milk, 1, undefined, 'op-00000001'));
    q.add(makeConsumeOp(milk, 2, undefined, 'op-00000002'));
    q.markError('op-00000001', 'x');
    q.retryErrors();
    expect(q.pending()).toHaveLength(2);
  });

  it('очереди пользователей изолированы', () => {
    const a = new OfflineQueue(1, st);
    const b = new OfflineQueue(2, st);
    a.add(makeConsumeOp(milk, 40));
    expect(b.size).toBe(0);
    expect(st.data.has('garage:queue:1')).toBe(true);
    expect(st.data.has('garage:queue:2')).toBe(false);
    expect(queueFor(1, st)).toBe(queueFor(1, st));
    expect(queueFor(1, st)).not.toBe(queueFor(2, st));
  });

  it('изменение из другой вкладки подхватывается', () => {
    const a = new OfflineQueue(1, st);
    const b = new OfflineQueue(1, st);
    expect(b.size).toBe(0);
    a.add(makeConsumeOp(milk, 40));
    expect(b.size).toBe(1);
  });

  it('clear стирает очередь из хранилища', () => {
    const q = new OfflineQueue(1, st);
    q.add(makeConsumeOp(milk, 40));
    q.clear();
    expect(st.data.has('garage:queue:1')).toBe(false);
    expect(new OfflineQueue(1, st).size).toBe(0);
  });

  it('без хранилища очередь живёт в памяти', () => {
    const q = new OfflineQueue(1, null);
    q.add(makeConsumeOp(milk, 40));
    expect(q.size).toBe(1);
  });

  it('нет места: освобождаем и повторяем; не вышло — QueueSaveError и записи нет', () => {
    const full = memoryStorage();
    const set = full.setItem;
    let blocked = true;
    full.setItem = (k, v) => {
      if (blocked && k.startsWith('garage:queue:')) throw new Error('quota');
      set(k, v);
    };
    const q = new OfflineQueue(1, full);
    const freer = vi.fn(() => {
      blocked = false;
    });
    setSpaceFreer(freer);
    q.add(makeConsumeOp(milk, 40, undefined, 'op-00000001'));
    expect(freer).toHaveBeenCalledTimes(1);
    expect(new OfflineQueue(1, full).size).toBe(1);

    blocked = true;
    setSpaceFreer(() => undefined);
    expect(() => q.add(makeConsumeOp(milk, 41, undefined, 'op-00000002'))).toThrow(QueueSaveError);
    expect(q.size).toBe(1);
    setSpaceFreer(null);
  });
});

describe('повреждённое хранилище', () => {
  it('мусор вместо JSON — пустая очередь', () => {
    expect(parseQueue('{не json')).toEqual([]);
    expect(parseQueue('null')).toEqual([]);
    expect(parseQueue('{"ops":5}')).toEqual([]);
    expect(parseQueue(null)).toEqual([]);
  });
  it('плохие записи отбрасываются, хорошие остаются', () => {
    const good = makeConsumeOp(milk, 40, undefined, 'op-00000001');
    const raw = JSON.stringify({ v: 1, ops: [good, { kind: 'consume' }, 7, { ...good, qty: 'много' }, { ...good, status: 'weird' }] });
    expect(parseQueue(raw)).toEqual([good]);
  });
  it('getItem бросает — очередь не падает', () => {
    const st = memoryStorage();
    st.getItem = () => {
      throw new Error('denied');
    };
    expect(new OfflineQueue(1, st).list()).toEqual([]);
  });
});
