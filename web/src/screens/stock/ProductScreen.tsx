// Карточка позиции /items/:id (макет «Карточка позиции», ФТ-3.2): шапка категории,
// крупный общий остаток, «вскрыта · запечатаны · мин.», горчичная «Расход», «Покупка»,
// экземпляры с действиями, ввод имеющегося остатка, покупки и журнал, меню «⋮».
import { IconArchive, IconArchiveOff, IconDotsVertical, IconPencil, IconPlus, IconSearchOff } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type ReactNode } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { errorText, inventory, isApiError, type ProductDetail, type ProductSummary, type StockUnit } from '../../api';
import { invalidateRefs, invalidateStock, keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { Sheet } from '../../components/Sheet';
import { BackLink } from '../../components/BackLink';
import { catClassOf, useStockRefs } from '../../components/refs';
import { Empty, Ico, LoadError } from '../../components/ui';
import { formatQty } from '../../lib/format';
import { productMeta, stickerText, unitLabel } from '../../lib/stock';
import { AddUnitSheet } from './AddUnitSheet';
import { ConsumeSheet } from './ConsumeSheet';
import { ProductHistory } from './ProductHistory';
import { ProductPhoto } from './ProductPhoto';
import { UnitRows } from './UnitRows';
import { UnitSheet } from './UnitSheet';

export function ProductScreen() {
  const id = Number(useParams().id);
  const detail = useQuery({
    queryKey: keys.product(id),
    queryFn: ({ signal }) => inventory.product(id, signal),
    enabled: Number.isInteger(id) && id > 0,
  });

  const back = <BackLink to="/" label="Остатки" />;

  if (detail.isPending && detail.fetchStatus !== 'idle') {
    return (
      <div className="stock-item" aria-busy="true">
        <div className="stock-item__bar">{back}</div>
        <div className="g-skel" aria-hidden="true">
          <div className="g-skel__strip" />
          <div className="g-skel__body">
            <div className="g-skel__line" />
            <div className="g-skel__num" />
            <div className="g-skel__line g-skel__line--short" />
          </div>
        </div>
      </div>
    );
  }
  if (!detail.data) {
    const fail: unknown = detail.error;
    const missing = !fail || isApiError(fail, 404);
    return (
      <div className="stock-item">
        <div className="stock-item__bar">{back}</div>
        {missing ? (
          <Empty id="item-err" icon={IconSearchOff} title="Позиция не найдена" text="Возможно, её адрес набран с ошибкой." />
        ) : (
          <LoadError id="item-err" error={fail} what="Позиция" onRetry={() => void detail.refetch()} />
        )}
      </div>
    );
  }
  return <ProductView detail={detail.data} back={back} />;
}

function ProductView({ detail, back }: { detail: ProductDetail; back: ReactNode }) {
  const { product: p, units } = detail;
  const refs = useStockRefs();
  const [consume, setConsume] = useState(false);
  const [unit, setUnit] = useState<StockUnit | null>(null);
  const [adding, setAdding] = useState(false);
  const [menu, setMenu] = useState(false);
  const cat = refs.categories.get(p.category_id);
  const cc = catClassOf(refs.categories, p);
  const head = [cat?.name, p.brand].filter(Boolean).join(' · ');
  const lastPlace = units.find((u) => u.location_id !== null)?.location_id ?? null;

  return (
    <div className="stock-item">
      <div className="stock-item__bar">
        {back}
        <button type="button" className="g-btn g-btn--icon g-btn--ghost" aria-label="Меню позиции: штрихкод, изменить, архив" onClick={() => setMenu(true)}>
          <Ico icon={IconDotsVertical} />
        </button>
      </div>

      <div className={`g-card ${cc}`}>
        <div className="g-card__head">
          <span>{head || 'Без категории'}</span>
          {p.archived && <span className="g-badge">в архиве</span>}
        </div>
        <div className="g-card__body">
          <h1 className="g-card__name stock-name-lg">
            {p.name}, {formatQty(p.package_size)} {unitLabel(p.unit)}
          </h1>
          <div className="stock-big">
            <span className="g-num">{formatQty(p.stock.total)}</span>
            <b>{unitLabel(p.unit)}</b>
          </div>
          <div className="stock-meta">{productMeta(p)}</div>
          {stickerText(p) && <span className="g-sticker">{stickerText(p)}</span>}
        </div>
      </div>

      <div className="app-two">
        <button
          type="button"
          className="g-btn g-btn--cta g-press"
          aria-disabled={p.stock.total === 0 || undefined}
          aria-describedby={p.stock.total === 0 ? 'item-empty-hint' : undefined}
          onClick={() => p.stock.total > 0 && setConsume(true)}
        >
          Расход
        </button>
        <Link className="g-btn g-press" to={`/purchases/new?product=${p.id}`}>
          Покупка
        </Link>
      </div>
      {p.stock.total === 0 && (
        <p className="g-field__hint" id="item-empty-hint">
          Нет в наличии: добавьте покупку или введите имеющийся остаток.
        </p>
      )}
      {p.note && <p className="g-muted">{p.note}</p>}

      <ProductPhoto product={p} />

      <h2 className="g-label">Экземпляры</h2>
      {units.length > 0 ? (
        <UnitRows units={units} unit={p.unit} catClass={cc} locations={refs.locations} onPick={setUnit} />
      ) : (
        <p className="g-muted">Экземпляров нет.</p>
      )}
      <button type="button" className="g-btn g-btn--block g-press" onClick={() => setAdding(true)}>
        <Ico icon={IconPlus} /> Ввести имеющийся остаток
      </button>

      <ProductHistory detail={detail} />

      <ConsumeSheet open={consume} onClose={() => setConsume(false)} productId={p.id} />
      <UnitSheet unit={unit} product={p} onClose={() => setUnit(null)} />
      <AddUnitSheet open={adding} product={p} onClose={() => setAdding(false)} defaultLocation={lastPlace} />
      <ProductMenu open={menu} product={p} onClose={() => setMenu(false)} />
    </div>
  );
}

/** Меню «⋮»: штрихкод, изменить, в архив / из архива (с «Отменить»). */
function ProductMenu({ open, product: p, onClose }: { open: boolean; product: ProductSummary; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const navigate = useNavigate();
  const archive = useMutation({
    mutationFn: (archived: boolean) => inventory.updateProduct(p.id, { archived }),
    onSuccess: async (r) => {
      onClose();
      await Promise.all([invalidateRefs(qc), invalidateStock(qc)]);
      toast.show({
        text: r.archived ? `В архиве: ${r.name}` : `Возвращено из архива: ${r.name}`,
        action: { label: 'Отменить', onClick: () => archive.mutate(!r.archived) },
      });
    },
    onError: (err) => toast.show({ text: errorText(err) }),
  });
  return (
    <Sheet open={open} onClose={onClose} title={p.name}>
      <div className="g-list">
        <div className="g-row">
          <div className="g-row__main">
            <div className="g-row__sub">Штрихкод</div>
            <div className="g-row__title">{p.barcode || 'не указан'}</div>
          </div>
        </div>
        <button type="button" className="g-row g-row--link app-row-btn" onClick={() => (onClose(), navigate(`/items/${p.id}/edit`))}>
          <Ico icon={IconPencil} />
          <span className="g-row__main g-row__title">Изменить</span>
        </button>
        <button type="button" className="g-row g-row--link app-row-btn" onClick={() => archive.mutate(!p.archived)} aria-busy={archive.isPending || undefined}>
          <Ico icon={p.archived ? IconArchiveOff : IconArchive} />
          <span className="g-row__main g-row__title">{p.archived ? 'Вернуть из архива' : 'В архив — больше не покупаю'}</span>
        </button>
      </div>
    </Sheet>
  );
}
