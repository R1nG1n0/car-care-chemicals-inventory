// Оценка средства (ФТ-15.4): интерактивные звёзды в карточке позиции и компактные в строке.
// Звёзды — радиогруппа «Оценка»: стрелки двигают выбор, повторное нажатие на текущую снимает оценку.
// Выбранная звезда чуть «подпрыгивает» (scale 0.9 → 1, 160 мс; при reduced-motion — без анимации).
import { IconStar, IconStarFilled } from '@tabler/icons-react';
import { useState, type KeyboardEvent } from 'react';
import { MAX_RATING, nextRating, ratingLabel } from '../lib/purposes';
import { Ico } from './ui';

const STARS = Array.from({ length: MAX_RATING }, (_, i) => i + 1);

type Props = {
  value: number | null;
  /** Новая оценка (null — снять). */
  onChange: (next: number | null) => void;
  /** Нет связи: звёзды неактивны (aria-disabled) и с подсказкой. */
  disabled?: boolean;
  hint?: string | undefined;
};

export function RatingStars({ value, onChange, disabled, hint }: Props) {
  const [popped, setPopped] = useState<number | null>(null);
  const focusable = value ?? 1;
  const choose = (n: number) => {
    if (disabled) return;
    const next = nextRating(value, n);
    setPopped(next);
    onChange(next);
  };
  const key = (e: KeyboardEvent<HTMLButtonElement>, n: number) => {
    const d = e.key === 'ArrowRight' || e.key === 'ArrowUp' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowDown' ? -1 : 0;
    if (!d) return;
    e.preventDefault();
    const to = Math.min(MAX_RATING, Math.max(1, n + d));
    if (to === n) return;
    choose(to);
    document.getElementById(`rating-star-${to}`)?.focus();
  };
  return (
    <div className="g-rating" role="radiogroup" aria-label="Оценка" title={hint}>
      {STARS.map((n) => {
        const on = value !== null && n <= value;
        return (
          <button
            key={n}
            id={`rating-star-${n}`}
            type="button"
            className={`g-star${on ? ' is-on' : ''}${popped === n && value === n ? ' is-pop' : ''}`}
            role="radio"
            aria-checked={value === n}
            aria-label={`${n} из ${MAX_RATING}`}
            aria-disabled={disabled || undefined}
            tabIndex={n === focusable ? 0 : -1}
            onClick={() => choose(n)}
            onKeyDown={(e) => key(e, n)}
            onAnimationEnd={() => setPopped(null)}
          >
            <Ico icon={on ? IconStarFilled : IconStar} size={26} />
          </button>
        );
      })}
    </div>
  );
}

/** Компактная оценка в строке позиции: «★ 4», текст для диктора. */
export function RatingMini({ value }: { value: number | null | undefined }) {
  if (!value) return null;
  return (
    <span className="g-rating-mini" role="img" aria-label={ratingLabel(value)}>
      <Ico icon={IconStarFilled} size={14} />
      {value}
    </span>
  );
}
