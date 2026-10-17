// Список покупок (ФТ-7): подписи строк и порядок пунктов.
import type { LowItem, ShoppingItem } from '../api';
import { formatMoney } from './format';
import { qtyText } from './stock';
import { formatDay } from './text';

/** «мин. 200 мл · 2 450 ₽ · дешевле в Автохим» (ФТ-7.3). */
export function lowSub(item: LowItem): string {
  const p = item.product;
  const parts: string[] = [];
  if (p.min_stock !== null) parts.push(`мин. ${qtyText(p.min_stock, p.unit)}`);
  if (item.last_price === null) parts.push('не покупали');
  else {
    parts.push(formatMoney(item.last_price));
    if (item.cheapest_store && item.cheapest_store !== item.last_store) parts.push(`дешевле в ${item.cheapest_store}`);
    else if (item.last_store) parts.push(item.last_store);
  }
  return parts.join(' · ');
}

/** Подпись ручного пункта: «куплено 14.10», «есть 80 мл», «нет в наличии», «без позиции». */
export function manualSub(item: ShoppingItem): string {
  if (item.done_at !== null) return `куплено ${formatDay(item.done_at)}`;
  const p = item.product;
  if (!p) return 'без позиции';
  return p.stock.total > 0 ? `есть ${qtyText(p.stock.total, p.unit)}` : 'нет в наличии';
}

export const manualTitle = (item: ShoppingItem) => item.product?.name ?? item.title;

/** Невыполненные сверху (новые первыми), выполненные — вниз (недавно отмеченные первыми). */
export function sortManual(items: readonly ShoppingItem[]): ShoppingItem[] {
  return [...items].sort((a, b) => {
    const ad = a.done_at !== null;
    const bd = b.done_at !== null;
    if (ad !== bd) return ad ? 1 : -1;
    if (ad && bd) return b.done_at! - a.done_at! || b.id - a.id;
    return b.created_at - a.created_at || b.id - a.id;
  });
}

/** Отметка «куплено» сразу в кэше, до ответа сервера. */
export function toggleDone(items: readonly ShoppingItem[], id: number, done: boolean, nowSec: number): ShoppingItem[] {
  return items.map((i) => (i.id !== id ? i : done ? { ...i, done_at: nowSec } : { ...i, done_at: null, created_at: nowSec }));
}
