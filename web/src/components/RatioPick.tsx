// Выбор пропорции (ФТ-17.2.1): чипы пропорций позиции + «Своя 1:N» с полем ввода.
// Без пропорций у позиции (калькулятор без позиции) — только поле 1:N.
import { useState, type KeyboardEvent } from 'react';
import { parseQty } from '../lib/qty';
import { ratioText, type Dilution } from '../lib/dilution';

type Props = {
  id: string;
  dilutions: readonly Dilution[];
  /** Текст поля: пустое допустимо. */
  ratio: string;
  onChange: (ratio: string) => void;
  compact?: boolean | undefined;
  error?: string | undefined;
};

const digits = (s: string) => s.replace(/[^\d\s]/gu, '');

export function RatioPick({ id, dilutions, ratio, onChange, compact, error }: Props) {
  const hasChips = dilutions.length > 0;
  const matches = (d: Dilution) => d.ratio === parseQty(ratio);
  // Своя пропорция: у позиции нет такой, либо пропорций у позиции нет вовсе
  const [custom, setCustom] = useState(() => !hasChips || (ratio.trim() !== '' && !dilutions.some(matches)));
  const ratioId = `${id}-ratio`;

  const options = [...dilutions.map((d) => ({ key: `d${d.id}`, d })), ...(hasChips ? [{ key: 'own', d: null }] : [])];
  const isOn = (o: (typeof options)[number]) => (o.d ? !custom && matches(o.d) : custom);
  const pick = (o: (typeof options)[number]) => {
    if (o.d) {
      setCustom(false);
      onChange(String(o.d.ratio));
    } else setCustom(true);
  };
  const key = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    const d = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0;
    if (!d) return;
    e.preventDefault();
    const j = (i + d + options.length) % options.length;
    pick(options[j]!);
    document.getElementById(`${id}-opt-${options[j]!.key}`)?.focus();
  };
  const focusable = Math.max(0, options.findIndex(isOn));

  return (
    <div className={`g-field${error ? ' g-field--error' : ''}`}>
      <span className={compact ? 'g-sr' : 'g-label'} id={`${id}-ratio-l`}>
        Пропорция
      </span>
      {hasChips && (
        <div className="g-catpick" role="radiogroup" aria-labelledby={`${id}-ratio-l`}>
          {options.map((o, i) => (
            <button
              key={o.key}
              id={`${id}-opt-${o.key}`}
              type="button"
              className="g-catpick__opt"
              role="radio"
              aria-checked={isOn(o)}
              tabIndex={i === focusable ? 0 : -1}
              onClick={() => pick(o)}
              onKeyDown={(e) => key(e, i)}
            >
              {o.d ? `${o.d.name} ${ratioText(o.d.ratio)}` : 'Своя 1:N'}
            </button>
          ))}
        </div>
      )}
      {(custom || !hasChips) && (
        <div className="dil-own">
          <div className="g-qty__value">
            <b aria-hidden="true">1:</b>
            <input
              id={ratioId}
              inputMode="numeric"
              autoComplete="off"
              maxLength={4}
              value={ratio}
              placeholder="10"
              aria-label="Своя пропорция: частей воды на часть концентрата"
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? `${ratioId}-err` : undefined}
              onChange={(e) => onChange(digits(e.target.value))}
            />
          </div>
          <span className="g-field__hint">частей воды на 1 часть концентрата</span>
        </div>
      )}
      {error && (
        <span className="g-field__error" id={`${ratioId}-err`} role="alert">
          {error}
        </span>
      )}
    </div>
  );
}
