// Работа без связи (ФТ-12): сохранённые данные, очередь расхода, оценка остатков, отправка.
// Связка React Query с lib/offline*.ts и lib/snapshot.ts.
import { hashKey, type QueryClient } from '@tanstack/react-query';
import { api, inventory, isApiError, isNetworkError, NetworkError, type ApplyResult, type ConsumeResult, type ProductSummary, type Recipe, type Session } from '../api';
import { isOnline } from '../api/connection';
import { ESTIMATE_ROOTS, estimateByKey, pendingDeltas } from '../lib/offlineEstimate';
import { makeApplyOp, makeConsumeOp, newOpId, dropQueue, queueFor, setSpaceFreer, subscribeAllQueues, type OfflineQueue, type QueueOp } from '../lib/offlineQueue';
import { flushQueue, type FlushResult } from '../lib/offlineSync';
import { replaceProducts } from '../lib/stock';
import { isPersistableKey, SnapshotStore } from '../lib/snapshot';
import { invalidateStock } from './queries';
import type { ToastApi } from './toast';

const meKey = ['me'] as const;
let snaps = new SnapshotStore();

/** Последние данные сервера (без оценки) по ключам запросов: от них считается оценка. */
const raw = new Map<string, { key: readonly unknown[]; data: unknown }>();
let applying = false;

export const userIdOf = (qc: QueryClient): number | null => qc.getQueryData<Session | null>(meKey)?.user.id ?? null;

export function queueOf(qc: QueryClient): OfflineQueue | null {
  const uid = userIdOf(qc);
  return uid === null ? null : queueFor(uid);
}

/** Когда данные на телефоне в последний раз обновлялись с сервера, мс. */
export function lastSyncOf(qc: QueryClient): number | null {
  const uid = userIdOf(qc);
  return uid === null ? null : snaps.lastSync(uid);
}

const estimable = (key: readonly unknown[]) => ESTIMATE_ROOTS.includes(key[0] as string);

/** Пересчитывает оценку остатков во всех загруженных запросах от данных сервера. */
export function applyEstimates(qc: QueryClient): void {
  const d = pendingDeltas(queueOf(qc)?.list() ?? []);
  applying = true;
  try {
    for (const { key, data } of raw.values()) {
      const q = qc.getQueryCache().find({ queryKey: key, exact: true });
      if (!q) continue;
      const next = estimateByKey(key, data, d);
      if (q.state.data !== next) qc.setQueryData(key, next, { updatedAt: q.state.dataUpdatedAt });
    }
  } finally {
    applying = false;
  }
}

/**
 * Подключается один раз при создании QueryClient: подставляет сохранённое,
 * сохраняет каждую успешную загрузку, держит оценку остатков.
 */
export function installOffline(qc: QueryClient, store: SnapshotStore = snaps): void {
  snaps = store;
  // Не хватило места для записи в очередь: снимки карточек позиций можно перечитать, запись — нет
  setSpaceFreer(() => {
    const uid = userIdOf(qc);
    if (uid !== null) snaps.prune(uid, 0);
  });
  hydrate(qc);
  // После перезапуска без связи остаток показывается уже с вычетом очереди (ФТ-12.3.2)
  applyEstimates(qc);
  qc.getQueryCache().subscribe((event) => {
    if (event.type === 'removed') {
      raw.delete(event.query.queryHash);
      return;
    }
    if (applying || event.type !== 'updated' || event.action.type !== 'success') return;
    const { queryKey: key, queryHash: hash, state } = event.query;
    if (key[0] === meKey[0]) {
      if (state.data) snaps.saveMe(state.data as Session, state.dataUpdatedAt);
      else snaps.clearMe();
      return;
    }
    const uid = userIdOf(qc);
    if (uid === null) return;
    if (isPersistableKey(key)) snaps.save(uid, key, state.data, state.dataUpdatedAt);
    if (estimable(key)) {
      raw.set(hash, { key, data: state.data });
      if (queueOf(qc)?.pending().length) applyEstimates(qc);
    }
  });
  subscribeAllQueues(() => applyEstimates(qc));
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', (e) => {
      if (e.key?.startsWith('garage:queue:')) queueOf(qc)?.refresh();
    });
  }
}

function hydrate(qc: QueryClient): void {
  const me = snaps.loadMe();
  if (!me) return;
  applying = true;
  try {
    qc.setQueryData(meKey, me.session, { updatedAt: me.at });
    for (const e of snaps.loadAll(me.session.user.id)) {
      qc.setQueryData(e.key, e.data, { updatedAt: e.at });
      if (estimable(e.key)) raw.set(hashKey(e.key), { key: e.key, data: e.data });
    }
  } finally {
    applying = false;
  }
}

// ---------- Запись в очередь ----------

export type QueuedMark = { queued: QueueOp };

const QUEUED_TEXT = 'Записано без связи — отправится, когда появится VPN';

function enqueue(qc: QueryClient, op: QueueOp): QueueOp {
  const q = queueOf(qc);
  if (!q) throw new NetworkError('offline');
  q.add(op); // нет места на телефоне — QueueSaveError, «Записано» не показывается
  // Связь могла вернуться, а флаг ещё нет: проверка запустит отправку при смене состояния
  void api.health().catch(() => undefined);
  return op;
}

/** Расход: на сервер, а без связи (или при потере ответа) — в очередь. */
export async function consumeOrQueue(qc: QueryClient, v: { product: ProductSummary; qty: number; unitId?: number | undefined }): Promise<{ sent: ConsumeResult } | QueuedMark> {
  const op = makeConsumeOp(v.product, v.qty, v.unitId);
  if (!isOnline()) return { queued: enqueue(qc, op) };
  try {
    const sent = await inventory.consume({ product_id: v.product.id, qty: v.qty, unit_id: v.unitId, client_op_id: op.op_id, occurred_at: op.occurred_at });
    return { sent };
  } catch (err) {
    // Тот же номер операции: если запрос всё же дошёл, повтор его не задвоит
    if (isNetworkError(err)) return { queued: enqueueAfterLoss(qc, op, err) };
    throw err;
  }
}

/** Запись после потери ответа; не вышло сохранить — данные перечитываются (запрос мог дойти). */
function enqueueAfterLoss(qc: QueryClient, op: QueueOp, err: NetworkError): QueueOp {
  try {
    return enqueue(qc, op);
  } catch (e) {
    if (err.maybeSaved) void invalidateStock(qc);
    throw e;
  }
}

/** Применение шаблона: так же, как расход. */
export async function applyOrQueue(qc: QueryClient, recipe: Recipe, items: { product_id: number; qty: number }[]): Promise<{ sent: ApplyResult } | QueuedMark> {
  const op = makeApplyOp(recipe, items);
  if (!isOnline()) return { queued: enqueue(qc, op) };
  try {
    const sent = await inventory.applyRecipe(recipe.id, items, { client_op_id: op.op_id, occurred_at: op.occurred_at });
    return { sent };
  } catch (err) {
    if (isNetworkError(err)) return { queued: enqueueAfterLoss(qc, op, err) };
    throw err;
  }
}

/** Уведомление «Записано без связи» с «Отменить» (запись удаляется, оценка возвращается). */
export function toastQueued(qc: QueryClient, toast: ToastApi, op: QueueOp, text = QUEUED_TEXT): void {
  toast.show({ text, action: { label: 'Отменить', onClick: () => queueOf(qc)?.remove(op.op_id) } });
}

// ---------- Отправка ----------

/**
 * Свежие сводки позиций от сервера — в «сырые» данные (не в кэш экрана: там уже оценка,
 * и вычитание задвоилось бы). Кэш обновит applyEstimates, когда запись уйдёт из очереди.
 */
function sentToCache(qc: QueryClient, result: ConsumeResult | ApplyResult): void {
  freshToRaw(qc, 'products' in result ? result.products : [result.product]);
}

/**
 * Свежие сводки при непустой очереди (расход или отмена со связью): тоже в «сырые»,
 * затем пересчёт оценки — иначе они легли бы поверх оценки и запомнились как данные
 * сервера. Очередь пуста — false, сводки кладутся в кэш как обычно.
 */
export function applyFreshWithQueue(qc: QueryClient, fresh: readonly ProductSummary[]): boolean {
  if (!queueOf(qc)?.pending().length) return false;
  freshToRaw(qc, fresh);
  applyEstimates(qc);
  return true;
}

function freshToRaw(qc: QueryClient, fresh: readonly ProductSummary[]): void {
  const uid = userIdOf(qc);
  for (const entry of raw.values()) {
    const { key, data } = entry;
    if (key[0] === 'products' && Array.isArray(data)) entry.data = replaceProducts(data as ProductSummary[], fresh);
    else if (key[0] === 'product') {
      const f = fresh.find((p) => p.id === key[1]);
      if (f) entry.data = { ...(data as object), product: f };
    } else continue;
    if (uid !== null && isPersistableKey(key)) snaps.save(uid, key, entry.data, Date.now());
  }
}

/** Куки общие для вкладок: если в другой вошёл другой пользователь, чужую очередь не отправляем. */
async function verifyOwner(q: OfflineQueue): Promise<'ok' | 'auth' | 'network'> {
  try {
    const me = await api.me();
    return me.user.id === q.userId ? 'ok' : 'auth';
  } catch (err) {
    return isApiError(err, 401) ? 'auth' : 'network';
  }
}

/**
 * Отправляет очередь текущего пользователя. Параллельный вызов присоединяется к идущей отправке.
 * Итог показывается уведомлением; null — отправлять нечего.
 */
export async function flushOffline(qc: QueryClient, toast: ToastApi | null, opts: { retryErrors?: boolean } = {}): Promise<FlushResult | null> {
  const q = queueOf(qc);
  if (!q) return null;
  if (!opts.retryErrors && q.pending().length === 0) return null;
  const res = await flushQueue(q, { consume: inventory.consume, applyRecipe: inventory.applyRecipe }, { ...opts, verify: () => verifyOwner(q), onSent: (_op, r) => sentToCache(qc, r) });
  if (res.joined) return res;
  if (res.sent > 0) await invalidateStock(qc);
  if (res.stopped === 'auth') void qc.invalidateQueries({ queryKey: meKey });
  if (toast && (res.sent > 0 || res.failed > 0)) {
    const parts = [res.sent > 0 ? `Отправлено: ${res.sent}` : '', res.failed > 0 ? `не принято: ${res.failed}` : ''].filter(Boolean);
    toast.show({ text: parts.join(' · ').replace(/^./, (c) => c.toUpperCase()) });
  }
  return res;
}

/** Выход: данные, сессия и очередь пользователя стираются (ФТ-12.5.2). */
export function forgetUser(uid: number): void {
  snaps.forget(uid);
  dropQueue(uid);
}

export { newOpId };
