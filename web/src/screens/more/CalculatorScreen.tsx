// Калькулятор разбавления /more/calculator (ФТ-17.3): «Сколько концентрата» (объём
// раствора + пропорция → концентрат и вода) и «Сколько раствора получится» (есть
// концентрат + пропорция → раствор и вода). Все вычисления — на телефоне, запросов
// нет. С ?product=ID подставляются пропорции позиции (из сохранённых данных).
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { useSearchParams } from 'react-router';
import { inventory } from '../../api';
import { keys } from '../../app/queries';
import { AnimatedNumber } from '../../components/AnimatedNumber';
import { BackLink } from '../../components/BackLink';
import { RatioPick } from '../../components/RatioPick';
import { SolutionFields } from '../../components/SolutionFields';
import { useProducts } from '../../components/refs';
import { calcErrors, calculate, type CalcMode } from '../../lib/calculator';
import { dilutionsOf, initialRatio, ratioText } from '../../lib/dilution';
import { formatQty } from '../../lib/format';

const MODES: { mode: CalcMode; label: string }[] = [
  { mode: 'forward', label: 'Сколько концентрата' },
  { mode: 'reverse', label: 'Сколько раствора получится' },
];

function Row({ name, value }: { name: string; value: number }) {
  return (
    <div className="dil-result__row">
      <span className="dil-result__name">{name}</span>
      <span className="dil-big">
        <AnimatedNumber value={value} />
        <small>мл</small>
      </span>
    </div>
  );
}

export function CalculatorScreen() {
  const [search] = useSearchParams();
  const raw = Number(search.get('product'));
  const productId = Number.isInteger(raw) && raw > 0 ? raw : null;
  const list = useProducts();
  const fromList = list.data?.find((p) => p.id === productId);
  // Архивной позиции нет в общем списке — берём из карточки (или из сохранённых данных)
  const detail = useQuery({ queryKey: keys.product(productId ?? 0), queryFn: ({ signal }) => inventory.product(productId!, signal), enabled: productId !== null && !fromList });
  const product = fromList ?? detail.data?.product;
  const dilutions = product?.unit === 'ml' ? dilutionsOf(product) : [];

  return <Calculator key={`${productId ?? 0}-${dilutions.map((d) => d.id).join(',')}`} productId={productId} name={product?.name} dilutions={dilutions} />;
}

function Calculator({ productId, name, dilutions }: { productId: number | null; name: string | undefined; dilutions: ReturnType<typeof dilutionsOf> }) {
  const [mode, setMode] = useState<CalcMode>('forward');
  const [volume, setVolume] = useState('');
  const [concentrate, setConcentrate] = useState('');
  const [ratio, setRatio] = useState(() => initialRatio(dilutions, null));
  const input = { mode, volume, concentrate, ratio };
  const result = calculate(input);
  const errors = calcErrors(input);

  return (
    <div className="app-stack">
      <BackLink to={productId ? `/items/${productId}` : '/more'} label={productId ? 'Позиция' : 'Ещё'} />
      <h1 className="g-title">Калькулятор</h1>
      <p className="shop-hint">{name ? `Разбавление: ${name}. Пропорции позиции — чипами, можно ввести свою.` : 'Разбавление концентрата водой. Считает на телефоне, связь не нужна.'}</p>

      <div className="g-seg" role="group" aria-label="Что посчитать">
        {MODES.map((m) => (
          <button key={m.mode} type="button" aria-pressed={mode === m.mode} onClick={() => setMode(m.mode)}>
            {m.label}
          </button>
        ))}
      </div>

      <div className="dil-calc g-enter" key={mode}>
        {mode === 'forward' ? (
          <SolutionFields id="calc" dilutions={dilutions} value={{ volume, ratio }} onChange={(v) => {
              setVolume(v.volume);
              setRatio(v.ratio);
            }} errors={errors} showMix={false} />
        ) : (
          <>
            <div className={`g-qty${errors.concentrate ? ' g-field--error' : ''}`}>
              <label className="g-label" htmlFor="calc-conc">
                Есть концентрата
              </label>
              <div className="g-qty__value">
                <input
                  id="calc-conc"
                  inputMode="numeric"
                  autoComplete="off"
                  placeholder="0"
                  value={concentrate}
                  aria-invalid={errors.concentrate ? true : undefined}
                  aria-describedby={errors.concentrate ? 'calc-conc-err' : undefined}
                  onChange={(e) => setConcentrate(e.target.value.replace(/[^\d\s]/gu, ''))}
                />
                <b>мл</b>
              </div>
              {errors.concentrate && (
                <div className="g-field__error" id="calc-conc-err" role="alert">
                  {errors.concentrate}
                </div>
              )}
            </div>
            <RatioPick id="calc" dilutions={dilutions} ratio={ratio} onChange={setRatio} error={errors.ratio} />
          </>
        )}

        <div className="dil-result" role="status" aria-live="polite">
          {!result && <span className="g-muted">{mode === 'forward' ? 'Введите объём раствора и пропорцию' : 'Введите концентрат и пропорцию'}</span>}
          {result?.mode === 'forward' && (
            <>
              <Row name="Концентрат" value={result.concentrate} />
              <Row name="Вода" value={result.water} />
              <span className="g-muted">
                Раствор {formatQty(result.volume)}&nbsp;мл, {ratioText(result.ratio)}
              </span>
            </>
          )}
          {result?.mode === 'reverse' && (
            <>
              <Row name="Раствор" value={result.solution} />
              <Row name="Вода" value={result.water} />
              <span className="g-muted">
                Концентрат {formatQty(result.concentrate)}&nbsp;мл, {ratioText(result.ratio)}
              </span>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
