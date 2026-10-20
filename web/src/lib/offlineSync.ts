// Отправка очереди (ФТ-12.4): по порядку, по одной записи.
// Успех — запись удаляется; нехватка и отказ сервера — запись остаётся с пометкой,
// отправка идёт дальше; 401 и пропавшая связь — отправка останавливается, очередь цела.
import { ApiError, isApiError, isNetworkError } from '../api/client';
import type { ApplyResult, ConsumeResult } from '../api/inventory';
import { shortageMessage, type OfflineQueue, type QueueOp } from './offlineQueue';

/** То, чем очередь ходит на сервер (подменяется в тестах). */
export type SendApi = {
  consume(b: { product_id: number; qty: number; unit_id?: number; client_op_id: string; occurred_at: number }): Promise<ConsumeResult>;
  applyRecipe(id: number, items: { product_id: number; qty: number }[], op: { client_op_id: string; occurred_at: number }): Promise<ApplyResult>;
};

export type FlushResult = {
  /** Записей отправлено. */
  sent: number;
  /** Записей, которые сервер не принял (остались с пометкой). */
  failed: number;
  /** Шаблонов, у которых часть строк не хватило (запись при этом принята). */
  partial: number;
  /** Почему остановились раньше: сессия истекла / связи нет. */
  stopped: 'auth' | 'network' | null;
  /** Отправка уже шла — этот вызов присоединился к ней. */
  joined: boolean;
};

export type FlushOptions = {
  /** Записи с ошибкой тоже отправить заново («Отправить сейчас»). */
  retryErrors?: boolean;
  /** Перед отправкой: сессия принадлежит владельцу очереди? Иначе отправлять нельзя (две вкладки). */
  verify?: () => Promise<'ok' | 'auth' | 'network'>;
  /** Запись принята сервером: обновить кэш (до удаления записи из очереди). */
  onSent?: (op: QueueOp, result: ConsumeResult | ApplyResult) => void;
};

/** Неудачных попыток подряд из-за сбоя сервера, после которых запись помечается ошибкой. */
export const MAX_SERVER_FAILS = 3;
export const SERVER_FAIL_TEXT = 'Сервер не принял, попробуйте позже';

type Verdict = { kind: 'stop'; reason: 'auth' | 'network'; server?: boolean } | { kind: 'reject'; message: string; available?: number };

/** Что делать с ошибкой отправки одной записи. */
export function judge(err: unknown, op: QueueOp): Verdict {
  if (isApiError(err, 401)) return { kind: 'stop', reason: 'auth' };
  if (isNetworkError(err)) return { kind: 'stop', reason: 'network' };
  if (err instanceof ApiError) {
    // 429 и сбой самого сервера — повторим позже, запись не виновата
    if (err.status === 429) return { kind: 'stop', reason: 'network' };
    if (err.status >= 500) return { kind: 'stop', reason: 'network', server: true };
    if (err.status === 409 && err.available !== undefined && op.kind === 'consume') {
      return { kind: 'reject', message: shortageMessage(err.available, op.unit), available: err.available };
    }
    return { kind: 'reject', message: err.message };
  }
  return { kind: 'stop', reason: 'network' };
}

async function sendOne(op: QueueOp, api: SendApi): Promise<ConsumeResult | ApplyResult> {
  const idem = { client_op_id: op.op_id, occurred_at: op.occurred_at };
  if (op.kind === 'consume') {
    return api.consume({ product_id: op.product_id, qty: op.qty, ...(op.unit_id !== undefined ? { unit_id: op.unit_id } : {}), ...idem });
  }
  return api.applyRecipe(
    op.recipe_id,
    op.items.map((i) => ({ product_id: i.product_id, qty: i.qty })),
    idem,
  );
}

const running = new WeakMap<OfflineQueue, Promise<FlushResult>>();

/** Две отправки одной очереди одновременно не идут: второй вызов получает итог первой. */
export function flushQueue(queue: OfflineQueue, api: SendApi, opts: FlushOptions = {}): Promise<FlushResult> {
  const active = running.get(queue);
  if (active) return active.then((r) => ({ ...r, joined: true }));
  const run = doFlush(queue, api, opts).finally(() => running.delete(queue));
  running.set(queue, run);
  return run;
}

export function isFlushing(queue: OfflineQueue): boolean {
  return running.has(queue);
}

async function doFlush(queue: OfflineQueue, api: SendApi, opts: FlushOptions): Promise<FlushResult> {
  const res: FlushResult = { sent: 0, failed: 0, partial: 0, stopped: null, joined: false };
  if (opts.verify) {
    const who = await opts.verify();
    if (who !== 'ok') return { ...res, stopped: who };
  }
  if (opts.retryErrors) queue.retryErrors();
  const order = queue.pending().map((o) => o.op_id);
  for (const id of order) {
    // Запись могли удалить или поправить, пока шла отправка предыдущих
    const op = queue.get(id);
    if (!op || op.status !== 'pending') continue;
    try {
      const result = await sendOne(op, api);
      opts.onSent?.(op, result);
      queue.remove(id);
      res.sent++;
      if ('shortages' in result && result.shortages.length > 0) res.partial++;
    } catch (err) {
      const v = judge(err, op);
      if (v.kind === 'stop') {
        // Сбой сервера на одной и той же записи не должен блокировать очередь навсегда
        if (v.server && queue.bumpFails(id) >= MAX_SERVER_FAILS) {
          queue.markError(id, SERVER_FAIL_TEXT);
          res.failed++;
          continue;
        }
        res.stopped = v.reason;
        break;
      }
      queue.markError(id, v.message, v.available);
      res.failed++;
    }
  }
  return res;
}
