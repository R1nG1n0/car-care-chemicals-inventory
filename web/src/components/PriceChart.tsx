// История цены позиции (ФТ-10.2.4): маленький график цены за 100 мл / за шт
// по покупкам. Рисуется SVG без библиотек; строится от двух покупок.
import type { PurchaseRow, Unit } from '../api';
import { chartGeometry, priceChangeText, priceRangeText, priceSeries } from '../lib/reports';
import { purchaseDay } from '../lib/text';

const W = 320;
const H = 96;

export function PriceChart({ rows, unit }: { rows: readonly PurchaseRow[]; unit: Unit }) {
  const series = priceSeries(rows, unit);
  const values = series.map((p) => p.value);
  const chart = chartGeometry(values, W, H);
  if (!chart) return null;
  const first = series[0];
  const last = series[series.length - 1];
  const summary = `${priceRangeText(values, unit)}; ${priceChangeText(values)}`;
  return (
    <section className="rep-price" aria-labelledby="price-chart-h">
      <h2 className="g-label" id="price-chart-h">
        Цена {unit === 'pcs' ? 'за шт' : 'за 100 мл'}
      </h2>
      <div className="g-card rep-price__card">
        <svg className="rep-price__svg" viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`График цены: ${summary}`}>
          <path className="rep-price__grid" d={`M0 ${H - 0.5} H${W}`} />
          <path className="rep-price__line" d={chart.path} />
          {chart.points.map((p, i) => (
            <circle key={i} className={`rep-price__dot${i === chart.points.length - 1 ? ' rep-price__dot--last' : ''}`} cx={p.x} cy={p.y} r="4.5" />
          ))}
        </svg>
        <div className="rep-price__axis">
          <span>{first ? purchaseDay(first.date) : ''}</span>
          <span>{last ? purchaseDay(last.date) : ''}</span>
        </div>
        <p className="rep-price__sum">
          <b>{priceRangeText(values, unit)}</b>
          <span className="g-muted"> · {priceChangeText(values)}</span>
        </p>
      </div>
    </section>
  );
}
