// Граничные случаи ввода количества (ФТ-5.8): пустое поле, границы 1…остаток,
// шаги для шт, значение по умолчанию, выбор бутылки.
import { describe, expect, it } from 'vitest';
import type { StockUnit } from '../api';
import { applyStep, bottleOptions, clampQty, consumeHint, consumeToastText, consumeUnitId, defaultBottle, defaultConsumeQty, parseQty, stepButtons } from './stockQty';

const sp = (s: string) => s.replace(/[\u00a0\u202f]/gu, ' ');
const stock = (total: number, open: number | null = null, sealed = 0) => ({
  total,
  open_qty: open,
  open_capacity: open === null ? null : 1000,
  sealed_count: sealed,
  units_count: sealed + (open === null ? 0 : 1),
});
const unit = (p: Partial<StockUnit>): StockUnit => ({
  id: 1,
  product_id: 1,
  location_id: null,
  initial_qty: 1000,
  remaining_qty: 1000,
  status: 'sealed',
  opened_at: null,
  created_at: 1,
  unit_price: null,
  ...p,
});

describe('parseQty — поле количества', () => {
  it('ноль, ведущие нули, неразрывные пробелы', () => {
    expect(parseQty('0')).toBe(0);
    expect(parseQty('0040')).toBe(40);
    expect(parseQty('1\u00a0000')).toBe(1000);
    expect(parseQty(' 40 ')).toBe(40);
  });
  it('пусто, дробь, минус, буквы, 10 цифр — null', () => {
    for (const s of ['', '   ', '1.5', '1,5', '-5', '−5', '+5', '5мл', '1e3', '1234567890']) expect(parseQty(s)).toBeNull();
  });
});

describe('clampQty', () => {
  it('дробь округляется, max null/undefined — без верха', () => {
    expect(clampQty(2.6, 0, 10)).toBe(3);
    expect(clampQty(1e6, 0, null)).toBe(1e6);
    expect(clampQty(1e6, 0, undefined)).toBe(1e6);
  });
  it('max ниже min (пустой остаток) — держится на min', () => {
    expect(clampQty(50, 0, 0)).toBe(0);
    expect(clampQty(50, 1, 0)).toBe(1);
  });
});

describe('кнопки шагов', () => {
  it('для шт шаги не зависят от настроек профиля', () => {
    expect(stepButtons('pcs', { step_small: 25, step_large: 100 })).toEqual([-1, 1, 5]);
  });
  it('для мл без настроек — 10 и 50', () => expect(stepButtons('ml', undefined)).toEqual([-50, -10, 10, 50]));
  it('шт: пустое поле +1 → 1, −1 от нуля не уходит в минус, +5 упирается в остаток', () => {
    expect(applyStep('', 1, 0, 3)).toBe(1);
    expect(applyStep('0', -1, 0, 3)).toBe(0);
    expect(applyStep('2', 5, 0, 3)).toBe(3);
  });
  it('мл: вручную введено больше остатка — любая кнопка возвращает в границы', () => {
    expect(applyStep('900', -10, 0, 400)).toBe(400);
    expect(applyStep('900', 10, 0, 400)).toBe(400);
  });
  it('мусор в поле считается нулём', () => expect(applyStep('abc', 10, 0, 400)).toBe(10));
  it('−50 от 30 — ноль, а не минус (П-12)', () => expect(applyStep('30', -50, 0, 400)).toBe(0));
});

describe('значение по умолчанию для расхода', () => {
  const p = (last: number | null, total: number, u: 'ml' | 'pcs' = 'ml') => ({ unit: u, last_consume_qty: last, stock: stock(total) });
  it('прошлое 600 при остатке 400 — 400', () => expect(defaultConsumeQty(p(600, 400), undefined)).toBe(400));
  it('шт без прошлого — 1, даже если шаг профиля 25', () => expect(defaultConsumeQty(p(null, 10, 'pcs'), { step_small: 25, step_large: 100 })).toBe(1));
  it('мл без прошлого — малый шаг профиля', () => expect(defaultConsumeQty(p(null, 1000), { step_small: 25, step_large: 100 })).toBe(25));
  it('малый шаг больше остатка — остаток', () => expect(defaultConsumeQty(p(null, 5), undefined)).toBe(5));
  it('пусто — 0', () => expect(defaultConsumeQty(p(40, 0), undefined)).toBe(0));
});

describe('подсказка расхода', () => {
  it('без прошлого и без вскрытой — только «Всего …» с заглавной', () => {
    expect(sp(consumeHint({ unit: 'ml', last_consume_qty: null, stock: stock(2000, null, 2) }))).toBe('Всего 2 000 мл');
  });
  it('шт — «в пачке»', () => {
    expect(sp(consumeHint({ unit: 'pcs', last_consume_qty: 1, stock: { ...stock(16, 6, 1), open_capacity: 10 } }))).toBe(
      'В прошлый раз 1 шт · в пачке 6 шт · всего 16 шт',
    );
  });
});

describe('выбор бутылки', () => {
  it('вскрытые с одинаковым opened_at — меньший id', () => {
    const opts = bottleOptions([unit({ id: 9, status: 'open', opened_at: 5, remaining_qty: 10 }), unit({ id: 4, status: 'open', opened_at: 5, remaining_qty: 20 })], 'ml');
    expect(opts.map((o) => o.unitId)).toEqual([4]);
  });
  it('вскрытая без opened_at считается самой ранней', () => {
    const opts = bottleOptions([unit({ id: 2, status: 'open', opened_at: 7 }), unit({ id: 3, status: 'open', opened_at: null })], 'ml');
    expect(opts[0]!.unitId).toBe(3);
  });
  it('списанные не предлагаются; пачки подписаны «пачка»', () => {
    const opts = bottleOptions(
      [unit({ id: 1, status: 'written_off', remaining_qty: 0 }), unit({ id: 2, status: 'open', opened_at: 1, remaining_qty: 6 }), unit({ id: 3, remaining_qty: 10 })],
      'pcs',
    );
    expect(opts.map((o) => sp(o.label))).toEqual(['Вскрытая пачка · 6', 'Новая пачка · 10']);
    expect(defaultBottle(opts)).toBe(2);
  });
  it('пустой список — по умолчанию ничего', () => expect(defaultBottle([])).toBeUndefined());
});

describe('уведомление о расходе', () => {
  it('без закончившихся', () => expect(sp(consumeToastText('Gsf', 'ml', 40, { finished_unit_ids: [] }))).toBe('Списано 40 мл · Gsf'));
  it('пачки: одна и пять', () => {
    expect(sp(consumeToastText('Салфетки', 'pcs', 10, { finished_unit_ids: [1] }))).toBe('Списано 10 шт · Салфетки · пачка закончилась');
    expect(sp(consumeToastText('Салфетки', 'pcs', 50, { finished_unit_ids: [1, 2, 3, 4, 5] }))).toBe('Списано 50 шт · Салфетки · закончились 5 пачек');
  });
  it('две бутылки', () => expect(sp(consumeToastText('Gsf', 'ml', 1200, { finished_unit_ids: [1, 2] }))).toBe('Списано 1 200 мл · Gsf · закончились 2 бутылки'));
});

describe('consumeUnitId — какую бутылку слать в /api/consume', () => {
  const two = [
    { unitId: 5, kind: 'open' as const, label: '', qty: 300 },
    { unitId: 7, kind: 'sealed' as const, label: '', qty: 1000 },
  ];
  it('одна бутылка или ни одной — не слать', () => {
    expect(consumeUnitId([], 7)).toBeUndefined();
    expect(consumeUnitId([two[1]!], 7)).toBeUndefined();
  });
  it('две — выбранная', () => expect(consumeUnitId(two, 7)).toBe(7));
  it('выбор пропал — вскрытая по умолчанию', () => {
    expect(consumeUnitId(two, 99)).toBe(5);
    expect(consumeUnitId(two, undefined)).toBe(5);
  });
});
