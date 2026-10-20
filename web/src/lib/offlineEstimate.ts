// Оценка остатков по неотправленным записям (ФТ-12.3.2): на экране остаток уже
// уменьшен на записанное. Оценка считается от последних данных сервера и ничего
// не меняет в них — отмена записи просто возвращает исходные числа.
import type { ProductDetail, ProductSummary, Recipe } from '../api';
import type { QueueOp } from './offlineQueue';

/** Сколько списать по каждой позиции: только записи, которые ждут отправки. */
export function pendingDeltas(ops: readonly QueueOp[]): Map<number, number> {
  const m = new Map<number, number>();
  const add = (id: number, qty: number) => m.set(id, (m.get(id) ?? 0) + qty);
  for (const op of ops) {
    if (op.status !== 'pending') continue;
    if (op.kind === 'consume') add(op.product_id, op.qty);
    else for (const i of op.items) add(i.product_id, i.qty);
  }
  return m;
}

/** Сводка позиции после списания qty: общий остаток и вскрытая бутылка, «мало» пересчитывается. */
export function estimateSummary(p: ProductSummary, qty: number): ProductSummary {
  if (qty <= 0) return p;
  const total = Math.max(0, p.stock.total - qty);
  let { open_qty, open_capacity } = p.stock;
  if (open_qty !== null) {
    if (qty < open_qty) open_qty -= qty;
    else {
      open_qty = null;
      open_capacity = null;
    }
  }
  return {
    ...p,
    stock: { ...p.stock, total, open_qty, open_capacity },
    low: p.min_stock !== null && total < p.min_stock,
  };
}

const pick = (p: ProductSummary, d: ReadonlyMap<number, number>) => estimateSummary(p, d.get(p.id) ?? 0);

export function estimateProducts(list: readonly ProductSummary[], d: ReadonlyMap<number, number>): ProductSummary[] {
  return list.map((p) => pick(p, d));
}

export function estimateDetail(detail: ProductDetail, d: ReadonlyMap<number, number>): ProductDetail {
  const product = pick(detail.product, d);
  return product === detail.product ? detail : { ...detail, product };
}

export function estimateRecipe(r: Recipe, d: ReadonlyMap<number, number>): Recipe {
  if (!r.items.some((i) => d.has(i.product_id))) return r;
  return {
    ...r,
    items: r.items.map((i) => {
      const qty = d.get(i.product_id) ?? 0;
      if (qty <= 0) return i;
      const available = Math.max(0, i.available - qty);
      return { ...i, available, short: available < i.qty, product: estimateSummary(i.product, qty) };
    }),
  };
}

/** Оценка для данных запроса по его ключу; чужие ключи возвращаются как есть. */
export function estimateByKey(key: readonly unknown[], data: unknown, d: ReadonlyMap<number, number>): unknown {
  if (d.size === 0 || data == null) return data;
  switch (key[0]) {
    case 'products':
      return Array.isArray(data) ? estimateProducts(data as ProductSummary[], d) : data;
    case 'product':
      return estimateDetail(data as ProductDetail, d);
    case 'recipes':
      return Array.isArray(data) ? (data as Recipe[]).map((r) => estimateRecipe(r, d)) : data;
    case 'recipe':
      return estimateRecipe(data as Recipe, d);
    default:
      return data;
  }
}

/** Ключи запросов, к данным которых применяется оценка. */
export const ESTIMATE_ROOTS: readonly string[] = ['products', 'product', 'recipes', 'recipe'];
