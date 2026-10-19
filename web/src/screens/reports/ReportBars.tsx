// Собственные диаграммы отчётов (ФТ-10.2.5): список с полоской доли и столбики по месяцам.
// Без библиотек и без инлайновых стилей в разметке: доля задаётся переменной --lv
// (React пишет её через CSSOM — политика безопасности это допускает), рисует scaleX/scaleY.
import type { CSSProperties } from 'react';
import { formatMoney } from '../../lib/format';
import { barLevels, compactMoney, labelStep, MAX_VALUE_LABELS, monthFull, monthLabel, percentText, shareOf, visibleLevel } from '../../lib/reports';

const lv = (level: number) => ({ '--lv': level.toFixed(3) }) as CSSProperties;

export type ShareRow = {
  key: string | number;
  name: string;
  /** Что показать справа: «2 380 ₽», «100 мл». */
  valueText: string;
  /** Доля полоски: число, из которого считается доля от total; 0 — полоски нет. */
  value: number;
  /** Подпись под названием; доля в процентах добавляется сама. */
  sub?: string;
  /** Класс цвета категории: cat--blue. */
  catClass?: string;
};

/** Список с горизонтальной полоской доли; полоски вырастают по очереди. */
export function ShareList({ rows, total, label }: { rows: readonly ShareRow[]; total: number; label: string }) {
  return (
    <ul className="g-list rep-list" aria-label={label}>
      {rows.map((r) => {
        const share = shareOf(r.value, total);
        return (
          <li key={r.key} className={`rep-share${r.catClass ? ` ${r.catClass}` : ''}`}>
            <div className="rep-share__head">
              <span className="rep-share__name">{r.name}</span>
              <span className="g-num rep-share__val">{r.valueText}</span>
            </div>
            <div className="rep-track" aria-hidden="true">
              <span className="rep-fill" style={lv(visibleLevel(share))} />
            </div>
            <div className="g-row__sub">{[r.value > 0 ? percentText(share) : '', r.sub].filter(Boolean).join(' · ')}</div>
          </li>
        );
      })}
    </ul>
  );
}

export type MonthValue = { month: string; value: number };

/** Столбики по месяцам; значения над столбиками — пока их не больше MAX_VALUE_LABELS. */
export function MonthBars({ items, label }: { items: readonly MonthValue[]; label: string }) {
  const levels = barLevels(items.map((i) => i.value));
  const step = labelStep(items.length);
  const withYear = items.length > 12;
  const showValues = items.length <= MAX_VALUE_LABELS;
  return (
    <figure className="rep-months">
      <ul className="rep-months__plot" aria-label={label}>
        {items.map((it, i) => (
          <li key={it.month} className="rep-col">
            <span className="rep-col__val" aria-hidden="true">
              {showValues && it.value > 0 ? compactMoney(it.value) : ''}
            </span>
            <span className="rep-col__bar" aria-hidden="true">
              <span className="rep-col__fill" style={lv(levels[i] ?? 0)} />
            </span>
            <span className="rep-col__lab" aria-hidden="true">
              {i % step === 0 ? monthLabel(it.month, withYear) : ''}
            </span>
            <span className="g-sr">
              {monthFull(it.month)}: {formatMoney(it.value)}
            </span>
          </li>
        ))}
      </ul>
      <figcaption className="g-muted rep-months__cap">{showValues ? 'Суммы над столбиками — в рублях' : 'Высота столбика — сумма за месяц'}</figcaption>
    </figure>
  );
}
