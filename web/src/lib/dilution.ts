// Разбавление концентратов (ФТ-17): расчёт, проверка пропорций позиции и раствора,
// подписи журнала и очереди. Расчёт тот же, что на сервере: интерфейс показывает
// число и оценивает остаток без связи, но в запрос уходит раствор, а не концентрат.
import type { Dilution, Solution } from '../api';
import { formatQty } from './format';
import { MAX_QTY, parseQty } from './qty';
import { charCount } from './validate';

// Раствор: volume_ml — объём готового раствора, мл; ratio — N в пропорции 1:N (частей воды на часть концентрата)
export type { Dilution, Solution };

export const MAX_RATIO = 1000;
export const MAX_DILUTIONS = 10;
export const MAX_DILUTION_NAME = 40;
/** Быстрые объёмы раствора, мл (ФТ-17.3.1). */
export const QUICK_VOLUMES = [500, 750, 1000, 5000] as const;

const NBSP = '\u00a0';

/** «1:10». */
export const ratioText = (ratio: number): string => `1:${ratio}`;

/** Подпись быстрого объёма: «500 мл», «1 л», «5 л». */
export function quickVolumeText(ml: number): string {
  return ml >= 1000 && ml % 1000 === 0 ? `${ml / 1000}${NBSP}л` : `${formatQty(ml)}${NBSP}мл`;
}

/**
 * Концентрат на объём раствора: объём / (N + 1), округление к ближайшему целому
 * (половины вверх), не меньше 1 мл. Целочисленная арифметика — без ошибок дробей.
 */
export function concentrateFor(volumeMl: number, ratio: number): number {
  const parts = ratio + 1;
  return Math.max(1, Math.floor((2 * volumeMl + parts) / (2 * parts)));
}

/** Концентрат раствора. */
export const concentrateOf = (s: Solution): number => concentrateFor(s.volume_ml, s.ratio);

/** Воды на раствор: объём минус концентрат (не меньше нуля). */
export function waterFor(volumeMl: number, ratio: number): number {
  return Math.max(0, volumeMl - concentrateFor(volumeMl, ratio));
}

/** Обратный расчёт: сколько раствора выйдет из концентрата и сколько на это воды. */
export function fromConcentrate(concentrateMl: number, ratio: number): { solution: number; water: number } {
  return { solution: concentrateMl * (ratio + 1), water: concentrateMl * ratio };
}

/** Объём раствора 1…100 000 мл. */
export const validVolume = (n: number | null): n is number => n !== null && Number.isInteger(n) && n >= 1 && n <= MAX_QTY;
/** N пропорции 1…1000. */
export const validRatio = (n: number | null): n is number => n !== null && Number.isInteger(n) && n >= 1 && n <= MAX_RATIO;

export const VOLUME_ERROR = 'Объём — от 1 до 100 000 мл';
export const RATIO_ERROR = 'Пропорция — от 1:1 до 1:1000';

/** Раствор из полей ввода; что-то не так — null. */
export function parseSolution(volume: string, ratio: string): Solution | null {
  const v = parseQty(volume);
  const r = parseQty(ratio);
  return validVolume(v) && validRatio(r) ? { volume_ml: v, ratio: r } : null;
}

/** Что именно не так с полями раствора (null — всё верно). */
export function solutionError(volume: string, ratio: string): { volume?: string; ratio?: string } | null {
  const e: { volume?: string; ratio?: string } = {};
  if (!validVolume(parseQty(volume))) e.volume = VOLUME_ERROR;
  if (!validRatio(parseQty(ratio))) e.ratio = RATIO_ERROR;
  return e.volume || e.ratio ? e : null;
}

/** «Концентрат 91 мл + вода 909 мл». */
export function mixText(s: Solution): string {
  const c = concentrateOf(s);
  return `Концентрат ${formatQty(c)}${NBSP}мл + вода ${formatQty(waterFor(s.volume_ml, s.ratio))}${NBSP}мл`;
}

/** «раствор 1 000 мл 1:10» — без большой буквы, для середины строки. */
export function solutionPhrase(s: Solution): string {
  return `раствор ${formatQty(s.volume_ml)}${NBSP}мл ${ratioText(s.ratio)}`;
}

/** Очередь без связи: «Раствор 1 000 мл 1:10 (91 мл)». */
export function solutionQueueText(s: Solution, concentrate: number = concentrateOf(s)): string {
  return `Раствор ${formatQty(s.volume_ml)}${NBSP}мл ${ratioText(s.ratio)} (${formatQty(concentrate)}${NBSP}мл)`;
}

/** Журнал: «Раствор 1 000 мл 1:10 → 91 мл». */
export function solutionJournalText(s: Solution, concentrate: number): string {
  return `Раствор ${formatQty(s.volume_ml)}${NBSP}мл ${ratioText(s.ratio)} → ${formatQty(concentrate)}${NBSP}мл`;
}

/** Раствор движения журнала; null — расход был обычным количеством. */
export function movementSolution(m: { solution_ml?: number | null; solution_ratio?: number | null }): Solution | null {
  return m.solution_ml != null && m.solution_ratio != null ? { volume_ml: m.solution_ml, ratio: m.solution_ratio } : null;
}

// ---------- Пропорции позиции ----------

export type DilutionInput = { id?: number; name: string; ratio: number };

/** Пропорции позиции; у старых сохранённых данных поля нет. */
export const dilutionsOf = (p: { dilutions?: readonly Dilution[] | undefined }): readonly Dilution[] => p.dilutions ?? [];

/** «Предмойка 1:10 · Салон 1:20». */
export function dilutionsText(list: readonly Pick<Dilution, 'name' | 'ratio'>[]): string {
  return list.map((d) => `${d.name} ${ratioText(d.ratio)}`).join(' · ');
}

/** Строка редактора пропорций: ratio — текст поля. id есть у уже сохранённой пропорции. */
export type DilutionDraft = { key: number; id?: number | undefined; name: string; ratio: string };
export type DilutionRowErrors = { name?: string; ratio?: string };

export function draftsOf(list: readonly Dilution[]): DilutionDraft[] {
  return list.map((d, i) => ({ key: i, id: d.id, name: d.name, ratio: String(d.ratio) }));
}

type DilutionsResult = { ok: true; value: DilutionInput[] } | { ok: false; form?: string; rows: Record<number, DilutionRowErrors> };

/** Проверка списка пропорций: до 10, название 1–40 символов, 1:N с N от 1 до 1000. Пустой список — норма. */
export function validateDilutions(drafts: readonly DilutionDraft[]): DilutionsResult {
  const rows: Record<number, DilutionRowErrors> = {};
  let form: string | undefined;
  if (drafts.length > MAX_DILUTIONS) form = `Пропорций — не больше ${MAX_DILUTIONS}`;
  const value: DilutionInput[] = [];
  for (const d of drafts) {
    const e: DilutionRowErrors = {};
    const name = d.name.trim();
    if (name === '') e.name = 'Введите название';
    else if (charCount(name) > MAX_DILUTION_NAME) e.name = `Не длиннее ${MAX_DILUTION_NAME} символов`;
    const r = parseQty(d.ratio);
    if (!validRatio(r)) e.ratio = 'От 1 до 1000';
    if (e.name || e.ratio) rows[d.key] = e;
    else value.push({ ...(d.id !== undefined ? { id: d.id } : {}), name, ratio: r! });
  }
  return form || Object.keys(rows).length > 0 ? { ok: false, ...(form ? { form } : {}), rows } : { ok: true, value };
}

// ---------- Режим и пропорция, выбранные в прошлый раз ----------

export type DilutionMode = 'concentrate' | 'solution';
export type DilutionPref = { mode: DilutionMode; ratio: number | null };

type Store = Pick<Storage, 'getItem' | 'setItem'>;

const prefKey = (productId: number) => `garage:dilution:${productId}`;

function browserStore(): Store | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
}

/** Последний режим и пропорция по позиции; нет, повреждено или хранилище закрыто — null. */
export function loadPref(productId: number, store: Store | null = browserStore()): DilutionPref | null {
  try {
    const raw = store?.getItem(prefKey(productId));
    if (!raw) return null;
    const v = JSON.parse(raw) as { mode?: unknown; ratio?: unknown };
    const mode = v.mode === 'solution' ? 'solution' : v.mode === 'concentrate' ? 'concentrate' : null;
    if (!mode) return null;
    return { mode, ratio: typeof v.ratio === 'number' && validRatio(v.ratio) ? v.ratio : null };
  } catch {
    return null;
  }
}

export function savePref(productId: number, pref: DilutionPref, store: Store | null = browserStore()): void {
  try {
    store?.setItem(prefKey(productId), JSON.stringify(pref));
  } catch {
    // место закончилось или хранилище закрыто — запоминание необязательно
  }
}

/** Начальная пропорция в поле: запомненная, иначе первая у позиции, иначе пусто. */
export function initialRatio(dilutions: readonly Dilution[], pref: DilutionPref | null): string {
  if (pref?.ratio != null) return String(pref.ratio);
  return dilutions[0] ? String(dilutions[0].ratio) : '';
}
