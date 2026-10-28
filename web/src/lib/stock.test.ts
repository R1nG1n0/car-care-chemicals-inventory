import { describe, expect, it } from 'vitest';
import type { Location, ProductSummary, Stock } from '../api';
import {
  crossedMin,
  locationChip,
  matchProduct,
  meterLevel,
  meterValues,
  normalize,
  openLine,
  parseFilter,
  productMeta,
  replaceProducts,
  stickerText,
  stockCounts,
  visibleProducts,
} from './stock';

const norm = (s: string) => s.replace(/\s/gu, ' ');

const stock = (s: Partial<Stock>): Stock => ({ total: 0, open_qty: null, open_capacity: null, sealed_count: 0, units_count: 0, ...s });

const prod = (over: Partial<ProductSummary>): ProductSummary => ({
  id: 1,
  category_id: 1,
  name: 'Koch Chemie Gsf',
  brand: 'Koch Chemie',
  unit: 'ml',
  package_size: 1000,
  barcode: '',
  min_stock: null,
  note: '',
  archived: false,
  stock: stock({ total: 2320, open_qty: 320, open_capacity: 1000, sealed_count: 2, units_count: 3 }),
  low: false,
  location_ids: [5],
  last_consume_qty: null,
  photo_updated_at: null,
  purpose_ids: [],
  ph: null,
  rating: null,
  do_not_buy: false,
  ...over,
});

describe('поиск', () => {
  it('без учёта регистра, с кириллицей и ё', () => {
    expect(normalize('  Шампунь  ЁЛКА ')).toBe('шампунь елка');
    expect(matchProduct(prod({ name: 'Шампунь Ёжик' }), 'шам')).toBe(true);
    expect(matchProduct(prod({ name: 'Шампунь Ёжик' }), 'ежик')).toBe(true);
  });
  it('по первым буквам названия и бренда, слова в любом порядке', () => {
    expect(matchProduct(prod({}), 'koch')).toBe(true);
    expect(matchProduct(prod({ name: 'Gsf', brand: 'Koch Chemie' }), 'gsf koch')).toBe(true);
    expect(matchProduct(prod({}), 'sonax')).toBe(false);
  });
  it('по штрихкоду', () => {
    expect(matchProduct(prod({ barcode: '4007218000123' }), '400721')).toBe(true);
    expect(matchProduct(prod({ barcode: '' }), '123')).toBe(false);
  });
  it('пустой запрос находит всё', () => {
    expect(matchProduct(prod({}), '   ')).toBe(true);
  });
});

describe('фильтры', () => {
  const list = [
    prod({ id: 1, low: true, location_ids: [5], category_id: 1 }),
    prod({ id: 2, name: 'Sonax', brand: 'Sonax', stock: stock({}), location_ids: [], category_id: 2 }),
    prod({ id: 3, name: 'Gyeon', brand: 'Gyeon', location_ids: [6], category_id: 2 }),
  ];
  it('разбор из адреса', () => {
    expect(parseFilter('low')).toBe('low');
    expect(parseFilter('loc:5')).toBe('loc:5');
    expect(parseFilter('cat:x')).toBe('all');
    expect(parseFilter(null)).toBe('all');
  });
  it('мало, нет в наличии, место, категория и поиск вместе', () => {
    expect(visibleProducts(list, '', 'low').map((p) => p.id)).toEqual([1]);
    expect(visibleProducts(list, '', 'out').map((p) => p.id)).toEqual([2]);
    expect(visibleProducts(list, '', 'loc:6').map((p) => p.id)).toEqual([3]);
    expect(visibleProducts(list, '', 'cat:2').map((p) => p.id)).toEqual([2, 3]);
    expect(visibleProducts(list, 'son', 'cat:2').map((p) => p.id)).toEqual([2]);
  });
  it('счётчики', () => {
    expect(stockCounts(list)).toEqual({ all: 3, low: 1, out: 1 });
  });
});

describe('подпись остатка', () => {
  it('вскрыта и запечатаны', () => {
    expect(norm(openLine(prod({}).stock, 'ml'))).toBe('вскрыта: 320 из 1 000 · ещё 2 запечатаны');
    expect(norm(openLine(stock({ total: 1320, open_qty: 320, open_capacity: 1000, sealed_count: 1 }), 'ml'))).toBe('вскрыта: 320 из 1 000 · ещё 1 запечатана');
    expect(openLine(stock({ total: 320, open_qty: 320, open_capacity: 1000 }), 'ml')).toBe('вскрыта: 320 из 1 000');
  });
  it('штуки', () => {
    expect(openLine(stock({ total: 6, open_qty: 6, open_capacity: 10 }), 'pcs')).toBe('в пачке 6 из 10');
    expect(openLine(stock({ total: 26, open_qty: 6, open_capacity: 10, sealed_count: 2 }), 'pcs')).toBe('в пачке 6 из 10 · ещё 2 пачки');
    expect(openLine(stock({ total: 5, sealed_count: 5 }), 'pcs', 1)).toBe('запечатаны: 5 шт');
    expect(openLine(stock({ total: 20, sealed_count: 2 }), 'pcs', 10)).toBe('запечатаны: 2 пачки × 10');
  });
  it('все запечатаны и пусто', () => {
    expect(openLine(stock({ total: 500, sealed_count: 1 }), 'ml')).toBe('запечатана');
    expect(norm(openLine(stock({ total: 2000, sealed_count: 2 }), 'ml', 1000))).toBe('запечатаны: 2 × 1 000');
    expect(norm(openLine(stock({ total: 1501, sealed_count: 2 }), 'ml', 1000))).toBe('запечатаны: 2 · всего 1 501');
    expect(openLine(stock({}), 'ml')).toBe('нет в наличии');
  });
  it('мета карточки с минимумом', () => {
    expect(norm(productMeta(prod({ min_stock: 500 })))).toBe('вскрыта: 320 из 1 000 · ещё 2 запечатаны · мин. 500 мл');
  });
});

describe('шкала', () => {
  it('доля вскрытой, полная запечатанная, пусто', () => {
    expect(meterLevel(stock({ total: 320, open_qty: 320, open_capacity: 1000 }))).toBeCloseTo(0.32);
    expect(meterLevel(stock({ total: 1000, sealed_count: 1 }))).toBe(1);
    expect(meterLevel(stock({}))).toBe(0);
    expect(meterValues(stock({ total: 2000, sealed_count: 2 }))).toEqual({ now: 1000, max: 1000 });
  });
});

describe('места, наклейка, порог', () => {
  const locs = new Map<number, Location>([
    [5, { id: 5, name: 'Полка 1', sort: 0, archived: false, units_count: 1, checked_at: null }],
    [6, { id: 6, name: 'Коробка 3', sort: 1, archived: false, units_count: 1, checked_at: null }],
  ]);
  it('чип места', () => {
    expect(locationChip([5], locs)).toBe('Полка 1');
    expect(locationChip([5, 6], locs)).toBe('Полка 1 +1');
    expect(locationChip([], locs)).toBeNull();
  });
  it('наклейка', () => {
    expect(stickerText(prod({ low: true }))).toBe('мало!');
    expect(stickerText(prod({ stock: stock({}) }))).toBe('нет');
    expect(stickerText(prod({}))).toBeNull();
  });
  it('пересечение минимума — только переход из «хватает» в «мало»', () => {
    expect(crossedMin({ low: false }, { low: true })).toBe(true);
    expect(crossedMin({ low: true }, { low: true })).toBe(false);
    expect(crossedMin(undefined, { low: true })).toBe(false);
  });
  it('замена сводок в списке', () => {
    const fresh = prod({ id: 2, name: 'Новая' });
    expect(replaceProducts([prod({ id: 1 }), prod({ id: 2 })], [fresh]).map((p) => p.name)).toEqual(['Koch Chemie Gsf', 'Новая']);
  });
});
