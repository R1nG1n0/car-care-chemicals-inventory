// Отчёты (ФТ-10.2): расчёт периода, масштаб столбиков и графика цены, подписи.
// Чистые функции без обращения к часам и сети — покрыты тестами (reports.test.ts).
// Даты периода — строки «ГГГГ-ММ-ДД» по местному календарю, как в API.
import type { PurchaseRow, Unit } from '../api';
import { formatMoney } from './format';

export type PeriodKind = 'month' | 'quarter' | 'year' | 'custom';
export type Period = { kind: PeriodKind; from: string; to: string };

/** Сегодняшняя дата без времени: месяц 1–12. */
export type Ymd = { y: number; m: number; d: number };

/** Наибольший период по договору API. */
export const MAX_YEARS = 5;

export const PERIOD_KINDS: readonly { kind: PeriodKind; label: string }[] = [
  { kind: 'month', label: 'Месяц' },
  { kind: 'quarter', label: 'Квартал' },
  { kind: 'year', label: 'Год' },
  { kind: 'custom', label: 'Свой' },
];

const pad = (n: number) => String(n).padStart(2, '0');
const iso = (y: number, m: number, d: number) => `${String(y).padStart(4, '0')}-${pad(m)}-${pad(d)}`;
const lastDay = (y: number, m: number) => new Date(Date.UTC(y, m, 0)).getUTCDate();

export function ymdOf(date: Date): Ymd {
  return { y: date.getFullYear(), m: date.getMonth() + 1, d: date.getDate() };
}

/** «ГГГГ-ММ-ДД» → части; не календарная дата — null (31 февраля тоже). */
export function parseIso(s: string): Ymd | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/u.exec(s);
  if (!m) return null;
  const y = Number(m[1]);
  const mo = Number(m[2]);
  const d = Number(m[3]);
  if (mo < 1 || mo > 12 || d < 1 || d > lastDay(y, mo)) return null;
  return { y, m: mo, d };
}

/** Месяц, квартал или год, в который попадает дата ref. */
export function periodOf(kind: Exclude<PeriodKind, 'custom'>, ref: Ymd): Period {
  if (kind === 'year') return { kind, from: iso(ref.y, 1, 1), to: iso(ref.y, 12, 31) };
  if (kind === 'quarter') {
    const first = Math.floor((ref.m - 1) / 3) * 3 + 1;
    return { kind, from: iso(ref.y, first, 1), to: iso(ref.y, first + 2, lastDay(ref.y, first + 2)) };
  }
  return { kind, from: iso(ref.y, ref.m, 1), to: iso(ref.y, ref.m, lastDay(ref.y, ref.m)) };
}

/** Соседний период (-1 — прошлый, +1 — следующий). У «своего» сдвига нет. */
export function shiftPeriod(p: Period, delta: number): Period {
  const start = parseIso(p.from);
  if (p.kind === 'custom' || !start) return p;
  const step = p.kind === 'year' ? 12 : p.kind === 'quarter' ? 3 : 1;
  const idx = start.y * 12 + (start.m - 1) + delta * step;
  return periodOf(p.kind, { y: Math.floor(idx / 12), m: (((idx % 12) + 12) % 12) + 1, d: 1 });
}

/** Следующий период не в будущем: его начало не позже сегодняшнего дня. */
export function canGoNext(p: Period, today: Ymd): boolean {
  if (p.kind === 'custom') return false;
  return shiftPeriod(p, 1).from <= iso(today.y, today.m, today.d);
}

/**
 * Смена вида периода: тот же отрезок времени. Если в текущем периоде есть сегодняшний
 * день — новый период содержит его (год → квартал даёт текущий квартал), иначе — начало
 * текущего. Свой — копия границ.
 */
export function switchKind(p: Period, kind: PeriodKind, today: Ymd): Period {
  if (kind === p.kind) return p;
  if (kind === 'custom') return { kind, from: p.from, to: p.to };
  const now = iso(today.y, today.m, today.d);
  const ref = p.from <= now && now <= p.to ? today : (parseIso(p.from) ?? today);
  return periodOf(kind, ref);
}

const MONTHS = ['Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь', 'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь'];
const MONTHS_SHORT = ['янв', 'фев', 'мар', 'апр', 'май', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'];
const ROMAN = ['I', 'II', 'III', 'IV'];

const dmy = (s: string) => {
  const p = parseIso(s);
  return p ? `${pad(p.d)}.${pad(p.m)}.${p.y}` : s;
};

/** «Октябрь 2026», «III квартал 2026», «2026 год», «01.10.2026 — 14.10.2026». */
export function periodTitle(p: Period): string {
  const s = parseIso(p.from);
  if (!s || p.kind === 'custom') return `${dmy(p.from)} — ${dmy(p.to)}`;
  if (p.kind === 'year') return `${s.y} год`;
  if (p.kind === 'quarter') return `${ROMAN[Math.floor((s.m - 1) / 3)]} квартал ${s.y}`;
  return `${MONTHS[s.m - 1]} ${s.y}`;
}

/** Проверка «своего» периода до запроса (те же правила, что у API); пусто — всё верно. */
export function customError(from: string, to: string): string {
  const f = parseIso(from);
  const t = parseIso(to);
  if (!f || !t) return 'Укажите обе даты';
  if (to < from) return 'Конец периода раньше начала';
  if (iso(f.y + MAX_YEARS, f.m, Math.min(f.d, lastDay(f.y + MAX_YEARS, f.m))) < to) return `Период не больше ${MAX_YEARS} лет`;
  return '';
}

/** Смещение часового пояса для параметра tz: минуты к востоку от UTC (МСК — 180). */
export function tzOffset(date = new Date()): number {
  return -date.getTimezoneOffset() || 0;
}

// ---------- Доли и столбики ----------

/** Доля value от total (0…1); total 0 — 0. */
export function shareOf(value: number, total: number): number {
  return total > 0 ? Math.min(1, Math.max(0, value / total)) : 0;
}

/** «42%»; меньше процента — «<1%»; нуль — «0%». */
export function percentText(share: number): string {
  if (share <= 0) return '0%';
  const p = Math.round(share * 100);
  return p < 1 ? '<1%' : `${p}%`;
}

/** Наименьшая видимая доля полоски: ненулевое значение не должно пропадать. */
export const MIN_LEVEL = 0.03;

/** Высота или длина полоски (0…1): ненулевое — не меньше MIN_LEVEL. */
export function visibleLevel(share: number): number {
  return share <= 0 ? 0 : Math.max(MIN_LEVEL, Math.min(1, share));
}

/** Уровни столбиков: максимум — 1, остальные в его долях. */
export function barLevels(values: readonly number[]): number[] {
  const max = Math.max(0, ...values);
  return values.map((v) => visibleLevel(shareOf(v, max)));
}

/** Через сколько столбиков подписывать месяц, чтобы подписи не слипались. */
export function labelStep(count: number): number {
  return count <= 12 ? 1 : Math.ceil(count / 6);
}

/** «окт»; при длинном периоде добавляется год: «окт 26». */
export function monthLabel(month: string, withYear: boolean): string {
  const m = /^(\d{4})-(\d{2})$/u.exec(month);
  if (!m) return month;
  const short = MONTHS_SHORT[Number(m[2]) - 1] ?? month;
  return withYear ? `${short} ${(m[1] ?? '').slice(2)}` : short;
}

/** «Октябрь 2026» из «2026-10» — для озвучивания столбика. */
export function monthFull(month: string): string {
  const m = /^(\d{4})-(\d{2})$/u.exec(month);
  return m ? `${(MONTHS[Number(m[2]) - 1] ?? month).toLowerCase()} ${m[1]}` : month;
}

/** Короткая сумма над столбиком, рубли: «950», «2,4 тыс.», «1,2 млн». */
export function compactMoney(kopecks: number): string {
  const rub = Math.round(kopecks / 100);
  const one = (n: number) => String(Math.round(n * 10) / 10).replace('.', ',');
  if (rub >= 1_000_000) return `${one(rub / 1_000_000)} млн`;
  if (rub >= 1000) return `${one(rub / 1000)} тыс.`;
  return String(rub);
}

/** Над столбиками подписывают значения, пока их немного. */
export const MAX_VALUE_LABELS = 7;

// ---------- История цены ----------

export type PricePoint = { date: string; value: number };

/**
 * Точки графика цены за 100 мл / за шт по покупкам, от старых к новым.
 * API отдаёт покупки новыми сверху; строки без цены за единицу пропускаются.
 */
export function priceSeries(rows: readonly PurchaseRow[], unit: Unit): PricePoint[] {
  const out: PricePoint[] = [];
  for (const r of [...rows].reverse()) {
    const value = unit === 'pcs' ? r.price_per_unit : r.price_per_100;
    if (value !== undefined) out.push({ date: r.date, value });
  }
  return out;
}

export type ChartPoint = { x: number; y: number };
export type Chart = { points: ChartPoint[]; path: string; min: number; max: number };

/**
 * Координаты линии в окне w×h с полями: по горизонтали точки равноудалены,
 * по вертикали — от минимума (внизу) до максимума (вверху). Все цены равны —
 * линия по центру. Точек меньше двух — график не строится.
 */
export function chartGeometry(values: readonly number[], w: number, h: number, padX = 12, padY = 14): Chart | null {
  if (values.length < 2) return null;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min;
  const points = values.map((v, i) => ({
    x: round(padX + ((w - 2 * padX) * i) / (values.length - 1)),
    y: round(span === 0 ? h / 2 : padY + (h - 2 * padY) * (1 - (v - min) / span)),
  }));
  return { points, path: points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x} ${p.y}`).join(' '), min, max };
}

const round = (n: number) => Math.round(n * 100) / 100;

const unitWord = (unit: Unit) => (unit === 'pcs' ? 'за шт' : 'за 100 мл');

/** «От 110 ₽ до 135 ₽ за 100 мл». Все цены равны — «Всегда 119 ₽ за 100 мл». */
export function priceRangeText(values: readonly number[], unit: Unit): string {
  if (values.length === 0) return '';
  const min = Math.min(...values);
  const max = Math.max(...values);
  if (min === max) return `Всегда ${formatMoney(min)} ${unitWord(unit)}`;
  return `От ${formatMoney(min)} до ${formatMoney(max)} ${unitWord(unit)}`;
}

/** Изменение последней цены к предыдущей: «+8% к прошлой покупке», «без изменений». */
export function priceChangeText(values: readonly number[]): string {
  if (values.length < 2) return '';
  const prev = values[values.length - 2] ?? 0;
  const last = values[values.length - 1] ?? 0;
  if (prev <= 0 || prev === last) return 'без изменений к прошлой покупке';
  const pct = Math.round(((last - prev) / prev) * 100);
  if (pct === 0) return 'почти без изменений к прошлой покупке';
  return `${pct > 0 ? '+' : '−'}${Math.abs(pct)}% к прошлой покупке`;
}
