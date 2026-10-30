// Семь дней: день недели, дата, мин/макс, осадки, две оценки с причинами.
import type { Weather, WeatherDay, WeatherVerdict } from '../../api';
import { LevelIcon, WeatherIcon } from '../../components/WeatherIcon';
import { dayMonth, dayName, levelText, precipText, tempText, VERDICT_TITLE, type VerdictKind } from '../../lib/weather';

function Verdict({ kind, v, best }: { kind: VerdictKind; v: WeatherVerdict; best: boolean }) {
  return (
    <li className={`wx-verdict wx-verdict--${v.level}`}>
      <LevelIcon level={v.level} />
      <span className="wx-verdict__main">
        <span className="wx-verdict__title">
          <span aria-hidden="true">{VERDICT_TITLE[kind]}</span>
          <span className="wx-sr">{levelText(kind, v.level)}</span>
          {best && <span className="g-badge g-badge--accent wx-verdict__best">лучший день</span>}
        </span>
        {v.reasons.length > 0 && <span className="wx-verdict__why">{v.reasons.join('; ')}</span>}
      </span>
    </li>
  );
}

function Day({ day, w, today }: { day: WeatherDay; w: Weather; today: boolean }) {
  return (
    <li className="wx-day">
      <div className="wx-day__head">
        <div className="wx-day__name">
          <b>{dayName(w.days, day.date)}</b>
          <span className="wx-day__date">{dayMonth(day.date)}</span>
          {today && <span className="g-badge">сегодня</span>}
        </div>
        <WeatherIcon code={day.code} />
        <div className="wx-day__temp">
          <b>{tempText(day.t_max)}</b>
          <span>{tempText(day.t_min)}</span>
        </div>
      </div>
      <div className="wx-day__precip">Осадки: {precipText(day.precip_mm, day.precip_prob)}</div>
      <ul className="wx-verdicts" aria-label="Оценки дня">
        <Verdict kind="wash" v={day.wash} best={w.best.wash === day.date} />
        <Verdict kind="coat" v={day.coat} best={w.best.coat === day.date} />
      </ul>
    </li>
  );
}

/** today — дата «сегодня» места: бейдж только у равного ей дня. */
export function WeatherDays({ w, today }: { w: Weather; today: string }) {
  return (
    <ol className="wx-days g-stagger" aria-label="Прогноз на неделю">
      {w.days.map((d) => (
        <Day key={d.date} day={d} w={w} today={d.date === today} />
      ))}
    </ol>
  );
}
