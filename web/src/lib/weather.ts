// Погода (ФТ-16): чистые функции для плашки, экрана и профиля.
import type { Weather, WeatherDay, WeatherLevel, WeatherPlace, WeatherVerdict } from '../api';
import { formatDay, formatTime } from './text';

export type WeatherKind = 'clear' | 'partly' | 'cloud' | 'fog' | 'drizzle' | 'rain' | 'snow' | 'storm';

/** Код WMO → вид погоды и подпись. Неизвестный код — «облачно». */
export function weatherKind(code: number): { kind: WeatherKind; label: string } {
  if (code === 0) return { kind: 'clear', label: 'Ясно' };
  if (code === 1 || code === 2) return { kind: 'partly', label: 'Малооблачно' };
  if (code === 3) return { kind: 'cloud', label: 'Пасмурно' };
  if (code === 45 || code === 48) return { kind: 'fog', label: 'Туман' };
  if (code >= 51 && code <= 57) return { kind: 'drizzle', label: 'Морось' };
  if ((code >= 61 && code <= 67) || (code >= 80 && code <= 82)) return { kind: 'rain', label: 'Дождь' };
  if ((code >= 71 && code <= 77) || code === 85 || code === 86) return { kind: 'snow', label: 'Снег' };
  if (code >= 95 && code <= 99) return { kind: 'storm', label: 'Гроза' };
  return { kind: 'cloud', label: 'Облачно' };
}

export type VerdictKind = 'wash' | 'coat';

export const VERDICT_TITLE: Record<VerdictKind, string> = { wash: 'мойка', coat: 'покрытие' };
export const LEVEL_WORD: Record<WeatherLevel, string> = { good: 'хорошо', meh: 'так себе', bad: 'плохо' };

/** «мойка: хорошо» — для экранных дикторов и подсказок. */
export function levelText(kind: VerdictKind, level: WeatherLevel): string {
  return `${VERDICT_TITLE[kind]}: ${LEVEL_WORD[level]}`;
}

const UTC_DAYS = ['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб'] as const;

/** Разбор «2026-10-30» без часовых поясов. */
function parseDate(date: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/u.exec(date);
  return m ? new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]))) : null;
}

/** «чт» по дате; сервер присылает день недели сам, это запасной путь. */
export function weekdayOf(date: string): string {
  const d = parseDate(date);
  return d ? UTC_DAYS[d.getUTCDay()]! : '';
}

/** «30.10» из «2026-10-30». */
export function dayMonth(date: string): string {
  const m = /^\d{4}-(\d{2})-(\d{2})$/u.exec(date);
  return m ? `${m[2]}.${m[1]}` : date;
}

/** Подпись дня: у сервера есть weekday — берём его. */
export function dayName(days: readonly WeatherDay[], date: string): string {
  return days.find((d) => d.date === date)?.weekday || weekdayOf(date);
}

/** «−3°» (настоящий минус), округление до целого. */
export function tempText(t: number): string {
  const r = Math.round(t);
  return r < 0 ? `−${-r}°` : `${r}°`;
}

/** «2,5» / «0» / «12»: до десятых, запятая, без «,0». */
export function mmText(mm: number): string {
  const r = Math.round(mm * 10) / 10;
  return String(r).replace('.', ',');
}

/** «2,5 мм · 60 %»; без осадков — «без осадков». */
export function precipText(mm: number, prob: number): string {
  if (mm <= 0 && prob <= 0) return 'без осадков';
  return `${mmText(mm)}\u00a0мм\u00a0·\u00a0${Math.round(prob)}\u00a0%`;
}

/** «Донецк — Донецкая область, Украина». */
export function placeLabel(p: WeatherPlace): string {
  const rest = [p.admin1, p.country].filter((x): x is string => !!x && x !== p.name);
  return rest.length ? `${p.name} — ${rest.join(', ')}` : p.name;
}

/** «обновлено 14:05»; прогноз мог устареть — с пометкой. */
export function updatedText(fetchedAt: number | null, stale: boolean): string {
  if (fetchedAt === null) return stale ? 'Прогноз мог устареть' : '';
  const base = `обновлено ${formatTime(fetchedAt * 1000)}`;
  return stale ? `${base} · прогноз мог устареть` : base;
}

/** Показывать ли погоду вообще: настроена и есть дни. */
export function hasForecast(w: Weather | undefined): w is Weather {
  return !!w && w.enabled && w.days.length > 0;
}

export type PlaceNow = { date: string; hour: number };

const pad2 = (n: number) => String(n).padStart(2, '0');

/** «Сейчас» в поясе места: дата и час. Нет смещения (старый снимок) — по часам устройства. */
export function placeNow(utcOffset: number | undefined, nowMs: number): PlaceNow {
  if (utcOffset === undefined) {
    const d = new Date(nowMs);
    return { date: `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`, hour: d.getHours() };
  }
  const d = new Date(nowMs + utcOffset * 1000);
  return { date: `${d.getUTCFullYear()}-${pad2(d.getUTCMonth() + 1)}-${pad2(d.getUTCDate())}`, hour: d.getUTCHours() };
}

/** Прогноз без дней раньше «сегодня места»; best, указывающий на ушедший день, сбрасывается. */
export function liveWeather(w: Weather | undefined, nowMs: number): Weather | undefined {
  if (!w || !w.enabled) return w;
  const now = placeNow(w.utc_offset, nowMs);
  const days = w.days.filter((d) => d.date >= now.date);
  if (days.length === w.days.length) return w;
  const keep = (date: string | null) => (date && days.some((d) => d.date === date) ? date : null);
  return { ...w, days, best: { wash: keep(w.best.wash), coat: keep(w.best.coat) } };
}

/** Весь прогноз устарел: дни были, но все раньше сегодняшнего. */
export function isOutdated(w: Weather | undefined, nowMs: number): boolean {
  return !!w && w.enabled && w.days.length > 0 && liveWeather(w, nowMs)!.days.length === 0;
}

/** Часы ленты: только если они за сегодня места (первый день снимка = сегодня), с текущего часа места. */
export function hoursToday(w: Weather, nowMs: number): Weather['today_hours'] {
  const now = placeNow(w.utc_offset, nowMs);
  if (w.days[0]?.date !== now.date) return [];
  return upcomingHours(w.today_hours, now.hour);
}

export const STALE_AFTER_S = 3 * 3600;

/** «данные на 30.10 14:05» — при stale или если прогнозу больше трёх часов. */
export function dataNote(w: Weather, nowMs: number): string {
  if (w.fetched_at === null) return '';
  if (!w.stale && nowMs / 1000 - w.fetched_at <= STALE_AFTER_S) return '';
  return `данные на ${formatDay(w.fetched_at)} ${formatTime(w.fetched_at * 1000)}`;
}

export type BannerLine = {
  /** «Сегодня» или день недели, если сегодняшнего дня в прогнозе уже нет. */
  lead: string;
  note: string;
  today: WeatherDay;
  /** Первая причина того, что сегодня не всё хорошо; пусто, если всё хорошо. */
  reason: string;
  /** «Лучше: мойка — сб, покрытие — вт»; пусто, если сегодня всё хорошо или хороших дней нет. */
  better: string;
};

/** Первая причина: сначала мойка, потом покрытие. */
export function firstReason(day: WeatherDay): string {
  for (const v of [day.wash, day.coat] satisfies WeatherVerdict[]) {
    if (v.level !== 'good' && v.reasons[0]) return v.reasons[0];
  }
  return '';
}

/** «мойка — сб, покрытие — вт» для того, что сегодня не хорошо. */
export function betterText(w: Weather): string {
  const today = w.days[0];
  if (!today) return '';
  const parts: string[] = [];
  const add = (kind: VerdictKind, level: WeatherLevel, date: string | null) => {
    if (level !== 'good' && date) parts.push(`${VERDICT_TITLE[kind]} — ${dayName(w.days, date)}`);
  };
  add('wash', today.wash.level, w.best.wash);
  add('coat', today.coat.level, w.best.coat);
  return parts.join(', ');
}

/** Содержимое плашки на главной. */
export function bannerLine(raw: Weather | undefined, nowMs: number = Date.now()): BannerLine | null {
  const w = liveWeather(raw, nowMs);
  if (!hasForecast(w)) return null;
  const today = w.days[0]!;
  const better = betterText(w);
  const isToday = today.date === placeNow(w.utc_offset, nowMs).date;
  return { lead: isToday ? 'Сегодня' : dayName(w.days, today.date), note: dataNote(w, nowMs), today, reason: firstReason(today), better: better ? `Лучше: ${better}` : '' };
}

/** Весь день хороший для обоих дел. */
export function allGood(day: WeatherDay): boolean {
  return day.wash.level === 'good' && day.coat.level === 'good';
}

export const BANNER_COLLAPSE_KEY = 'garage:weather-banner:collapsed';

type Store = Pick<Storage, 'getItem' | 'setItem'>;

function browserStore(): Store | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
}

/** Плашка свёрнута? Хранилище может быть недоступно (private mode) — тогда развёрнута. */
export function readCollapsed(store: Store | null = browserStore()): boolean {
  try {
    return store?.getItem(BANNER_COLLAPSE_KEY) === '1';
  } catch {
    return false;
  }
}

export function writeCollapsed(value: boolean, store: Store | null = browserStore()): void {
  try {
    store?.setItem(BANNER_COLLAPSE_KEY, value ? '1' : '0');
  } catch {
    /* не запомнится — не страшно */
  }
}

export const MIN_PLACE_QUERY = 2;
export const PLACE_DEBOUNCE_MS = 400;

/** Запрос годится для поиска города: после обрезки пробелов не короче двух символов. */
export function placeQuery(raw: string): string {
  const q = raw.trim();
  return q.length >= MIN_PLACE_QUERY ? q : '';
}

export const PLACES_TOO_MANY = 'Слишком частые запросы. Подождите немного и повторите.';
export const WEATHER_UNAVAILABLE = 'Прогноз сейчас недоступен';

/** Текст ошибки поиска: 429 — понятное ожидание. */
export function placesErrorText(status: number | undefined, retryAfter: number | undefined, fallback: string): string {
  if (status === 429) return retryAfter ? `Слишком частые запросы. Повторите через ${Math.ceil(retryAfter)}\u00a0с.` : PLACES_TOO_MANY;
  return fallback;
}

/**
 * Часы сегодняшней ленты начиная с текущего («15:00» и позже): прошедшие не нужны.
 * Время — местное для места прогноза; для своего города совпадает с часами телефона.
 * Если все часы прошли (поздний вечер), показываем последние три.
 */
export function upcomingHours<T extends { time: string }>(hours: readonly T[], nowHour: number): T[] {
  const left = hours.filter((h) => Number.parseInt(h.time, 10) >= nowHour);
  return left.length > 0 ? left : hours.slice(-3);
}
