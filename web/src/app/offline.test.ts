import { QueryClient } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, inventory, ApiError, NetworkError, type ApplyResult, type ConsumeResult, type ProductSummary, type Recipe, type Session } from '../api';
import { reportConnection } from '../api/connection';
import { applyProducts } from './useRevert';
import { makeConsumeOp, OfflineQueue, queueFor, resetQueuesForTest } from '../lib/offlineQueue';
import { SnapshotStore } from '../lib/snapshot';
import { memoryStorage } from '../lib/testStorage';
import { applyOrQueue, consumeOrQueue, flushOffline, installOffline, toastQueued } from './offline';
import type { ToastApi } from './toast';

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
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

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
  it('сначала сохраняет запрос на диске; фоновая отправка не дублирует текущий запрос', async () => {
    const q = boot([]);
    let finish!: (r: ConsumeResult) => void;
    const spy = vi.spyOn(inventory, 'consume').mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    const pending = consumeOrQueue(qc, { product: product(1000), qty: 40 });
    const persisted = new OfflineQueue(1, st).list();
    expect(persisted).toHaveLength(1);
    expect(persisted[0]).toMatchObject({ attempted: true, qty: 40 });
    expect(spy.mock.calls[0]![0].client_op_id).toBe(persisted[0]!.op_id);
    await flushOffline(qc, null);
    expect(spy).toHaveBeenCalledTimes(1);
    expect(total()).toBe(1000);
    finish(result(960));
    await expect(pending).resolves.toEqual({ sent: result(960) });
    expect(q.size).toBe(0);
    expect(new OfflineQueue(1, st).size).toBe(0);
    // Шкала обновляется обработчиком экрана после ухода шторки.
    expect(total()).toBe(1000);
    applyProducts(qc, [product(960)]);
    expect(total()).toBe(960);
  });

  it('старая кнопка отмены не удаляет запись, которую уже начали отправлять', () => {
    const q = boot([40]);
    const toast: ToastApi = { show: vi.fn(), hide: vi.fn() };
    toastQueued(qc, toast, q.list()[0]!);
    const first = vi.mocked(toast.show).mock.calls[0]![0];
    q.markAttempted(q.list()[0]!.op_id);
    first.action!.onClick();
    expect(q.size).toBe(1);
    expect(vi.mocked(toast.show).mock.calls[1]![0].text).toContain('Проверяем');
  });

  it('нет места: даже со связью не отправляет несохранённый расход', async () => {
    boot([]);
    st.setItem = () => { throw new Error('quota'); };
    const spy = vi.spyOn(inventory, 'consume');
    await expect(consumeOrQueue(qc, { product: product(1000), qty: 40 })).rejects.toThrow('Не удалось сохранить запись на телефоне');
    expect(spy).not.toHaveBeenCalled();
  });

  it('явный отказ сервера убирает подготовленный запрос и возвращает оценку остатка', async () => {
    const q = boot([]);
    vi.spyOn(inventory, 'consume').mockRejectedValue(new ApiError(409, 'Не хватает', undefined, undefined, 0));
    await expect(consumeOrQueue(qc, { product: product(1000), qty: 40 })).rejects.toBeInstanceOf(ApiError);
    expect(q.size).toBe(0);
    expect(total()).toBe(1000);
  });

  it('обрыв успешного ответа и перезапуск: повторяет тот же номер, списывает только один раз', async () => {
    boot([]);
    vi.spyOn(api, 'health').mockRejectedValue(new NetworkError());
    let stock = 1000;
    const accepted = new Map<string, ConsumeResult>();
    const ids: string[] = [];
    vi.stubGlobal('fetch', vi.fn(async (_url: string, init: RequestInit) => {
      const body = JSON.parse(init.body as string) as { client_op_id: string; qty: number };
      ids.push(body.client_op_id);
      if (!accepted.has(body.client_op_id)) {
        stock -= body.qty;
        accepted.set(body.client_op_id, result(stock));
      }
      if (ids.length === 1) return new Response(new ReadableStream({ start(c) { c.error(new TypeError('Обрыв тела')); } }), { status: 201 });
      return new Response(JSON.stringify(accepted.get(body.client_op_id)), { status: 200 });
    }));
    const out = await consumeOrQueue(qc, { product: product(1000), qty: 40 });
    expect(out).toHaveProperty('queued');
    expect(stock).toBe(960);
    expect(new OfflineQueue(1, st).list()[0]).toMatchObject({ attempted: true, op_id: ids[0] });
    // Телефон закрыли после фиксации на сервере, затем открыли снова.
    resetQueuesForTest();
    queueFor(1, st);
    qc = new QueryClient();
    installOffline(qc, new SnapshotStore(st));
    reportConnection(true);
    await expect(flushOffline(qc, null)).resolves.toMatchObject({ sent: 1 });
    expect(ids).toEqual([ids[0], ids[0]]);
    expect(stock).toBe(960);
    expect(new OfflineQueue(1, st).size).toBe(0);
    expect(total()).toBe(960);
  });

  it('шаблон также сохраняется до запроса и повторяется после потери ответа', async () => {
    const q = boot([]);
    vi.spyOn(api, 'health').mockRejectedValue(new NetworkError());
    const recipe = { id: 2, name: 'Мойка', items: [{ product_id: 7, product: product(1000) }] } as Recipe;
    const spy = vi.spyOn(inventory, 'applyRecipe').mockImplementationOnce(async () => {
      expect(new OfflineQueue(1, st).list()[0]).toMatchObject({ kind: 'apply', attempted: true });
      throw new NetworkError('timeout');
    });
    await expect(applyOrQueue(qc, recipe, [{ product_id: 7, qty: 40 }])).resolves.toHaveProperty('queued');
    const id = q.list()[0]!.op_id;
    const response: ApplyResult = { run_id: 1, movements: [], products: [product(960)], shortages: [], cost: null };
    spy.mockResolvedValue(response);
    reportConnection(true);
    await flushOffline(qc, null);
    expect(spy.mock.calls.map((c) => c[2]?.client_op_id)).toEqual([id, id]);
    expect(q.size).toBe(0);
    expect(total()).toBe(960);
  });

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
