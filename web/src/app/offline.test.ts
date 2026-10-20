import { QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, inventory, NetworkError, type ConsumeResult, type ProductSummary, type Session } from '../api';
import { reportConnection } from '../api/connection';
import { applyProducts } from './useRevert';
import { makeConsumeOp, queueFor, resetQueuesForTest } from '../lib/offlineQueue';
import { SnapshotStore } from '../lib/snapshot';
import { memoryStorage } from '../lib/testStorage';
import { consumeOrQueue, flushOffline, installOffline } from './offline';

const session: Session = { user: { id: 1, login: 'u', is_owner: false }, settings: { step_small: 10, step_large: 50 } };
const product = (total: number): ProductSummary =>
  ({ id: 7, name: 'Шампунь', unit: 'ml', min_stock: null, low: false, stock: { total, open_qty: null, open_capacity: null, sealed_count: 1, units_count: 1 } }) as ProductSummary;
const result = (total: number) => ({ movements: [], product: product(total), opened_unit_ids: [], finished_unit_ids: [], cost: null }) as ConsumeResult;

let st: ReturnType<typeof memoryStorage>;
let qc: QueryClient;
const total = (key: readonly unknown[] = ['products', {}]) => {
  const d = qc.getQueryData<ProductSummary[]>(key);
  return d?.[0]?.stock.total;
};

/** Телефон «после перезапуска»: на диске сессия, снимок списка и очередь. */
function boot(ops: number[]) {
  const store = new SnapshotStore(st);
  store.saveMe(session, 50);
  store.save(1, ['products', {}], [product(1000)], 100);
  const q = queueFor(1, st);
  ops.forEach((n, i) => q.add(makeConsumeOp({ id: 7, name: 'Шампунь', unit: 'ml' }, n, undefined, `op-0000000${i + 1}`)));
  qc = new QueryClient();
  installOffline(qc, store);
  return q;
}

beforeEach(() => {
  st = memoryStorage();
  resetQueuesForTest();
  reportConnection(true);
  vi.spyOn(api, 'me').mockResolvedValue(session);
});
afterEach(() => vi.restoreAllMocks());

describe('сохранённое на телефоне', () => {
  it('после перезапуска остаток показан с вычетом очереди', () => {
    boot([10, 20]);
    expect(qc.getQueryData(['me'])).toEqual(session);
    expect(total()).toBe(970);
  });

  it('отмена записи возвращает остаток', () => {
    const q = boot([10, 20]);
    q.remove('op-00000001');
    expect(total()).toBe(980);
    q.remove('op-00000002');
    expect(total()).toBe(1000);
  });
});

describe('отправка и оценка', () => {
  it('обрыв посередине не задваивает вычет', async () => {
    const q = boot([10, 20]);
    vi.spyOn(inventory, 'consume').mockResolvedValueOnce(result(990)).mockRejectedValueOnce(new NetworkError('offline'));
    const r = await flushOffline(qc, null);
    expect(r).toMatchObject({ sent: 1, stopped: 'network' });
    expect(q.size).toBe(1);
    // сервер: 990, ждёт ещё 20 → 970, а не 960
    expect(total()).toBe(970);

    vi.spyOn(inventory, 'consume').mockResolvedValue(result(970));
    await flushOffline(qc, null);
    expect(q.size).toBe(0);
    expect(total()).toBe(970);
  });

  it('свежие данные со связью при непустой очереди не задваивают вычет', () => {
    boot([20]);
    expect(total()).toBe(980);
    // обычный расход 30 со связью: сервер прислал 970 (без учёта очереди)
    applyProducts(qc, [product(970)]);
    expect(total()).toBe(950);
    // и снова те же данные — вычет не копится
    applyProducts(qc, [product(970)]);
    expect(total()).toBe(950);
  });

  it('чужая сессия: очередь не отправляется', async () => {
    const q = boot([10]);
    vi.spyOn(api, 'me').mockResolvedValue({ ...session, user: { ...session.user, id: 2 } });
    const spy = vi.spyOn(inventory, 'consume');
    const r = await flushOffline(qc, null);
    expect(r).toMatchObject({ sent: 0, stopped: 'auth' });
    expect(spy).not.toHaveBeenCalled();
    expect(q.size).toBe(1);
  });
});

describe('запись в очередь', () => {
  it('без связи расход уходит в очередь без запроса', async () => {
    boot([]);
    reportConnection(false);
    vi.spyOn(api, 'health').mockRejectedValue(new NetworkError());
    const spy = vi.spyOn(inventory, 'consume');
    const out = await consumeOrQueue(qc, { product: product(1000), qty: 40 });
    expect('queued' in out).toBe(true);
    expect(spy).not.toHaveBeenCalled();
    expect(total()).toBe(960);
  });

  it('нет места на телефоне: ошибка вместо «Записано», остаток не меняется', async () => {
    boot([]);
    reportConnection(false);
    const set = st.setItem;
    st.setItem = (k, v) => {
      if (k.startsWith('garage:queue:')) throw new Error('quota');
      set(k, v);
    };
    await expect(consumeOrQueue(qc, { product: product(1000), qty: 40 })).rejects.toThrow('Не удалось сохранить запись на телефоне');
    expect(queueFor(1, st).size).toBe(0);
    expect(total()).toBe(1000);
  });
});
