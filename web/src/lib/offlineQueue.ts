// Очередь расхода без связи (ФТ-12.3): записи лежат в localStorage телефона
// отдельно для каждого пользователя и переживают закрытие приложения.
// Логика не зависит от React и браузера: хранилище передаётся снаружи.
import type { ProductSummary, Recipe, Unit } from '../api';
import { qtyText } from './stock';

export type StorageLike = {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
  readonly length?: number;
  key?(index: number): string | null;
};

/** Хранилище браузера; недоступно (приватный режим, запрет) — null. */
export function browserStorage(): StorageLike | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
}

/** ждёт отправки / сервер отказал и ждёт решения владельца */
export type OpStatus = 'pending' | 'error';

type OpBase = {
  /** Номер операции (client_op_id): повтор с ним не списывает второй раз. */
  op_id: string;
  /** Время на телефоне, секунды Unix. */
  occurred_at: number;
  status: OpStatus;
  error?: string;
  /** Подряд неудачных попыток из-за сбоя сервера (5xx); после трёх запись помечается ошибкой. */
  fails?: number;
  /** Сколько было на складе при отказе «не хватает» — для «Списать сколько есть». */
  available?: number;
};

export type ConsumeOp = OpBase & {
  kind: 'consume';
  product_id: number;
  unit_id?: number;
  qty: number;
  /** Снимок названия и единицы — для показа, пока позиция не перечитана. */
  name: string;
  unit: Unit;
};

export type ApplyItem = { product_id: number; qty: number; name: string; unit: Unit };

export type ApplyOp = OpBase & {
  kind: 'apply';
  recipe_id: number;
  recipe_name: string;
  items: ApplyItem[];
};

export type QueueOp = ConsumeOp | ApplyOp;

const nowSec = (): number => Math.floor(Date.now() / 1000);

/** Номер операции: 8–64 символа [A-Za-z0-9_-]. randomUUID есть не везде (нужен защищённый контекст). */
export function newOpId(): string {
  const c = globalThis.crypto as Crypto | undefined;
  if (typeof c?.randomUUID === 'function') return c.randomUUID();
  const bytes = new Uint8Array(16);
  if (typeof c?.getRandomValues === 'function') c.getRandomValues(bytes);
  else for (let i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

export function makeConsumeOp(product: Pick<ProductSummary, 'id' | 'name' | 'unit'>, qty: number, unitId?: number, id = newOpId(), at = nowSec()): ConsumeOp {
  const op: ConsumeOp = { op_id: id, kind: 'consume', occurred_at: at, status: 'pending', product_id: product.id, qty, name: product.name, unit: product.unit };
  if (unitId !== undefined) op.unit_id = unitId;
  return op;
}

export function makeApplyOp(recipe: Pick<Recipe, 'id' | 'name' | 'items'>, items: { product_id: number; qty: number }[], id = newOpId(), at = nowSec()): ApplyOp {
  const info = new Map(recipe.items.map((i) => [i.product_id, i.product]));
  return {
    op_id: id,
    kind: 'apply',
    occurred_at: at,
    status: 'pending',
    recipe_id: recipe.id,
    recipe_name: recipe.name,
    items: items.map((i) => ({ product_id: i.product_id, qty: i.qty, name: info.get(i.product_id)?.name ?? `№${i.product_id}`, unit: info.get(i.product_id)?.unit ?? 'ml' })),
  };
}

// ---------- Показ ----------

export function opTitle(op: QueueOp): string {
  return op.kind === 'consume' ? op.name : `Шаблон «${op.recipe_name}»`;
}

/** Количество для строки: «40 мл» или «3 строки: …» для шаблона. */
export function opQtyText(op: QueueOp): string {
  if (op.kind === 'consume') return qtyText(op.qty, op.unit);
  return op.items.map((i) => `${i.name} ${qtyText(i.qty, i.unit)}`).join(', ');
}

export function shortageMessage(available: number, unit: Unit): string {
  return `Не хватает: на складе ${qtyText(available, unit)}`;
}

/** Позиции, которых касаются записи (метка «не отправлено» на карточках). */
export function opProductIds(ops: readonly QueueOp[]): Set<number> {
  const ids = new Set<number>();
  for (const op of ops) {
    if (op.kind === 'consume') ids.add(op.product_id);
    else for (const i of op.items) ids.add(i.product_id);
  }
  return ids;
}

// ---------- Разбор сохранённого ----------

const isNum = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);
const isUnit = (v: unknown): v is Unit => v === 'ml' || v === 'pcs';

function validOp(o: unknown): o is QueueOp {
  if (typeof o !== 'object' || o === null) return false;
  const r = o as Record<string, unknown>;
  if (typeof r.op_id !== 'string' || r.op_id === '' || !isNum(r.occurred_at)) return false;
  if (r.status !== 'pending' && r.status !== 'error') return false;
  if (r.kind === 'consume') return isNum(r.product_id) && isNum(r.qty) && typeof r.name === 'string' && isUnit(r.unit);
  if (r.kind === 'apply') {
    return (
      isNum(r.recipe_id) &&
      typeof r.recipe_name === 'string' &&
      Array.isArray(r.items) &&
      r.items.every((i: unknown) => typeof i === 'object' && i !== null && isNum((i as ApplyItem).product_id) && isNum((i as ApplyItem).qty) && isUnit((i as ApplyItem).unit))
    );
  }
  return false;
}

/** Повреждённое хранилище не ломает приложение: плохие записи отбрасываются. */
export function parseQueue(raw: string | null): QueueOp[] {
  if (!raw) return [];
  try {
    const data = JSON.parse(raw) as { ops?: unknown };
    return Array.isArray(data.ops) ? data.ops.filter(validOp) : [];
  } catch {
    return [];
  }
}

// ---------- Очередь ----------

/** Запись не удалось сохранить на телефоне (нет места): о ней нельзя говорить «записано». */
export class QueueSaveError extends Error {
  constructor() {
    super('Не удалось сохранить запись на телефоне');
    this.name = 'QueueSaveError';
  }
}

/** Освобождает место в localStorage (снимки карточек позиций); задаётся приложением. */
let spaceFreer: (() => void) | null = null;
export function setSpaceFreer(fn: (() => void) | null): void {
  spaceFreer = fn;
}

type Listener = () => void;
const anyListeners = new Set<Listener>();

/** Подписка на изменения любой очереди (пересчёт оценки остатков). */
export function subscribeAllQueues(l: Listener): () => void {
  anyListeners.add(l);
  return () => anyListeners.delete(l);
}

export class OfflineQueue {
  private ops: QueueOp[] = [];
  private raw: string | null = null;
  private loaded = false;
  /** Запись в хранилище не удалась — до конца сеанса очередь живёт в памяти. */
  private volatile = false;
  private readonly listeners = new Set<Listener>();

  constructor(
    readonly userId: number,
    private readonly storage: StorageLike | null,
  ) {}

  get storageKey(): string {
    return `garage:queue:${this.userId}`;
  }

  /** Записи по порядку. Тот же массив, пока ничего не изменилось (для useSyncExternalStore). */
  list(): readonly QueueOp[] {
    if (this.volatile || !this.storage) return this.ops;
    let raw: string | null;
    try {
      raw = this.storage.getItem(this.storageKey);
    } catch {
      return this.ops;
    }
    if (this.loaded && raw === this.raw) return this.ops;
    this.loaded = true;
    this.raw = raw;
    this.ops = parseQueue(raw);
    return this.ops;
  }

  get size(): number {
    return this.list().length;
  }

  pending(): QueueOp[] {
    return this.list().filter((o) => o.status === 'pending');
  }

  get(id: string): QueueOp | undefined {
    return this.list().find((o) => o.op_id === id);
  }

  /**
   * Добавляет запись. Не хватило места — освобождаем (снимки карточек) и повторяем;
   * не вышло — QueueSaveError, запись не добавляется (запрос не теряется молча).
   */
  add(op: QueueOp): void {
    const next = [...this.list(), op];
    if (this.storage && !this.volatile) {
      const raw = JSON.stringify({ v: 1, ops: next });
      try {
        this.storage.setItem(this.storageKey, raw);
      } catch {
        try {
          spaceFreer?.();
          this.storage.setItem(this.storageKey, raw);
        } catch {
          throw new QueueSaveError();
        }
      }
    }
    this.write(next);
  }

  remove(id: string): boolean {
    const cur = this.list();
    if (!cur.some((o) => o.op_id === id)) return false;
    this.write(cur.filter((o) => o.op_id !== id));
    return true;
  }

  /** Заменяет запись по номеру; нет такой — false. */
  update(id: string, fn: (op: QueueOp) => QueueOp): boolean {
    const cur = this.list();
    if (!cur.some((o) => o.op_id === id)) return false;
    this.write(cur.map((o) => (o.op_id === id ? fn(o) : o)));
    return true;
  }

  markError(id: string, error: string, available?: number): void {
    this.update(id, (o) => {
      const next: QueueOp = { ...o, status: 'error', error };
      if (available !== undefined) next.available = available;
      else delete next.available;
      return next;
    });
  }

  /**
   * Новое количество (строки шаблона — по productId): запись снова ждёт отправки.
   * Только у записей с ошибкой: их сервер не запомнил. У ждущих номер операции мог
   * уже сработать (ответ потерян) — повтор с ним вернёт старый результат; такие только удаляются.
   */
  setQty(id: string, qty: number, productId?: number): boolean {
    if (this.get(id)?.status !== 'error') return false;
    return this.update(id, (o) => {
      const base: QueueOp = o.kind === 'consume' ? { ...o, qty } : { ...o, items: o.items.map((i) => (i.product_id === productId ? { ...i, qty } : i)) };
      return resetOp(base);
    });
  }

  /** Все записи с ошибкой снова ждут отправки («Отправить сейчас»). */
  retryErrors(): void {
    const cur = this.list();
    if (!cur.some((o) => o.status === 'error')) return;
    this.write(cur.map((o) => (o.status === 'error' ? resetOp(o) : o)));
  }

  /** Ещё одна неудачная попытка из-за сбоя сервера; возвращает счётчик подряд. */
  bumpFails(id: string): number {
    let n = 0;
    this.update(id, (o) => {
      n = (o.fails ?? 0) + 1;
      return { ...o, fails: n };
    });
    return n;
  }

  clear(): void {
    this.write([]);
    try {
      this.storage?.removeItem(this.storageKey);
    } catch {
      // нечего удалять
    }
    this.raw = null;
  }

  subscribe(l: Listener): () => void {
    this.listeners.add(l);
    return () => this.listeners.delete(l);
  }

  /** Хранилище изменила другая вкладка. */
  refresh(): void {
    this.notify();
  }

  private write(ops: QueueOp[]): void {
    this.ops = ops;
    this.loaded = true;
    const raw = JSON.stringify({ v: 1, ops });
    try {
      this.storage?.setItem(this.storageKey, raw);
      this.raw = raw;
      this.volatile = !this.storage;
    } catch {
      this.volatile = true;
    }
    this.notify();
  }

  private notify(): void {
    for (const l of this.listeners) l();
    for (const l of anyListeners) l();
  }
}

function resetOp(o: QueueOp): QueueOp {
  const next: QueueOp = { ...o, status: 'pending' };
  delete next.error;
  delete next.available;
  delete next.fails;
  return next;
}

// ---------- Очереди по пользователям ----------

const queues = new Map<number, OfflineQueue>();

/** Очередь пользователя; очередь другого пользователя недоступна и не отправляется (ФТ-12.4.6). */
export function queueFor(userId: number, storage: StorageLike | null = browserStorage()): OfflineQueue {
  let q = queues.get(userId);
  if (!q) {
    q = new OfflineQueue(userId, storage);
    queues.set(userId, q);
  }
  return q;
}

/** Выход: очередь пользователя стирается (ФТ-12.5.2). */
export function dropQueue(userId: number): void {
  const q = queues.get(userId) ?? new OfflineQueue(userId, browserStorage());
  q.clear();
  queues.delete(userId);
}

/** Только для тестов: забыть созданные очереди. */
export function resetQueuesForTest(): void {
  queues.clear();
}
