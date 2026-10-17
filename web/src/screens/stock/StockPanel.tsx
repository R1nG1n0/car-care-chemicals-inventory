// Панель выбранной позиции на компьютере (макет «Остатки — компьютер»): остаток,
// экземпляры, последняя покупка и расход. Горчичная кнопка на экране одна — «Списать».
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { inventory, type ProductSummary } from '../../api';
import { keys } from '../../app/queries';
import { catVar, useStockRefs } from '../../components/StockRefs';
import { formatMoney, formatQty } from '../../lib/format';
import { openLine, unitLabel } from '../../lib/stock';
import { purchaseDay } from '../../lib/stockMoney';
import { ConsumeForm } from './ConsumeForm';
import { UnitRows } from './UnitRows';

export function StockPanel({ product: p }: { product: ProductSummary }) {
  const refs = useStockRefs();
  const detail = useQuery({ queryKey: keys.product(p.id), queryFn: ({ signal }) => inventory.product(p.id, signal) });
  const last = detail.data?.purchases[0];

  return (
    <aside className="stock-panel" aria-label={`Выбрано: ${p.name}`}>
      <div className="stock-between">
        <span className="g-label">Выбрано</span>
        <Link className="g-btn g-btn--sm g-btn--ghost" to={`/items/${p.id}`}>
          Открыть карточку
        </Link>
      </div>
      <div className="g-card__name stock-name-lg">{p.name}</div>
      <div className="stock-big stock-big--md">
        <span className="g-num">{formatQty(p.stock.total)}</span>
        <b>{unitLabel(p.unit)}</b>
      </div>
      <div className="g-row__sub">{openLine(p.stock, p.unit)}</div>
      {detail.data && detail.data.units.length > 0 && (
        <UnitRows units={detail.data.units} unit={p.unit} catColor={catVar(refs.categories, p)} locations={refs.locations} />
      )}
      {last && (
        <div className="g-row__sub">
          Последняя покупка: {purchaseDay(last.date)}
          {last.store ? ` · ${last.store}` : ''} · {formatMoney(last.price_per_package)}
        </div>
      )}
      <div className="stock-gap">
        <ConsumeForm key={p.id} product={p} idPrefix="panel" />
      </div>
    </aside>
  );
}
