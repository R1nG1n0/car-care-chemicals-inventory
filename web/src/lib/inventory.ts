// Инвентаризация (ФТ-10.1): порядок мест, прогресс, сводка, состояние проверки
// в localStorage. Чистые функции — покрыты тестами (inventory.test.ts).
import type { Location } from '../api';
import { plural } from './text';

/** Итог по экземпляру: подтверждён, исправлен остаток, не найден, переложен. */
export type Verdict = 'ok' | 'fixed' | 'missing' | 'moved';

export const VERDICTS: readonly Verdict[] = ['ok', 'fixed', 'missing', 'moved'];

/** Отметка экземпляра. name и detail нужны, чтобы показать строку, когда экземпляр уже ушёл из места. */
export type Mark = {
  verdict: Verdict;
  name: string;
  /** Подпись итога: «320 → 250 мл», «в Коробка 3», «списано 400 мл». */
  detail: string;
};

export type CheckState = {
  v: 1;
  locationId: number;
  /** Когда проверка начата, секунды Unix. */
  startedAt: number;
  /** Отметки по id экземпляра. */
  marks: Record<string, Mark>;
};

export const REASON_FIX = 'Инвентаризация';
export const REASON_MISSING = 'Инвентаризация: не найден';

export function newState(locationId: number, startedAt: number): CheckState {
  return { v: 1, locationId, startedAt, marks: {} };
}

/** Ставит или заменяет отметку; исходное состояние не меняется. */
export function withMark(s: CheckState, unitId: number, m: Mark): CheckState {
  return { ...s, marks: { ...s.marks, [String(unitId)]: m } };
}

export function withoutMark(s: CheckState, unitId: number): CheckState {
  const marks = Object.fromEntries(Object.entries(s.marks).filter(([k]) => k !== String(unitId)));
  return { ...s, marks };
}

/**
 * Места для выбора: без архивных; никогда не проверявшиеся — сверху (по порядку
 * справочника), затем проверенные давно, в конце — проверенные недавно.
 */
export function sortLocations(list: readonly Location[]): Location[] {
  return list
    .filter((l) => !l.archived)
    .sort((a, b) => {
      const ca = a.checked_at ?? -Infinity;
      const cb = b.checked_at ?? -Infinity;
      if (ca !== cb) return ca < cb ? -1 : 1;
      return a.sort - b.sort || a.id - b.id;
    });
}

const pad = (n: number) => String(n).padStart(2, '0');

/** «ДД.ММ» по местному времени. */
export function shortDate(seconds: number): string {
  const d = new Date(seconds * 1000);
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}`;
}

/** Подпись под местом: «проверено 03.10» / «не проверялось». */
export function checkedLabel(checkedAt: number | null): string {
  return checkedAt === null ? 'не проверялось' : `проверено ${shortDate(checkedAt)}`;
}

/** Месяцев без проверки не считаем: «давно» — больше 30 дней или никогда. */
export const STALE_DAYS = 30;
export function isStale(checkedAt: number | null, nowSeconds: number): boolean {
  return checkedAt === null || nowSeconds - checkedAt > STALE_DAYS * 86_400;
}

export type Progress = {
  done: number;
  total: number;
  /** Доля 0…1 для шкалы. */
  level: number;
};

/**
 * Прогресс «проверено N из M». Экземпляры в списке — текущие в месте; отметки без
 * экземпляра в списке (переложен, списан) тоже считаются проверенными и входят в M.
 */
export function progressOf(unitIds: readonly number[], marks: Readonly<Record<string, Mark>>): Progress {
  const inList = new Set(unitIds);
  let done = 0;
  let gone = 0;
  for (const key of Object.keys(marks)) {
    if (inList.has(Number(key))) done += 1;
    else gone += 1;
  }
  const total = inList.size + gone;
  return { done: done + gone, total, level: total === 0 ? 0 : (done + gone) / total };
}

/** Отметки экземпляров, которых уже нет в месте, — по порядку id. */
export function goneMarks(unitIds: readonly number[], marks: Readonly<Record<string, Mark>>): { id: number; mark: Mark }[] {
  const inList = new Set(unitIds);
  return Object.entries(marks)
    .map(([k, mark]) => ({ id: Number(k), mark }))
    .filter((e) => !inList.has(e.id))
    .sort((a, b) => a.id - b.id);
}

/**
 * Порядок строк на экране места. Строки не прыгают при обновлении списка: прежние id остаются
 * на своих местах, пока экземпляр в списке или отмечен (ушедший с отметкой остаётся
 * строкой-итогом); новые экземпляры — в конец, по порядку списка.
 */
export function mergeOrder(prev: readonly number[], current: readonly number[], marks: Readonly<Record<string, Mark>>): number[] {
  const inList = new Set(current);
  const kept = prev.filter((id) => inList.has(id) || marks[String(id)] !== undefined);
  const known = new Set(kept);
  const fresh = current.filter((id) => !known.has(id));
  for (const id of fresh) known.add(id);
  const gone = Object.keys(marks)
    .map(Number)
    .filter((id) => !known.has(id))
    .sort((a, b) => a - b);
  return [...kept, ...fresh, ...gone];
}

export type Summary = Record<Verdict, number> & { unchecked: number };

/** Сводка завершения: сколько подтверждено, исправлено, не найдено, переложено и не проверено. */
export function summarize(unitIds: readonly number[], marks: Readonly<Record<string, Mark>>): Summary {
  const out: Summary = { ok: 0, fixed: 0, missing: 0, moved: 0, unchecked: 0 };
  for (const m of Object.values(marks)) out[m.verdict] += 1;
  for (const id of unitIds) if (!marks[String(id)]) out.unchecked += 1;
  return out;
}

const EXEMPLARS = ['экземпляр', 'экземпляра', 'экземпляров'] as const;

export function exemplars(n: number): string {
  return `${n} ${plural(n, EXEMPLARS)}`;
}

export const VERDICT_TEXT: Record<Verdict, string> = {
  ok: 'На месте',
  fixed: 'Остаток исправлен',
  missing: 'Не найдено',
  moved: 'Переложено',
};

/** Сводка по строкам: только ненулевые, в порядке подтверждено → исправлено → не найдено → переложено. */
export function summaryLines(s: Summary): { verdict: Verdict; count: number; label: string }[] {
  const label: Record<Verdict, string> = { ok: 'Подтверждено', fixed: 'Исправлено', missing: 'Не найдено', moved: 'Перемещено' };
  return VERDICTS.filter((v) => s[v] > 0).map((v) => ({ verdict: v, count: s[v], label: label[v] }));
}

// ---------- Хранение в браузере ----------

/** Ключ: на одном устройстве могут входить разные пользователи. */
export function storageKey(userId: number, locationId: number): string {
  return `garage.inventory.${userId}.${locationId}`;
}

export function serializeState(s: CheckState): string {
  return JSON.stringify(s);
}

function isMark(m: unknown): m is Mark {
  if (typeof m !== 'object' || m === null) return false;
  const r = m as Record<string, unknown>;
  return (VERDICTS as readonly unknown[]).includes(r['verdict']) && typeof r['name'] === 'string' && typeof r['detail'] === 'string';
}

/** Разбирает сохранённое состояние; битое или от другого места — null. */
export function parseState(raw: string | null, locationId: number): CheckState | null {
  if (!raw) return null;
  try {
    const o = JSON.parse(raw) as Record<string, unknown>;
    if (o['v'] !== 1 || o['locationId'] !== locationId || typeof o['startedAt'] !== 'number') return null;
    const marksRaw = o['marks'];
    if (typeof marksRaw !== 'object' || marksRaw === null || Array.isArray(marksRaw)) return null;
    const marks: Record<string, Mark> = {};
    for (const [k, m] of Object.entries(marksRaw)) {
      if (!/^\d+$/u.test(k) || !isMark(m)) return null;
      marks[k] = { verdict: m.verdict, name: m.name, detail: m.detail };
    }
    return { v: 1, locationId, startedAt: o['startedAt'], marks };
  } catch {
    return null;
  }
}

/** Читает состояние из localStorage; недоступное хранилище — как «ничего нет». */
export function loadState(userId: number, locationId: number): CheckState | null {
  try {
    return parseState(globalThis.localStorage.getItem(storageKey(userId, locationId)), locationId);
  } catch {
    return null;
  }
}

/** Пустое состояние (без отметок) не храним. */
export function saveState(userId: number, s: CheckState): void {
  try {
    const key = storageKey(userId, s.locationId);
    if (Object.keys(s.marks).length === 0) globalThis.localStorage.removeItem(key);
    else globalThis.localStorage.setItem(key, serializeState(s));
  } catch {
    /* хранилище закрыто или переполнено: проверка идёт, но после перезагрузки начнётся заново */
  }
}

export function clearState(userId: number, locationId: number): void {
  try {
    globalThis.localStorage.removeItem(storageKey(userId, locationId));
  } catch {
    /* см. saveState */
  }
}
