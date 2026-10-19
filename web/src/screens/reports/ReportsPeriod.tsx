// Выбор периода отчёта (ФТ-10.2.1): месяц, квартал, год, свой (с–по).
import { IconChevronLeft, IconChevronRight } from '@tabler/icons-react';
import { Ico } from '../../components/ui';
import { canGoNext, customError, PERIOD_KINDS, periodTitle, shiftPeriod, switchKind, type Period, type Ymd } from '../../lib/reports';

type Props = {
  period: Period;
  today: Ymd;
  onChange: (p: Period) => void;
};

export function ReportsPeriod({ period, today, onChange }: Props) {
  const own = period.kind === 'custom';
  const error = own ? customError(period.from, period.to) : '';
  const next = canGoNext(period, today);
  return (
    <section className="rep-period" aria-label="Период отчёта">
      <div className="g-filters" role="group" aria-label="Вид периода">
        {PERIOD_KINDS.map((k) => (
          <button key={k.kind} type="button" className="g-filter" aria-pressed={period.kind === k.kind} onClick={() => onChange(switchKind(period, k.kind, today))}>
            {k.label}
          </button>
        ))}
      </div>
      {own ? (
        <div className="rep-custom">
          <div className="rep-custom__dates">
            <div className="g-field">
              <label className="g-label" htmlFor="rep-from">
                С
              </label>
              <input
                id="rep-from"
                className="g-input"
                type="date"
                value={period.from}
                aria-invalid={error ? true : undefined}
                aria-describedby={error ? 'rep-err' : undefined}
                onChange={(e) => onChange({ ...period, from: e.target.value })}
              />
            </div>
            <div className="g-field">
              <label className="g-label" htmlFor="rep-to">
                По
              </label>
              <input
                id="rep-to"
                className="g-input"
                type="date"
                value={period.to}
                aria-invalid={error ? true : undefined}
                aria-describedby={error ? 'rep-err' : undefined}
                onChange={(e) => onChange({ ...period, to: e.target.value })}
              />
            </div>
          </div>
          {error && (
            <p className="g-field__error" id="rep-err" role="alert">
              {error}
            </p>
          )}
        </div>
      ) : (
        <div className="rep-nav">
          <button type="button" className="g-btn g-btn--icon g-press" aria-label="Предыдущий период" onClick={() => onChange(shiftPeriod(period, -1))}>
            <Ico icon={IconChevronLeft} />
          </button>
          <h2 className="rep-nav__title" aria-live="polite">
            {periodTitle(period)}
          </h2>
          <button
            type="button"
            className="g-btn g-btn--icon g-press"
            aria-label="Следующий период"
            aria-disabled={!next || undefined}
            onClick={() => next && onChange(shiftPeriod(period, 1))}
          >
            <Ico icon={IconChevronRight} />
          </button>
        </div>
      )}
    </section>
  );
}
