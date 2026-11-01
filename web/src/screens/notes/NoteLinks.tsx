// Привязки заметки (ФТ-18.1): выбранные позиции и шаблоны — чипы с «убрать», выбор поиском.
// В режиме просмотра чипы — ссылки на позицию или шаблон.
import { IconSearch, IconX } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useId, useState } from 'react';
import { Link } from 'react-router';
import { inventory } from '../../api';
import { keys } from '../../app/queries';
import { useProducts } from '../../components/refs';
import { Ico } from '../../components/ui';
import { toggleId } from '../../lib/purposes';
import { matchProduct, normalize } from '../../lib/stock';

const SHOWN = 6;

type Props = { products: number[]; recipes: number[]; onProducts: (v: number[]) => void; onRecipes: (v: number[]) => void };

/** Названия для чипов: позиции и шаблоны из кэша; нет в списке (архив, удалено) — «Позиция № …». */
export function useLinkNames() {
  const products = useProducts();
  const recipes = useQuery({ queryKey: keys.recipes, queryFn: ({ signal }) => inventory.recipes(signal) });
  const p = new Map((products.data ?? []).map((x) => [x.id, x.name]));
  const r = new Map((recipes.data ?? []).map((x) => [x.id, x.name]));
  return {
    product: (id: number) => p.get(id) ?? `Позиция № ${id}`,
    recipe: (id: number) => r.get(id) ?? `Шаблон № ${id}`,
    products: products.data ?? [],
    recipes: recipes.data ?? [],
  };
}

/** Чипы привязок для просмотра: каждая — ссылка. */
export function LinkChips({ products, recipes }: { products: readonly number[]; recipes: readonly number[] }) {
  const names = useLinkNames();
  if (products.length + recipes.length === 0) return null;
  return (
    <ul className="note-chips" aria-label="Привязки">
      {products.map((id) => (
        <li key={`p${id}`}>
          <Link className="g-chip note-chip" to={`/items/${id}`}>
            {names.product(id)}
          </Link>
        </li>
      ))}
      {recipes.map((id) => (
        <li key={`r${id}`}>
          <Link className="g-chip note-chip" to={`/templates/${id}`}>
            Шаблон: {names.recipe(id)}
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function NoteLinks({ products, recipes, onProducts, onRecipes }: Props) {
  const names = useLinkNames();
  const [q, setQ] = useState('');
  const listId = useId();
  const typed = normalize(q);
  const foundP = typed ? names.products.filter((p) => !products.includes(p.id) && matchProduct(p, q)).slice(0, SHOWN) : [];
  const foundR = typed ? names.recipes.filter((r) => !recipes.includes(r.id) && normalize(r.name).includes(typed)).slice(0, SHOWN) : [];

  return (
    <div className="g-field note-links">
      <label className="g-label" htmlFor={`${listId}-q`}>
        Привязать к позициям и шаблонам <span className="g-field__opt">· необязательно</span>
      </label>
      {products.length + recipes.length > 0 && (
        <ul className="note-chips" aria-label="Выбрано">
          {products.map((id) => (
            <li key={`p${id}`}>
              <button type="button" className="g-chip note-chip note-chip--x g-press" aria-label={`Убрать привязку: ${names.product(id)}`} onClick={() => onProducts(toggleId(products, id))}>
                {names.product(id)} <Ico icon={IconX} size={14} />
              </button>
            </li>
          ))}
          {recipes.map((id) => (
            <li key={`r${id}`}>
              <button type="button" className="g-chip note-chip note-chip--x g-press" aria-label={`Убрать привязку: шаблон ${names.recipe(id)}`} onClick={() => onRecipes(toggleId(recipes, id))}>
                Шаблон: {names.recipe(id)} <Ico icon={IconX} size={14} />
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="note-find">
        <Ico icon={IconSearch} size={18} />
        <input
          id={`${listId}-q`}
          className="g-input"
          type="search"
          autoComplete="off"
          placeholder="Название позиции или шаблона"
          value={q}
          aria-controls={listId}
          onChange={(e) => setQ(e.target.value)}
        />
      </div>
      <div id={listId} className="g-list" hidden={typed === ''}>
        {typed !== '' && foundP.length + foundR.length === 0 && <p className="g-field__hint">Ничего не найдено</p>}
        {foundP.map((p) => (
          <button
            key={`p${p.id}`}
            type="button"
            className="g-row g-row--link app-row-btn"
            onClick={() => {
              onProducts(toggleId(products, p.id));
              setQ('');
            }}
          >
            <span className="g-row__main g-row__title">{p.name}</span>
            <span className="g-row__sub">{p.brand}</span>
          </button>
        ))}
        {foundR.map((r) => (
          <button
            key={`r${r.id}`}
            type="button"
            className="g-row g-row--link app-row-btn"
            onClick={() => {
              onRecipes(toggleId(recipes, r.id));
              setQ('');
            }}
          >
            <span className="g-row__main g-row__title">{r.name}</span>
            <span className="g-row__sub">шаблон</span>
          </button>
        ))}
      </div>
    </div>
  );
}
