// История на экране позиции (ФТ-3.2): покупки (дата, магазин, цена за 100 мл / шт)
// и журнал движений; расход, списание и корректировку можно отменить с подтверждением.
import { useState } from 'react';
import type { Movement, ProductDetail } from '../../api';
import { PriceChart } from '../../components/PriceChart';
import { Sheet } from '../../components/Sheet';
import { useStockRefs } from '../../components/refs';
import { busyProps } from '../../components/ui';
import { formatMoney } from '../../lib/format';
import { unitPriceText } from '../../lib/money';
import { canRevert, movementTitle, revertQuestion } from '../../lib/movements';
import { formatDay, formatTime, purchaseDay } from '../../lib/text';
import { useRevert } from '../../app/useRevert';

export function ProductHistory({ detail }: { detail: ProductDetail }) {
  const { product: p, purchases, movements } = detail;
  const refs = useStockRefs();
  const revert = useRevert();
  const [ask, setAsk] = useState<Movement | null>(null);
  const [last, setLast] = useState<Movement | null>(null);
  const [busy, setBusy] = useState(false);
  if (ask && ask !== last) setLast(ask);
  const shown = ask ?? last;

  const place = (id: number | null) => (id === null ? 'без места' : (refs.locations.get(id)?.name ?? 'место'));
  const sub = (m: Movement) => {
    const parts = [`${formatDay(m.created_at)} ${formatTime(m.created_at * 1000)}`];
    if (m.type === 'move') parts.push(`${place(m.from_location_id)} → ${place(m.to_location_id)}`);
    if (m.reason && m.reason !== 'Начальный остаток') parts.push(m.reason);
    if (m.cost !== null && m.cost !== 0) parts.push(formatMoney(Math.abs(m.cost)));
    return parts.join(' · ');
  };

  const confirm = () => {
    if (!ask || busy) return;
    setBusy(true);
    revert([ask.id], 'Движение отменено')
      .then(() => setAsk(null))
      .catch(() => undefined)
      .finally(() => setBusy(false));
  };

  return (
    <>
      <PriceChart rows={purchases} unit={p.unit} />

      <h2 className="g-label">Покупки</h2>
      {purchases.length === 0 ? (
        <p className="g-muted">Покупок ещё не было.</p>
      ) : (
        <div className="g-list">
          {purchases.map((r) => (
            <div key={`${r.purchase_id}-${r.price_per_package}`} className="g-row">
              <div className="g-row__main">
                <div className="g-row__title">
                  {purchaseDay(r.date)}
                  {r.store ? ` · ${r.store}` : ''}
                </div>
                <div className="g-row__sub">
                  {[`${r.packages} шт`, `${formatMoney(r.price_per_package)} за упаковку`, unitPriceText(r, p.unit)].filter(Boolean).join(' · ')}
                </div>
              </div>
              <span className="g-num stock-qty-sm">{formatMoney(r.packages * r.price_per_package)}</span>
            </div>
          ))}
        </div>
      )}

      <h2 className="g-label">Журнал</h2>
      {movements.length === 0 ? (
        <p className="g-muted">Движений пока нет.</p>
      ) : (
        <div className="g-list">
          {movements.map((m) => (
            <div key={m.id} className="g-row">
              <div className="g-row__main">
                <div className="g-row__title">{movementTitle(m, p.unit)}</div>
                <div className="g-row__sub">{sub(m)}</div>
              </div>
              {canRevert(m) && (
                <button type="button" className="g-btn g-btn--sm g-btn--ghost" onClick={() => setAsk(m)} aria-label={`Отменить: ${movementTitle(m, p.unit)}`}>
                  Отменить
                </button>
              )}
            </div>
          ))}
        </div>
      )}

      <Sheet open={ask !== null} onClose={() => setAsk(null)} title="Отменить движение">
        {shown && <p>{revertQuestion(shown, p.unit)}</p>}
        <div className="app-two">
          <button type="button" className="g-btn g-press" onClick={() => setAsk(null)}>
            Не отменять
          </button>
          <button type="button" className="g-btn g-btn--danger g-press" onClick={confirm} {...busyProps(busy)}>
            {busy ? 'Отменяю…' : 'Отменить'}
          </button>
        </div>
      </Sheet>
    </>
  );
}
