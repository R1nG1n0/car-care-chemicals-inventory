// Поиск и выбор позиции: поле поиска (мгновенно, по загруженному списку),
// строки-кнопки с цветом категории и остатком, внизу — «Создать позицию «…»».
// С value — показывает выбранную позицию и «Сменить».
import { IconCirclePlus, IconSearch, IconX } from '@tabler/icons-react';
import { useState, type CSSProperties } from 'react';
import type { ProductSummary } from '../api';
import { matchProduct, normalize, qtyText } from '../lib/stock';
import { Ico } from './ui';
import { catVar, useProducts, useStockRefs } from './StockRefs';

export type ProductPickerProps = {
  /** id поля поиска — для связи с подписью снаружи (htmlFor). */
  id: string;
  /** Выбранная позиция (id) — показывается строкой с «Сменить»; null/undefined — сразу поиск. */
  value?: number | null;
  onChange: (p: ProductSummary) => void;
  /** Не показывать эти позиции (уже есть в покупке или шаблоне). */
  excludeIds?: readonly number[];
  /** Пункт «Создать позицию «…»» с набранным текстом; без обработчика пункта нет. */
  onCreate?: (name: string) => void;
  /** Подпись над полем; пустая строка — без подписи. */
  label?: string;
  placeholder?: string;
  autoFocus?: boolean;
  /** Сколько строк показывать, по умолчанию 30. */
  limit?: number;
};

export function ProductPicker({
  id,
  value,
  onChange,
  excludeIds,
  onCreate,
  label = 'Позиция',
  placeholder = 'Название, бренд, штрихкод',
  autoFocus,
  limit = 30,
}: ProductPickerProps) {
  const products = useProducts();
  const refs = useStockRefs();
  const [q, setQ] = useState('');
  const [editing, setEditing] = useState(false);
  const all = products.data ?? [];
  const selected = value != null ? all.find((p) => p.id === value) : undefined;

  if (selected && !editing) {
    return (
      <div className="g-field">
        {label && <span className="g-label">{label}</span>}
        <div className="stock-picked" style={{ '--cat': catVar(refs.categories, selected) } as CSSProperties}>
          <span className="g-cat-sq" />
          <div className="g-row__main">
            <div className="g-row__title">{selected.name}</div>
            <div className="g-row__sub">{subLine(selected, refs.categories.get(selected.category_id)?.name)}</div>
          </div>
          <button type="button" className="g-btn g-btn--sm g-btn--ghost" onClick={() => setEditing(true)} aria-label={`Сменить позицию ${selected.name}`}>
            Сменить
          </button>
        </div>
      </div>
    );
  }

  const skip = new Set(excludeIds ?? []);
  const found = all.filter((p) => !skip.has(p.id) && matchProduct(p, q));
  const shown = found.slice(0, limit);
  const typed = q.trim();
  const exact = typed !== '' && all.some((p) => normalize(p.name) === normalize(typed));
  const listId = `${id}-list`;

  const pick = (p: ProductSummary) => {
    setEditing(false);
    setQ('');
    onChange(p);
  };

  return (
    <div className="g-field stock-picker">
      {label && (
        <label className="g-label" htmlFor={id}>
          {label}
        </label>
      )}
      <div className="g-search stock-search" role="search">
        <span className="g-search__icon">
          <Ico icon={IconSearch} />
        </span>
        <input
          id={id}
          className="g-input"
          type="search"
          autoComplete="off"
          enterKeyHint="search"
          placeholder={placeholder}
          value={q}
          aria-controls={listId}
          data-autofocus={autoFocus || undefined}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && shown.length === 1) {
              e.preventDefault();
              pick(shown[0]!);
            }
          }}
        />
        {q && (
          <button type="button" className="g-btn g-btn--icon g-btn--ghost stock-search__clear" aria-label="Очистить поиск" onClick={() => setQ('')}>
            <Ico icon={IconX} />
          </button>
        )}
      </div>
      <div className="g-list stock-picker__list" id={listId} aria-busy={products.isPending || undefined}>
        {products.isPending && <div className="g-row g-row__sub">Загружаю позиции…</div>}
        {shown.map((p) => (
          <button key={p.id} type="button" className="g-row g-row--link app-row-btn" style={{ '--cat': catVar(refs.categories, p) } as CSSProperties} onClick={() => pick(p)}>
            <span className="g-cat-sq" />
            <span className="g-row__main">
              <span className="g-row__title stock-block">{p.name}</span>
              <span className="g-row__sub stock-block">{subLine(p, refs.categories.get(p.category_id)?.name)}</span>
            </span>
            {p.low && <span className="g-badge g-badge--danger">мало</span>}
          </button>
        ))}
        {found.length > shown.length && <div className="g-row g-row__sub">Ещё {found.length - shown.length} — уточните поиск</div>}
        {!products.isPending && shown.length === 0 && !onCreate && <div className="g-row g-row__sub">Ничего не нашлось</div>}
        {onCreate && typed !== '' && !exact && (
          <button type="button" className="g-row g-row--link app-row-btn" onClick={() => onCreate(typed)}>
            <Ico icon={IconCirclePlus} />
            <span className="g-row__main g-row__title">Создать позицию «{typed}»</span>
          </button>
        )}
      </div>
    </div>
  );
}

function subLine(p: ProductSummary, category: string | undefined): string {
  return [category, p.brand, p.stock.total > 0 ? qtyText(p.stock.total, p.unit) : 'нет в наличии'].filter(Boolean).join(' · ');
}
