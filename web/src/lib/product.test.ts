import { describe, expect, it } from 'vitest';
import { unitPriceText } from './money';
import { canRevert, movementTitle } from './movements';
import { allBrands, parsePh, phInput, suggestBrands, validateProduct, type ProductFields } from './product';
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
  purpose_ids: [],
  ph: '',
  incompatible: '',
  instruction: '',
  ...o,
});

describe('форма позиции', () => {
  it('годная форма', () => {
    const r = validateProduct(fields({ min_stock: '500' }));
    expect(r).toEqual({
      ok: true,
      value: { name: 'Green Star', brand: 'Koch Chemie', category_id: 3, unit: 'ml', package_size: 1000, min_stock: 500, barcode: '', note: '', purpose_ids: [], ph: null, incompatible: '', instruction: '' },
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

describe('карточка средства в форме (ФТ-15.2.1)', () => {
  it('pH: пусто, запятая и точка, один знак, 0–14', () => {
    expect(parsePh('')).toBeNull();
    expect(parsePh('  ')).toBeNull();
    expect(parsePh('7,5')).toBe(7.5);
    expect(parsePh('7.5')).toBe(7.5);
    expect(parsePh('0')).toBe(0);
    expect(parsePh('14')).toBe(14);
    expect(parsePh('14,0')).toBe(14);
    expect(parsePh('14,1')).toBeUndefined();
    expect(parsePh('15')).toBeUndefined();
    expect(parsePh('7,55')).toBeUndefined();
    expect(parsePh('-1')).toBeUndefined();
    expect(parsePh('кисл')).toBeUndefined();
    expect(parsePh('7,')).toBeUndefined();
  });
  it('pH в поле: запятая, пусто для null', () => {
    expect(phInput(7.5)).toBe('7,5');
    expect(phInput(null)).toBe('');
    expect(phInput(undefined)).toBe('');
  });
  it('в значение попадают назначения без повторов, pH числом, тексты обрезаны', () => {
    const r = validateProduct(fields({ purpose_ids: [2, 5, 2], ph: '9,5', incompatible: ' кислоты ', instruction: ' Развести 1:10 ' }));
    expect(r.ok && r.value).toMatchObject({ purpose_ids: [2, 5], ph: 9.5, incompatible: 'кислоты', instruction: 'Развести 1:10' });
  });
  it('ошибка pH', () => {
    const r = validateProduct(fields({ ph: '20' }));
    expect(!r.ok && r.errors.ph).toBeTruthy();
  });
  it('«Не смешивать с» — до 300 символов, инструкция — до 2000', () => {
    expect(validateProduct(fields({ incompatible: 'я'.repeat(300) })).ok).toBe(true);
    const a = validateProduct(fields({ incompatible: 'я'.repeat(301) }));
    expect(!a.ok && a.errors.incompatible).toBe('Не длиннее 300 символов');
    expect(validateProduct(fields({ instruction: 'я'.repeat(2000) })).ok).toBe(true);
    const b = validateProduct(fields({ instruction: 'я'.repeat(2001) }));
    expect(!b.ok && b.errors.instruction).toBe('Не длиннее 2000 символов');
  });
  it('эмодзи считается за один символ', () => {
    expect(validateProduct(fields({ incompatible: '🧪'.repeat(300) })).ok).toBe(true);
  });
});
