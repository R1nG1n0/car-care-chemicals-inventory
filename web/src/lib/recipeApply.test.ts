import { describe, expect, it } from 'vitest';
import type { Movement } from '../api';
import { applyItems, clampQty, lastManualConsume, parseQty, qtyChanged, rowStep, shortageText, validateRecipe } from './recipeApply';

let seq = 1;
const mv = (p: Partial<Movement>): Movement => ({
  id: seq++,
  unit_id: 1,
  product_id: 1,
  type: 'consume',
  qty_delta: -10,
  from_location_id: null,
  to_location_id: null,
  reason: '',
  recipe_run_id: null,
  cost: null,
  created_at: 100,
  ...p,
});

describe('lastManualConsume', () => {
  it('берёт последнюю группу ручного расхода и складывает экземпляры одной позиции', () => {
    const log = [
      mv({ type: 'purchase', qty_delta: 1000, created_at: 300 }),
      mv({ product_id: 9, qty_delta: -50, created_at: 250, recipe_run_id: 4 }),
      mv({ product_id: 2, unit_id: 5, qty_delta: -15, created_at: 200 }),
      mv({ product_id: 2, unit_id: 4, qty_delta: -25, created_at: 200 }),
      mv({ product_id: 3, qty_delta: -40, created_at: 100 }),
    ];
    expect(lastManualConsume(log)).toEqual([{ product_id: 2, qty: 40 }]);
  });
  it('нет ручного расхода — null', () => {
    expect(lastManualConsume([mv({ recipe_run_id: 1 }), mv({ type: 'writeoff' })])).toBeNull();
    expect(lastManualConsume([])).toBeNull();
  });
  it('несколько позиций одного момента — по порядку списания', () => {
    const log = [mv({ product_id: 7, qty_delta: -3, created_at: 50 }), mv({ product_id: 6, qty_delta: -20, created_at: 50 })];
    expect(lastManualConsume(log)).toEqual([
      { product_id: 6, qty: 20 },
      { product_id: 7, qty: 3 },
    ]);
  });
});

describe('нехватка', () => {
  it('хватает — без текста', () => expect(shortageText(40, 2320, 'ml')).toBeNull());
  it('не хватает — сколько спишется', () => expect(shortageText(100, 80, 'ml')).toBe('Есть только 80 мл — спишется 80'));
  it('нет совсем', () => expect(shortageText(1, 0, 'pcs')).toMatch(/Нет в наличии/));
});

describe('количество', () => {
  it('parseQty', () => {
    expect(parseQty('')).toBe(0);
    expect(parseQty('1 200')).toBe(1200);
    expect(parseQty('12a')).toBeNull();
    expect(parseQty('999999')).toBe(100_000);
  });
  it('clampQty и шаг', () => {
    expect(clampQty(-5)).toBe(0);
    expect(rowStep('ml', 10)).toBe(10);
    expect(rowStep('pcs', 10)).toBe(1);
  });
  it('applyItems пропускает нули', () => {
    expect(applyItems([{ product_id: 1, qty: 0 }, { product_id: 2, qty: 5 }])).toEqual([{ product_id: 2, qty: 5 }]);
  });
  it('qtyChanged', () => {
    const a = [{ product_id: 1, qty: 5 }];
    expect(qtyChanged(a, [{ product_id: 1, qty: 5 }])).toBe(false);
    expect(qtyChanged(a, [{ product_id: 1, qty: 6 }])).toBe(true);
    expect(qtyChanged([], a)).toBe(true);
  });
});

describe('validateRecipe', () => {
  it('правильный шаблон → тело запроса', () => {
    const r = validateRecipe(' Мойка ', [{ key: 1, product_id: 3, qty: '40' }]);
    expect(r).toEqual({ ok: true, value: { name: 'Мойка', items: [{ product_id: 3, qty: 40 }] } });
  });
  it('ошибки имени, строк, повторов и количества', () => {
    const r = validateRecipe('', [
      { key: 1, product_id: 3, qty: '0' },
      { key: 2, product_id: 3, qty: '10' },
      { key: 3, product_id: null, qty: 'x' },
    ]);
    expect(r.ok).toBe(false);
    if (r.ok) return;
    expect(r.errors.name).toBeTruthy();
    expect(r.errors.byRow[1]?.qty).toBeTruthy();
    expect(r.errors.byRow[2]?.product).toMatch(/уже есть/);
    expect(r.errors.byRow[3]).toMatchObject({ product: 'Выберите позицию', qty: 'От 1 до 100 000' });
  });
  it('пустой шаблон', () => {
    const r = validateRecipe('А', []);
    expect(!r.ok && r.errors.rows).toBeTruthy();
  });
});
