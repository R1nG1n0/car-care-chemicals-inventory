// Форма расхода одной позиции: количество (ФТ-5.8), подсказка «В прошлый раз…»,
// выбор бутылки (ФТ-5.2), горчичная «Списать N мл». Используется в шторке и в
// панели на компьютере. Ошибка «Не хватает» (409) — под полем.
// У позиции с пропорциями (ФТ-17.2) — переключатель «Концентрат / Раствор»: в режиме
// раствора вводится объём и пропорция, уходит раствор, концентрат считает сервер.
import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Link } from 'react-router';
import { errorText, inventory, type ProductSummary } from '../../api';
import { keys } from '../../app/queries';
import { useSession } from '../../app/session';
import { ModeSwitch } from '../../components/ModeSwitch';
import { QtyInput } from '../../components/QtyInput';
import { SolutionFields, type SolutionDraft } from '../../components/SolutionFields';
import { busyProps } from '../../components/ui';
import { concentrateOf, dilutionsOf, initialRatio, loadPref, parseSolution, savePref, solutionError, type DilutionMode } from '../../lib/dilution';
import { qtyText } from '../../lib/stock';
import { parseQty, validateConsume } from '../../lib/qty';
import { bottleOptions, consumeHint, consumeUnitId, defaultConsumeQty } from '../../lib/stockQty';
import { useConsume } from './useConsume';

type Props = { product: ProductSummary; idPrefix: string; onDone?: () => void; autoFocus?: boolean };

export function ConsumeForm({ product: p, idPrefix, onDone, autoFocus }: Props) {
  const session = useSession();
  const detail = useQuery({ queryKey: keys.product(p.id), queryFn: ({ signal }) => inventory.product(p.id, signal) });
  const [qty, setQty] = useState(() => String(defaultConsumeQty(p, session.data?.settings)));
  const [chosen, setChosen] = useState<number | undefined>();
  const [error, setError] = useState<string>();
  const dilutions = p.unit === 'ml' ? dilutionsOf(p) : [];
  const [pref] = useState(() => (dilutions.length > 0 ? loadPref(p.id) : null));
  const [mode, setMode] = useState<DilutionMode>(pref?.mode === 'solution' ? 'solution' : 'concentrate');
  const [sol, setSol] = useState<SolutionDraft>(() => ({ volume: '', ratio: initialRatio(dilutions, pref) }));
  const [solErr, setSolErr] = useState<{ volume?: string | undefined; ratio?: string | undefined }>();
  const consume = useConsume(onDone);
  const cta = useRef<HTMLButtonElement>(null);
  // После выбора позиции в шторке фокус — на «Списать»: клавиатура на телефоне не всплывает
  useEffect(() => {
    if (autoFocus) cta.current?.focus({ preventScroll: true });
  }, [autoFocus]);

  const options = bottleOptions(detail.data?.units ?? [], p.unit);
  const unitId = consumeUnitId(options, chosen);
  const solution = mode === 'solution' ? parseSolution(sol.volume, sol.ratio) : null;
  const n = mode === 'solution' ? (solution ? concentrateOf(solution) : 0) : (parseQty(qty) ?? 0);
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
    if (mode === 'solution') {
      const bad = solutionError(sol.volume, sol.ratio);
      if (bad || !solution) return setSolErr(bad ?? {});
      const invalid = validateConsume(n, total, p.unit);
      if (invalid) return setError(invalid);
    } else {
      const invalid = validateConsume(parseQty(qty), total, p.unit);
      if (invalid) return setError(invalid);
    }
    setError(undefined);
    setSolErr(undefined);
    if (dilutions.length > 0) savePref(p.id, { mode, ratio: mode === 'solution' ? solution?.ratio ?? null : (pref?.ratio ?? null) });
    // «Нет связи» — ничего не списано; ответ не пришёл — неизвестно, данные перечитываются
    consume.mutate({ product: p, qty: n, unitId, ...(solution ? { solution } : {}) }, { onError: (err) => setError(errorText(err)) });
  };

  const edit = (v: string) => {
    setQty(v);
    setError(undefined);
  };
  const editSol = (v: SolutionDraft) => {
    setSol(v);
    setSolErr(undefined);
    setError(undefined);
  };

  return (
    <form className="stock-gap" onSubmit={submit} noValidate>
      {dilutions.length > 0 && (
        <ModeSwitch
          id={`${idPrefix}-mode`}
          mode={mode}
          onChange={(m) => {
            setMode(m);
            setError(undefined);
            setSolErr(undefined);
          }}
        />
      )}
      {mode === 'solution' ? (
        <div className="stock-gap g-enter">
          <SolutionFields id={`${idPrefix}-sol`} dilutions={dilutions} value={sol} onChange={editSol} errors={solErr} />
          <p className="g-qty__hint">{consumeHint(p)}</p>
          {error && (
            <p className="g-field__error" role="alert">
              {error}
            </p>
          )}
        </div>
      ) : (
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
      )}
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
        {consume.isPending ? 'Списываю…' : mode === 'solution' && !solution ? 'Списать' : `Списать ${qtyText(n, p.unit)}`}
      </button>
    </form>
  );
}
