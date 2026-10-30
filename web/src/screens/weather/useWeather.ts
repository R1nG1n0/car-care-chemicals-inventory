// Прогноз погоды: один запрос на банер, экран и профиль. С point — прогноз
// выбранного на экране города (отдельный кэш, место по умолчанию не трогает).
import { useQuery } from '@tanstack/react-query';
import { weather, type WeatherPoint } from '../../api';
import { keys, WEATHER_STALE_MS } from '../../app/queries';

export function useWeather(point?: WeatherPoint | null) {
  return useQuery({
    queryKey: point ? keys.weatherAt(point.lat, point.lon, point.name) : keys.weather,
    queryFn: ({ signal }) => weather.get(signal, point ?? undefined),
    staleTime: WEATHER_STALE_MS,
    refetchOnWindowFocus: false,
  });
}
