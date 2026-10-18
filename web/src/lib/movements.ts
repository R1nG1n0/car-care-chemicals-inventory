// Журнал движений на карточке позиции: подписи, что можно отменить, вопрос отмены.
import type { Movement, Unit } from '../api';
import { qtyText } from './stock';

const TYPE: Record<Movement['type'], string> = {
  purchase: 'Покупка',
  consume: 'Расход',
  writeoff: 'Списание',
  move: 'Перемещение',
  adjust: 'Корректировка',
};

/** «Расход · 40 мл», «Корректировка · +20 мл», «Перемещение». */
export function movementTitle(m: Pick<Movement, 'type' | 'qty_delta' | 'reason'>, unit: Unit): string {
  const name = m.type === 'adjust' && m.reason === 'Начальный остаток' ? 'Начальный остаток' : TYPE[m.type];
  if (m.type === 'move' || m.qty_delta === 0) return name;
  const abs = qtyText(Math.abs(m.qty_delta), unit);
  if (m.type === 'adjust') return `${name} · ${m.qty_delta > 0 ? '+' : '−'}${abs}`;
  return `${name} · ${abs}`;
}

/** Отменить можно расход, списание и корректировку (docs/API.md, revert). */
export function canRevert(m: Pick<Movement, 'type'>): boolean {
  return m.type === 'consume' || m.type === 'writeoff' || m.type === 'adjust';
}

/** Вопрос подтверждения отмены из журнала. */
export function revertQuestion(m: Pick<Movement, 'type' | 'qty_delta' | 'reason'>, unit: Unit): string {
  return `Отменить: ${movementTitle(m, unit).toLocaleLowerCase('ru-RU')}? Остаток пересчитается.`;
}
