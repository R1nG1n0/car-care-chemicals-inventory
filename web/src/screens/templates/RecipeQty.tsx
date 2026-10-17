// Количество в строке шаблона (.g-num-input): число вписывается вручную,
// рядом кнопки ± 44×44 (ФТ-8.4). Без анимации — число меняется сразу.
import { IconMinus, IconPlus } from '@tabler/icons-react';
import { Ico } from '../../components/ui';
import { clampQty, parseQty } from '../../lib/recipeApply';
import { unitLabel } from '../../lib/stock';
import type { Unit } from '../../api';

type Props = { id: string; name: string; unit: Unit; value: string; step: number; invalid?: boolean; describedBy?: string | undefined; onChange: (v: string) => void };

export function RecipeQty({ id, name, unit, value, step, invalid, describedBy, onChange }: Props) {
  const u = unitLabel(unit);
  const bump = (d: number) => onChange(String(clampQty((parseQty(value) ?? 0) + d)));
  return (
    <div className="shop-tpl-ctl">
      <button type="button" className="g-btn g-btn--icon g-press" aria-label={`Минус ${step} ${u}: ${name}`} onClick={() => bump(-step)}>
        <Ico icon={IconMinus} />
      </button>
      <input
        id={id}
        className="g-num-input"
        inputMode="numeric"
        autoComplete="off"
        value={value}
        aria-label={`Количество, ${u}: ${name}`}
        aria-invalid={invalid || undefined}
        aria-describedby={describedBy}
        onChange={(e) => onChange(e.target.value.replace(/[^\d]/g, ''))}
      />
      <b>{u}</b>
      <button type="button" className="g-btn g-btn--icon g-press" aria-label={`Плюс ${step} ${u}: ${name}`} onClick={() => bump(step)}>
        <Ico icon={IconPlus} />
      </button>
    </div>
  );
}
