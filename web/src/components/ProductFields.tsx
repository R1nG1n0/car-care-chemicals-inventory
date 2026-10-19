// Поля формы позиции — общие для формы /items/new и шторки «Новая позиция»:
// бренд с подсказкой из введённых ранее и полным списком по кнопке (.g-suggest) и выбор категории чипами
// с цветной точкой (.g-catpick, радиогруппа, стрелки). idPrefix разводит id,
// если на странице две формы.
import { useQuery } from '@tanstack/react-query';
import { IconCheck, IconChevronDown } from '@tabler/icons-react';
import { useEffect, useRef, useState, type KeyboardEvent } from 'react';
import { inventory, type Category } from '../api';
import { keys } from '../app/queries';
import { allBrands, suggestBrands } from '../lib/product';
import { Ico } from './ui';
import { catClass } from './refs';

export function BrandField({ value, onChange, error, idPrefix = 'np' }: { value: string; onChange: (v: string) => void; error?: string | undefined; idPrefix?: string }) {
  const brands = useQuery({ queryKey: keys.brands, queryFn: ({ signal }) => inventory.brands(signal), staleTime: 60_000 });
  // typing — подсказка по набранному (до 5), all — весь список по кнопке справа
  const [mode, setMode] = useState<'closed' | 'typing' | 'all'>('closed');
  const [active, setActive] = useState(0);
  const box = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const all = mode === 'all';
  const list = all ? allBrands(brands.data ?? []) : mode === 'typing' ? suggestBrands(brands.data ?? [], value) : [];
  const shown = list.length > 0 || all;
  const cur = Math.min(active, Math.max(list.length - 1, 0));
  const optId = (i: number) => `${idPrefix}-brand-${i}`;

  // Список, открытый кнопкой без фокуса в поле (телефон: без клавиатуры), закрывается касанием снаружи
  useEffect(() => {
    if (mode === 'closed') return;
    const outside = (e: PointerEvent) => {
      if (!box.current?.contains(e.target as Node)) setMode('closed');
    };
    document.addEventListener('pointerdown', outside);
    return () => document.removeEventListener('pointerdown', outside);
  }, [mode]);

  // Выбранная стрелками строка не уходит за край прокручиваемого списка
  useEffect(() => {
    if (all) document.getElementById(optId(cur))?.scrollIntoView({ block: 'nearest' });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [all, cur]);

  const pick = (b: string) => {
    onChange(b);
    setMode('closed');
  };
  const toggle = (byKeyboard: boolean) => {
    if (all) {
      setMode('closed');
      return;
    }
    const sorted = allBrands(brands.data ?? []);
    const at = sorted.findIndex((b) => b.toLocaleLowerCase('ru-RU') === value.trim().toLocaleLowerCase('ru-RU'));
    setActive(Math.max(at, 0));
    setMode('all');
    // С клавиатуры фокус уходит в поле, чтобы работали стрелки; касанием — нет (не открываем клавиатуру)
    if (byKeyboard) input.current?.focus();
  };
  const key = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape' && shown) {
      e.stopPropagation();
      setMode('closed');
      return;
    }
    if (e.key === 'ArrowDown' && !shown && e.altKey) {
      e.preventDefault();
      toggle(true);
      return;
    }
    if (!shown || list.length === 0) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive((cur + 1) % list.length);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive((cur - 1 + list.length) % list.length);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      pick(list[cur]!);
    }
  };
  const selected = (b: string) => b.toLocaleLowerCase('ru-RU') === value.trim().toLocaleLowerCase('ru-RU');

  return (
    <div className={`g-field stock-suggest${error ? ' g-field--error' : ''}`} ref={box}>
      <label className="g-label" htmlFor={`${idPrefix}-brand`}>
        Бренд <span className="g-field__opt">· необязательно</span>
      </label>
      <div className="g-brand">
        <input
          ref={input}
          className="g-input"
          id={`${idPrefix}-brand`}
          autoComplete="off"
          maxLength={60}
          role="combobox"
          aria-expanded={shown}
          aria-controls={`${idPrefix}-brand-list`}
          aria-autocomplete="list"
          aria-activedescendant={shown && list.length > 0 ? optId(cur) : undefined}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? `${idPrefix}-brand-err` : undefined}
          value={value}
          onChange={(e) => {
            onChange(e.target.value);
            setMode('typing');
            setActive(0);
          }}
          onFocus={() => setMode((m) => (m === 'all' ? m : 'typing'))}
          onBlur={(e) => {
            // Уход фокуса на кнопку списка не закрывает его — переключит сама кнопка
            if (!box.current?.contains(e.relatedTarget as Node | null)) setMode('closed');
          }}
          onKeyDown={key}
        />
        <button
          type="button"
          className="g-brand__toggle"
          aria-label={all ? 'Скрыть бренды' : 'Все бренды'}
          aria-expanded={all}
          aria-controls={`${idPrefix}-brand-list`}
          // mousedown не забирает фокус у поля: иначе оно закроет список раньше нажатия
          onMouseDown={(e) => e.preventDefault()}
          onClick={(e) => toggle(e.detail === 0)}
        >
          <Ico icon={IconChevronDown} />
        </button>
      </div>
      {shown && (
        <ul className={`g-suggest${all ? ' g-suggest--all' : ''}`} id={`${idPrefix}-brand-list`} role="listbox" aria-label={all ? 'Все бренды' : 'Введённые ранее'}>
          {list.length === 0 && (
            <li className="g-suggest__empty" role="presentation">
              {brands.isPending ? 'Загружаю бренды…' : 'Брендов пока нет — впишите первый, он появится здесь.'}
            </li>
          )}
          {list.map((b, i) => (
            <li
              key={b}
              id={optId(i)}
              className="g-suggest__opt"
              role="option"
              aria-selected={i === cur}
              // mousedown, а не click: иначе поле теряет фокус и список закрывается раньше
              onMouseDown={(e) => {
                e.preventDefault();
                pick(b);
              }}
            >
              {b}
              {all && selected(b) && <Ico icon={IconCheck} size={18} />}
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
