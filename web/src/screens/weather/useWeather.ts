// Прогноз погоды: один запрос на банер, экран и профиль.
import { useQuery } from '@tanstack/react-query';
import { weather } from '../../api';
import { keys, WEATHER_STALE_MS } from '../../app/queries';

export function useWeather() {
  return useQuery({
    queryKey: keys.weather,
    queryFn: ({ signal }) => weather.get(signal),
    staleTime: WEATHER_STALE_MS,
    refetchOnWindowFocus: false,
  });
}
