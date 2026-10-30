// Калькулятор разбавления (ФТ-17.3): два режима, считает на телефоне без запросов.
import { concentrateFor, fromConcentrate, parseSolution, RATIO_ERROR, VOLUME_ERROR, validRatio, waterFor } from './dilution';
import { MAX_QTY, parseQty } from './qty';

export type CalcMode = 'forward' | 'reverse';

export const CONCENTRATE_ERROR = 'Концентрат — от 1 до 100 000 мл';

export type CalcInput = { mode: CalcMode; volume: string; concentrate: string; ratio: string };
export type CalcResult =
  | { mode: 'forward'; volume: number; ratio: number; concentrate: number; water: number }
  | { mode: 'reverse'; concentrate: number; ratio: number; solution: number; water: number };

/** Результат или null, если поля ещё не заполнены верно. */
export function calculate(i: CalcInput): CalcResult | null {
  if (i.mode === 'forward') {
    const s = parseSolution(i.volume, i.ratio);
    return s ? { mode: 'forward', volume: s.volume_ml, ratio: s.ratio, concentrate: concentrateFor(s.volume_ml, s.ratio), water: waterFor(s.volume_ml, s.ratio) } : null;
  }
  const c = parseQty(i.concentrate);
  const r = parseQty(i.ratio);
  if (c === null || c < 1 || c > MAX_QTY || !validRatio(r)) return null;
  return { mode: 'reverse', concentrate: c, ratio: r, ...fromConcentrate(c, r) };
}

/** Ошибки полей, которые уже начали вводить (пустое поле — не ошибка, а «ещё не ввели»). */
export function calcErrors(i: CalcInput): { volume?: string; concentrate?: string; ratio?: string } {
  const e: { volume?: string; concentrate?: string; ratio?: string } = {};
  const filled = (s: string) => s.replace(/\s/gu, '') !== '';
  if (i.mode === 'forward' && filled(i.volume) && parseSolution(i.volume, '1') === null) e.volume = VOLUME_ERROR;
  if (i.mode === 'reverse' && filled(i.concentrate)) {
    const c = parseQty(i.concentrate);
    if (c === null || c < 1 || c > MAX_QTY) e.concentrate = CONCENTRATE_ERROR;
  }
  if (filled(i.ratio) && !validRatio(parseQty(i.ratio))) e.ratio = RATIO_ERROR;
  return e;
}
