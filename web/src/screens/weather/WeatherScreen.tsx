// Экран «Погода» (ФТ-16.3.2): место и время обновления, 7 дней с оценками, сегодня по часам.
import { IconCloudOff, IconMapPin } from '@tabler/icons-react';
import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { isApiError } from '../../api';
import { useOnline } from '../../app/session';
import { BackLink } from '../../components/BackLink';
import { Empty, Ico, LoadError, SkelList } from '../../components/ui';
import { hasForecast, hoursToday, isOutdated, liveWeather, placeNow, updatedText, WEATHER_UNAVAILABLE } from '../../lib/weather';
import { formatDay, formatTime } from '../../lib/text';
import { WeatherDays } from './WeatherDays';
import { WeatherHours } from './WeatherHours';
import { useWeather } from './useWeather';

/** Текущее время, обновляется раз в минуту: час в ленте и «сегодня» не застывают. */
function useNow(): number {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 60_000);
    return () => window.clearInterval(t);
  }, []);
  return now;
}

export function WeatherScreen() {
  const q = useWeather();
  const online = useOnline();
  const now = useNow();
  const raw = q.data;
  const w = liveWeather(raw, now);
  let body;
  if (q.isPending) {
    body = <SkelList rows={4} label="Прогноз загружается" />;
  } else if (!w) {
    body = isApiError(q.error, 502) ? (
      <Empty id="wx-down" icon={IconCloudOff} title={WEATHER_UNAVAILABLE} text="Остальное приложение работает. Попробуйте позже." actions={<button type="button" className="g-btn g-btn--block g-press" onClick={() => void q.refetch()}>Повторить</button>} />
    ) : (
      <LoadError id="wx-err" error={q.error} what="Прогноз" onRetry={() => void q.refetch()} />
    );
  } else if (!w.enabled) {
    body = <Empty id="wx-off" icon={IconCloudOff} title="Погода не настроена" text="Выберите город в профиле — прогноз появится здесь." actions={<Link className="g-btn g-btn--block g-press" to="/profile">Открыть профиль</Link>} />;
  } else if (isOutdated(raw, now)) {
    body = <Empty id="wx-old" icon={IconCloudOff} title="Прогноз устарел" text="Сохранённый прогноз закончился. Подключитесь к сети — он обновится." />;
  } else if (!hasForecast(w)) {
    body = <Empty id="wx-empty" icon={IconCloudOff} title="Прогноза пока нет" text="Сервер ещё не получил данные. Загляните через минуту." />;
  } else {
    const fetched = w.fetched_at;
    body = (
      <>
        <div className="wx-meta">
          {w.location && (
            <span className="wx-meta__place">
              <Ico icon={IconMapPin} size={18} />
              {w.location.name}
              {w.location.custom ? '' : ' (по умолчанию)'}
            </span>
          )}
          <span className="wx-meta__upd" role="status">
            {!online && fetched !== null ? `Нет связи · данные на ${formatDay(fetched)} ${formatTime(fetched * 1000)}` : updatedText(fetched, w.stale)}
          </span>
        </div>
        <WeatherHours hours={hoursToday(w, now)} />
        <WeatherDays w={w} today={placeNow(w.utc_offset, now).date} />
        <p className="wx-credit">
          <a href="https://open-meteo.com/" target="_blank" rel="noopener">
            Погода: Open-Meteo.com
          </a>
        </p>
      </>
    );
  }
  return (
    <div className="app-stack wx-screen">
      <BackLink to="/more" label="Назад" history />
      <h1 className="g-title">Погода</h1>
      {body}
    </div>
  );
}
