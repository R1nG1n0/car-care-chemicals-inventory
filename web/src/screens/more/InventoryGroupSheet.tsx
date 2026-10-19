// Шторка «Не хватает…» для группы одинаковых запечатанных экземпляров: сколько на самом деле
// на месте, а недостающие — не найдены (списываются) или лежат в другом месте (перемещаются).
import { useState, type FormEvent } from 'react';
import { errorText, type InventoryUnit, type Location } from '../../api';
import { Sheet } from '../../components/Sheet';
import { busyProps } from '../../components/ui';
import { parseQty } from '../../lib/qty';
import { plural } from '../../lib/text';
import { SHEET_LEAVE_MS, type After } from './InventorySheet';
import type { GroupActions, MissingHow } from './useInventoryActions';

const EXEMPLARS = ['экземпляр', 'экземпляра', 'экземпляров'] as const;

type Props = { units: InventoryUnit[] | null; places: Location[]; actions: GroupActions; onClose: () => void };

export function InventoryGroupSheet({ units, places, actions, onClose }: Props) {
  const [last, setLast] = useState(units);
  if (units && units !== last) setLast(units);
  const list = units ?? last;
  return (
    <Sheet open={units !== null} onClose={onClose} title={list?.[0] ? list[0].product.name : ''}>
      {list && list.length > 0 && <Body key={list.map((u) => u.id).join(',')} units={list} places={places} actions={actions} onClose={onClose} />}
    </Sheet>
  );
}

function Body({ units, places, actions, onClose }: { units: InventoryUnit[]; places: Location[]; actions: GroupActions; onClose: () => void }) {
  const total = units.length;
  const here = units[0]?.location_id;
  const others = places.filter((l) => l.id !== here);
  const [count, setCount] = useState(String(total - 1));
  const [how, setHow] = useState<'lost' | 'moved'>('lost');
  const [target, setTarget] = useState<number | null>(null);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const present = parseQty(count);
  const missing = present === null ? null : total - present;
  const missingText = missing !== null && missing > 0 ? `${missing} ${plural(missing, EXEMPLARS)}` : '';

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    if (present === null || present >= total) return setError(`Укажите число от 0 до ${total - 1}: сколько на месте на самом деле`);
    let mode: MissingHow = { kind: 'lost' };
    if (how === 'moved') {
      const to = others.find((l) => l.id === target);
      if (!to) return setError('Выберите место');
      mode = { kind: 'moved', to };
    }
    setBusy(true);
    actions.shortage(units, present, mode).then(
      (after: After) => {
        onClose();
        setTimeout(after, SHEET_LEAVE_MS);
      },
      (err: unknown) => {
        setError(errorText(err));
        setBusy(false);
      },
    );
  };

  return (
    <form className="stock-gap" onSubmit={submit} noValidate>
      <div className={`g-field${error && how === 'lost' ? ' g-field--error' : ''}`}>
        <label className="g-label" htmlFor="inv-gcount">
          Сколько на месте на самом деле
        </label>
        <input
          id="inv-gcount"
          className="g-input"
          inputMode="numeric"
          autoComplete="off"
          autoFocus
          value={count}
          onChange={(e) => (setCount(e.target.value), setError(undefined))}
          aria-describedby="inv-gcount-hint"
        />
        <span className="g-muted" id="inv-gcount-hint">
          В учёте {total} {plural(total, EXEMPLARS)}
          {missingText && ` · не хватает ${missingText}`}
        </span>
      </div>

      <div className="g-seg" role="group" aria-label="Что с недостающими">
        <button type="button" aria-pressed={how === 'lost'} onClick={() => (setHow('lost'), setError(undefined))}>
          Не найдены
        </button>
        <button type="button" aria-pressed={how === 'moved'} onClick={() => (setHow('moved'), setError(undefined))}>
          В другом месте
        </button>
      </div>

      {how === 'lost' ? (
        <p className="g-muted">Недостающие будут списаны с причиной «Инвентаризация: не найден». Это можно отменить сразу после.</p>
      ) : others.length === 0 ? (
        <p className="g-muted">Других мест нет — добавьте их в разделе «Ещё → Места».</p>
      ) : (
        <div className="g-field">
          <span className="g-label" id="inv-gplaces">
            Где лежат на самом деле
          </span>
          <div className="stock-places" role="group" aria-labelledby="inv-gplaces">
            {others.map((l) => (
              <button key={l.id} type="button" className="g-filter" aria-pressed={target === l.id} onClick={() => (setTarget(l.id), setError(undefined))}>
                {l.name}
              </button>
            ))}
          </div>
        </div>
      )}

      {error && (
        <span className="g-field__error" role="alert">
          {error}
        </span>
      )}

      <button type="submit" className={`g-btn ${how === 'lost' ? 'g-btn--danger' : 'g-btn--accent'} g-btn--block g-press`} {...busyProps(busy)}>
        {busy ? 'Сохраняю…' : how === 'lost' ? `Списать${missingText ? ` ${missingText}` : ''}` : `Переложить${missingText ? ` ${missingText}` : ''}`}
      </button>
    </form>
  );
}
