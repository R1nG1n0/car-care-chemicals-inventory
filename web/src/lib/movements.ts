// Журнал движений на карточке позиции: подписи, что можно отменить, вопрос отмены.
import type { Movement, Unit } from '../api';
import { movementSolution, solutionPhrase } from './dilution';
import { qtyText } from './stock';

const TYPE: Record<Movement['type'], string> = {
  purchase: 'Покупка',
  consume: 'Расход',
  writeoff: 'Списание',
  move: 'Перемещение',
  adjust: 'Корректировка',
};

/**
 * «Расход · 40 мл», «Корректировка · +20 мл», «Перемещение»; расход раствором (ФТ-17.2.3) —
 * «Расход · раствор 1 000 мл 1:10 → 91 мл». total — концентрат всего расхода, если он шёл
 * из двух бутылок (по умолчанию — столько, сколько списано этим движением).
 */
export function movementTitle(m: Pick<Movement, 'type' | 'qty_delta' | 'reason' | 'solution_ml' | 'solution_ratio'>, unit: Unit, total?: number): string {
  const name = m.type === 'adjust' && m.reason === 'Начальный остаток' ? 'Начальный остаток' : TYPE[m.type];
  if (m.type === 'move' || m.qty_delta === 0) return name;
  const sol = m.type === 'consume' ? movementSolution(m) : null;
  if (sol) return `${name} · ${solutionPhrase(sol)} → ${qtyText(total ?? Math.abs(m.qty_delta), unit)}`;
  const abs = qtyText(Math.abs(m.qty_delta), unit);
  if (m.type === 'adjust') return `${name} · ${m.qty_delta > 0 ? '+' : '−'}${abs}`;
  return `${name} · ${abs}`;
}

/** Отменить можно расход, списание и корректировку (docs/API.md, revert). */
export function canRevert(m: Pick<Movement, 'type'>): boolean {
  return m.type === 'consume' || m.type === 'writeoff' || m.type === 'adjust';
}

/** Вопрос подтверждения отмены из журнала. */
export function revertQuestion(m: Pick<Movement, 'type' | 'qty_delta' | 'reason' | 'solution_ml' | 'solution_ratio'>, unit: Unit): string {
  return `Отменить: ${movementTitle(m, unit).toLocaleLowerCase('ru-RU')}? Остаток пересчитается.`;
}

/** Концентрат расхода раствором по группе запроса (batch_id): складывается из движений обеих бутылок. */
export function solutionTotals(movements: readonly Pick<Movement, 'type' | 'qty_delta' | 'batch_id' | 'product_id' | 'solution_ml'>[]): Map<number, number> {
  const out = new Map<number, number>();
  for (const m of movements) {
    if (m.type === 'consume' && m.solution_ml != null) out.set(m.batch_id, (out.get(m.batch_id) ?? 0) + Math.abs(m.qty_delta));
  }
  return out;
}
