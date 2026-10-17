// Действия с экземпляром во второй шторке: переместить (ФТ-5.4), скорректировать
// остаток с причиной (ФТ-5.5), списать с причиной (ФТ-5.3). После корректировки и
// списания — уведомление с «Отменить» (ФТ-5.6).
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { inventory, isApiError, isNetworkError, type MovementType, type ProductSummary, type StockUnit } from '../../api';
import { invalidateStock } from '../../app/queries';
import { useToast } from '../../app/toast';
import { QtyInput } from '../../components/QtyInput';
import { Sheet } from '../../components/Sheet';
import { useStockRefs } from '../../components/StockRefs';
import { busyProps, Field } from '../../components/ui';
import { qtyText } from '../../lib/stock';
import { parseQty } from '../../lib/stockQty';
import { useRevert } from './useConsume';
import { unitState } from './UnitRows';

type Mode = 'move' | 'adjust' | 'writeoff';
const REASONS = ['Испорчено', 'Выброшено', 'Отдано', 'Ошибка ввода'];

type Props = { unit: StockUnit | null; product: ProductSummary; onClose: () => void };

export function UnitSheet({ unit, product, onClose }: Props) {
  const [last, setLast] = useState(unit);
  if (unit && unit !== last) setLast(unit);
  const u = unit ?? last;
  return (
    <Sheet open={unit !== null} onClose={onClose} title={u ? `${unitState(u, product.unit)} · ${qtyText(u.remaining_qty, product.unit)}` : ''}>
      {u && <UnitBody key={u.id} u={u} product={product} onClose={onClose} />}
    </Sheet>
  );
}

function UnitBody({ u, product: p, onClose }: { u: StockUnit; product: ProductSummary; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const revert = useRevert();
  const refs = useStockRefs();
  const [mode, setMode] = useState<Mode>('move');
  const [place, setPlace] = useState<number | null>(u.location_id);
  const [qty, setQty] = useState(String(u.remaining_qty));
  const [reason, setReason] = useState('');
  const [error, setError] = useState<string>();
  const places = refs.locationList.filter((l) => !l.archived);

  // Движение для «Отменить»: сервер возвращает экземпляр, id берём из журнала
  const offerUndo = async (type: MovementType, text: string) => {
    const list = await inventory.movements({ product_id: p.id }).catch(() => []);
    const m = list.find((x) => x.unit_id === u.id && x.type === type);
    toast.show(m ? { text, action: { label: 'Отменить', onClick: () => void revert([m.id]).catch(() => undefined) } } : { text });
  };

  const mut = useMutation({
    mutationFn: async (): Promise<() => void> => {
      if (mode === 'move') {
        const r = await inventory.moveUnit(u.id, place!);
        const name = refs.locations.get(r.location_id ?? 0)?.name ?? 'другое место';
        return () => toast.show({ text: `Перемещено: ${name}` });
      }
      const n = parseQty(qty)!;
      if (mode === 'adjust') {
        await inventory.adjustUnit(u.id, n, reason.trim());
        return () => void offerUndo('adjust', `Остаток исправлен: ${qtyText(n, p.unit)}`);
      }
      await inventory.writeoffUnit(u.id, reason.trim(), n);
      return () => void offerUndo('writeoff', `Списано ${qtyText(n, p.unit)} · ${reason.trim()}`);
    },
    onSuccess: async (after) => {
      onClose();
      await invalidateStock(qc);
      after();
    },
    onError: (error) => {
      const err: unknown = error;
      if (isNetworkError(err)) setError('Нет связи — изменения не сохранены');
      else setError(isApiError(err) ? err.message : 'Не получилось');
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (mut.isPending) return;
    const n = parseQty(qty);
    if (mode === 'move') {
      if (place === null) return setError('Выберите место');
      if (place === u.location_id) return setError('Экземпляр уже лежит здесь');
    } else if (mode === 'adjust') {
      if (n === null || n > u.initial_qty) return setError(`От 0 до ${qtyText(u.initial_qty, p.unit)}`);
      if (n === u.remaining_qty) return setError('Остаток не изменился');
    } else {
      if (n === null || n < 1 || n > u.remaining_qty) return setError(`От 1 до ${qtyText(u.remaining_qty, p.unit)}`);
      if (reason.trim() === '') return setError('Укажите причину');
    }
    setError(undefined);
    mut.mutate();
  };

  const switchTo = (m: Mode) => {
    setMode(m);
    setError(undefined);
    setReason('');
    setQty(String(u.remaining_qty));
  };

  const cta =
    mode === 'move' ? 'Переместить' : mode === 'adjust' ? 'Сохранить остаток' : `Списать ${qtyText(parseQty(qty) ?? 0, p.unit)}`;

  return (
    <form className="stock-gap" onSubmit={submit} noValidate>
      <div className="g-seg" role="group" aria-label="Действие">
        <button type="button" aria-pressed={mode === 'move'} onClick={() => switchTo('move')}>
          Переместить
        </button>
        <button type="button" aria-pressed={mode === 'adjust'} onClick={() => switchTo('adjust')}>
          Остаток
        </button>
        <button type="button" aria-pressed={mode === 'writeoff'} onClick={() => switchTo('writeoff')}>
          Списать
        </button>
      </div>

      {mode === 'move' &&
        (places.length === 0 ? (
          <p className="g-muted">Мест пока нет — добавьте их в разделе «Ещё».</p>
        ) : (
          <div className="g-field">
            <span className="g-label" id="unit-places">
              Куда
            </span>
            <div className="stock-places" role="group" aria-labelledby="unit-places">
              {places.map((l) => (
                <button key={l.id} type="button" className="g-filter" aria-pressed={place === l.id} onClick={() => (setPlace(l.id), setError(undefined))}>
                  {l.name}
                  {l.id === u.location_id ? ' · сейчас' : ''}
                </button>
              ))}
            </div>
            {error && <span className="g-field__error" role="alert">{error}</span>}
          </div>
        ))}

      {mode !== 'move' && (
        <>
          <QtyInput
            id="unit-qty"
            label={mode === 'adjust' ? 'Фактический остаток' : 'Сколько списать'}
            value={qty}
            onChange={(v) => (setQty(v), setError(undefined))}
            unit={p.unit}
            min={mode === 'adjust' ? 0 : 1}
            max={mode === 'adjust' ? u.initial_qty : u.remaining_qty}
            hint={`Сейчас ${qtyText(u.remaining_qty, p.unit)} из ${qtyText(u.initial_qty, p.unit)}${mode === 'adjust' ? ' · разница запишется в журнал' : ''}`}
            error={error}
          />
          {mode === 'writeoff' && (
            <div className="stock-reasons" role="group" aria-label="Частые причины">
              {REASONS.map((r) => (
                <button key={r} type="button" className="g-filter" aria-pressed={reason === r} onClick={() => (setReason(r), setError(undefined))}>
                  {r}
                </button>
              ))}
            </div>
          )}
          <Field
            id="unit-reason"
            label={mode === 'writeoff' ? 'Причина' : 'Причина · необязательно'}
            value={reason}
            maxLength={80}
            onChange={(e) => (setReason(e.target.value), setError(undefined))}
          />
        </>
      )}

      <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(mut.isPending)}>
        {mut.isPending ? 'Сохраняю…' : cta}
      </button>
    </form>
  );
}
