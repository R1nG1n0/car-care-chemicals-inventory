// Деньги (ФТ-4.4): человек вводит рубли с копейками, сервер хранит копейки
// целым числом. Здесь же — цена за 100 мл / за шт в истории покупок.
import type { PurchaseRow, Unit } from '../api';
import { formatMoney } from './format';

/** Наибольшая цена упаковки по договору API: 1 000 000 ₽ в копейках. */
export const MAX_PRICE_KOPECKS = 100_000_000;

/**
 * «2 450», «2450,5», «2450.50 ₽» → копейки. Пусто или не число — null.
 * Разделитель копеек — запятая или точка, не больше двух знаков;
 * пробелы (в том числе неразрывные) между разрядами допускаются.
 */
export function parseRubles(input: string): number | null {
  const s = input.replace(/\s/g, '').replace(/₽$/, '').replace(/руб\.?$/i, '');
  const m = /^(\d+)(?:[.,](\d{0,2}))?$/.exec(s);
  if (!m) return null;
  const rub = Number(m[1]);
  const kop = Number((m[2] ?? '').padEnd(2, '0'));
  const total = rub * 100 + kop;
  return Number.isSafeInteger(total) ? total : null;
}

/** Копейки → текст для поля ввода: «2450», «2450,50». */
export function kopecksToInput(kopecks: number): string {
  const rub = Math.trunc(kopecks / 100);
  const kop = Math.abs(kopecks % 100);
  return kop === 0 ? String(rub) : `${rub},${String(kop).padStart(2, '0')}`;
}

/** «119 ₽ за 100 мл» / «90 ₽ за шт»; цены нет — null. */
export function unitPriceText(row: PurchaseRow, unit: Unit): string | null {
  if (unit === 'pcs') return row.price_per_unit !== undefined ? `${formatMoney(row.price_per_unit)} за шт` : null;
  return row.price_per_100 !== undefined ? `${formatMoney(row.price_per_100)} за 100 мл` : null;
}
