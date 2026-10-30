// Экран «Погода» (ФТ-16.3.2): место и время обновления, 7 дней с оценками, сегодня по часам.
// «Другой город» — прогноз любого города (адрес ?lat=&lon=&name=), город по умолчанию
// при этом не меняется; его можно сделать городом по умолчанию одной кнопкой.
import { IconCloudOff, IconMapPin, IconMapSearch } from '@tabler/icons-react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { errorText, isApiError, weather, weatherPointQuery, type WeatherPlace, type WeatherPoint } from '../../api';
import { keys } from '../../app/queries';
import { useNeedsOnline } from '../../app/useOffline';
import { CitySearch } from '../../components/CitySearch';
import { useOnline } from '../../app/session';
import { BackLink } from '../../components/BackLink';
import { busyProps, Empty, Ico, LoadError, SkelList } from '../../components/ui';
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

/** Город из адреса экрана; неполные или неверные параметры — город по умолчанию. */
function pointFrom(params: URLSearchParams): WeatherPoint | null {
  const lat = Number.parseFloat(params.get('lat') ?? '');
  const lon = Number.parseFloat(params.get('lon') ?? '');
  const name = (params.get('name') ?? '').trim();
  if (!Number.isFinite(lat) || !Number.isFinite(lon) || Math.abs(lat) > 90 || Math.abs(lon) > 180 || !name) return null;
  return { name, lat, lon };
}

export function WeatherScreen() {
  const [params, setParams] = useSearchParams();
  const point = pointFrom(params);
  const q = useWeather(point);
  const client = useQueryClient();
  const needs = useNeedsOnline();
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState('');
  const show = (p: WeatherPlace) => {
    setSearching(false);
    setError('');
    setParams(new URLSearchParams(weatherPointQuery({ name: p.name, lat: p.lat, lon: p.lon })));
  };
  const makeDefault = useMutation({
    mutationFn: (p: WeatherPoint) => weather.setLocation(p),
    onSuccess: () => {
      setError('');
      void client.invalidateQueries({ queryKey: keys.weather });
      setParams(new URLSearchParams());
    },
    onError: (err) => setError(errorText(err)),
  });
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
              {w.preview ? '' : ' (по умолчанию)'}
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
      <div className="wx-city">
        {searching ? (
          <>
            <CitySearch label="Город для прогноза" offlineText="другой город можно посмотреть только онлайн." onPick={show} autoFocus />
            <button type="button" className="g-btn g-btn--ghost g-btn--sm" onClick={() => setSearching(false)}>
              Отмена
            </button>
          </>
        ) : (
          <div className="wx-city__actions">
            <button type="button" className="g-btn g-btn--sm g-press" {...needs.props} onClick={needs.guard(() => setSearching(true))}>
              <Ico icon={IconMapSearch} /> Другой город
            </button>
            {point && (
              <>
                <button
                  type="button"
                  className="g-btn g-btn--sm g-press"
                  {...needs.props}
                  {...busyProps(makeDefault.isPending)}
                  onClick={needs.guard(() => !makeDefault.isPending && makeDefault.mutate(point))}
                >
                  Сделать городом по умолчанию
                </button>
                <button type="button" className="g-btn g-btn--ghost g-btn--sm" onClick={() => setParams(new URLSearchParams())}>
                  К городу по умолчанию
                </button>
              </>
            )}
          </div>
        )}
        {error && (
          <p className="g-field__error" role="alert">
            {error}
          </p>
        )}
      </div>
      {body}
    </div>
  );
}
