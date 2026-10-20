import { describe, expect, it } from 'vitest';
import type { ProductSummary, Recipe } from '../api';
import { estimateByKey, estimateRecipe, estimateSummary, pendingDeltas } from './offlineEstimate';
import { makeConsumeOp } from './offlineQueue';

const p = (over: Partial<ProductSummary> = {}): ProductSummary =>
  ({
    id: 7,
    name: 'Шампунь',
    unit: 'ml',
    min_stock: 300,
    low: false,
    stock: { total: 1000, open_qty: 400, open_capacity: 1000, sealed_count: 1, units_count: 2 },
    ...over,
  }) as ProductSummary;

describe('оценка остатков', () => {
  it('уменьшает общий остаток и вскрытую бутылку, пересчитывает «мало»', () => {
    const r = estimateSummary(p(), 150);
    expect(r.stock).toMatchObject({ total: 850, open_qty: 250 });
    expect(r.low).toBe(false);
    expect(estimateSummary(p(), 750).low).toBe(true);
  });
  it('не уходит ниже нуля; вскрытая закончилась — шкалы нет', () => {
    const r = estimateSummary(p(), 5000);
    expect(r.stock).toMatchObject({ total: 0, open_qty: null, open_capacity: null });
  });
  it('исходные данные не меняются, нулевое списание возвращает тот же объект', () => {
    const src = p();
    estimateSummary(src, 100);
    expect(src.stock.total).toBe(1000);
    expect(estimateSummary(src, 0)).toBe(src);
  });
  it('считает только ожидающие записи, суммирует по позиции', () => {
    const a = makeConsumeOp(p(), 10);
    const b = makeConsumeOp(p(), 20);
    const c = { ...makeConsumeOp(p(), 99), status: 'error' as const };
    expect([...pendingDeltas([a, b, c])]).toEqual([[7, 30]]);
  });
  it('карточка, список и шаблон', () => {
    const d = new Map([[7, 100]]);
    expect((estimateByKey(['products', {}], [p()], d) as ProductSummary[])[0]!.stock.total).toBe(900);
    expect(estimateByKey(['categories', true], [1], d)).toEqual([1]);
    const r = { id: 1, items: [{ product_id: 7, qty: 950, available: 1000, short: false, product: p() }] } as unknown as Recipe;
    const e = estimateRecipe(r, d);
    expect(e.items[0]).toMatchObject({ available: 900, short: true });
    expect(estimateRecipe(r, new Map())).toBe(r);
  });
});
