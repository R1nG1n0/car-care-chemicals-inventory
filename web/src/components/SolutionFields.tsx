// Раствор (ФТ-17.2.1, ФТ-17.3): объём готового раствора с быстрыми объёмами
// 500 / 750 / 1 л / 5 л, пропорция чипами (пропорции позиции + «Своя 1:N») и
// строка «Концентрат 91 мл + вода 909 мл». Поля — строки: пустое поле допустимо.
import { AnimatedNumber } from './AnimatedNumber';
import { RatioPick } from './RatioPick';
import { parseQty } from '../lib/qty';
import { concentrateFor, QUICK_VOLUMES, quickVolumeText, validRatio, validVolume, waterFor, type Dilution } from '../lib/dilution';

export type SolutionDraft = { volume: string; ratio: string };

type Props = {
  id: string;
  dilutions: readonly Dilution[];
  value: SolutionDraft;
  onChange: (v: SolutionDraft) => void;
  /** Строка шаблона: поле мельче. */
  compact?: boolean;
  /** Ошибки показываются под полями. */
  errors?: { volume?: string | undefined; ratio?: string | undefined } | undefined;
  /** Строка «Концентрат + вода» под полями (в шаблоне и калькуляторе её рисует сам экран). */
  showMix?: boolean;
  describedBy?: string | undefined;
};

const digits = (s: string) => s.replace(/[^\d\s]/gu, '');

export function SolutionFields({ id, dilutions, value, onChange, compact, errors, showMix = true, describedBy }: Props) {
  const volume = parseQty(value.volume);
  const ratio = parseQty(value.ratio);
  const ok = validVolume(volume) && validRatio(ratio);
  const volId = `${id}-vol`;

  return (
    <div className="dil">
      <div className={`g-qty${compact ? ' app-qty--compact' : ''}${errors?.volume ? ' g-field--error' : ''}`}>
        <label className={compact ? 'g-sr' : 'g-label'} htmlFor={volId}>
          Объём раствора
        </label>
        <div className="g-qty__value">
          <input
            id={volId}
            inputMode="numeric"
            autoComplete="off"
            enterKeyHint="done"
            value={value.volume}
            placeholder="0"
            aria-invalid={errors?.volume ? true : undefined}
            aria-describedby={[errors?.volume ? `${volId}-err` : '', describedBy ?? ''].filter(Boolean).join(' ') || undefined}
            onChange={(e) => onChange({ ...value, volume: digits(e.target.value) })}
          />
          <b>мл</b>
        </div>
        <div className="dil-quick" role="group" aria-label="Быстрый объём">
          {QUICK_VOLUMES.map((ml) => (
            <button key={ml} type="button" className="g-btn g-press" aria-pressed={volume === ml} onClick={() => onChange({ ...value, volume: String(ml) })}>
              {quickVolumeText(ml)}
            </button>
          ))}
        </div>
        {errors?.volume && (
          <div className="g-field__error" id={`${volId}-err`} role="alert">
            {errors.volume}
          </div>
        )}
      </div>

      <RatioPick id={id} compact={compact} dilutions={dilutions} ratio={value.ratio} onChange={(ratio) => onChange({ ...value, ratio })} error={errors?.ratio} />

      {showMix && (
        <p className="dil-mix" role="status" aria-live="polite">
          {ok ? <Mix volume={volume} ratio={ratio} /> : <span className="g-muted">Введите объём и пропорцию</span>}
        </p>
      )}
    </div>
  );
}

/** «Концентрат 91 мл + вода 909 мл» с плавной сменой чисел. */
export function Mix({ volume, ratio }: { volume: number; ratio: number }) {
  return (
    <>
      Концентрат <AnimatedNumber value={concentrateFor(volume, ratio)} />&nbsp;мл + вода <AnimatedNumber value={waterFor(volume, ratio)} />&nbsp;мл
    </>
  );
}
