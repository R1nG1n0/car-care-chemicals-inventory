// Профиль → Погода (ФТ-16.1.2): текущее место, поиск города, выбор из найденных, сброс к месту по умолчанию.
// Правка места — только со связью (ФТ-12.2.3).
import { IconMapPin, IconSearch } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { errorText, isApiError, weather, type WeatherPlace } from '../../api';
import { keys } from '../../app/queries';
import { useNeedsOnline } from '../../app/useOffline';
import { busyProps, Ico } from '../../components/ui';
import { placeLabel, placeQuery, placesErrorText, PLACE_DEBOUNCE_MS } from '../../lib/weather';
import { useWeather } from '../weather/useWeather';

/** Значение, которое следует за вводом с задержкой. */
function useDebounced(value: string, ms: number): string {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setV(value), ms);
    return () => window.clearTimeout(t);
  }, [value, ms]);
  return v;
}

export function WeatherPlaceSection() {
  const client = useQueryClient();
  const online = useNeedsOnline();
  const forecast = useWeather();
  const [text, setText] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const query = placeQuery(useDebounced(text, PLACE_DEBOUNCE_MS));
  const typing = placeQuery(text) !== query;

  const places = useQuery({
    queryKey: keys.weatherPlaces(query),
    queryFn: ({ signal }) => weather.places(query, signal),
    enabled: query !== '' && !online.offline,
    staleTime: 5 * 60_000,
    retry: false,
  });

  const done = (msg: string) => {
    setText('');
    setError('');
    setNotice(msg);
    void client.invalidateQueries({ queryKey: keys.weather });
  };
  const pick = useMutation({
    mutationFn: (p: WeatherPlace) => weather.setLocation({ name: p.name, lat: p.lat, lon: p.lon }),
    onSuccess: (loc) => done(`Место: ${loc.name}`),
    onError: (err) => setError(errorText(err)),
  });
  const reset = useMutation({
    mutationFn: weather.resetLocation,
    onSuccess: () => done('Возвращено место по умолчанию'),
    onError: (err) => setError(errorText(err)),
  });

  const w = forecast.data;
  if (forecast.isPending) return null;
  // Погода не настроена на сервере и своего места нет — раздел не нужен (ФТ-16.1.4)
  if (w && !w.enabled && !w.location) return null;
  const loc = w?.location ?? null;
  const searchError = places.isError ? (isApiError(places.error) ? placesErrorText(places.error.status, places.error.retryAfter, errorText(places.error)) : errorText(places.error)) : '';
  const list = places.data ?? [];

  return (
    <section className="g-section" aria-labelledby="pf-wx">
      <span className="g-label" id="pf-wx">
        Погода
      </span>
      <div className="g-row">
        <div className="g-row__main">
          <div className="g-row__sub">{loc ? (loc.custom ? 'Моё место' : 'Место по умолчанию') : 'Место'}</div>
          <div className="g-row__title">{loc ? loc.name : 'Не выбрано'}</div>
        </div>
        <Ico icon={IconMapPin} />
      </div>
      <div className="g-search">
        <span className="g-search__icon">
          <Ico icon={IconSearch} />
        </span>
        <input
          className="g-input"
          type="search"
          placeholder="Найти город"
          aria-label="Найти город для прогноза"
          autoComplete="off"
          enterKeyHint="search"
          value={text}
          disabled={online.offline}
          onChange={(e) => {
            setText(e.target.value);
            setNotice('');
          }}
        />
      </div>
      {online.hint && <p className="g-field__hint">{online.hint}: место можно менять только онлайн.</p>}
      <p className="g-field__hint" role="status">
        {notice || (query && places.isFetching ? 'Ищу…' : query && !typing && places.isSuccess && list.length === 0 ? 'Ничего не найдено. Проверьте название.' : '')}
      </p>
      {list.length > 0 && !typing && (
        <div className="g-list g-enter" role="list" aria-label="Найденные города">
          {list.map((p) => (
            <button key={`${p.lat},${p.lon}`} type="button" role="listitem" className="g-row g-row--link app-row-btn" {...busyProps(pick.isPending)} onClick={() => !pick.isPending && pick.mutate(p)}>
              <span className="g-row__main g-row__title">{placeLabel(p)}</span>
            </button>
          ))}
        </div>
      )}
      {loc?.custom && (
        <button type="button" className="g-btn g-btn--ghost g-btn--block" {...online.props} {...busyProps(reset.isPending)} onClick={online.guard(() => !reset.isPending && reset.mutate())}>
          Сбросить к месту по умолчанию
        </button>
      )}
      {(error || searchError) && (
        <p className="g-field__error" role="alert">
          {error || searchError}
        </p>
      )}
    </section>
  );
}
