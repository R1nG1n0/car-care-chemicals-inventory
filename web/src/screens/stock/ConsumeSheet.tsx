// Шторка расхода (макет «Расход — шторка + отмена»): «Одна позиция / Шаблон»,
// позиция с «Сменить», форма расхода. Без позиции (/?consume=1) — сначала выбор.
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { inventory } from '../../api';
import { keys } from '../../app/queries';
import { ProductPicker } from '../../components/ProductPicker';
import { Sheet } from '../../components/Sheet';
import { useProducts, useStockRefs } from '../../components/StockRefs';
import { locationChip } from '../../lib/stock';
import { ConsumeForm } from './ConsumeForm';

type Props = {
  open: boolean;
  onClose: () => void;
  /** Позиция для расхода; null — сначала выбор позиции. */
  productId: number | null;
};

export function ConsumeSheet({ open, onClose, productId }: Props) {
  // Новое содержимое на каждое открытие (количество подставляется заново);
  // во время ухода шторка показывает то же, что было
  const [shown, setShown] = useState({ open, id: productId, n: 0 });
  if (open && (!shown.open || shown.id !== productId)) setShown({ open, id: productId, n: shown.n + 1 });
  else if (!open && shown.open) setShown({ ...shown, open: false });
  return (
    <Sheet open={open} onClose={onClose} label="Расход">
      <ConsumeBody key={shown.n} initial={shown.id} onClose={onClose} />
    </Sheet>
  );
}

function ConsumeBody({ initial, onClose }: { initial: number | null; onClose: () => void }) {
  const navigate = useNavigate();
  const refs = useStockRefs();
  const products = useProducts();
  const [id, setId] = useState(initial);
  const [picking, setPicking] = useState(initial === null);
  // Архивной позиции нет в общем списке — берём из карточки
  const detail = useQuery({
    queryKey: keys.product(id ?? 0),
    queryFn: ({ signal }) => inventory.product(id!, signal),
    enabled: id !== null,
  });
  const p = products.data?.find((x) => x.id === id) ?? detail.data?.product;

  return (
    <>
      <div className="g-seg" role="group" aria-label="Способ">
        <button type="button" aria-pressed="true">
          Одна позиция
        </button>
        <button
          type="button"
          aria-pressed="false"
          onClick={() => {
            onClose();
            navigate('/templates');
          }}
        >
          Шаблон
        </button>
      </div>
      {picking || !p ? (
        id !== null && !picking ? (
          <p className="g-muted">Загружаю позицию…</p>
        ) : (
          <ProductPicker
            id="consume-pick"
            label="Что списать"
            autoFocus
            onChange={(x) => {
              setId(x.id);
              setPicking(false);
            }}
            onCreate={(name) => {
              onClose();
              navigate(`/items/new?name=${encodeURIComponent(name)}`);
            }}
          />
        )
      ) : (
        <>
          <div className="stock-between">
            <div>
              <div className="g-card__name">{p.name}</div>
              <div className="g-row__sub">
                {[refs.categories.get(p.category_id)?.name, locationChip(p.location_ids, refs.locations)].filter(Boolean).join(' · ')}
              </div>
            </div>
            <button type="button" className="g-btn g-btn--sm g-btn--ghost" onClick={() => setPicking(true)}>
              Сменить
            </button>
          </div>
          <ConsumeForm key={p.id} product={p} idPrefix="consume" onDone={onClose} autoFocus />
        </>
      )}
    </>
  );
}
