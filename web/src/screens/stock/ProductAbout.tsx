// Блок «О средстве» (ФТ-15.2.2) и «Есть дома для того же» (ФТ-15.5.3) в карточке позиции.
import { IconAlertTriangle } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Link } from 'react-router';
import { inventory, type ProductDetail } from '../../api';
import { keys } from '../../app/queries';
import { useStockRefs } from '../../components/refs';
import { Ico } from '../../components/ui';
import { aboutOf, isLongText, phText, purposesOf, wantsAnalogs } from '../../lib/purposes';
import { qtyText } from '../../lib/stock';

export function ProductAbout({ detail }: { detail: ProductDetail }) {
  const refs = useStockRefs();
  const { product: p } = detail;
  const { incompatible, instruction } = aboutOf(detail);
  const purposes = purposesOf(p.purpose_ids, refs.purposes);
  const ph = phText(p.ph);
  const [open, setOpen] = useState(false);
  if (purposes.length === 0 && ph === null && !incompatible && !instruction) return null;
  const long = isLongText(instruction);

  return (
    <section className="about" aria-labelledby="item-about-h">
      <h2 className="g-label" id="item-about-h">
        О средстве
      </h2>
      {purposes.length > 0 && (
        <ul className="about__chips" aria-label="Назначения">
          {purposes.map((x) => (
            <li key={x.id} className="g-chip">
              {x.name}
            </li>
          ))}
        </ul>
      )}
      {ph !== null && <div className="about__ph">pH {ph}</div>}
      {incompatible && (
        <div className="about__warn" role="note">
          <Ico icon={IconAlertTriangle} />
          <span>Не смешивать с: {incompatible}</span>
        </div>
      )}
      {instruction && (
        <div>
          <p className={`about__text${long && !open ? ' about__text--fold' : ''}`} id="item-instruction">
            {instruction}
          </p>
          {long && (
            <button type="button" className="g-btn g-btn--sm g-btn--ghost g-press about__more" aria-expanded={open} aria-controls="item-instruction" onClick={() => setOpen((v) => !v)}>
              {open ? 'Свернуть' : 'Показать полностью'}
            </button>
          )}
        </div>
      )}
    </section>
  );
}

/** «Есть дома для того же»: показывается, когда позиции нет или она ниже минимума. Без связи и без аналогов — ничего. */
export function ProductAnalogs({ detail }: { detail: ProductDetail }) {
  const p = detail.product;
  const wanted = wantsAnalogs(p);
  const analogs = useQuery({
    queryKey: keys.analogs(p.id),
    queryFn: ({ signal }) => inventory.analogs(p.id, signal),
    enabled: wanted,
    retry: false,
  });
  const list = analogs.data ?? [];
  if (!wanted || list.length === 0) return null;
  return (
    <section className="app-stack" aria-labelledby="item-analogs-h">
      <h2 className="g-label" id="item-analogs-h">
        Есть дома для того же
      </h2>
      <div className="g-list">
        {list.map((a) => (
          <Link key={a.id} to={`/items/${a.id}`} className="g-row g-row--link">
            <div className="g-row__main">
              <div className="g-row__title">{a.name}</div>
              {a.brand && <div className="g-row__sub">{a.brand}</div>}
            </div>
            <span className="g-badge">{qtyText(a.stock.total, a.unit)}</span>
          </Link>
        ))}
      </div>
    </section>
  );
}
