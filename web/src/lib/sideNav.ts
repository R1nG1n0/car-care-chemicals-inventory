// Счётчики бокового меню на компьютере: «мало» у остатков, пункты к покупке,
// неотправленные записи. Чистые функции — экран только рисует.
import type { ProductSummary } from '../api';

export type ShoppingCountSource = {
  low: readonly unknown[];
  manual: readonly { done_at: number | null }[];
};

/** Сколько позиций ниже минимума (как плашка «N мало» на главной). */
export function lowCount(products: readonly ProductSummary[] | undefined): number {
  return products ? products.filter((p) => p.low).length : 0;
}

/** Сколько осталось купить: позиции ниже минимума и невычеркнутые ручные пункты. */
export function toBuyCount(list: ShoppingCountSource | undefined): number {
  if (!list) return 0;
  return list.low.length + list.manual.filter((m) => m.done_at === null).length;
}

/** Текст счётчика: пусто при нуле, «99+» для больших чисел. */
export function countText(n: number): string {
  if (n <= 0) return '';
  return n > 99 ? '99+' : String(n);
}

/** Короткая версия сборки для подписи внизу меню («dev» — локальная сборка). */
export function versionText(version: string | undefined): string {
  if (!version) return '';
  return version === 'dev' ? 'версия для разработки' : `версия ${version.slice(0, 7)}`;
}
