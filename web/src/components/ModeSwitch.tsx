// Переключатель «Концентрат / Раствор» (ФТ-17.2.1): сегментированный, как «мл / шт» в форме
// позиции. compact — для строки шаблона: подпись только для диктора.
import type { DilutionMode } from '../lib/dilution';

type Props = { id: string; mode: DilutionMode; onChange: (m: DilutionMode) => void; compact?: boolean };

const OPTIONS: { mode: DilutionMode; label: string }[] = [
  { mode: 'concentrate', label: 'Концентрат' },
  { mode: 'solution', label: 'Раствор' },
];

export function ModeSwitch({ id, mode, onChange, compact }: Props) {
  const group = (
    <div className="g-seg" role="group" {...(compact ? { 'aria-label': 'Чем задать количество' } : { 'aria-labelledby': `${id}-l` })}>
      {OPTIONS.map((o) => (
        <button key={o.mode} type="button" aria-pressed={mode === o.mode} onClick={() => mode !== o.mode && onChange(o.mode)}>
          {o.label}
        </button>
      ))}
    </div>
  );
  if (compact) return group;
  return (
    <div className="g-field">
      <span className="g-label" id={`${id}-l`}>
        Чем задать количество
      </span>
      {group}
    </div>
  );
}
