// Форма расхода одной позиции: количество (ФТ-5.8), подсказка «В прошлый раз…»,
// выбор бутылки (ФТ-5.2), горчичная «Списать N мл». Используется в шторке и в
// панели на компьютере. Ошибка «Не хватает» (409) — под полем.
import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link } from 'react-router';
import { inventory, isApiError, isNetworkError, type ProductSummary } from '../../api';
import { keys } from '../../app/queries';
import { useSession } from '../../app/session';
import { QtyInput } from '../../components/QtyInput';
import { busyProps } from '../../components/ui';
import { qtyText } from '../../lib/stock';
import { bottleOptions, consumeHint, defaultBottle, defaultConsumeQty, parseQty } from '../../lib/stockQty';
import { useConsume } from './useConsume';

type Props = { product: ProductSummary; idPrefix: string; onDone?: () => void; autoFocus?: boolean };

export function ConsumeForm({ product: p, idPrefix, onDone, autoFocus }: Props) {
  const session = useSession();
  const detail = useQuery({ queryKey: keys.product(p.id), queryFn: ({ signal }) => inventory.product(p.id, signal) });
  const [qty, setQty] = useState(() => String(defaultConsumeQty(p, session.data?.settings)));
  const [chosen, setChosen] = useState<number | undefined>();
  const [error, setError] = useState<string>();
  const consume = useConsume(onDone);
  const cta = useRef<HTMLButtonElement>(null);
  // После выбора позиции в шторке фокус — на «Списать»: клавиатура на телефоне не всплывает
  useEffect(() => {
    if (autoFocus) cta.current?.focus({ preventScroll: true });
  }, [autoFocus]);

  const options = bottleOptions(detail.data?.units ?? [], p.unit);
  const unitId = options.some((o) => o.unitId === chosen) ? chosen : defaultBottle(options);
  const n = parseQty(qty) ?? 0;
  const total = p.stock.total;

  if (total === 0) {
    return (
      <div className="stock-gap">
        <p className="g-muted">Нет в наличии — сначала добавьте покупку или введите имеющийся остаток.</p>
        <Link className="g-btn g-btn--block g-press" to={`/purchases/new?product=${p.id}`}>
          Покупка
        </Link>
      </div>
    );
  }

  const submit = (e?: FormEvent) => {
    e?.preventDefault();
    if (consume.isPending) return;
    if (n <= 0) return setError('Введите количество');
    if (n > total) return setError(`Не хватает: есть ${qtyText(total, p.unit)}`);
    setError(undefined);
    consume.mutate(
      { product: p, qty: n, unitId: options.length > 1 ? unitId : undefined },
      {
        onError: (error) => {
          const err: unknown = error;
          if (isNetworkError(err)) setError('Нет связи — ничего не списано');
          else setError(isApiError(err) ? err.message : 'Не получилось списать');
        },
      },
    );
  };

  const edit = (v: string) => {
    setQty(v);
    setError(undefined);
  };

  return (
    <form className="stock-gap" onSubmit={submit} noValidate>
      <QtyInput
        id={`${idPrefix}-qty`}
        value={qty}
        onChange={edit}
        unit={p.unit}
        min={0}
        max={total}
        hint={consumeHint(p)}
        error={error}
        onEnter={() => submit()}
      />
      {options.length > 1 && (
        <div className="g-field">
          <span className="g-label" id={`${idPrefix}-bottle`}>
            {p.unit === 'pcs' ? 'Из пачки' : 'Из бутылки'}
          </span>
          <div className="g-seg" role="group" aria-labelledby={`${idPrefix}-bottle`}>
            {options.map((o) => (
              <button key={o.unitId} type="button" aria-pressed={o.unitId === unitId} onClick={() => setChosen(o.unitId)}>
                {o.label}
              </button>
            ))}
          </div>
        </div>
      )}
      <button
        ref={cta}
        type="submit"
        className="g-btn g-btn--cta g-btn--block g-press"
        data-autofocus={autoFocus || undefined}
        aria-disabled={n <= 0 || undefined} {...busyProps(consume.isPending)}>
        {consume.isPending ? 'Списываю…' : `Списать ${qtyText(n, p.unit)}`}
      </button>
    </form>
  );
}
