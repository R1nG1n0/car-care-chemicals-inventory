// Количество (мл или шт): один разбор поля и одни границы на весь сайт.
// Ввод расхода, корректировки, шаблона и остатка разбирается здесь.
import type { Unit } from '../api';
import { qtyText } from './stock';

/** Наибольшее количество за раз по договору API (расход, строка шаблона, упаковка). */
export const MAX_QTY = 100_000;

/** Целое из поля: пробелы-разряды допустимы; пусто или не число — null. */
export function parseQty(s: string): number | null {
  const t = s.replace(/[\s\u00a0]/gu, '');
  if (!/^\d{1,9}$/u.test(t)) return null;
  return Number(t);
}

/** В границах [min, max]; max null/undefined — без верхней границы. */
export function clampQty(n: number, min: number, max: number | null | undefined): number {
  if (!Number.isFinite(n)) return min;
  const hi = max === null || max === undefined ? Infinity : Math.max(min, max);
  return Math.min(hi, Math.max(min, Math.round(n)));
}

/**
 * Проверка расхода одной позиции (POST /api/consume): больше нуля, не больше
 * остатка и не больше 100 000 за раз. null — можно отправлять.
 */
export function validateConsume(n: number | null, total: number, unit: Unit): string | null {
  if (n === null || n <= 0) return 'Введите количество';
  if (n > total) return `Не хватает: есть ${qtyText(total, unit)}`;
  if (n > MAX_QTY) return `За раз — не больше ${qtyText(MAX_QTY, unit)}`;
  return null;
}
