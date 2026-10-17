// «Ввести имеющийся остаток» — то, что уже лежит дома без покупки (POST /api/units):
// объём упаковки (по умолчанию размер упаковки позиции), сколько в ней осталось, место.
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { inventory, isApiError, isNetworkError, type ProductSummary } from '../../api';
import { invalidateStock } from '../../app/queries';
import { useToast } from '../../app/toast';
import { QtyInput } from '../../components/QtyInput';
import { Sheet } from '../../components/Sheet';
import { useStockRefs } from '../../components/StockRefs';
import { busyProps, Field } from '../../components/ui';
import { qtyText, unitLabel } from '../../lib/stock';
import { parseQty } from '../../lib/stockQty';

type Props = { open: boolean; product: ProductSummary; onClose: () => void; defaultLocation: number | null };

export function AddUnitSheet({ open, product, onClose, defaultLocation }: Props) {
  return (
    <Sheet open={open} onClose={onClose} title="Имеющийся остаток">
      <AddUnitBody product={product} onClose={onClose} defaultLocation={defaultLocation} />
    </Sheet>
  );
}

function AddUnitBody({ product: p, onClose, defaultLocation }: Omit<Props, 'open'>) {
  const qc = useQueryClient();
  const toast = useToast();
  const refs = useStockRefs();
  const [initial, setInitial] = useState(String(p.package_size));
  const [left, setLeft] = useState(String(p.package_size));
  const [place, setPlace] = useState<number | null>(defaultLocation);
  const [errors, setErrors] = useState<{ initial?: string; left?: string; form?: string }>({});
  const places = refs.locationList.filter((l) => !l.archived);
  const cap = parseQty(initial);

  const mut = useMutation({
    mutationFn: (b: { remaining_qty: number; initial_qty: number }) => inventory.addUnit({ product_id: p.id, ...b, location_id: place }),
    onSuccess: async (u) => {
      onClose();
      await invalidateStock(qc);
      toast.show({ text: `Добавлено: ${qtyText(u.remaining_qty, p.unit)} · ${p.name}` });
    },
    onError: (error) => {
      const err: unknown = error;
      if (isNetworkError(err)) return setErrors({ form: 'Нет связи — не сохранено' });
      if (!isApiError(err)) return setErrors({ form: 'Не получилось' });
      if (err.field === 'initial_qty') setErrors({ initial: err.message });
      else if (err.field === 'remaining_qty') setErrors({ left: err.message });
      else setErrors({ form: err.message });
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (mut.isPending) return;
    const n = parseQty(left);
    if (cap === null || cap < 1 || cap > 100_000) return setErrors({ initial: 'Целое число от 1 до 100 000' });
    if (n === null || n < 1 || n > cap) return setErrors({ left: `От 1 до ${qtyText(cap, p.unit)}` });
    setErrors({});
    mut.mutate({ remaining_qty: n, initial_qty: cap });
  };

  return (
    <form className="stock-gap" onSubmit={submit} noValidate>
      <p className="g-muted">Для того, что уже лежит дома. Полная упаковка станет запечатанной, начатая — вскрытой.</p>
      <Field
        id="add-initial"
        label={p.unit === 'pcs' ? 'Штук в полной пачке' : 'Объём упаковки, мл'}
        inputMode="numeric"
        value={initial}
        onChange={(e) => (setInitial(e.target.value), setErrors({}))}
        error={errors.initial}
      />
      <QtyInput
        id="add-left"
        label={`Сколько осталось, ${unitLabel(p.unit)}`}
        value={left}
        onChange={(v) => (setLeft(v), setErrors({}))}
        unit={p.unit}
        min={1}
        max={cap}
        error={errors.left}
      />
      {places.length > 0 && (
        <div className="g-field">
          <span className="g-label" id="add-places">
            Место
          </span>
          <div className="stock-places" role="group" aria-labelledby="add-places">
            <button type="button" className="g-filter" aria-pressed={place === null} onClick={() => setPlace(null)}>
              Без места
            </button>
            {places.map((l) => (
              <button key={l.id} type="button" className="g-filter" aria-pressed={place === l.id} onClick={() => setPlace(l.id)}>
                {l.name}
              </button>
            ))}
          </div>
        </div>
      )}
      {errors.form && (
        <p className="g-field__error" role="alert">
          {errors.form}
        </p>
      )}
      <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(mut.isPending)}>
        {mut.isPending ? 'Сохраняю…' : 'Добавить остаток'}
      </button>
    </form>
  );
}
