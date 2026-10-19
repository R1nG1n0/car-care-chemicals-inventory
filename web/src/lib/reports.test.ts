import { describe, expect, it } from 'vitest';
import type { PurchaseRow } from '../api';
import {
  barLevels,
  canGoNext,
  chartGeometry,
  compactMoney,
  customError,
  labelStep,
  MIN_LEVEL,
  monthFull,
  monthLabel,
  parseIso,
  percentText,
  periodOf,
  periodTitle,
  priceChangeText,
  priceRangeText,
  priceSeries,
  shareOf,
  shiftPeriod,
  switchKind,
  visibleLevel,
} from './reports';

const ref = { y: 2026, m: 10, d: 14 };

describe('период', () => {
  it('месяц, квартал и год вокруг даты', () => {
    expect(periodOf('month', ref)).toEqual({ kind: 'month', from: '2026-10-01', to: '2026-10-31' });
    expect(periodOf('quarter', ref)).toEqual({ kind: 'quarter', from: '2026-10-01', to: '2026-12-31' });
    expect(periodOf('quarter', { y: 2026, m: 5, d: 2 })).toEqual({ kind: 'quarter', from: '2026-04-01', to: '2026-06-30' });
    expect(periodOf('year', ref)).toEqual({ kind: 'year', from: '2026-01-01', to: '2026-12-31' });
  });
  it('февраль: 28 и 29 дней', () => {
    expect(periodOf('month', { y: 2026, m: 2, d: 1 }).to).toBe('2026-02-28');
    expect(periodOf('month', { y: 2024, m: 2, d: 1 }).to).toBe('2024-02-29');
  });
  it('сдвиг через границу года', () => {
    const jan = periodOf('month', { y: 2026, m: 1, d: 5 });
    expect(shiftPeriod(jan, -1)).toEqual({ kind: 'month', from: '2025-12-01', to: '2025-12-31' });
    expect(shiftPeriod(shiftPeriod(jan, -1), 1)).toEqual(jan);
    expect(shiftPeriod(periodOf('quarter', { y: 2026, m: 1, d: 1 }), -1).from).toBe('2025-10-01');
    expect(shiftPeriod(periodOf('year', ref), -1)).toEqual({ kind: 'year', from: '2025-01-01', to: '2025-12-31' });
  });
  it('«свой» период не сдвигается', () => {
    const own = { kind: 'custom' as const, from: '2026-01-05', to: '2026-02-03' };
    expect(shiftPeriod(own, 1)).toBe(own);
    expect(canGoNext(own, ref)).toBe(false);
  });
  it('вперёд нельзя дальше текущего периода', () => {
    expect(canGoNext(periodOf('month', ref), ref)).toBe(false);
    expect(canGoNext(periodOf('month', { y: 2026, m: 9, d: 1 }), ref)).toBe(true);
    expect(canGoNext(periodOf('year', { y: 2025, m: 3, d: 1 }), ref)).toBe(true);
    expect(canGoNext(periodOf('year', ref), ref)).toBe(false);
    // Месяц начинается сегодня — следующий уже в будущем
    expect(canGoNext(periodOf('month', { y: 2026, m: 9, d: 1 }), { y: 2026, m: 10, d: 1 })).toBe(true);
    expect(canGoNext(periodOf('month', { y: 2026, m: 9, d: 1 }), { y: 2026, m: 9, d: 30 })).toBe(false);
  });
  it('смена вида сохраняет время', () => {
    const month = periodOf('month', { y: 2026, m: 8, d: 3 });
    expect(switchKind(month, 'quarter', ref)).toEqual({ kind: 'quarter', from: '2026-07-01', to: '2026-09-30' });
    expect(switchKind(month, 'year', ref).from).toBe('2026-01-01');
    expect(switchKind(month, 'custom', ref)).toEqual({ kind: 'custom', from: '2026-08-01', to: '2026-08-31' });
    expect(switchKind(month, 'month', ref)).toBe(month);
  });
  it('смена вида внутри текущего периода даёт текущий', () => {
    expect(switchKind(periodOf('year', ref), 'quarter', ref)).toEqual({ kind: 'quarter', from: '2026-10-01', to: '2026-12-31' });
    expect(switchKind(periodOf('year', ref), 'month', ref).from).toBe('2026-10-01');
    expect(switchKind(periodOf('year', { y: 2025, m: 4, d: 1 }), 'month', ref).from).toBe('2025-01-01');
  });
  it('названия', () => {
    expect(periodTitle(periodOf('month', ref))).toBe('Октябрь 2026');
    expect(periodTitle(periodOf('quarter', ref))).toBe('IV квартал 2026');
    expect(periodTitle(periodOf('quarter', { y: 2026, m: 1, d: 1 }))).toBe('I квартал 2026');
    expect(periodTitle(periodOf('year', ref))).toBe('2026 год');
    expect(periodTitle({ kind: 'custom', from: '2026-10-01', to: '2026-10-14' })).toBe('01.10.2026 — 14.10.2026');
  });
  it('разбор даты отвергает несуществующие', () => {
    expect(parseIso('2026-02-31')).toBeNull();
    expect(parseIso('2026-13-01')).toBeNull();
    expect(parseIso('26-1-1')).toBeNull();
    expect(parseIso('2024-02-29')).toEqual({ y: 2024, m: 2, d: 29 });
  });
});

describe('проверка своего периода', () => {
  it('пусто, верно, конец раньше начала, больше пяти лет', () => {
    expect(customError('', '2026-01-01')).toBe('Укажите обе даты');
    expect(customError('2026-01-01', '2026-02-30')).toBe('Укажите обе даты');
    expect(customError('2026-01-01', '2026-01-01')).toBe('');
    expect(customError('2026-02-01', '2026-01-31')).toBe('Конец периода раньше начала');
    expect(customError('2021-01-01', '2026-01-01')).toBe('');
    expect(customError('2021-01-01', '2026-01-02')).toBe('Период не больше 5 лет');
    expect(customError('2024-02-29', '2029-02-28')).toBe('');
    expect(customError('2024-02-29', '2029-03-01')).toBe('Период не больше 5 лет');
  });
});

describe('доли и столбики', () => {
  it('доля и проценты', () => {
    expect(shareOf(25, 100)).toBe(0.25);
    expect(shareOf(5, 0)).toBe(0);
    expect(shareOf(200, 100)).toBe(1);
    expect(percentText(0.4249)).toBe('42%');
    expect(percentText(0.004)).toBe('<1%');
    expect(percentText(0)).toBe('0%');
    expect(percentText(1)).toBe('100%');
  });
  it('ненулевое значение остаётся видимым', () => {
    expect(visibleLevel(0)).toBe(0);
    expect(visibleLevel(0.001)).toBe(MIN_LEVEL);
    expect(visibleLevel(0.5)).toBe(0.5);
  });
  it('уровни столбиков по максимуму', () => {
    expect(barLevels([50, 100, 0])).toEqual([0.5, 1, 0]);
    expect(barLevels([0, 0])).toEqual([0, 0]);
    expect(barLevels([1, 1000])).toEqual([MIN_LEVEL, 1]);
    expect(barLevels([])).toEqual([]);
  });
  it('подписи месяцев не слипаются', () => {
    expect(labelStep(1)).toBe(1);
    expect(labelStep(12)).toBe(1);
    expect(labelStep(13)).toBe(3);
    expect(labelStep(61)).toBe(11);
  });
  it('название месяца', () => {
    expect(monthLabel('2026-10', false)).toBe('окт');
    expect(monthLabel('2026-01', true)).toBe('янв 26');
    expect(monthLabel('bad', false)).toBe('bad');
    expect(monthFull('2026-03')).toBe('март 2026');
  });
  it('короткая сумма', () => {
    expect(compactMoney(0)).toBe('0');
    expect(compactMoney(95000)).toBe('950');
    expect(compactMoney(238000)).toBe('2,4 тыс.');
    expect(compactMoney(1_000_000)).toBe('10 тыс.');
    expect(compactMoney(150_000_000)).toBe('1,5 млн');
  });
});

const row = (id: number, date: string, p100?: number, unit?: number): PurchaseRow => ({
  purchase_id: id,
  date,
  store: null,
  packages: 1,
  price_per_package: 1,
  price_per_100: p100,
  price_per_unit: unit,
});

describe('история цены', () => {
  it('ряд — от старых к новым, строки без цены пропускаются', () => {
    const rows = [row(3, '2026-10-01', 130), row(2, '2026-09-01'), row(1, '2026-08-01', 110)];
    expect(priceSeries(rows, 'ml')).toEqual([
      { date: '2026-08-01', value: 110 },
      { date: '2026-10-01', value: 130 },
    ]);
    expect(priceSeries([row(2, '2026-09-01', undefined, 95), row(1, '2026-08-01', undefined, 90)], 'pcs').map((p) => p.value)).toEqual([90, 95]);
    expect(priceSeries(rows, 'pcs')).toEqual([]);
  });
  it('график строится от двух точек', () => {
    expect(chartGeometry([100], 300, 100)).toBeNull();
    const c = chartGeometry([100, 200, 150], 300, 100, 10, 10);
    expect(c?.points).toEqual([
      { x: 10, y: 90 },
      { x: 150, y: 10 },
      { x: 290, y: 50 },
    ]);
    expect(c?.path).toBe('M10 90 L150 10 L290 50');
    expect([c?.min, c?.max]).toEqual([100, 200]);
  });
  it('равные цены — линия по центру', () => {
    expect(chartGeometry([5, 5], 100, 60)?.points.map((p) => p.y)).toEqual([30, 30]);
  });
  it('подписи диапазона и изменения', () => {
    expect(priceRangeText([11000, 13500, 12000], 'ml')).toBe('От 110 ₽ до 135 ₽ за 100 мл');
    expect(priceRangeText([9000, 9000], 'pcs')).toBe('Всегда 90 ₽ за шт');
    expect(priceRangeText([], 'ml')).toBe('');
    expect(priceChangeText([10000, 10800])).toBe('+8% к прошлой покупке');
    expect(priceChangeText([10000, 9000])).toBe('−10% к прошлой покупке');
    expect(priceChangeText([10000, 10000])).toBe('без изменений к прошлой покупке');
    expect(priceChangeText([10000, 10004])).toBe('почти без изменений к прошлой покупке');
    expect(priceChangeText([1])).toBe('');
  });
});
