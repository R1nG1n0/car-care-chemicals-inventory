import { describe, expect, it } from 'vitest';
import type { ProductDetail, ProductSummary, Purpose } from '../api';
import { aboutOf, isLongText, nextRating, phText, purposeChoices, purposesOf, ratingLabel, toggleId, wantsAnalogs } from './purposes';
import { applyRating, applyRatingToDetail } from './ratingPatch';
import { matchProduct, parsePurposeParam, visibleProducts } from './stock';

const pur = (id: number, name: string, archived = false): Purpose => ({ id, name, sort: id, archived });
const list = [pur(1, 'ЛКП'), pur(2, 'Стекло'), pur(3, 'Кожа', true)];

const prod = (over: Partial<ProductSummary>): ProductSummary =>
  ({
    id: 1,
    name: 'Шампунь',
    brand: '',
    barcode: '',
    low: false,
    category_id: 1,
    location_ids: [],
    stock: { total: 500, open_qty: null, open_capacity: null, sealed_count: 1, units_count: 1 },
    purpose_ids: [],
    ph: null,
    rating: null,
    do_not_buy: false,
    ...over,
  }) as ProductSummary;

describe('выбор назначений', () => {
  it('архивные не предлагаются, но выбранные архивные остаются', () => {
    expect(purposeChoices(list, []).map((p) => p.name)).toEqual(['ЛКП', 'Стекло']);
    expect(purposeChoices(list, [3]).map((p) => p.name)).toEqual(['ЛКП', 'Стекло', 'Кожа']);
  });
  it('чип добавляет и убирает', () => {
    expect(toggleId([], 2)).toEqual([2]);
    expect(toggleId([2], 1)).toEqual([2, 1]);
    expect(toggleId([2, 1], 2)).toEqual([1]);
  });
  it('назначения позиции — в порядке справочника, неизвестные пропускаются', () => {
    const m = new Map(list.map((p) => [p.id, p]));
    expect(purposesOf([2, 1, 99], m).map((p) => p.name)).toEqual(['ЛКП', 'Стекло']);
    expect(purposesOf(undefined, m)).toEqual([]);
  });
});

describe('звёзды (ФТ-15.4.1)', () => {
  it('повторное нажатие на текущую снимает оценку', () => {
    expect(nextRating(null, 4)).toBe(4);
    expect(nextRating(2, 4)).toBe(4);
    expect(nextRating(4, 4)).toBeNull();
  });
  it('подпись для диктора', () => {
    expect(ratingLabel(4)).toBe('Оценка 4 из 5');
    expect(ratingLabel(null)).toBe('Без оценки');
  });
  it('оптимистичная правка меняет только названные поля', () => {
    const p = prod({ rating: 3, do_not_buy: false });
    expect(applyRating(p, { rating: null })).toMatchObject({ rating: null, do_not_buy: false });
    expect(applyRating(p, { do_not_buy: true })).toMatchObject({ rating: 3, do_not_buy: true });
    expect(p.rating).toBe(3);
  });
  it('правка карточки в кэше; пустого кэша нет — ничего', () => {
    const d = { product: prod({ rating: 1 }), units: [], purchases: [], movements: [] } as ProductDetail;
    expect(applyRatingToDetail(d, { rating: 5 })?.product.rating).toBe(5);
    expect(applyRatingToDetail(undefined, { rating: 5 })).toBeUndefined();
  });
});

describe('о средстве', () => {
  it('тексты берутся из product или из detail, пробелы обрезаются', () => {
    const base = { units: [], purchases: [], movements: [] };
    expect(aboutOf({ ...base, product: { ...prod({}), incompatible: ' кислоты ', instruction: 'Разбавить' } })).toEqual({ incompatible: 'кислоты', instruction: 'Разбавить' });
    expect(aboutOf({ ...base, product: prod({}), incompatible: 'хлор' })).toEqual({ incompatible: 'хлор', instruction: '' });
    expect(aboutOf({ ...base, product: prod({}) })).toEqual({ incompatible: '', instruction: '' });
  });
  it('pH с запятой', () => {
    expect(phText(7.5)).toBe('7,5');
    expect(phText(7)).toBe('7');
    expect(phText(null)).toBeNull();
  });
  it('длинная инструкция сворачивается', () => {
    expect(isLongText('коротко')).toBe(false);
    expect(isLongText('я'.repeat(281))).toBe(true);
    expect(isLongText('1\n2\n3\n4\n5\n6')).toBe(true);
  });
});

describe('аналоги: показывать ли (ФТ-15.5.3)', () => {
  const stock = (total: number) => ({ total, open_qty: null, open_capacity: null, sealed_count: 0, units_count: 0 });
  it('нет в наличии или ниже минимума', () => {
    expect(wantsAnalogs({ low: false, stock: stock(0) })).toBe(true);
    expect(wantsAnalogs({ low: true, stock: stock(100) })).toBe(true);
    expect(wantsAnalogs({ low: false, stock: stock(100) })).toBe(false);
  });
});

describe('фильтр и поиск по назначению (ФТ-15.3)', () => {
  const names = new Map(list.map((p) => [p.id, p.name]));
  const items = [
    prod({ id: 1, name: 'Шампунь', purpose_ids: [1] }),
    prod({ id: 2, name: 'Очиститель', purpose_ids: [2, 1] }),
    prod({ id: 3, name: 'Крем', purpose_ids: [3], low: true }),
    prod({ id: 4, name: 'Старая позиция' }),
  ];
  const ids = (x: ProductSummary[]) => x.map((p) => p.id);

  it('параметр ?purpose=', () => {
    expect(parsePurposeParam('3')).toBe(3);
    expect(parsePurposeParam('0')).toBeNull();
    expect(parsePurposeParam('abc')).toBeNull();
    expect(parsePurposeParam('-2')).toBeNull();
    expect(parsePurposeParam(null)).toBeNull();
  });
  it('фильтр по назначению', () => {
    expect(ids(visibleProducts(items, '', 'all', { id: 1, names }))).toEqual([1, 2]);
    expect(ids(visibleProducts(items, '', 'all', { id: null, names }))).toEqual([1, 2, 3, 4]);
  });
  it('сочетается с остальными фильтрами и поиском', () => {
    expect(ids(visibleProducts(items, '', 'low', { id: 3, names }))).toEqual([3]);
    expect(ids(visibleProducts(items, '', 'low', { id: 1, names }))).toEqual([]);
    expect(ids(visibleProducts(items, 'очист', 'all', { id: 1, names }))).toEqual([2]);
  });
  it('поиск находит и по названию назначения', () => {
    expect(ids(visibleProducts(items, 'кожа', 'all', { id: null, names }))).toEqual([3]);
    expect(ids(visibleProducts(items, 'стекло', 'all', { id: null, names }))).toEqual([2]);
    expect(matchProduct(items[0]!, 'стекло')).toBe(false);
  });
  it('старые снимки без purpose_ids не ломают фильтр', () => {
    const old = [{ ...prod({ id: 9 }), purpose_ids: undefined } as unknown as ProductSummary];
    expect(visibleProducts(old, '', 'all', { id: 1, names })).toEqual([]);
    expect(visibleProducts(old, 'кожа', 'all', { id: null, names })).toEqual([]);
    expect(visibleProducts(old, '', 'all', { id: null, names })).toHaveLength(1);
  });
});
