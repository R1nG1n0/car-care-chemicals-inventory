// Поля формы позиции — общие для формы /items/new и шторки «Новая позиция»:
// бренд с подсказкой из введённых ранее (.g-suggest) и выбор категории чипами
// с цветной точкой (.g-catpick, радиогруппа, стрелки). idPrefix разводит id,
// если на странице две формы.
import { useQuery } from '@tanstack/react-query';
import { useState, type KeyboardEvent } from 'react';
import { inventory, type Category } from '../api';
import { keys } from '../app/queries';
import { suggestBrands } from '../lib/product';
import { catClass } from './refs';

export function BrandField({ value, onChange, error, idPrefix = 'np' }: { value: string; onChange: (v: string) => void; error?: string | undefined; idPrefix?: string }) {
  const brands = useQuery({ queryKey: keys.brands, queryFn: ({ signal }) => inventory.brands(signal), staleTime: 60_000 });
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const list = open ? suggestBrands(brands.data ?? [], value) : [];
  const shown = list.length > 0;

  const pick = (b: string) => {
    onChange(b);
    setOpen(false);
  };
  const key = (e: KeyboardEvent<HTMLInputElement>) => {
    if (!shown) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive((a) => (a + 1) % list.length);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive((a) => (a - 1 + list.length) % list.length);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      pick(list[Math.min(active, list.length - 1)]!);
    } else if (e.key === 'Escape') {
      e.stopPropagation();
      setOpen(false);
    }
  };

  return (
    <div className={`g-field stock-suggest${error ? ' g-field--error' : ''}`}>
      <label className="g-label" htmlFor={`${idPrefix}-brand`}>
        Бренд <span className="g-field__opt">· необязательно</span>
      </label>
      <input
        className="g-input"
        id={`${idPrefix}-brand`}
        autoComplete="off"
        maxLength={60}
        role="combobox"
        aria-expanded={shown}
        aria-controls={`${idPrefix}-brand-list`}
        aria-autocomplete="list"
        aria-activedescendant={shown ? `${idPrefix}-brand-${Math.min(active, list.length - 1)}` : undefined}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? `${idPrefix}-brand-err` : undefined}
        value={value}
        onChange={(e) => {
          onChange(e.target.value);
          setOpen(true);
          setActive(0);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={key}
      />
      {shown && (
        <ul className="g-suggest" id={`${idPrefix}-brand-list`} role="listbox" aria-label="Введённые ранее">
          {list.map((b, i) => (
            <li
              key={b}
              id={`${idPrefix}-brand-${i}`}
              className="g-suggest__opt"
              role="option"
              aria-selected={i === Math.min(active, list.length - 1)}
              // mousedown, а не click: иначе поле теряет фокус и список закрывается раньше
              onMouseDown={(e) => {
                e.preventDefault();
                pick(b);
              }}
            >
              {b}
            </li>
          ))}
        </ul>
      )}
      {error && (
        <span className="g-field__error" id={`${idPrefix}-brand-err`}>
          {error}
        </span>
      )}
    </div>
  );
}

type CatProps = { categories: Category[]; value: number | null; onChange: (id: number) => void; error?: string | undefined; idPrefix?: string };

export function CategoryPick({ categories, value, onChange, error, idPrefix = 'np' }: CatProps) {
  const focusable = value ?? categories[0]?.id;
  const key = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    const d = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0;
    if (!d || categories.length === 0) return;
    e.preventDefault();
    const next = categories[(i + d + categories.length) % categories.length]!;
    onChange(next.id);
    document.getElementById(`${idPrefix}-cat-${next.id}`)?.focus();
  };
  return (
    <div className={`g-field${error ? ' g-field--error' : ''}`}>
      <span className="g-label" id={`${idPrefix}-cat-l`}>
        Категория
      </span>
      {categories.length === 0 ? (
        <p className="g-muted">Категорий нет — добавьте их в разделе «Ещё».</p>
      ) : (
        <div className="g-catpick" id={`${idPrefix}-category_id`} tabIndex={-1} role="radiogroup" aria-labelledby={`${idPrefix}-cat-l`} aria-describedby={error ? `${idPrefix}-category_id-err` : undefined}>
          {categories.map((c, i) => (
            <button
              key={c.id}
              id={`${idPrefix}-cat-${c.id}`}
              type="button"
              className={`g-catpick__opt ${catClass(c.color)}`}
              role="radio"
              aria-checked={c.id === value}
              tabIndex={c.id === focusable ? 0 : -1}
              onClick={() => onChange(c.id)}
              onKeyDown={(e) => key(e, i)}
            >
              <span className="g-catpick__dot" />
              {c.name}
            </button>
          ))}
        </div>
      )}
      {error && (
        <span className="g-field__error" id={`${idPrefix}-category_id-err`}>
          {error}
        </span>
      )}
    </div>
  );
}
