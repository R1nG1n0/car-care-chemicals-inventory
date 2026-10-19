// Форма позиции (ФТ-3): те же правила, что на сервере (POST /api/products),
// и подсказка бренда из введённых ранее.
import type { ProductInput, Unit } from '../api';
import { MAX_QTY, parseQty } from './qty';
import { charCount, type FieldErrors } from './validate';

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

/** Поля формы позиции, которые сервер может назвать в field ошибки. */
export const PRODUCT_FIELDS = ['name', 'brand', 'category_id', 'unit', 'package_size', 'min_stock', 'barcode', 'note'] as const;

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
  if (size === null || size < 1 || size > MAX_QTY) e.package_size = 'Целое число от 1 до 100 000';
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
/** Все бренды для выпадающего списка — по алфавиту, без учёта регистра. */
export function allBrands(all: readonly string[]): string[] {
  return [...all].sort((a, b) => a.localeCompare(b, 'ru-RU', { sensitivity: 'base' }));
}

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
