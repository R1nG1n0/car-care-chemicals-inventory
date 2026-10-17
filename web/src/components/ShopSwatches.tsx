// Выбор цвета категории (.g-swatches): радиогруппа из восьми токенов --cat-*,
// стрелки переключают выбор, как у нативных радиокнопок.
import { IconCheck } from '@tabler/icons-react';
import { useRef, type KeyboardEvent } from 'react';
import { CAT_COLORS, type CatColor } from '../api';
import { catClass } from './ShopUi';
import { Ico } from './ui';

export const CAT_COLOR_NAMES: Record<CatColor, string> = {
  blue: 'синий',
  berry: 'ягодный',
  olive: 'оливковый',
  ochre: 'охра',
  petrol: 'петроль',
  plum: 'сливовый',
  slate: 'сланцевый',
  umber: 'умбра',
};

type Props = { value: CatColor; onChange: (c: CatColor) => void; label: string };

export function ShopSwatches({ value, onChange, label }: Props) {
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const onKey = (e: KeyboardEvent, i: number) => {
    const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
    if (!step) return;
    e.preventDefault();
    const next = (i + step + CAT_COLORS.length) % CAT_COLORS.length;
    onChange(CAT_COLORS[next]!);
    refs.current[next]?.focus();
  };
  return (
    <div className="g-swatches" role="radiogroup" aria-label={label}>
      {CAT_COLORS.map((c, i) => (
        <button
          key={c}
          ref={(el) => {
            refs.current[i] = el;
          }}
          type="button"
          role="radio"
          aria-checked={c === value}
          aria-label={CAT_COLOR_NAMES[c]}
          tabIndex={c === value ? 0 : -1}
          className={`g-swatch ${catClass(c)}`}
          onClick={() => onChange(c)}
          onKeyDown={(e) => onKey(e, i)}
        >
          <Ico icon={IconCheck} />
        </button>
      ))}
    </div>
  );
}
