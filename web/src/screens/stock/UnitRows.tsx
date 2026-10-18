// Строки экземпляров: состояние, остаток, место, шкала. На экране позиции строка
// нажимается и открывает действия, в панели на компьютере — только показ.
import type { Location, StockUnit, Unit } from '../../api';
import { Meter } from '../../components/ProductCard';
import { formatQty } from '../../lib/format';
import { qtyText } from '../../lib/stock';

export function unitState(u: StockUnit, unit: Unit): string {
  if (u.status === 'open') return unit === 'pcs' ? 'Вскрыта пачка' : 'Вскрыта';
  return unit === 'pcs' ? 'Пачка запечатана' : 'Запечатана';
}

type Props = {
  units: StockUnit[];
  unit: Unit;
  /** Класс цвета категории (refs.catClassOf). */
  catClass: string;
  locations: ReadonlyMap<number, Location>;
  onPick?: (u: StockUnit) => void;
};

export function UnitRows({ units, unit, catClass, locations, onPick }: Props) {
  return (
    <div className={`g-list ${catClass}`}>
      {units.map((u) => {
        const place = u.location_id !== null ? (locations.get(u.location_id)?.name ?? 'Место') : 'Без места';
        // Внутри кнопки — только span; шкала в кнопке декоративная (числа — в метке)
        const inner = (
          <span className="g-row__main stock-unit">
            <span className="stock-between">
              <span className="g-row__title">{unitState(u, unit)}</span>
              <span className="g-num stock-qty-sm">{qtyText(u.remaining_qty, unit)}</span>
            </span>
            <span className="stock-unit__line g-row__sub">
              <span>{place}</span>
              <span>из {formatQty(u.initial_qty)}</span>
            </span>
            <Meter
              level={u.initial_qty ? u.remaining_qty / u.initial_qty : 0}
              low={false}
              now={u.remaining_qty}
              max={u.initial_qty}
              label="Остаток экземпляра"
              decorative={Boolean(onPick)}
            />
          </span>
        );
        return onPick ? (
          <button
            key={u.id}
            type="button"
            className="g-row g-row--link app-row-btn"
            aria-label={`${unitState(u, unit)}, ${qtyText(u.remaining_qty, unit)} из ${qtyText(u.initial_qty, unit)}, ${place}: действия`}
            onClick={() => onPick(u)}
          >
            {inner}
          </button>
        ) : (
          <div key={u.id} className="g-row">
            {inner}
          </div>
        );
      })}
    </div>
  );
}
