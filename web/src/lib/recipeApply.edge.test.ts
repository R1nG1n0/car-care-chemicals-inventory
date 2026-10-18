// Граничные случаи шаблонов (ФТ-8): «последний расход» из журнала, нехватка,
// границы количества и формы против правил docs/API.md.
import { describe, expect, it } from 'vitest';
import type { Movement } from '../api';
import { applyItems, lastManualConsume, MAX_QTY, parseRowQty, qtyChanged, shortageText, validateRecipe, type RecipeRowDraft } from './recipeApply';

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
  // Группа нажатия по умолчанию — по времени; отдельные нажатия задают batch_id явно
  batch_id: p.batch_id ?? p.created_at ?? 100,
});
const sp = (s: string | null) => s?.replace(/[\u00a0\u202f]/gu, ' ') ?? null;
const rows = (n: number): RecipeRowDraft[] => Array.from({ length: n }, (_, i) => ({ key: i, product_id: i + 1, qty: '10' }));

describe('lastManualConsume — «Из последнего расхода» (ФТ-8.5)', () => {
  it('пустой журнал — null', () => expect(lastManualConsume([])).toBeNull());
  it('только списания, корректировки и перемещения — null', () => {
    expect(lastManualConsume([mv({ type: 'writeoff' }), mv({ type: 'adjust', qty_delta: 20 }), mv({ type: 'move', qty_delta: 0 })])).toBeNull();
  });
  it('расход по шаблону сверху пропускается, берётся ручной под ним', () => {
    const log = [mv({ product_id: 7, recipe_run_id: 3, created_at: 500 }), mv({ product_id: 7, recipe_run_id: 3, created_at: 500 }), mv({ product_id: 2, qty_delta: -40, created_at: 400 })];
    expect(lastManualConsume(log)).toEqual([{ product_id: 2, qty: 40 }]);
  });
  it('ручной и шаблонный в одну секунду — шаблонный в группу не входит', () => {
    const log = [mv({ product_id: 2, qty_delta: -40, created_at: 400 }), mv({ product_id: 7, qty_delta: -50, recipe_run_id: 3, created_at: 400 })];
    expect(lastManualConsume(log)).toEqual([{ product_id: 2, qty: 40 }]);
  });
  it('добор из двух бутылок (П-5: 500 + 100) — одна строка 600', () => {
    const log = [mv({ unit_id: 2, qty_delta: -100, created_at: 300 }), mv({ unit_id: 1, qty_delta: -500, created_at: 300 })];
    expect(lastManualConsume(log)).toEqual([{ product_id: 1, qty: 600 }]);
  });
  it('расход постарше другой секунды не подмешивается', () => {
    const log = [mv({ qty_delta: -40, created_at: 301 }), mv({ qty_delta: -60, created_at: 300 })];
    expect(lastManualConsume(log)).toEqual([{ product_id: 1, qty: 40 }]);
  });
  // Группа — по batch_id: два отдельных нажатия «Списать» в одну секунду
  // не сливаются, берётся последнее.
  it('два нажатия в одну секунду — только последнее', () => {
    const log = [mv({ qty_delta: -40, created_at: 300, batch_id: 12 }), mv({ qty_delta: -60, created_at: 300, batch_id: 11 })];
    expect(lastManualConsume(log)).toEqual([{ product_id: 1, qty: 40 }]);
  });
  it('добор из второй бутылки в одном нажатии — одна группа даже в разные секунды', () => {
    const log = [mv({ unit_id: 2, qty_delta: -100, created_at: 301, batch_id: 20 }), mv({ unit_id: 1, qty_delta: -500, created_at: 300, batch_id: 20 })];
    expect(lastManualConsume(log)).toEqual([{ product_id: 1, qty: 600 }]);
  });
});

describe('нехватка', () => {
  it('ровно столько, сколько есть — хватает', () => expect(shortageText(80, 80, 'ml')).toBeNull());
  it('ноль при пустом складе — не нехватка', () => expect(shortageText(0, 0, 'ml')).toBeNull());
  it('шт и отрицательный остаток', () => {
    expect(sp(shortageText(5, 2, 'pcs'))).toBe('Есть только 2 шт — спишется 2');
    expect(shortageText(5, -1, 'pcs')).toMatch(/Нет в наличии/u);
  });
});

describe('количество строки', () => {
  it('пробелы-разряды, неразрывный пробел', () => {
    expect(parseRowQty('1 000')).toBe(1000);
    expect(parseRowQty('1\u00a0000')).toBe(1000);
  });
  it('дробь, минус — null', () => {
    expect(parseRowQty('1,5')).toBeNull();
    expect(parseRowQty('-5')).toBeNull();
  });
  it('выше предела — не обрезается молча', () => expect(parseRowQty('250000')).toBe(250_000));
  it('ровно предел — можно', () => expect(validateRecipe('Мойка', [{ key: 1, product_id: 1, qty: String(MAX_QTY) }]).ok).toBe(true));
  it('applyItems: все нули — пустое тело (сервер ответил бы 400)', () => expect(applyItems([{ product_id: 1, qty: 0 }])).toEqual([]));
  it('qtyChanged: строку убрали — изменилось', () => expect(qtyChanged([{ product_id: 1, qty: 10 }], [{ product_id: 1, qty: 10 }, { product_id: 2, qty: 5 }])).toBe(true));
  it('qtyChanged: тот же порядок и количества — нет', () => expect(qtyChanged([{ product_id: 1, qty: 10 }], [{ product_id: 1, qty: 10 }])).toBe(false));
});

describe('validateRecipe против docs/API.md (имя 1–60, 1–30 строк, без повторов)', () => {
  it('ровно 30 строк — можно, 31 — нельзя', () => {
    expect(validateRecipe('Мойка', rows(30)).ok).toBe(true);
    const r = validateRecipe('Мойка', rows(31));
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.errors.rows).toMatch(/30/u);
  });
  it('имя: 60 символов (эмодзи — один символ) можно, 61 — нельзя, пробелы обрезаются', () => {
    expect(validateRecipe('🧽'.repeat(60), rows(1)).ok).toBe(true);
    expect(validateRecipe('a'.repeat(61), rows(1)).ok).toBe(false);
    const r = validateRecipe('  Мойка  ', rows(1));
    expect(r.ok && r.value.name).toBe('Мойка');
  });
  it('пустое количество и ноль — ошибка строки', () => {
    const r = validateRecipe('Мойка', [{ key: 5, product_id: 1, qty: '' }, { key: 6, product_id: 2, qty: '0' }]);
    expect(r.ok).toBe(false);
    if (!r.ok) expect(Object.keys(r.errors.byRow)).toEqual(['5', '6']);
  });
  it('количество с пробелами-разрядами уходит числом', () => {
    const r = validateRecipe('Мойка', [{ key: 1, product_id: 1, qty: '1 000' }]);
    expect(r.ok && r.value.items).toEqual([{ product_id: 1, qty: 1000 }]);
  });
  // Было: 250 000 молча превращалось в 100 000. Теперь — ошибка «От 1 до 100 000».
  it('количество больше 100 000 — ошибка, а не тихая подмена', () => {
    const r = validateRecipe('Мойка', [{ key: 1, product_id: 1, qty: '250000' }]);
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.errors.byRow[1]?.qty).toBe('От 1 до 100 000');
  });
});
