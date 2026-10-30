// Поиск города (геокодер Open-Meteo через сервер, ФТ-16.1.2): поле с задержкой,
// список найденных, состояние и ошибки. Что делать с выбранным городом, решает
// экран: профиль сохраняет город по умолчанию, экран погоды показывает прогноз.
import { IconSearch } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { errorText, isApiError, weather, type WeatherPlace } from '../api';
import { keys } from '../app/queries';
import { useNeedsOnline } from '../app/useOffline';
import { placeLabel, placeQuery, placesErrorText, PLACE_DEBOUNCE_MS } from '../lib/weather';
import { busyProps, Ico } from './ui';

/** Значение, которое следует за вводом с задержкой. */
function useDebounced(value: string, ms: number): string {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setV(value), ms);
    return () => window.clearTimeout(t);
  }, [value, ms]);
  return v;
}

type Props = {
  /** Выбран город из списка. */
  onPick: (p: WeatherPlace) => void;
  /** Идёт сохранение выбора — кнопки списка заняты. */
  busy?: boolean;
  label: string;
  /** Подпись под полем без связи. */
  offlineText: string;
  autoFocus?: boolean;
};

export function CitySearch({ onPick, busy = false, label, offlineText, autoFocus = false }: Props) {
  const online = useNeedsOnline();
  const [text, setText] = useState('');
  const query = placeQuery(useDebounced(text, PLACE_DEBOUNCE_MS));
  const typing = placeQuery(text) !== query;

  const places = useQuery({
    queryKey: keys.weatherPlaces(query),
    queryFn: ({ signal }) => weather.places(query, signal),
    enabled: query !== '' && !online.offline,
    staleTime: 5 * 60_000,
    retry: false,
  });

  const error = places.isError ? (isApiError(places.error) ? placesErrorText(places.error.status, places.error.retryAfter, errorText(places.error)) : errorText(places.error)) : '';
  const list = places.data ?? [];

  return (
    <>
      <div className="g-search">
        <span className="g-search__icon">
          <Ico icon={IconSearch} />
        </span>
        <input
          className="g-input"
          type="search"
          placeholder="Найти город"
          aria-label={label}
          autoComplete="off"
          enterKeyHint="search"
          // Поиск открыт нажатием «Другой город» — сразу к вводу
          autoFocus={autoFocus}
          value={text}
          disabled={online.offline}
          onChange={(e) => setText(e.target.value)}
        />
      </div>
      {online.hint && <p className="g-field__hint">{online.hint}: {offlineText}</p>}
      <p className="g-field__hint" role="status">
        {query && places.isFetching ? 'Ищу…' : query && !typing && places.isSuccess && list.length === 0 ? 'Ничего не найдено. Проверьте название.' : ''}
      </p>
      {list.length > 0 && !typing && (
        <div className="g-list g-enter" role="list" aria-label="Найденные города">
          {list.map((p) => (
            <button
              key={`${p.lat},${p.lon}`}
              type="button"
              role="listitem"
              className="g-row g-row--link app-row-btn"
              {...busyProps(busy)}
              onClick={() => {
                if (busy) return;
                setText('');
                onPick(p);
              }}
            >
              <span className="g-row__main g-row__title">{placeLabel(p)}</span>
            </button>
          ))}
        </div>
      )}
      {error && (
        <p className="g-field__error" role="alert">
          {error}
        </p>
      )}
    </>
  );
}
