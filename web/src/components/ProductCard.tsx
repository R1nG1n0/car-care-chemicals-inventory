// Карточка позиции на главной (.g-card, макет «Остатки — главная»): полоса цвета
// категории, название, чип места, крупный общий остаток, шкала вскрытой бутылки,
// подпись «вскрыта: 320 из 1 000 · ещё 2 запечатаны», наклейка «мало!» / «нет».
// Вся карточка — одна ссылка. Свайп добавляет обёртка (screens/stock/StockSwipe).
//
// Шкала плавно меняется только при изменении остатка (motion.css). Наклейка
// «прилепляется» (.g-sticker--new), только когда остаток пересёк минимум на глазах,
// а не при открытии экрана; снятая наклейка гаснет (.is-leaving).
import { useState, type CSSProperties, type MouseEvent, type Ref } from 'react';
import { Link } from 'react-router';
import type { ProductSummary } from '../api';
import { formatQty } from '../lib/format';
import { locationChip, meterLevel, meterValues, openLine, qtyText, stickerText, unitLabel } from '../lib/stock';
import { usePendingProductIds } from '../app/useOffline';
import { usePresence } from './hooks';
import { ProductThumb } from './ProductThumb';
import { catClassOf, useStockRefs } from './refs';

export type ProductCardProps = {
  product: ProductSummary;
  /** Куда ведёт карточка, по умолчанию /items/:id. */
  to?: string;
  /** Выбранная карточка (панель позиции на компьютере): aria-current="true". */
  current?: boolean;
  onClick?: (e: MouseEvent<HTMLAnchorElement>) => void;
  ref?: Ref<HTMLAnchorElement>;
  className?: string;
};

export function ProductCard({ product: p, to, current, onClick, ref, className }: ProductCardProps) {
  const refs = useStockRefs();
  const cat = refs.categories.get(p.category_id);
  const place = locationChip(p.location_ids, refs.locations);
  const meter = meterValues(p.stock);
  const open = openLine(p.stock, p.unit, p.package_size);
  const sticker = stickerText(p);
  const unsent = usePendingProductIds().has(p.id);
  // Метка ссылки заменяет содержимое для диктора — поэтому в ней всё главное:
  // остаток, подпись вскрытой и текст наклейки («мало!», «нет»)
  const label = [p.name, cat?.name, place, qtyText(p.stock.total, p.unit), open, sticker, unsent ? 'не отправлено' : ''].filter(Boolean).join(', ');

  return (
    <Link
      ref={ref}
      to={to ?? `/items/${p.id}`}
      className={`g-card g-press stock-card ${catClassOf(refs.categories, p)}${className ? ` ${className}` : ''}`}
      aria-current={current ? 'true' : undefined}
      aria-label={label}
      onClick={onClick}
      draggable={false}
    >
      <div className="g-card__strip" />
      <div className="g-card__body">
        <div className="g-card__top">
          <span className="stock-card__lead">
            <ProductThumb id={p.id} updatedAt={p.photo_updated_at} />
            <span className="g-card__name">{p.name}</span>
          </span>
          {place && <span className="g-chip">{place}</span>}
        </div>
        <div className="g-card__qty">
          <span className="g-num">{formatQty(p.stock.total)}</span>
          <b>{unitLabel(p.unit)}</b>
        </div>
        <Meter level={meterLevel(p.stock)} low={p.low} now={meter.now} max={meter.max} />
        <div className="g-card__open">{open}</div>
        {unsent && <span className="g-badge">не отправлено</span>}
      </div>
      <Sticker text={sticker} />
    </Link>
  );
}

/**
 * Шкала вскрытой бутылки: доля задаётся --lv, меняется трансформацией.
 * decorative — внутри кнопки: span без роли (числа уже в метке кнопки).
 */
export function Meter({ level, low, now, max, label = 'Во вскрытой', className, decorative }: { level: number; low: boolean; now: number; max: number; label?: string; className?: string; decorative?: boolean }) {
  const cls = `g-meter${low ? ' g-meter--low' : ''}${className ? ` ${className}` : ''}`;
  const fill = { '--lv': level.toFixed(3) } as CSSProperties;
  if (decorative) {
    return (
      <span className={cls} aria-hidden="true">
        <span className="g-meter__fill" style={fill} />
      </span>
    );
  }
  return (
    <div
      className={cls}
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuenow={now}
      aria-valuemax={max}
    >
      <div className="g-meter__fill" style={fill} />
    </div>
  );
}

/** Наклейка с появлением при пересечении порога и уходом при снятии. */
function Sticker({ text }: { text: string | null }) {
  const [last, setLast] = useState(text);
  const [prev, setPrev] = useState(text);
  const [fresh, setFresh] = useState(false);
  if (text !== prev) {
    setPrev(text);
    // Появилась после открытия экрана — значит, порог пересечён на глазах
    if (text !== null && prev === null) setFresh(true);
    if (text !== null) setLast(text);
  }
  const { mounted, leaving } = usePresence(text !== null, 200);
  if (!mounted || last === null) return null;
  return <span className={`g-sticker${fresh ? ' g-sticker--new' : ''}${leaving ? ' is-leaving' : ''}`}>{text ?? last}</span>;
}
