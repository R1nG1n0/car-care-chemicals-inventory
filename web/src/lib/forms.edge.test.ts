// Проверка форм против правил docs/API.md: позиция, покупка, справочники, шаги,
// логин и пароль — на самых границах.
import { describe, expect, it } from 'vitest';
import { validateProduct, type ProductFields } from './product';
import { refNameError, storeUrlError } from './refsValidate';
import { emptyLine, MAX_LINES, purchaseHasErrors, toPurchaseInput, validatePurchase, type LineDraft, type PurchaseDraft } from './shopPurchase';
import { loginError, passwordError, validateSignup, validateSteps } from './validate';

const fields = (f: Partial<ProductFields> = {}): ProductFields => ({
  name: 'Koch Chemie Gsf',
  brand: 'Koch Chemie',
  category_id: 3,
  unit: 'ml',
  package_size: '1000',
  min_stock: '500',
  barcode: '',
  note: '',
  ...f,
});
const errs = (f: Partial<ProductFields>) => {
  const r = validateProduct(fields(f));
  return r.ok ? {} : r.errors;
};

describe('позиция (POST /api/products)', () => {
  it('объём упаковки 1 и 100 000 — можно, 0 и 100 001 — нельзя', () => {
    expect(errs({ package_size: '1' }).package_size).toBeUndefined();
    expect(errs({ package_size: '100 000' }).package_size).toBeUndefined();
    expect(errs({ package_size: '0' }).package_size).toBeDefined();
    expect(errs({ package_size: '100001' }).package_size).toBeDefined();
    expect(errs({ package_size: '' }).package_size).toBeDefined();
    expect(errs({ package_size: '1,5' }).package_size).toBeDefined();
  });
  it('минимум: пусто → null, 0 → 0, 1 000 000 можно, больше и минус — нельзя', () => {
    const empty = validateProduct(fields({ min_stock: '  ' }));
    expect(empty.ok && empty.value.min_stock).toBeNull();
    const zero = validateProduct(fields({ min_stock: '0' }));
    expect(zero.ok && zero.value.min_stock).toBe(0);
    expect(errs({ min_stock: '1 000 000' }).min_stock).toBeUndefined();
    expect(errs({ min_stock: '1000001' }).min_stock).toBeDefined();
    expect(errs({ min_stock: '-1' }).min_stock).toBeDefined();
  });
  it('штрихкод: пробелы убираются, 32 цифры можно, 33 и буквы — нельзя', () => {
    const r = validateProduct(fields({ barcode: '4607 0721 9012 4' }));
    expect(r.ok && r.value.barcode).toBe('4607072190124');
    expect(errs({ barcode: '1'.repeat(32) }).barcode).toBeUndefined();
    expect(errs({ barcode: '1'.repeat(33) }).barcode).toBeDefined();
    expect(errs({ barcode: '46O7' }).barcode).toBeDefined();
  });
  it('имя 80 символов, бренд 60, заметка 500 — считаются символы, не UTF-16', () => {
    expect(errs({ name: '🧴'.repeat(80) }).name).toBeUndefined();
    expect(errs({ name: 'x'.repeat(81) }).name).toBeDefined();
    expect(errs({ name: '   ' }).name).toBeDefined();
    expect(errs({ brand: '' }).brand).toBeUndefined();
    expect(errs({ brand: 'x'.repeat(61) }).brand).toBeDefined();
    expect(errs({ note: '🧽'.repeat(500) }).note).toBeUndefined();
    expect(errs({ note: 'x'.repeat(501) }).note).toBeDefined();
  });
  it('без категории — ошибка поля category_id', () => expect(errs({ category_id: null }).category_id).toBeDefined());
});

const line = (key: number, l: Partial<LineDraft> = {}): LineDraft => ({ ...emptyLine(key, key), price: '1190', ...l });
const draft = (d: Partial<PurchaseDraft> = {}): PurchaseDraft => ({ date: '2026-10-17', store_id: null, lines: [line(1)], ...d });

describe('покупка (POST /api/purchases)', () => {
  it('30 строк можно, 31 — нельзя', () => {
    const many = (n: number) => Array.from({ length: n }, (_, i) => line(i + 1));
    expect(purchaseHasErrors(validatePurchase(draft({ lines: many(MAX_LINES) })))).toBe(false);
    expect(validatePurchase(draft({ lines: many(MAX_LINES + 1) })).lines).toBeDefined();
  });
  it('упаковок 1 и 100 — можно, 0, 101 и дробь — нельзя', () => {
    for (const n of [1, 100]) expect(validatePurchase(draft({ lines: [line(1, { packages: n })] })).byLine[1]?.packages).toBeUndefined();
    for (const n of [0, 101, 1.5]) expect(validatePurchase(draft({ lines: [line(1, { packages: n })] })).byLine[1]?.packages).toBeDefined();
  });
  it('цена ровно 1 000 000 ₽ — можно, на копейку больше — нельзя', () => {
    expect(validatePurchase(draft({ lines: [line(1, { price: '1 000 000' })] })).byLine[1]).toBeUndefined();
    expect(validatePurchase(draft({ lines: [line(1, { price: '1 000 000,01' })] })).byLine[1]?.price).toMatch(/1 000 000/u);
  });
  it('цена из одних пробелов — «Укажите цену»', () => {
    expect(validatePurchase(draft({ lines: [line(1, { price: '   ' })] })).byLine[1]?.price).toMatch(/Укажите/u);
  });
  it('неверные даты', () => {
    for (const date of ['17.10.2026', '2026-10-7', '2026-13-01', '']) expect(validatePurchase(draft({ date })).date).toBeDefined();
  });
  // Было: Date.parse в V8 принимает «2026-02-30» (переносит на 2 марта). Теперь
  // дата сверяется с календарём.
  it('30 февраля — неверная дата, 29 февраля високосного — верная', () => {
    expect(validatePurchase(draft({ date: '2026-02-30' })).date).toBeDefined();
    expect(validatePurchase(draft({ date: '2025-02-29' })).date).toBeDefined();
    expect(validatePurchase(draft({ date: '2028-02-29' })).date).toBeUndefined();
  });
  it('тело запроса: «2 450,50» → 245050 копеек', () => {
    expect(toPurchaseInput(draft({ lines: [line(1, { price: '2 450,50', packages: 2 })] })).lines[0]).toMatchObject({ packages: 2, price_per_package: 245_050 });
  });
});

describe('справочники', () => {
  it('имя: 40 символов можно, 41 — нельзя, эмодзи — один символ', () => {
    expect(refNameError('x'.repeat(40))).toBeUndefined();
    expect(refNameError('x'.repeat(41))).toBeDefined();
    expect(refNameError('📦'.repeat(40))).toBeUndefined();
    expect(refNameError('  Коробка 3  ')).toBeUndefined();
  });
  it('ссылка магазина: схема без адреса, пробел внутри, ftp — нельзя; регистр схемы и пробелы по краям — можно', () => {
    for (const u of ['http://', 'https://a b', 'ftp://x', 'javascript:alert(1)', 'www.ozon.ru']) expect(storeUrlError(u)).toBeDefined();
    expect(storeUrlError('  HTTPS://ozon.ru  ')).toBeUndefined();
  });
});

describe('шаги и учётные данные (ТБ-7)', () => {
  it('шаги: 1000 и 5000 можно, равные — нельзя, минус — нельзя', () => {
    expect(validateSteps('1000', '5000').ok).toBe(true);
    expect(validateSteps('1', '2').ok).toBe(true);
    expect(validateSteps('50', '50').ok).toBe(false);
    expect(validateSteps('-1', '50').ok).toBe(false);
    expect(validateSteps('', '').ok).toBe(false);
  });
  it('логин ровно 3 и 32 символа; кириллица и пробел по краям — нельзя', () => {
    expect(loginError('abc')).toBeUndefined();
    expect(loginError('a'.repeat(32))).toBeUndefined();
    expect(loginError(' abc')).toBeDefined();
    expect(loginError('аbc')).toBeDefined(); // первая «а» — кириллица
  });
  it('пароль: 128 эмодзи можно, 129 — нельзя', () => {
    expect(passwordError('🔑'.repeat(128))).toBeUndefined();
    expect(passwordError('x'.repeat(129))).toBeDefined();
  });
  it('регистрация: при плохом пароле повтор не ругается', () => {
    expect(validateSignup({ login: 'friend', password: 'short', repeat: 'other' })).toEqual({ password: 'Не короче 8 символов' });
  });
});
