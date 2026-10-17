// История покупок (постранично, новые сверху) и состав одной покупки (ФТ-4).
import { IconChevronRight, IconReceipt } from '@tabler/icons-react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Link, useParams } from 'react-router';
import { inventory, isApiError, type PurchaseShort } from '../../api';
import { keys } from '../../app/queries';
import { ShopBack, ShopLoadError, ShopSkelList } from '../../components/ShopUi';
import { busyProps, Empty, Ico } from '../../components/ui';
import { formatMoney } from '../../lib/format';
import { purchaseDay } from '../../lib/stockMoney';
import { plural } from '../../lib/text';

const PAGE = 50;
const POSITIONS = ['позиция', 'позиции', 'позиций'] as const;
const PACKAGES = ['упаковка', 'упаковки', 'упаковок'] as const;

export function PurchasesHistory() {
  const q = useInfiniteQuery({
    // Первый элемент ключа — 'purchases': invalidateStock обновляет и историю
    queryKey: [...keys.purchases, 'pages'],
    queryFn: ({ pageParam, signal }) => inventory.purchases(pageParam, signal),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last: PurchaseShort[]) => (last.length >= PAGE ? last[last.length - 1]!.id : undefined),
  });
  const rows = q.data?.pages.flat() ?? [];

  return (
    <div className="app-stack app-stack--fill">
      <div className="app-stack">
        <ShopBack to="/shopping" label="Купить" />
        <h1 className="g-title">Покупки</h1>
      </div>
      {q.isPending && <ShopSkelList label="Покупки загружаются" rows={5} />}
      {q.isError && !q.data && <ShopLoadError id="pur-err" error={q.error} what="Покупки" onRetry={() => void q.refetch()} />}
      {q.data && rows.length === 0 && (
        <Empty
          id="pur-empty"
          icon={IconReceipt}
          title="Покупок пока нет"
          text="Добавьте первую покупку — остатки появятся сами."
          actions={
            <Link to="/purchases/new" className="g-btn g-btn--block g-press">
              Новая покупка
            </Link>
          }
        />
      )}
      {rows.length > 0 && (
        <div className="app-stack">
          <div className="g-list">
            {rows.map((p) => (
              <Link key={p.id} to={`/purchases/${p.id}`} className="g-row g-row--link">
                <span className="g-row__main">
                  <span className="g-row__title shop-block">
                    {purchaseDay(p.date)}
                    {p.store ? ` · ${p.store}` : ''}
                  </span>
                  <span className="g-row__sub shop-block">
                    {p.lines_count} {plural(p.lines_count, POSITIONS)}
                  </span>
                </span>
                <span className="shop-num">{formatMoney(p.total)}</span>
                <span className="g-row__go">
                  <Ico icon={IconChevronRight} />
                </span>
              </Link>
            ))}
          </div>
          {q.hasNextPage && (
            <button type="button" className="g-btn g-btn--block g-press" onClick={() => !q.isFetchingNextPage && void q.fetchNextPage()} {...busyProps(q.isFetchingNextPage)}>
              {q.isFetchingNextPage ? 'Загружаю…' : 'Показать ещё'}
            </button>
          )}
        </div>
      )}
    </div>
  );
}

export function PurchaseView() {
  const id = Number(useParams().id);
  const q = useQuery({ queryKey: keys.purchase(id), queryFn: ({ signal }) => inventory.purchase(id, signal), enabled: Number.isInteger(id) && id > 0 });
  const p = q.data;
  const packages = p?.lines.reduce((s, l) => s + l.packages, 0) ?? 0;

  return (
    <div className="app-stack app-stack--fill">
      <div className="app-stack">
        <ShopBack to="/purchases" label="Покупки" />
        <h1 className="g-title">{p ? `${purchaseDay(p.date)}${p.store ? ` · ${p.store}` : ''}` : 'Покупка'}</h1>
      </div>
      {q.isPending && <ShopSkelList label="Покупка загружается" rows={3} />}
      {q.isError && isApiError(q.error, 404) && <Empty id="pv-404" icon={IconReceipt} title="Покупка не найдена" text="Возможно, ссылка устарела." />}
      {q.isError && !isApiError(q.error, 404) && <ShopLoadError id="pv-err" error={q.error} what="Покупка" onRetry={() => void q.refetch()} />}
      {p && (
        <div className="app-stack">
          <div className="g-list">
            {p.lines.map((l) => (
              <Link key={l.id} to={`/items/${l.product_id}`} className="g-row g-row--link">
                <span className="g-row__main">
                  <span className="g-row__title shop-block">{l.product}</span>
                  <span className="g-row__sub shop-block">
                    {l.packages} {plural(l.packages, PACKAGES)} × {formatMoney(l.price_per_package)}
                  </span>
                </span>
                <span className="shop-num">{formatMoney(l.price_per_package * l.packages)}</span>
                <span className="g-row__go">
                  <Ico icon={IconChevronRight} />
                </span>
              </Link>
            ))}
          </div>
          <div className="shop-between">
            <span className="g-muted">
              Итого · {packages} {plural(packages, PACKAGES)}
            </span>
            <span className="shop-sum">{formatMoney(p.total)}</span>
          </div>
          {p.note && <p className="g-muted">{p.note}</p>}
          <p className="shop-hint">Покупка целиком не отменяется. Ошибку исправьте корректировкой или списанием в карточке позиции.</p>
        </div>
      )}
    </div>
  );
}
