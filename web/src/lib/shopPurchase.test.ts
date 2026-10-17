import { describe, expect, it } from 'vitest';
import {
  clampPackages,
  emptyLine,
  packagesTotal,
  purchaseHasErrors,
  purchaseTotal,
  todayISO,
  toPurchaseInput,
  validatePurchase,
  type LineDraft,
} from './shopPurchase';

const line = (key: number, p: Partial<LineDraft> = {}): LineDraft => ({ ...emptyLine(key, 10 + key), price: '100', ...p });

describe('итог покупки', () => {
  it('сумма цен × упаковки в копейках', () => {
    const lines = [line(1, { packages: 2, price: '2 450' }), line(2, { price: '99,90' })];
    expect(purchaseTotal(lines)).toBe(2 * 245_000 + 9_990);
    expect(packagesTotal(lines)).toBe(3);
  });
  it('строка без цены не ломает итог', () => {
    expect(purchaseTotal([line(1, { price: '' }), line(2, { price: 'xx' }), line(3, { price: '5' })])).toBe(500);
  });
});

describe('clampPackages', () => {
  it('от 1 до 100, целое', () => {
    expect(clampPackages(0)).toBe(1);
    expect(clampPackages(101)).toBe(100);
    expect(clampPackages(2.7)).toBe(2);
    expect(clampPackages(Number.NaN)).toBe(1);
  });
});

describe('todayISO', () => {
  it('местная дата', () => expect(todayISO(new Date(2026, 9, 5, 23, 30))).toBe('2026-10-05'));
});

describe('validatePurchase', () => {
  const ok = { date: '2026-10-15', store_id: null, lines: [line(1)] };
  it('правильная покупка без ошибок', () => expect(purchaseHasErrors(validatePurchase(ok))).toBe(false));
  it('нет даты', () => expect(validatePurchase({ ...ok, date: '' }).date).toBeTruthy());
  it('нет строк', () => expect(validatePurchase({ ...ok, lines: [] }).lines).toBeTruthy());
  it('ошибки привязаны к строке по ключу', () => {
    const e = validatePurchase({ ...ok, lines: [line(1), line(7, { product_id: null, price: '' })] });
    expect(e.byLine[1]).toBeUndefined();
    expect(e.byLine[7]?.product).toBe('Выберите позицию');
    expect(e.byLine[7]?.price).toBeTruthy();
  });
  it('повтор позиции, неверная и слишком большая цена', () => {
    const e = validatePurchase({
      ...ok,
      lines: [line(1, { product_id: 5 }), line(2, { product_id: 5, price: '1,234' }), line(3, { price: '1000001' })],
    });
    expect(e.byLine[2]?.product).toMatch(/уже есть/);
    expect(e.byLine[2]?.price).toMatch(/число/);
    expect(e.byLine[3]?.price).toMatch(/1 000 000/);
  });
  it('цена 0 допустима', () => expect(purchaseHasErrors(validatePurchase({ ...ok, lines: [line(1, { price: '0' })] }))).toBe(false));
});

describe('toPurchaseInput', () => {
  it('рубли → копейки, место строки сохраняется', () => {
    const body = toPurchaseInput({ date: '2026-10-15', store_id: 2, lines: [line(1, { packages: 2, price: '2450,5', location_id: 4 })] });
    expect(body).toEqual({
      date: '2026-10-15',
      store_id: 2,
      note: '',
      lines: [{ product_id: 11, packages: 2, price_per_package: 245_050, location_id: 4 }],
    });
  });
});
