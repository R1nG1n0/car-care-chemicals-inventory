// Деньги и журнал на карточке позиции: разбор суммы, цена за 100 мл / за шт,
// даты покупок, подписи движений, проверка формы позиции.
import type { Movement, ProductInput, PurchaseRow, Unit } from '../api';
import { formatMoney } from './format';
import { qtyText } from './stock';
import { parseQty } from './stockQty';
import { charCount, type FieldErrors } from './validate';

/** «1 190,50» / «1190.5» / «1 190 ₽» → копейки; не сумма — null. */
export function parseMoney(s: string): number | null {
  const t = s.replace(/[\s\u00a0₽]/gu, '').replace(',', '.');
  const m = /^(\d{1,9})(?:\.(\d{0,2}))?$/u.exec(t);
  if (!m) return null;
  const kop = (m[2] ?? '').padEnd(2, '0');
  return Number(m[1]) * 100 + Number(kop);
}

/** «119 ₽ за 100 мл» / «90 ₽ за шт»; цены нет — null. */
export function unitPriceText(row: PurchaseRow, unit: Unit): string | null {
  if (unit === 'pcs') return row.price_per_unit !== undefined ? `${formatMoney(row.price_per_unit)} за шт` : null;
  return row.price_per_100 !== undefined ? `${formatMoney(row.price_per_100)} за 100 мл` : null;
}

/** «2026-10-02» → «02.10»; другой год — «02.10.2025». */
export function purchaseDay(date: string, nowYear = new Date().getFullYear()): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/u.exec(date);
  if (!m) return date;
  const dm = `${m[3]}.${m[2]}`;
  return Number(m[1]) === nowYear ? dm : `${dm}.${m[1]}`;
}

const TYPE: Record<Movement['type'], string> = {
  purchase: 'Покупка',
  consume: 'Расход',
  writeoff: 'Списание',
  move: 'Перемещение',
  adjust: 'Корректировка',
};

/** «Расход · 40 мл», «Корректировка · +20 мл», «Перемещение». */
export function movementTitle(m: Pick<Movement, 'type' | 'qty_delta' | 'reason'>, unit: Unit): string {
  const name = m.type === 'adjust' && m.reason === 'Начальный остаток' ? 'Начальный остаток' : TYPE[m.type];
  if (m.type === 'move' || m.qty_delta === 0) return name;
  const abs = qtyText(Math.abs(m.qty_delta), unit);
  if (m.type === 'adjust') return `${name} · ${m.qty_delta > 0 ? '+' : '−'}${abs}`;
  return `${name} · ${abs}`;
}

/** Отменить можно расход, списание и корректировку (docs/API.md, revert). */
export function canRevert(m: Pick<Movement, 'type'>): boolean {
  return m.type === 'consume' || m.type === 'writeoff' || m.type === 'adjust';
}

/** Вопрос подтверждения отмены из журнала. */
export function revertQuestion(m: Pick<Movement, 'type' | 'qty_delta' | 'reason'>, unit: Unit): string {
  return `Отменить: ${movementTitle(m, unit).toLocaleLowerCase('ru-RU')}? Остаток пересчитается.`;
}

export type ProductFields = {
  name: string;
  brand: string;
  category_id: number | null;
  unit: Unit;
  package_size: string;
  min_stock: string;
  barcode: string;
  note: string;
};

export type ProductErrors = FieldErrors<keyof ProductInput | 'form'>;

type Result = { ok: true; value: ProductInput } | { ok: false; errors: ProductErrors };

/** Форма позиции — те же правила, что на сервере (POST /api/products). */
export function validateProduct(f: ProductFields): Result {
  const e: ProductErrors = {};
  const name = f.name.trim();
  const brand = f.brand.trim();
  const barcode = f.barcode.replace(/\s/gu, '');
  if (name === '') e.name = 'Введите название';
  else if (charCount(name) > 80) e.name = 'Не длиннее 80 символов';
  if (charCount(brand) > 60) e.brand = 'Не длиннее 60 символов';
  if (f.category_id === null) e.category_id = 'Выберите категорию';
  const size = parseQty(f.package_size);
  if (size === null || size < 1 || size > 100_000) e.package_size = 'Целое число от 1 до 100 000';
  let min: number | null = null;
  if (f.min_stock.trim() !== '') {
    min = parseQty(f.min_stock);
    if (min === null || min > 1_000_000) e.min_stock = 'Целое число от 0 до 1 000 000 или пусто';
  }
  if (!/^\d{0,32}$/u.test(barcode)) e.barcode = 'Только цифры, до 32';
  if (charCount(f.note) > 500) e.note = 'Не длиннее 500 символов';
  if (Object.keys(e).length > 0) return { ok: false, errors: e };
  return {
    ok: true,
    value: { name, brand, category_id: f.category_id!, unit: f.unit, package_size: size!, min_stock: min, barcode, note: f.note.trim() },
  };
}

/** Подсказка бренда: введённые ранее, начинаются с набранного (или содержат), до 5. */
export function suggestBrands(all: readonly string[], typed: string): string[] {
  const q = typed.trim().toLocaleLowerCase('ru-RU');
  if (q === '') return [];
  const low = (s: string) => s.toLocaleLowerCase('ru-RU');
  const exact = all.some((b) => low(b) === q);
  if (exact) return [];
  const starts = all.filter((b) => low(b).startsWith(q));
  const has = all.filter((b) => !low(b).startsWith(q) && low(b).includes(q));
  return [...starts, ...has].slice(0, 5);
}
