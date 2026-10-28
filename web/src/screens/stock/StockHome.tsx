// Остатки — главная (макеты «Остатки — главная», «Остатки — компьютер», состояния):
// заголовок с плашкой «N мало», поиск по загруженному списку, фильтры-чипы, карточки.
// Свайп влево — расход, нажатие — карточка позиции. На ширине от 1024 px нажатие
// выбирает карточку, справа панель позиции с расходом. /?consume=1 — расход с выбором
// позиции (меню «+»), /?location=ID и /?filter=low|out — начальный фильтр.
// Кнопка сканера в строке поиска — ScanFlow (ФТ-9.2).
import { IconPackage, IconPlus, IconScan, IconSearch, IconSearchOff, IconX } from '@tabler/icons-react';
import { useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { DESKTOP, useMedia } from '../../components/hooks';
import { catClass, useProducts, useStockRefs } from '../../components/refs';
import { Empty, Ico, LoadError } from '../../components/ui';
import { parseFilter, parsePurposeParam, stockCounts, visibleProducts, type StockFilter } from '../../lib/stock';
import { ConsumeSheet } from './ConsumeSheet';
import { ScanFlow } from './ScanFlow';
import { StockList } from './StockList';
import { StockPanel } from './StockPanel';

export function StockHome() {
  const [params, setParams] = useSearchParams();
  const desktop = useMedia(DESKTOP);
  const products = useProducts();
  const refs = useStockRefs();
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState<StockFilter>(() =>
    params.get('location') ? parseFilter(`loc:${params.get('location')}`) : parseFilter(params.get('filter')),
  );
  const [purposeId, setPurposeId] = useState<number | null>(() => parsePurposeParam(params.get('purpose')));
  const [selected, setSelected] = useState<number | null>(null);
  const [swiped, setSwiped] = useState<number | null>(null);
  const [scanning, setScanning] = useState(false);

  const all = useMemo(() => products.data ?? [], [products.data]);
  const purposeNames = useMemo(() => new Map(refs.purposeList.map((p) => [p.id, p.name])), [refs.purposeList]);
  const items = useMemo(
    () => visibleProducts(all, q, filter, { id: purposeId, names: purposeNames }),
    [all, q, filter, purposeId, purposeNames],
  );
  const counts = stockCounts(all);
  const picking = params.get('consume') === '1';
  const sheetOpen = picking || swiped !== null;
  const current = desktop ? (all.find((p) => p.id === selected) ?? items[0] ?? null) : null;

  const closeSheet = () => {
    setSwiped(null);
    if (picking) {
      const next = new URLSearchParams(params);
      next.delete('consume');
      setParams(next, { replace: true });
    }
  };

  const places = refs.locationList.filter((l) => !l.archived && all.some((p) => p.location_ids.includes(l.id)));
  const cats = refs.categoryList.filter((c) => all.some((p) => p.category_id === c.id));
  // Назначения: только те, что есть у позиций; выбранное остаётся в ряду, даже если позиций не осталось
  const purposes = refs.purposeList.filter((x) => x.id === purposeId || (!x.archived && all.some((p) => p.purpose_ids?.includes(x.id))));
  const chip = (f: StockFilter, label: string, cls?: string, dot?: boolean) => (
    <button key={f} type="button" className={`g-filter${cls ? ` ${cls}` : ''}`} aria-pressed={filter === f} onClick={() => setFilter(f)}>
      {dot && <span className="g-catpick__dot stock-filters__dot" />}
      {label}
    </button>
  );

  let body;
  if (products.isPending) {
    body = <Skeleton />;
  } else if (products.isError && !products.data) {
    body = <LoadError id="stock-err" error={products.error} what="Остатки" onRetry={() => void products.refetch()} />;
  } else if (all.length === 0) {
    body = (
      <Empty
        id="stock-first"
        icon={IconPackage}
        title="Здесь будут остатки"
        text="Добавьте первую покупку — остатки появятся сами."
        actions={
          <>
            <Link className="g-btn g-btn--block g-press" to="/purchases/new">
              <Ico icon={IconPlus} /> Добавить покупку
            </Link>
            <Link className="g-btn g-btn--block g-press" to="/items/new">
              Создать позицию без покупки
            </Link>
          </>
        }
      />
    );
  } else {
    body = (
      <>
        <div className="g-search stock-search scan-search" role="search">
          <span className="g-search__icon">
            <Ico icon={IconSearch} />
          </span>
          <input
            className="g-input"
            type="search"
            placeholder="Шампунь, Koch, штрихкод"
            aria-label="Поиск по остаткам"
            autoComplete="off"
            enterKeyHint="search"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          {q && (
            <button type="button" className="g-btn g-btn--icon g-btn--ghost stock-search__clear" aria-label="Очистить поиск" onClick={() => setQ('')}>
              <Ico icon={IconX} />
            </button>
          )}
          <button type="button" className="g-btn g-btn--icon g-btn--ghost scan-search__btn" aria-label="Сканировать штрихкод" onClick={() => setScanning(true)}>
            <Ico icon={IconScan} />
          </button>
        </div>
        <div className="g-filters" role="group" aria-label="Фильтр">
          {chip('all', `Все\u00a0${counts.all}`)}
          {chip('low', `Мало\u00a0${counts.low}`)}
          {chip('out', 'Нет в наличии')}
          {places.map((l) => chip(`loc:${l.id}`, l.name))}
          {cats.map((c) => chip(`cat:${c.id}`, c.name, catClass(c.color), true))}
        </div>
        {purposes.length > 0 && (
          <div className="g-filters" role="group" aria-label="Назначение">
            {purposes.map((x) => (
              <button key={x.id} type="button" className="g-filter" aria-pressed={purposeId === x.id} onClick={() => setPurposeId(purposeId === x.id ? null : x.id)}>
                {x.name}
              </button>
            ))}
          </div>
        )}
        {items.length === 0 && (
          <NotFound
            q={q.trim()}
            onAll={() => {
              setFilter('all');
              setPurposeId(null);
            }}
          />
        )}
        <StockList
          items={items}
          selectedId={current?.id}
          onSwipe={(p) => setSwiped(p.id)}
          onCardClick={
            desktop
              ? (p, e) => {
                  e.preventDefault();
                  setSelected(p.id);
                }
              : undefined
          }
        />
      </>
    );
  }

  return (
    <div className={`stock-home${current ? ' stock-home--split' : ''}`}>
      <div className="stock-main">
        <div className="stock-between">
          <h1 className="g-title">Остатки</h1>
          {counts.low > 0 && <span className="g-badge g-badge--danger">{counts.low}&nbsp;мало</span>}
        </div>
        {body}
      </div>
      {current && <StockPanel product={current} />}
      <ConsumeSheet open={sheetOpen} onClose={closeSheet} productId={swiped} />
      <ScanFlow open={scanning} onClose={() => setScanning(false)} />
    </div>
  );
}

function NotFound({ q, onAll }: { q: string; onAll: () => void }) {
  if (q === '') {
    return (
      <Empty
        id="stock-none"
        icon={IconSearchOff}
        title="Ничего не нашлось"
        text="В этом фильтре пока пусто."
        actions={
          <button type="button" className="g-btn g-btn--block g-press" onClick={onAll}>
            Показать все
          </button>
        }
      />
    );
  }
  return (
    <Empty
      id="stock-none"
      icon={IconSearchOff}
      title="Ничего не нашлось"
      text={`Позиции «${q}» нет в каталоге. Проверьте написание или создайте её.`}
      actions={
        <Link className="g-btn g-btn--block g-press" to={`/items/new?name=${encodeURIComponent(q)}`}>
          Создать позицию «{q}»
        </Link>
      }
    />
  );
}

/** Загрузка: подложки по форме карточек, список помечен aria-busy. */
function Skeleton() {
  return (
    <>
      <div className="stock-skel-chips" aria-hidden="true">
        <span className="g-skel__chip" />
        <span className="g-skel__chip" />
        <span className="g-skel__chip" />
      </div>
      <div className="stock-list" aria-busy="true" aria-label="Остатки загружаются">
        {[58, 44, 66, 50].map((w) => (
          <div key={w} className="g-skel" aria-hidden="true">
            <div className="g-skel__strip" />
            <div className="g-skel__body">
              <div className="g-skel__line" style={{ width: `${w}%` }} />
              <div className="g-skel__num" />
              <div className="g-skel__bar" />
              <div className="g-skel__line g-skel__line--short" />
            </div>
          </div>
        ))}
      </div>
    </>
  );
}
