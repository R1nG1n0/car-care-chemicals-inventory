// Ввод количества (ФТ-5.8, .g-qty): поле с цифровой клавиатурой и кнопки шагов.
// Для мл — −большой −малый +малый +большой из настроек профиля, для шт — −1 +1 +5.
// Значение не уходит ниже min и выше max. Кнопки и ввод — без анимации.
import type { ReactNode } from 'react';
import type { Settings, Unit } from '../api';
import { useSession } from '../app/session';
import { unitLabel } from '../lib/stock';
import { applyStep, stepButtons, stepLabel, stepText } from '../lib/stockQty';

export type QtyInputProps = {
  id: string;
  /** Значение поля строкой: пустое поле допустимо, разбирает parseQty. */
  value: string;
  onChange: (value: string) => void;
  unit: Unit;
  label?: string;
  /** Нижняя граница для кнопок, по умолчанию 0. */
  min?: number;
  /** Верхняя граница для кнопок (для расхода — остаток); null — без границы. */
  max?: number | null;
  /** Шаги вместо настроек профиля. */
  steps?: Settings;
  hint?: ReactNode;
  error?: string | undefined;
  autoFocus?: boolean;
  onEnter?: () => void;
};

export function QtyInput({ id, value, onChange, unit, label = 'Количество', min = 0, max, steps, hint, error, autoFocus, onEnter }: QtyInputProps) {
  const session = useSession();
  const buttons = stepButtons(unit, steps ?? session.data?.settings);
  const u = unitLabel(unit);
  const hintId = `${id}-hint`;
  const errId = `${id}-err`;
  const described = [hint ? hintId : '', error ? errId : ''].filter(Boolean).join(' ') || undefined;

  return (
    <div className={`g-qty${error ? ' g-field--error' : ''}`}>
      <label className="g-label" htmlFor={id}>
        {label}
      </label>
      <div className="g-qty__value">
        <input
          id={id}
          inputMode="numeric"
          autoComplete="off"
          enterKeyHint="done"
          value={value}
          aria-invalid={error ? true : undefined}
          aria-describedby={described}
          data-autofocus={autoFocus || undefined}
          onChange={(e) => onChange(e.target.value.replace(/[^\d\s]/gu, ''))}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && onEnter) {
              e.preventDefault();
              onEnter();
            }
          }}
        />
        <b>{u}</b>
      </div>
      <div className={`g-qty__steps${buttons.length === 3 ? ' stock-qty__steps--3' : ''}`}>
        {buttons.map((d) => (
          <button key={d} type="button" className="g-btn g-press" aria-label={stepLabel(d, unit)} onClick={() => onChange(String(applyStep(value, d, min, max)))}>
            {stepText(d)}
          </button>
        ))}
      </div>
      {hint && (
        <div className="g-qty__hint" id={hintId}>
          {hint}
        </div>
      )}
      {error && (
        <div className="g-field__error" id={errId} role="alert">
          {error}
        </div>
      )}
    </div>
  );
}
