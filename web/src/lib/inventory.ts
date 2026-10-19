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
  /** Групповое действие: id первого экземпляра группы (у отметок одной группы он общий). */
  group?: number;
  /** Движение, созданное отметкой (корректировка, списание): по нему работает «Отменить». */
  mv?: number;
  /** Куда переложен (для «moved»): отмена возвращает, только если экземпляр всё ещё там. */
  to?: number;
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
  return withoutMarks(s, [unitId]);
}

/** Ставит несколько отметок разом (групповое действие). */
export function withMarks(s: CheckState, entries: readonly (readonly [number, Mark])[]): CheckState {
  const marks = { ...s.marks };
  for (const [id, m] of entries) marks[String(id)] = m;
  return { ...s, marks };
}

export function withoutMarks(s: CheckState, unitIds: readonly number[]): CheckState {
  const drop = new Set(unitIds.map(String));
  return { ...s, marks: Object.fromEntries(Object.entries(s.marks).filter(([k]) => !drop.has(k))) };
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

// ---------- Группы одинаковых запечатанных экземпляров ----------

/** Поля экземпляра, нужные для группировки. */
export type UnitLike = { id: number; status: string; initial_qty: number; remaining_qty: number; product: { id: number } };

/** Строка экрана: один экземпляр или группа (pending — ещё не проверены, done — отмечены разом). */
export type Row = { kind: 'unit'; id: number } | { kind: 'group'; key: string; ids: number[]; done: boolean };

/** Ключ группы: позиция + размер упаковки + остаток; только запечатанные. Вскрытые — по одному. */
export function groupKey(u: UnitLike): string | null {
  return u.status === 'sealed' ? `${u.product.id}:${u.initial_qty}:${u.remaining_qty}` : null;
}

function push<K>(m: Map<K, number[]>, k: K, id: number): void {
  const a = m.get(k);
  if (a) a.push(id);
  else m.set(k, [id]);
}

/**
 * Строки экрана по порядку id. Непроверенные запечатанные одинаковые экземпляры (от двух)
 * сливаются в группу на месте первого; отмеченные одним групповым действием (общий mark.group) —
 * в группу-итог. Остальное — отдельные строки.
 */
export function buildRows(order: readonly number[], byId: ReadonlyMap<number, UnitLike>, marks: Readonly<Record<string, Mark>>): Row[] {
  const pending = new Map<string, number[]>();
  const done = new Map<number, number[]>();
  for (const id of order) {
    const m = marks[String(id)];
    if (m) {
      if (m.group !== undefined) push(done, m.group, id);
      continue;
    }
    const u = byId.get(id);
    const k = u ? groupKey(u) : null;
    if (k) push(pending, k, id);
  }
  const rows: Row[] = [];
  const seen = new Set<string>();
  for (const id of order) {
    const m = marks[String(id)];
    const key = m ? (m.group !== undefined ? `d${m.group}` : null) : groupKeyOf(byId.get(id));
    const ids = m ? (m.group !== undefined ? done.get(m.group) : undefined) : key ? pending.get(key.slice(1)) : undefined;
    if (!key || !ids || ids.length < 2) {
      rows.push({ kind: 'unit', id });
    } else if (!seen.has(key)) {
      seen.add(key);
      rows.push({ kind: 'group', key, ids, done: m !== undefined });
    }
  }
  return rows;
}

const groupKeyOf = (u: UnitLike | undefined): string | null => {
  const k = u ? groupKey(u) : null;
  return k ? `p${k}` : null;
};

/** Итог по отметкам группы одной строкой: «на месте 28 · не найдено 2». */
export function groupSummary(marks: readonly Mark[]): string {
  const n = (v: Verdict) => marks.filter((m) => m.verdict === v).length;
  const parts: string[] = [];
  if (n('ok') > 0) parts.push(`на месте ${n('ok')}`);
  if (n('fixed') > 0) parts.push(`исправлено ${n('fixed')}`);
  if (n('missing') > 0) parts.push(`не найдено ${n('missing')}`);
  const moved = marks.filter((m) => m.verdict === 'moved');
  if (moved.length > 0) {
    const place = /«(.+)»/u.exec(moved[0]?.detail ?? '')?.[1];
    parts.push(`${moved.length} ${place ? `в «${place}»` : 'в другом месте'}`);
  }
  return parts.join(' · ');
}

/** Главный исход группы для значка: не найдено > переложено > исправлено > на месте. */
export function groupVerdict(marks: readonly Mark[]): Verdict {
  for (const v of ['missing', 'moved', 'fixed'] as const) if (marks.some((m) => m.verdict === v)) return v;
  return 'ok';
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

const OPTIONAL_IDS = ['group', 'mv', 'to'] as const;

function isMark(m: unknown): m is Mark {
  if (typeof m !== 'object' || m === null) return false;
  const r = m as Record<string, unknown>;
  const optionalOk = OPTIONAL_IDS.every((k) => r[k] === undefined || (typeof r[k] === 'number' && Number.isInteger(r[k])));
  return (VERDICTS as readonly unknown[]).includes(r['verdict']) && typeof r['name'] === 'string' && typeof r['detail'] === 'string' && optionalOk;
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
      for (const f of OPTIONAL_IDS) if (m[f] !== undefined) marks[k][f] = m[f];
    }
    return { v: 1, locationId, startedAt: o['startedAt'], marks };
  } catch {
    return null;
  }
}

/** Незавершённая проверка старше этого срока не предлагается и удаляется. */
export const STATE_TTL_DAYS = 30;

export function isExpired(startedAt: number, nowSeconds: number): boolean {
  return nowSeconds - startedAt > STATE_TTL_DAYS * 86_400;
}

/** Читает состояние из localStorage; недоступное хранилище — как «ничего нет». Просроченное удаляет. */
export function loadState(userId: number, locationId: number, nowSeconds: number = Math.floor(Date.now() / 1000)): CheckState | null {
  try {
    const key = storageKey(userId, locationId);
    const s = parseState(globalThis.localStorage.getItem(key), locationId);
    if (s && isExpired(s.startedAt, nowSeconds)) {
      globalThis.localStorage.removeItem(key);
      return null;
    }
    return s;
  } catch {
    return null;
  }
}

/** Выход пользователя: удаляет все его незавершённые проверки на этом устройстве. */
export function clearUserStates(userId: number): void {
  try {
    const prefix = `garage.inventory.${userId}.`;
    const ls = globalThis.localStorage;
    const found: string[] = [];
    for (let i = 0; i < ls.length; i += 1) {
      const k = ls.key(i);
      if (k?.startsWith(prefix)) found.push(k);
    }
    for (const k of found) ls.removeItem(k);
  } catch {
    /* см. saveState */
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
