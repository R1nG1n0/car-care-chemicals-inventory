// Строка экземпляра при инвентаризации: позиция, остаток, вскрыт/запечатан. Пока не проверен —
// «На месте» и «Другое…» (шторка с остальными действиями). После отметки на строке
// появляется метка исхода: короткая «печать» (transform/opacity), без неё при reduced-motion.
import { IconAlertTriangle, IconArrowsExchange, IconCheck, IconPencil, type TablerIcon } from '@tabler/icons-react';
import type { InventoryUnit } from '../../api';
import { Ico } from '../../components/ui';
import { formatQty } from '../../lib/format';
import { VERDICT_TEXT, type Mark, type Verdict } from '../../lib/inventory';
import { qtyText } from '../../lib/stock';

const ICON: Record<Verdict, TablerIcon> = { ok: IconCheck, fixed: IconPencil, missing: IconAlertTriangle, moved: IconArrowsExchange };

function unitLine(u: InventoryUnit): string {
  const p = u.product;
  if (u.status === 'open') return `${p.unit === 'pcs' ? 'Вскрыта пачка' : 'Вскрыта'} · ${qtyText(u.remaining_qty, p.unit)} из ${formatQty(u.initial_qty)}`;
  return `${p.unit === 'pcs' ? 'Пачка запечатана' : 'Запечатана'} · ${qtyText(u.remaining_qty, p.unit)}`;
}

type Props = {
  unit: InventoryUnit | null;
  mark: Mark | null;
  /** Только что отмечена: проигрывается анимация метки. */
  fresh: boolean;
  onConfirm: () => void;
  onMore: () => void;
};

export function InventoryRow({ unit, mark, fresh, onConfirm, onMore }: Props) {
  const name = unit ? unit.product.name : (mark?.name ?? '');
  const brand = unit?.product.brand;
  const verdict = mark?.verdict;
  return (
    <li className={`g-row inv-row${verdict ? ` inv-row--done inv-row--${verdict}` : ''}${fresh ? ' is-fresh' : ''}`}>
      <div className="inv-row__info">
        <span className="g-row__title inv-block">{name}</span>
        <span className="g-row__sub inv-block">
          {mark ? mark.detail : unit ? `${brand ? `${brand} · ` : ''}${unitLine(unit)}` : ''}
        </span>
      </div>
      {mark && verdict ? (
        <span className={`inv-badge inv-badge--${verdict}`}>
          <Ico icon={ICON[verdict]} size={18} />
          {VERDICT_TEXT[verdict]}
        </span>
      ) : (
        unit && (
          <div className="inv-row__actions">
            <button type="button" className="g-btn g-btn--sm g-btn--accent g-press" aria-label={`На месте: ${name}`} onClick={onConfirm}>
              <Ico icon={IconCheck} /> На месте
            </button>
            <button type="button" className="g-btn g-btn--sm g-press" aria-label={`Другое действие: ${name}`} onClick={onMore}>
              Другое
            </button>
          </div>
        )
      )}
    </li>
  );
}
