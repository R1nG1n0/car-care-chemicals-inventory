// Ввод количества (ФТ-5.8): разбор поля, кнопки шагов, границы, значение
// по умолчанию для расхода, выбор бутылки, подсказки и текст уведомления.
import type { ConsumeResult, ProductSummary, Settings, StockUnit, Unit } from '../api';
import { formatQty } from './format';
import { qtyText } from './stock';
import { plural } from './text';
import { clampQty, parseQty } from './qty';

// Разбор поля и границы — общие (lib/qty.ts); отсюда — для старых импортов.
export { clampQty, parseQty } from './qty';

/** Кнопки шагов: для мл — −большой −малый +малый +большой, для шт — −1 +1 +5. */
export function stepButtons(unit: Unit, settings: Settings | undefined): number[] {
  if (unit === 'pcs') return [-1, 1, 5];
  const small = settings?.step_small ?? 10;
  const large = settings?.step_large ?? 50;
  return [-large, -small, small, large];
}

/** Значение после кнопки: пустое поле считается нулём, результат в границах. */
export function applyStep(value: string, delta: number, min: number, max: number | null | undefined): number {
  return clampQty((parseQty(value) ?? 0) + delta, min, max);
}

/** «Минус 10 мл» для aria-label кнопки шага. */
export function stepLabel(delta: number, unit: Unit): string {
  return `${delta > 0 ? 'Плюс' : 'Минус'} ${qtyText(Math.abs(delta), unit)}`;
}

/** «+10» / «−50» на кнопке. */
export function stepText(delta: number): string {
  return `${delta > 0 ? '+' : '−'}${formatQty(Math.abs(delta))}`;
}

/**
 * Начальное значение расхода: прошлое количество или малый шаг (для шт — 1),
 * не ниже 1 и не выше остатка. Пусто — 0.
 */
export function defaultConsumeQty(p: Pick<ProductSummary, 'unit' | 'last_consume_qty' | 'stock'>, settings: Settings | undefined): number {
  const total = p.stock.total;
  if (total <= 0) return 0;
  const base = p.last_consume_qty ?? (p.unit === 'pcs' ? 1 : (settings?.step_small ?? 10));
  return clampQty(base, 1, total);
}

/** «В прошлый раз 40 мл · во вскрытой 320 мл». */
export function consumeHint(p: Pick<ProductSummary, 'unit' | 'last_consume_qty' | 'stock'>): string {
  const parts: string[] = [];
  if (p.last_consume_qty !== null) parts.push(`В прошлый раз ${qtyText(p.last_consume_qty, p.unit)}`);
  if (p.stock.open_qty !== null) parts.push(`${p.unit === 'pcs' ? 'в пачке' : 'во вскрытой'} ${qtyText(p.stock.open_qty, p.unit)}`);
  parts.push(`всего ${qtyText(p.stock.total, p.unit)}`);
  const s = parts.join(' · ');
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export type BottleOption = { unitId: number; kind: 'open' | 'sealed'; label: string; qty: number };

/**
 * Выбор бутылки (ФТ-5.2): вскрытая (самая ранняя) и новая (первая запечатанная).
 * Сервер отдаёт экземпляры вскрытыми первыми. Меньше двух вариантов — выбирать нечего.
 */
export function bottleOptions(units: readonly StockUnit[], unit: Unit): BottleOption[] {
  const live = units.filter((u) => u.status === 'open' || u.status === 'sealed');
  const open = live
    .filter((u) => u.status === 'open')
    .sort((a, b) => (a.opened_at ?? 0) - (b.opened_at ?? 0) || a.id - b.id)[0];
  const sealed = live.find((u) => u.status === 'sealed');
  const out: BottleOption[] = [];
  const word = unit === 'pcs' ? ['Вскрытая пачка', 'Новая пачка'] : ['Вскрытая', 'Новая'];
  if (open) out.push({ unitId: open.id, kind: 'open', label: `${word[0]} · ${formatQty(open.remaining_qty)}`, qty: open.remaining_qty });
  if (sealed) out.push({ unitId: sealed.id, kind: 'sealed', label: `${word[1]} · ${formatQty(sealed.remaining_qty)}`, qty: sealed.remaining_qty });
  return out;
}

/** Бутылка по умолчанию — вскрытая, если есть. */
export function defaultBottle(options: readonly BottleOption[]): number | undefined {
  return (options.find((o) => o.kind === 'open') ?? options[0])?.unitId;
}

/**
 * unit_id для POST /api/consume: только когда было из чего выбирать (две бутылки).
 * Иначе сервер сам берёт вскрытую, затем запечатанную. Выбор, которого уже нет
 * среди вариантов, заменяется бутылкой по умолчанию.
 */
export function consumeUnitId(options: readonly BottleOption[], chosen: number | undefined): number | undefined {
  if (options.length < 2) return undefined;
  return options.some((o) => o.unitId === chosen) ? chosen : defaultBottle(options);
}

const BOTTLES = ['бутылка', 'бутылки', 'бутылок'] as const;
const PACKS = ['пачка', 'пачки', 'пачек'] as const;

/** «Списано 40 мл · Koch Chemie Gsf» и «· бутылка закончилась» (ФТ-5.1). */
export function consumeToastText(name: string, unit: Unit, qty: number, r: Pick<ConsumeResult, 'finished_unit_ids'>): string {
  let s = `Списано ${qtyText(qty, unit)} · ${name}`;
  const n = r.finished_unit_ids.length;
  if (n > 0) {
    const what = plural(n, unit === 'pcs' ? PACKS : BOTTLES);
    s += n === 1 ? ` · ${what} закончилась` : ` · закончились ${n} ${what}`;
  }
  return s;
}
