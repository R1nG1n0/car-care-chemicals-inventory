// Шторка действий над экземпляром при инвентаризации (ФТ-10.1.2): исправить остаток
// (корректировка, причина «Инвентаризация»), «Нет на месте» (списать весь остаток,
// причина «Инвентаризация: не найден», подтверждение — сама кнопка), «В другом месте» (перемещение).
import { useState, type FormEvent } from 'react';
import { errorText, type InventoryUnit, type Location } from '../../api';
import { QtyInput } from '../../components/QtyInput';
import { Sheet } from '../../components/Sheet';
import { busyProps } from '../../components/ui';
import { parseQty } from '../../lib/qty';
import { qtyText } from '../../lib/stock';

/** Шторка уходит за 200 мс плюс запас (components/Sheet). */
export const SHEET_LEAVE_MS = 260;

type Mode = 'adjust' | 'missing' | 'move';

/** Действие возвращает уведомление: оно показывается после ухода шторки (пока шторка открыта, страница не прокручивается и панель «Завершить» уезжает). */
export type After = () => void;

export type SheetActions = {
  adjust: (u: InventoryUnit, qty: number) => Promise<After>;
  missing: (u: InventoryUnit) => Promise<After>;
  move: (u: InventoryUnit, to: Location) => Promise<After>;
  /** Остаток не менялся: то же, что «На месте». */
  confirm: (u: InventoryUnit) => void;
};

type Props = { unit: InventoryUnit | null; places: Location[]; actions: SheetActions; onClose: () => void };

export function InventorySheet({ unit, places, actions, onClose }: Props) {
  const [last, setLast] = useState(unit);
  if (unit && unit !== last) setLast(unit);
  const u = unit ?? last;
  return (
    <Sheet open={unit !== null} onClose={onClose} title={u ? u.product.name : ''}>
      {u && <Body key={u.id} u={u} places={places} actions={actions} onClose={onClose} />}
    </Sheet>
  );
}

function Body({ u, places, actions, onClose }: { u: InventoryUnit; places: Location[]; actions: SheetActions; onClose: () => void }) {
  const unit = u.product.unit;
  const others = places.filter((l) => l.id !== u.location_id);
  const [mode, setMode] = useState<Mode>('adjust');
  const [qty, setQty] = useState(String(u.remaining_qty));
  const [target, setTarget] = useState<number | null>(null);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const run = async (fn: () => Promise<After>) => {
    setBusy(true);
    try {
      const after = await fn();
      onClose();
      setTimeout(after, SHEET_LEAVE_MS);
    } catch (err) {
      setError(errorText(err));
      setBusy(false);
    }
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    if (mode === 'missing') return void run(() => actions.missing(u));
    if (mode === 'move') {
      const to = others.find((l) => l.id === target);
      if (!to) return setError('Выберите место');
      return void run(() => actions.move(u, to));
    }
    const n = parseQty(qty);
    if (n === null || n > u.initial_qty) return setError(`От 0 до ${qtyText(u.initial_qty, unit)}`);
    if (n === u.remaining_qty) return setError('Остаток не изменился — закройте и нажмите «На месте»');
    void run(() => actions.adjust(u, n));
  };

  const switchTo = (m: Mode) => {
    setMode(m);
    setError(undefined);
  };

  return (
    <form className="stock-gap" onSubmit={submit} noValidate>
      <div className="g-seg" role="group" aria-label="Действие">
        <button type="button" aria-pressed={mode === 'adjust'} onClick={() => switchTo('adjust')}>
          Остаток
        </button>
        <button type="button" aria-pressed={mode === 'missing'} onClick={() => switchTo('missing')}>
          Нет на месте
        </button>
        <button type="button" aria-pressed={mode === 'move'} onClick={() => switchTo('move')}>
          В другом месте
        </button>
      </div>

      {mode === 'adjust' && (
        <QtyInput
          id="inv-qty"
          label="Фактический остаток"
          value={qty}
          onChange={(v) => (setQty(v), setError(undefined))}
          unit={unit}
          max={u.initial_qty}
          hint={`В учёте ${qtyText(u.remaining_qty, unit)} из ${qtyText(u.initial_qty, unit)} · причина «Инвентаризация»`}
          error={error}
          autoFocus
        />
      )}

      {mode === 'missing' && (
        <>
          <p className="g-muted">
            Весь остаток — {qtyText(u.remaining_qty, unit)} — будет списан с причиной «Инвентаризация: не найден». Это можно отменить сразу после.
          </p>
          {error && (
            <span className="g-field__error" role="alert">
              {error}
            </span>
          )}
        </>
      )}

      {mode === 'move' &&
        (others.length === 0 ? (
          <p className="g-muted">Других мест нет — добавьте их в разделе «Ещё → Места».</p>
        ) : (
          <div className="g-field">
            <span className="g-label" id="inv-places">
              Где лежит на самом деле
            </span>
            <div className="stock-places" role="group" aria-labelledby="inv-places">
              {others.map((l) => (
                <button key={l.id} type="button" className="g-filter" aria-pressed={target === l.id} onClick={() => (setTarget(l.id), setError(undefined))}>
                  {l.name}
                </button>
              ))}
            </div>
            {error && (
              <span className="g-field__error" role="alert">
                {error}
              </span>
            )}
          </div>
        ))}

      <button
        type="submit"
        className={`g-btn ${mode === 'missing' ? 'g-btn--danger' : 'g-btn--accent'} g-btn--block g-press`}
        {...busyProps(busy)}
      >
        {busy ? 'Сохраняю…' : mode === 'adjust' ? 'Сохранить остаток' : mode === 'missing' ? 'Списать весь остаток' : 'Переложить'}
      </button>
    </form>
  );
}
