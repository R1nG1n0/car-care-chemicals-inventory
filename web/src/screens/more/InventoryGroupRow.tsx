// Группа одинаковых запечатанных экземпляров одной позиции: одна строка вместо тридцати.
// Непроверенная — «На месте все» и «Не хватает…»; проверенная — итог и «Снять отметку» / «Отменить».
import { IconCheck } from '@tabler/icons-react';
import type { InventoryUnit } from '../../api';
import { Ico } from '../../components/ui';
import { formatQty } from '../../lib/format';
import { groupSummary, groupVerdict, type Mark } from '../../lib/inventory';
import { DoneControls } from './InventoryRow';

type Props = {
  /** Экземпляры группы, которые ещё в месте (у проверенной группы могут быть не все). */
  units: InventoryUnit[];
  /** Отметки группы; пусто — группа не проверена. */
  marks: Mark[];
  count: number;
  fresh: boolean;
  busy: boolean;
  onConfirm: () => void;
  onShortage: () => void;
  onUndo: () => void;
};

export function InventoryGroupRow({ units, marks, count, fresh, busy, onConfirm, onShortage, onUndo }: Props) {
  const first = units[0];
  const name = first?.product.name ?? marks[0]?.name ?? '';
  const done = marks.length > 0;
  const verdict = done ? groupVerdict(marks) : null;
  const pack = first && first.product.unit === 'ml' ? ` · по ${formatQty(first.initial_qty)} мл` : '';
  const sub = done ? `${count} шт · ${groupSummary(marks)}` : `${first?.product.brand ? `${first.product.brand} · ` : ''}запечатаны: ${count}${pack}`;
  return (
    <li className={`g-row inv-row inv-row--group${verdict ? ` inv-row--done inv-row--${verdict}` : ''}${fresh ? ' is-fresh' : ''}`}>
      <div className="inv-row__info">
        <span className="g-row__title inv-block">{name}</span>
        <span className="g-row__sub inv-block">{sub}</span>
      </div>
      {done && verdict ? (
        <DoneControls verdict={verdict} name={name} busy={busy} onUndo={onUndo} />
      ) : (
        <div className="inv-row__actions">
          <button type="button" className="g-btn g-btn--sm g-btn--accent g-press" aria-label={`На месте все ${count}: ${name}`} onClick={onConfirm}>
            <Ico icon={IconCheck} /> На месте все
          </button>
          <button type="button" className="g-btn g-btn--sm g-press" aria-label={`Не хватает: ${name}`} onClick={onShortage}>
            Не хватает…
          </button>
        </div>
      )}
    </li>
  );
}
