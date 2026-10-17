import { describe, expect, it } from 'vitest';
import type { StockUnit } from '../api';
import {
  applyStep,
  bottleOptions,
  clampQty,
  consumeHint,
  consumeToastText,
  defaultBottle,
  defaultConsumeQty,
  parseQty,
  stepButtons,
  stepLabel,
  stepText,
} from './stockQty';

const norm = (s: string) => s.replace(/\s/gu, ' ');
const settings = { step_small: 10, step_large: 50 };
const st = (total: number, open: number | null = null) => ({
  total,
  open_qty: open,
  open_capacity: open === null ? null : 1000,
  sealed_count: 0,
  units_count: 1,
});

describe('поле количества', () => {
  it('разбирает целое с пробелами', () => {
    expect(parseQty('1 000')).toBe(1000);
    expect(parseQty('40')).toBe(40);
    expect(parseQty('')).toBeNull();
    expect(parseQty('4,5')).toBeNull();
    expect(parseQty('-3')).toBeNull();
  });
  it('границы', () => {
    expect(clampQty(-5, 0, 100)).toBe(0);
    expect(clampQty(500, 0, 320)).toBe(320);
    expect(clampQty(500, 0, null)).toBe(500);
  });
});

describe('кнопки шагов (ФТ-5.8)', () => {
  it('мл — из настроек, шт — −1 +1 +5', () => {
    expect(stepButtons('ml', settings)).toEqual([-50, -10, 10, 50]);
    expect(stepButtons('ml', { step_small: 5, step_large: 25 })).toEqual([-25, -5, 5, 25]);
    expect(stepButtons('pcs', settings)).toEqual([-1, 1, 5]);
  });
  it('не уходит ниже нуля и выше остатка', () => {
    expect(applyStep('40', -50, 0, 320)).toBe(0);
    expect(applyStep('300', 50, 0, 320)).toBe(320);
    expect(applyStep('', 10, 0, 320)).toBe(10);
  });
  it('подписи', () => {
    expect(stepText(-50)).toBe('−50');
    expect(stepText(10)).toBe('+10');
    expect(norm(stepLabel(-10, 'ml'))).toBe('Минус 10 мл');
    expect(norm(stepLabel(5, 'pcs'))).toBe('Плюс 5 шт');
  });
});

describe('значение по умолчанию', () => {
  it('прошлое количество, иначе малый шаг; не выше остатка, не ниже 1', () => {
    expect(defaultConsumeQty({ unit: 'ml', last_consume_qty: 40, stock: st(2320) }, settings)).toBe(40);
    expect(defaultConsumeQty({ unit: 'ml', last_consume_qty: null, stock: st(2320) }, settings)).toBe(10);
    expect(defaultConsumeQty({ unit: 'pcs', last_consume_qty: null, stock: st(6) }, settings)).toBe(1);
    expect(defaultConsumeQty({ unit: 'ml', last_consume_qty: 600, stock: st(260) }, settings)).toBe(260);
    expect(defaultConsumeQty({ unit: 'ml', last_consume_qty: 0, stock: st(260) }, settings)).toBe(1);
    expect(defaultConsumeQty({ unit: 'ml', last_consume_qty: 40, stock: st(0) }, settings)).toBe(0);
  });
  it('подсказка', () => {
    expect(norm(consumeHint({ unit: 'ml', last_consume_qty: 40, stock: st(2320, 320) }))).toBe('В прошлый раз 40 мл · во вскрытой 320 мл · всего 2 320 мл');
    expect(norm(consumeHint({ unit: 'ml', last_consume_qty: null, stock: st(1000) }))).toBe('Всего 1 000 мл');
  });
});

const unit = (o: Partial<StockUnit>): StockUnit => ({
  id: 1,
  product_id: 1,
  location_id: null,
  initial_qty: 1000,
  remaining_qty: 1000,
  status: 'sealed',
  opened_at: null,
  created_at: 0,
  unit_price: null,
  ...o,
});

describe('выбор бутылки (ФТ-5.2)', () => {
  it('вскрытая и новая, по умолчанию вскрытая', () => {
    const opts = bottleOptions(
      [unit({ id: 7, status: 'open', remaining_qty: 320, opened_at: 20 }), unit({ id: 6, status: 'open', remaining_qty: 100, opened_at: 10 }), unit({ id: 8 })],
      'ml',
    );
    expect(opts.map((o) => o.unitId)).toEqual([6, 8]);
    expect(norm(opts[1]!.label)).toBe('Новая · 1 000');
    expect(defaultBottle(opts)).toBe(6);
  });
  it('только запечатанные — один вариант', () => {
    const opts = bottleOptions([unit({ id: 3 }), unit({ id: 4 })], 'ml');
    expect(opts).toHaveLength(1);
    expect(defaultBottle(opts)).toBe(3);
    expect(defaultBottle([])).toBeUndefined();
  });
  it('законченные не предлагаются', () => {
    expect(bottleOptions([unit({ status: 'finished', remaining_qty: 0 })], 'ml')).toEqual([]);
  });
});

describe('уведомление о расходе', () => {
  it('с закончившейся бутылкой', () => {
    expect(norm(consumeToastText('Gsf', 'ml', 40, { finished_unit_ids: [] }))).toBe('Списано 40 мл · Gsf');
    expect(norm(consumeToastText('Gsf', 'ml', 600, { finished_unit_ids: [1] }))).toBe('Списано 600 мл · Gsf · бутылка закончилась');
    expect(norm(consumeToastText('Салфетки', 'pcs', 30, { finished_unit_ids: [1, 2] }))).toBe('Списано 30 шт · Салфетки · закончились 2 пачки');
  });
});
