import { describe, expect, it } from 'vitest';
import type { LowItem, ProductSummary, ShoppingItem } from '../api';
import { lowSub, manualSub, manualTitle, sortManual, toggleDone } from './shopList';

const product = (p: Partial<ProductSummary> = {}): ProductSummary => ({
  id: 1,
  category_id: 1,
  name: 'Gyeon Q²M Foam',
  brand: 'Gyeon',
  unit: 'ml',
  package_size: 1000,
  barcode: '',
  min_stock: 200,
  note: '',
  archived: false,
  stock: { total: 80, open_qty: 80, open_capacity: 1000, sealed_count: 0, units_count: 1 },
  low: true,
  location_ids: [],
  last_consume_qty: null,
  ...p,
});

const low = (l: Partial<LowItem> = {}): LowItem => ({ product: product(), last_price: 245_000, last_store: 'Ozon', cheapest_store: 'Автохим', ...l });

const item = (i: Partial<ShoppingItem>): ShoppingItem => ({ id: 1, product_id: null, title: 'Глина', product: null, done_at: null, created_at: 100, ...i });

describe('lowSub', () => {
  it('минимум, цена и где дешевле', () => expect(lowSub(low())).toBe('мин. 200 мл · 2 450 ₽ · дешевле в Автохим'));
  it('дешевле там же, где брали', () => expect(lowSub(low({ cheapest_store: 'Ozon' }))).toBe('мин. 200 мл · 2 450 ₽ · Ozon'));
  it('не покупали', () => expect(lowSub(low({ last_price: null, last_store: null, cheapest_store: null }))).toBe('мин. 200 мл · не покупали'));
});

describe('manualSub', () => {
  it('куплено', () => expect(manualSub(item({ done_at: Date.UTC(2026, 9, 14, 12) / 1000 }))).toBe('куплено 14.10'));
  it('с позицией', () => {
    expect(manualSub(item({ product: product() }))).toBe('есть 80 мл');
    expect(manualSub(item({ product: product({ stock: { total: 0, open_qty: null, open_capacity: null, sealed_count: 0, units_count: 0 } }) }))).toBe('нет в наличии');
  });
  it('просто текст', () => expect(manualSub(item({}))).toBe('без позиции'));
  it('заголовок — имя позиции или текст', () => {
    expect(manualTitle(item({ product: product() }))).toBe('Gyeon Q²M Foam');
    expect(manualTitle(item({}))).toBe('Глина');
  });
});

describe('порядок пунктов', () => {
  it('выполненные уходят вниз', () => {
    const list = [item({ id: 1, done_at: 500 }), item({ id: 2, created_at: 50 }), item({ id: 3, created_at: 90 }), item({ id: 4, done_at: 600 })];
    expect(sortManual(list).map((i) => i.id)).toEqual([3, 2, 4, 1]);
  });
  it('toggleDone отмечает и снимает отметку с подъёмом наверх', () => {
    const list = [item({ id: 1, created_at: 10 }), item({ id: 2, created_at: 20 })];
    const done = toggleDone(list, 2, true, 999);
    expect(done[1]!.done_at).toBe(999);
    expect(sortManual(done).map((i) => i.id)).toEqual([1, 2]);
    const back = toggleDone(done, 2, false, 1000);
    expect(back[1]).toMatchObject({ done_at: null, created_at: 1000 });
  });
});
