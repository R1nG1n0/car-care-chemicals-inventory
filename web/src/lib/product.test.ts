import { describe, expect, it } from 'vitest';
import { unitPriceText } from './money';
import { canRevert, movementTitle } from './movements';
import { allBrands, suggestBrands, validateProduct, type ProductFields } from './product';
import { purchaseDay } from './text';

const norm = (s: string | null) => (s ?? '').replace(/\s/gu, ' ');

describe('деньги', () => {
  it('цена за 100 мл и за шт', () => {
    const row = { purchase_id: 1, date: '2026-10-02', store: null, packages: 2, price_per_package: 119000, price_per_100: 11900 };
    expect(norm(unitPriceText(row, 'ml'))).toBe('119 ₽ за 100 мл');
    expect(norm(unitPriceText({ ...row, price_per_100: undefined, price_per_unit: 9000 }, 'pcs'))).toBe('90 ₽ за шт');
    expect(unitPriceText({ ...row, price_per_100: undefined }, 'ml')).toBeNull();
  });
  it('дата покупки', () => {
    expect(purchaseDay('2026-10-02', 2026)).toBe('02.10');
    expect(purchaseDay('2025-07-14', 2026)).toBe('14.07.2025');
  });
});

describe('журнал', () => {
  it('подписи движений', () => {
    expect(norm(movementTitle({ type: 'consume', qty_delta: -40, reason: '' }, 'ml'))).toBe('Расход · 40 мл');
    expect(norm(movementTitle({ type: 'adjust', qty_delta: 20, reason: '' }, 'ml'))).toBe('Корректировка · +20 мл');
    expect(norm(movementTitle({ type: 'adjust', qty_delta: 500, reason: 'Начальный остаток' }, 'ml'))).toBe('Начальный остаток · +500 мл');
    expect(movementTitle({ type: 'move', qty_delta: 0, reason: '' }, 'ml')).toBe('Перемещение');
  });
  it('что можно отменить', () => {
    expect(canRevert({ type: 'consume' })).toBe(true);
    expect(canRevert({ type: 'writeoff' })).toBe(true);
    expect(canRevert({ type: 'adjust' })).toBe(true);
    expect(canRevert({ type: 'purchase' })).toBe(false);
    expect(canRevert({ type: 'move' })).toBe(false);
  });
});

const fields = (o: Partial<ProductFields> = {}): ProductFields => ({
  name: ' Green Star ',
  brand: 'Koch Chemie',
  category_id: 3,
  unit: 'ml',
  package_size: '1 000',
  min_stock: '',
  barcode: '',
  note: '',
  ...o,
});

describe('форма позиции', () => {
  it('годная форма', () => {
    const r = validateProduct(fields({ min_stock: '500' }));
    expect(r).toEqual({
      ok: true,
      value: { name: 'Green Star', brand: 'Koch Chemie', category_id: 3, unit: 'ml', package_size: 1000, min_stock: 500, barcode: '', note: '' },
    });
    const empty = validateProduct(fields());
    expect(empty.ok && empty.value.min_stock).toBeNull();
  });
  it('ошибки по полям', () => {
    const r = validateProduct(fields({ name: '  ', category_id: null, package_size: '0', min_stock: 'x', barcode: '12a' }));
    expect(r.ok).toBe(false);
    if (!r.ok) expect(Object.keys(r.errors).sort()).toEqual(['barcode', 'category_id', 'min_stock', 'name', 'package_size']);
  });
});

describe('подсказка бренда', () => {
  const all = ['Gyeon', 'Koch Chemie', 'Sonax', 'Grass', 'Чистый Кот'];
  it('по началу, затем по вхождению, без учёта регистра', () => {
    expect(suggestBrands(all, 'ko')).toEqual(['Koch Chemie']);
    expect(suggestBrands(all, 'g')).toEqual(['Gyeon', 'Grass']);
    expect(suggestBrands(all, 'чис')).toEqual(['Чистый Кот']);
    expect(suggestBrands(all, '')).toEqual([]);
    expect(suggestBrands(all, 'sonax')).toEqual([]);
  });
});

describe('выпадающий список брендов', () => {
  it('все бренды по алфавиту без учёта регистра (русская сортировка: кириллица раньше), исходный массив не меняется', () => {
    const all = ['sonax', 'Gyeon', 'Чистый Кот', 'Koch Chemie', 'grass'];
    expect(allBrands(all)).toEqual(['Чистый Кот', 'grass', 'Gyeon', 'Koch Chemie', 'sonax']);
    expect(all[0]).toBe('sonax');
    expect(allBrands([])).toEqual([]);
  });
});
