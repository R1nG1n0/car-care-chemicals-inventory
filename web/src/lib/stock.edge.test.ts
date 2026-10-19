// Граничные случаи главной: поиск (ё/е, регистр, штрихкод, пробелы), подпись
// остатка, шкала, пересечение минимума, фильтры из адреса.
import { describe, expect, it } from 'vitest';
import type { ProductSummary, Stock } from '../api';
import { crossedMin, matchProduct, meterLevel, meterValues, normalize, openLine, parseFilter, passesFilter, stickerText } from './stock';

const sp = (s: string) => s.replace(/[\u00a0\u202f]/gu, ' ');
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
  stock: stock({ total: 1000, sealed_count: 1, units_count: 1 }),
  low: false,
  location_ids: [],
  last_consume_qty: null,
  photo_updated_at: null,
  ...over,
});

describe('normalize', () => {
  it('Ё заглавная, табуляция и неразрывный пробел', () => {
    expect(normalize('  ЁЖИК\tв\u00a0ТУМАНЕ ')).toBe('ежик в тумане');
  });
});

describe('поиск (ФТ-6.1, П-7)', () => {
  const p = prod({ name: 'Очиститель стёкол', brand: 'Grass', barcode: '4607072190124' });
  it('кириллица без учёта регистра, ё = е в обе стороны', () => {
    expect(matchProduct(p, 'ОЧИ')).toBe(true);
    expect(matchProduct(p, 'стекол')).toBe(true);
    expect(matchProduct(prod({ name: 'Очиститель стекол' }), 'стёк')).toBe(true);
  });
  it('по первым буквам бренда', () => expect(matchProduct(p, 'gra')).toBe(true));
  it('несколько пробелов между словами', () => expect(matchProduct(p, '  очи    gra ')).toBe(true));
  it('не находит лишнее', () => {
    expect(matchProduct(p, 'koch')).toBe(false);
    expect(matchProduct(p, 'очи koch')).toBe(false);
  });
  it('штрихкод: с начала, кусок, с пробелами-разрядами', () => {
    expect(matchProduct(p, '4607')).toBe(true);
    expect(matchProduct(p, '2190124')).toBe(true);
    expect(matchProduct(p, '4607 0721')).toBe(true);
    expect(matchProduct(p, '9999')).toBe(false);
  });
  it('пустой штрихкод не совпадает с цифрами', () => expect(matchProduct(prod({}), '123')).toBe(false));
  it('только пробелы — всё', () => expect(matchProduct(p, '   ')).toBe(true));
});

describe('подпись остатка', () => {
  it('вскрыта, одна запечатана', () => {
    expect(sp(openLine(stock({ total: 1320, open_qty: 320, open_capacity: 1000, sealed_count: 1 }), 'ml'))).toBe('вскрыта: 320 из 1 000 · ещё 1 запечатана');
  });
  it('вскрыта, пять запечатаны', () => {
    expect(sp(openLine(stock({ total: 5320, open_qty: 320, open_capacity: 1000, sealed_count: 5 }), 'ml'))).toBe('вскрыта: 320 из 1 000 · ещё 5 запечатаны');
  });
  it('шт: только вскрытая пачка', () => {
    expect(sp(openLine(stock({ total: 6, open_qty: 6, open_capacity: 10 }), 'pcs'))).toBe('в пачке 6 из 10');
  });
  it('шт: одна и пять запечатанных пачек', () => {
    expect(openLine(stock({ total: 10, sealed_count: 1 }), 'pcs')).toBe('пачка запечатана');
    expect(sp(openLine(stock({ total: 50, sealed_count: 5 }), 'pcs', 10))).toBe('запечатаны: 5 пачек × 10');
  });
  it('запечатанные с нецелым средним — без «× N», с суммой', () => {
    expect(sp(openLine(stock({ total: 1501, sealed_count: 2 }), 'ml', 750))).toBe('запечатаны: 2 · всего 1 501');
  });
  // Было: бутылки 1000 и 500 мл (сменили package_size между покупками) подписывались
  // «2 × 750». Теперь «× N» — только если остаток = число × размер упаковки позиции.
  it('1000 + 500 мл не подписываются «2 × 750»', () => {
    expect(openLine(stock({ total: 1500, sealed_count: 2 }), 'ml', 1000)).not.toMatch(/×/u);
    expect(openLine(stock({ total: 1500, sealed_count: 2 }), 'ml')).not.toMatch(/×/u);
  });
  it('размер упаковки сходится — «× N»', () => {
    expect(sp(openLine(stock({ total: 1500, sealed_count: 2 }), 'ml', 750))).toBe('запечатаны: 2 × 750');
  });
  it('П-5: после расхода 600 из двух по 500 — вскрыта 400 из 500', () => {
    expect(openLine(stock({ total: 400, open_qty: 400, open_capacity: 500 }), 'ml')).toBe('вскрыта: 400 из 500');
  });
});

describe('шкала', () => {
  it('вскрытая с нулевым объёмом не делит на ноль', () => {
    expect(meterLevel(stock({ total: 10, open_qty: 0, open_capacity: 0, sealed_count: 1 }))).toBe(1);
  });
  it('остаток больше объёма — не больше 1', () => {
    expect(meterLevel(stock({ total: 1200, open_qty: 1200, open_capacity: 1000 }))).toBe(1);
  });
  it('значения для role=meter у запечатанных разного объёма — средний', () => {
    expect(meterValues(stock({ total: 1500, sealed_count: 2 }))).toEqual({ now: 750, max: 750 });
  });
});

describe('наклейка и пересечение минимума', () => {
  it('нет в наличии важнее «мало»', () => expect(stickerText({ low: true, stock: stock({ total: 0 }) })).toBe('нет'));
  it('из «мало» в «мало» и из «мало» в норму — не пересечение', () => {
    expect(crossedMin({ low: true }, { low: true })).toBe(false);
    expect(crossedMin({ low: true }, { low: false })).toBe(false);
  });
});

describe('фильтры', () => {
  it('мусор и неизвестное — «все»', () => {
    for (const s of ['', 'LOW', 'loc:', 'loc:x', 'cat:-1', 'loc:1:2', 'in', null, undefined]) expect(parseFilter(s)).toBe('all');
  });
  it('ведущие нули в id', () => expect(parseFilter('loc:007')).toBe('loc:7'));
  it('место: позиция без мест не проходит', () => expect(passesFilter(prod({ location_ids: [] }), 'loc:5')).toBe(false));
});
