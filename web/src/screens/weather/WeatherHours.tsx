// Сегодня по часам: горизонтальная лента с прокруткой внутри блока (страница не скроллится вбок).
import { IconDroplet, IconWind } from '@tabler/icons-react';
import type { Weather } from '../../api';
import { Ico } from '../../components/ui';
import { WeatherIcon } from '../../components/WeatherIcon';
import { mmText, tempText } from '../../lib/weather';

/** hours — уже отобранные для «сейчас» места (lib/weather hoursToday). */
export function WeatherHours({ hours }: { hours: Weather['today_hours'] }) {
  if (hours.length === 0) return null;
  return (
    <section className="g-section" aria-labelledby="wx-hours">
      <span className="g-label" id="wx-hours">
        Сегодня по часам
      </span>
      <ul className="wx-hours" tabIndex={0} aria-label="Прогноз по часам, прокручивается вбок">
        {hours.map((h) => (
          <li key={h.time} className="wx-hour">
            <span className="wx-hour__time">{h.time}</span>
            <WeatherIcon code={h.code} />
            <b className="wx-hour__temp">{tempText(h.temp)}</b>
            <span className="wx-hour__row" title="Осадки">
              <Ico icon={IconDroplet} size={14} />
              {h.precip_mm > 0 ? `${mmText(h.precip_mm)}\u00a0мм` : `${Math.round(h.precip_prob)}\u00a0%`}
            </span>
            <span className="wx-hour__row" title="Ветер">
              <Ico icon={IconWind} size={14} />
              {mmText(h.wind)}&nbsp;м/с
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
