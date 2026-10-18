import { describe, expect, it } from 'vitest';
import type { Movement, ProductSummary } from '../api';
import { clampQty } from './qty';
import { applyItems, lastManualConsume, MAX_QTY, parseRowQty, qtyChanged, recipeSub, shortageReport, shortageText, validateRecipe } from './recipeApply';

let seq = 1;
// Группа нажатия по умолчанию — по времени: одно время — одно нажатие
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
  batch_id: p.batch_id ?? p.created_at ?? 100,
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
  it('parseRowQty', () => {
    expect(parseRowQty('')).toBe(0);
    expect(parseRowQty('1 200')).toBe(1200);
    expect(parseRowQty('12a')).toBeNull();
    // Без тихой обрезки: больше предела — ошибка формы, а не подмена числа
    expect(parseRowQty('999999')).toBe(999_999);
  });
  it('clampQty', () => {
    expect(clampQty(-5, 0, MAX_QTY)).toBe(0);
    expect(clampQty(250_000, 0, MAX_QTY)).toBe(MAX_QTY);
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

describe('отчёт и подпись шаблона', () => {
  const product = (name: string, unit: 'ml' | 'pcs') => ({ name, unit }) as ProductSummary;
  const items = [
    { product_id: 1, product: product('Koch Gsf', 'ml') },
    { product_id: 2, product: product('Салфетка', 'pcs') },
  ];
  const sp = (s: string) => s.replace(/[\u00a0\u202f]/gu, ' ');
  it('shortageReport: имена и количества, неизвестная позиция — «Позиция»', () => {
    const r = shortageReport(
      [
        { product_id: 1, requested: 600, consumed: 400 },
        { product_id: 2, requested: 3, consumed: 0 },
        { product_id: 9, requested: 1, consumed: 0 },
      ],
      items,
    );
    expect(sp(r)).toBe('Koch Gsf — списано 400 мл из 600 мл; Салфетка — списано 0 шт из 3 шт; Позиция');
  });
  it('recipeSub: число позиций и себестоимость', () => {
    expect(sp(recipeSub({ items: items as never, cost: 127_220 }))).toBe('2 позиции · ≈ 1 272,20 ₽');
    expect(recipeSub({ items: [items[0]!] as never, cost: null })).toBe('1 позиция');
  });
});
