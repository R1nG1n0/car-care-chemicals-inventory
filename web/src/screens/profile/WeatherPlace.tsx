// Профиль → Погода (ФТ-16.1.2): город по умолчанию — для плашки на главной,
// экрана погоды и бота. Поиск, выбор из найденных, сброс к городу из настроек
// сервера. Правка — только со связью (ФТ-12.2.3).
import { IconMapPin } from '@tabler/icons-react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { errorText, weather, type WeatherPlace } from '../../api';
import { keys } from '../../app/queries';
import { useNeedsOnline } from '../../app/useOffline';
import { CitySearch } from '../../components/CitySearch';
import { busyProps, Ico } from '../../components/ui';
import { useWeather } from '../weather/useWeather';

export function WeatherPlaceSection() {
  const client = useQueryClient();
  const online = useNeedsOnline();
  const forecast = useWeather();
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const done = (msg: string) => {
    setError('');
    setNotice(msg);
    void client.invalidateQueries({ queryKey: keys.weather });
  };
  const pick = useMutation({
    mutationFn: (p: WeatherPlace) => weather.setLocation({ name: p.name, lat: p.lat, lon: p.lon }),
    onSuccess: (loc) => done(`Город по умолчанию: ${loc.name}`),
    onError: (err) => setError(errorText(err)),
  });
  const reset = useMutation({
    mutationFn: weather.resetLocation,
    onSuccess: () => done('Возвращён город из настроек сервера'),
    onError: (err) => setError(errorText(err)),
  });

  const w = forecast.data;
  if (forecast.isPending) return null;
  // Погода не настроена на сервере и своего города нет — раздел не нужен (ФТ-16.1.4)
  if (w && !w.enabled && !w.location) return null;
  const loc = w?.location ?? null;

  return (
    <section className="g-section" aria-labelledby="pf-wx">
      <span className="g-label" id="pf-wx">
        Погода
      </span>
      <div className="g-row">
        <div className="g-row__main">
          <div className="g-row__sub">{loc ? (loc.custom ? 'Город по умолчанию' : 'Город по умолчанию · из настроек сервера') : 'Город по умолчанию'}</div>
          <div className="g-row__title">{loc ? loc.name : 'Не выбран'}</div>
        </div>
        <Ico icon={IconMapPin} />
      </div>
      <p className="g-field__hint">Его погода — на главной, на экране «Погода» и в боте. Посмотреть другой город можно на экране «Погода», не меняя этот.</p>
      <CitySearch label="Найти город по умолчанию" offlineText="город можно менять только онлайн." busy={pick.isPending} onPick={(p) => !pick.isPending && pick.mutate(p)} />
      {notice && (
        <p className="g-field__hint" role="status">
          {notice}
        </p>
      )}
      {loc?.custom && (
        <button type="button" className="g-btn g-btn--ghost g-btn--block" {...online.props} {...busyProps(reset.isPending)} onClick={online.guard(() => !reset.isPending && reset.mutate())}>
          Вернуть город из настроек сервера
        </button>
      )}
      {error && (
        <p className="g-field__error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
