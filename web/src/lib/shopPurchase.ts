// Форма новой покупки (ФТ-4): строки, итог, проверка перед отправкой.
import type { PurchaseInput } from '../api';
import { MAX_PRICE_KOPECKS, parseRubles } from './money';

export const MAX_LINES = 30;
export const MAX_PACKAGES = 100;

export type LineDraft = {
  /** Ключ строки в списке: не зависит от позиции и не меняется при удалении соседей. */
  key: number;
  product_id: number | null;
  packages: number;
  /** Цена за упаковку в рублях, как ввёл человек. */
  price: string;
  location_id: number | null;
};

export type PurchaseDraft = { date: string; store_id: number | null; lines: LineDraft[] };

export type LineErrors = Partial<Record<'product' | 'packages' | 'price', string>>;
export type PurchaseErrors = { date?: string; lines?: string; byLine: Record<number, LineErrors> };

export function emptyLine(key: number, product_id: number | null = null): LineDraft {
  return { key, product_id, packages: 1, price: '', location_id: null };
}

/** Сегодня в виде ГГГГ-ММ-ДД по местному времени (не UTC: вечером дата не «убегает»). */
export function todayISO(now = new Date()): string {
  const y = now.getFullYear();
  const m = String(now.getMonth() + 1).padStart(2, '0');
  const d = String(now.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

export function clampPackages(n: number): number {
  if (!Number.isFinite(n)) return 1;
  return Math.min(MAX_PACKAGES, Math.max(1, Math.trunc(n)));
}

/** Сумма строки в копейках; цена не введена или неверна — 0. */
export function lineTotal(l: LineDraft): number {
  const p = parseRubles(l.price);
  return p === null ? 0 : p * l.packages;
}

export function purchaseTotal(lines: LineDraft[]): number {
  return lines.reduce((s, l) => s + lineTotal(l), 0);
}

export function packagesTotal(lines: LineDraft[]): number {
  return lines.reduce((s, l) => s + l.packages, 0);
}

/** «ГГГГ-ММ-ДД» и такой день есть в календаре: «2026-02-30» — нет (Date.parse его принимает). */
export function isCalendarDate(s: string): boolean {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/u.exec(s);
  if (!m) return false;
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])];
  const t = new Date(Date.UTC(y, mo - 1, d));
  return t.getUTCFullYear() === y && t.getUTCMonth() === mo - 1 && t.getUTCDate() === d;
}

export function validatePurchase(d: PurchaseDraft): PurchaseErrors {
  const errors: PurchaseErrors = { byLine: {} };
  if (!isCalendarDate(d.date)) errors.date = 'Укажите дату покупки';
  if (d.lines.length === 0) errors.lines = 'Добавьте хотя бы одну позицию';
  if (d.lines.length > MAX_LINES) errors.lines = `В покупке — не больше ${MAX_LINES} строк`;
  const seen = new Set<number>();
  for (const l of d.lines) {
    const e: LineErrors = {};
    if (l.product_id === null) e.product = 'Выберите позицию';
    else if (seen.has(l.product_id)) e.product = 'Эта позиция уже есть в покупке — увеличьте число упаковок';
    if (l.product_id !== null) seen.add(l.product_id);
    if (l.packages < 1 || l.packages > MAX_PACKAGES || !Number.isInteger(l.packages)) e.packages = `От 1 до ${MAX_PACKAGES}`;
    const price = parseRubles(l.price);
    if (l.price.trim() === '') e.price = 'Укажите цену, можно 0';
    else if (price === null) e.price = 'Цена — число, например 2450 или 2450,50';
    else if (price > MAX_PRICE_KOPECKS) e.price = 'Не больше 1 000 000 ₽';
    if (Object.keys(e).length > 0) errors.byLine[l.key] = e;
  }
  return errors;
}

export function purchaseHasErrors(e: PurchaseErrors): boolean {
  return Boolean(e.date || e.lines) || Object.keys(e.byLine).length > 0;
}

/** Черновик → тело POST /api/purchases. Вызывать после validatePurchase без ошибок. */
export function toPurchaseInput(d: PurchaseDraft): PurchaseInput {
  return {
    date: d.date,
    store_id: d.store_id,
    note: '',
    lines: d.lines.map((l) => ({
      product_id: l.product_id!,
      packages: l.packages,
      price_per_package: parseRubles(l.price) ?? 0,
      location_id: l.location_id,
    })),
  };
}
