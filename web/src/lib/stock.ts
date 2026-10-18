// Раздел «Остатки»: поиск и фильтры по загруженному списку, подписи остатка,
// шкала вскрытой бутылки, пересечение минимума. Чистые функции — покрыты тестами.
import type { Location, ProductSummary, Stock, Unit } from '../api';
import { formatQty } from './format';
import { plural } from './text';

/** «мл» / «шт». */
export function unitLabel(unit: Unit): string {
  return unit === 'pcs' ? 'шт' : 'мл';
}

/** «40 мл» с неразрывным пробелом. */
export function qtyText(n: number, unit: Unit): string {
  return `${formatQty(n)}\u00a0${unitLabel(unit)}`;
}

/** Строка для сравнения: нижний регистр с кириллицей, «ё» = «е», пробелы схлопнуты. */
export function normalize(s: string): string {
  return s.toLocaleLowerCase('ru-RU').replace(/ё/gu, 'е').replace(/\s+/gu, ' ').trim();
}

/** Поиск по названию, бренду и штрихкоду, без учёта регистра, с первой буквы (ФТ-6.1). */
export function matchProduct(p: Pick<ProductSummary, 'name' | 'brand' | 'barcode'>, query: string): boolean {
  const q = normalize(query);
  if (q === '') return true;
  const hay = normalize(`${p.name} ${p.brand}`);
  // Все слова запроса должны найтись: «koch gsf» находит «Koch Chemie Gsf»
  if (q.split(' ').every((w) => hay.includes(w))) return true;
  return p.barcode !== '' && p.barcode.includes(q.replace(/\s/gu, ''));
}

/** Фильтр-чип главной: все, мало, нет в наличии, место, категория. */
export type StockFilter = 'all' | 'low' | 'out' | `loc:${number}` | `cat:${number}`;

export function parseFilter(s: string | null | undefined): StockFilter {
  if (s === 'low' || s === 'out') return s;
  const m = /^(loc|cat):(\d+)$/u.exec(s ?? '');
  if (m) return `${m[1] as 'loc' | 'cat'}:${Number(m[2])}`;
  return 'all';
}

export function passesFilter(p: ProductSummary, f: StockFilter): boolean {
  if (f === 'all') return true;
  if (f === 'low') return p.low;
  if (f === 'out') return p.stock.total === 0;
  const [kind, raw] = f.split(':') as ['loc' | 'cat', string];
  const id = Number(raw);
  return kind === 'loc' ? p.location_ids.includes(id) : p.category_id === id;
}

/** Видимые карточки: фильтр, затем поиск; порядок — как пришёл с сервера (по имени). */
export function visibleProducts(list: readonly ProductSummary[], query: string, f: StockFilter): ProductSummary[] {
  return list.filter((p) => passesFilter(p, f) && matchProduct(p, query));
}

export type StockCounts = { all: number; low: number; out: number };

export function stockCounts(list: readonly ProductSummary[]): StockCounts {
  return {
    all: list.length,
    low: list.filter((p) => p.low).length,
    out: list.filter((p) => p.stock.total === 0).length,
  };
}

const SEALED = ['запечатана', 'запечатаны', 'запечатаны'] as const;
const PACKS = ['пачка', 'пачки', 'пачек'] as const;

/**
 * Подпись под шкалой (дизайн-система 1.1): «вскрыта: 320 из 1 000 · ещё 2 запечатаны»,
 * «в пачке 6 из 10», «нет в наличии».
 *
 * «запечатаны: 2 × 1 000» — только когда объём доказан: остаток ровно равен
 * числу бутылок × размер упаковки позиции. Иначе (размер меняли между покупками,
 * 1000 + 500) — без «× N», с суммой: по одной сумме равенство объёмов не проверить.
 */
export function openLine(stock: Stock, unit: Unit, packageSize?: number): string {
  if (stock.total === 0) return 'нет в наличии';
  const n = stock.sealed_count;
  if (stock.open_qty !== null && stock.open_capacity !== null) {
    const open =
      unit === 'pcs'
        ? `в пачке ${formatQty(stock.open_qty)} из ${formatQty(stock.open_capacity)}`
        : `вскрыта: ${formatQty(stock.open_qty)} из ${formatQty(stock.open_capacity)}`;
    if (n === 0) return open;
    const rest = unit === 'pcs' ? `ещё ${n} ${plural(n, PACKS)}` : `ещё ${n} ${plural(n, SEALED)}`;
    return `${open} · ${rest}`;
  }
  // Вскрытой нет: все запечатаны
  if (n === 1) return unit === 'pcs' ? 'пачка запечатана' : 'запечатана';
  // Штучное по одной в упаковке: «5 пачек × 1» — шум, пишем «5 шт»
  if (unit === 'pcs' && packageSize === 1 && stock.total === n) return `запечатаны: ${n} шт`;
  const what = unit === 'pcs' ? `${n} ${plural(n, PACKS)}` : `${n}`;
  if (packageSize !== undefined && packageSize > 0 && stock.total === n * packageSize) return `запечатаны: ${what} × ${formatQty(packageSize)}`;
  return `запечатаны: ${what} · всего ${formatQty(stock.total)}`;
}

/** Доля вскрытой бутылки для --lv (0…1). Вскрытой нет — полная запечатанная или пусто. */
export function meterLevel(stock: Stock): number {
  if (stock.total === 0) return 0;
  if (stock.open_qty === null || !stock.open_capacity) return 1;
  return Math.min(1, Math.max(0, stock.open_qty / stock.open_capacity));
}

/** Значение и максимум шкалы для role="meter". */
export function meterValues(stock: Stock): { now: number; max: number } {
  if (stock.open_qty !== null && stock.open_capacity) return { now: stock.open_qty, max: stock.open_capacity };
  if (stock.total === 0) return { now: 0, max: 1 };
  const each = stock.sealed_count > 0 ? Math.round(stock.total / stock.sealed_count) : stock.total;
  return { now: each, max: each };
}

/** Чип места: «Полка 1», «Полка 1 +2»; мест нет — null. */
export function locationChip(ids: readonly number[], locations: ReadonlyMap<number, Location>): string | null {
  const names = ids.map((id) => locations.get(id)?.name).filter((n): n is string => !!n);
  if (names.length === 0) return null;
  return names.length === 1 ? names[0]! : `${names[0]} +${names.length - 1}`;
}

/** Наклейка на карточке: «нет» — пусто, «мало!» — ниже минимума. */
export function stickerText(p: Pick<ProductSummary, 'low' | 'stock'>): string | null {
  if (p.stock.total === 0) return 'нет';
  return p.low ? 'мало!' : null;
}

/** Остаток только что опустился ниже минимума — наклейка «прилепляется» (motion.css). */
export function crossedMin(before: Pick<ProductSummary, 'low'> | undefined, after: Pick<ProductSummary, 'low'>): boolean {
  return !!before && !before.low && after.low;
}

/** Строка мета под крупным числом в карточке позиции: «… · мин. 500 мл». */
export function productMeta(p: ProductSummary): string {
  const parts = [openLine(p.stock, p.unit, p.package_size)];
  if (p.min_stock !== null) parts.push(`мин. ${qtyText(p.min_stock, p.unit)}`);
  return parts.join(' · ');
}

/** Заменяет сводки позиций в загруженном списке (после расхода и отмены). */
export function replaceProducts(list: readonly ProductSummary[], fresh: readonly ProductSummary[]): ProductSummary[] {
  const byId = new Map(fresh.map((p) => [p.id, p]));
  return list.map((p) => byId.get(p.id) ?? p);
}
