// Погода для мойки и покрытий (ФТ-16): прогноз, поиск места, своё место.
import { request } from './client';

export type WeatherLevel = 'good' | 'meh' | 'bad';

/** Оценка дня для мойки или покрытия: уровень и причины («дождь в 15:00»). */
export type WeatherVerdict = { level: WeatherLevel; reasons: string[] };

export type WeatherLocation = {
  name: string;
  lat: number;
  lon: number;
  /** Своё место пользователя (не из конфигурации). */
  custom: boolean;
};

export type WeatherDay = {
  /** «2026-10-30». */
  date: string;
  /** «чт». */
  weekday: string;
  t_min: number;
  t_max: number;
  precip_mm: number;
  /** Вероятность осадков, %. */
  precip_prob: number;
  /** Код погоды WMO. */
  code: number;
  wash: WeatherVerdict;
  coat: WeatherVerdict;
};

export type WeatherHour = {
  /** «15:00». */
  time: string;
  temp: number;
  precip_mm: number;
  precip_prob: number;
  /** м/с. */
  wind: number;
  /** %. */
  humidity: number;
  code: number;
};

export type Weather = {
  /** Прогноз другого города по запросу, а не места по умолчанию. */
  preview?: boolean;
  /** false — погода не настроена: плашек и экрана нет. */
  enabled: boolean;
  location: WeatherLocation | null;
  /** Unix-секунды получения прогноза сервером. */
  fetched_at: number | null;
  /** Смещение времени места от UTC, секунды; в старых снимках поля нет. */
  utc_offset?: number;
  /** Новый прогноз получить не удалось, показан последний. */
  stale: boolean;
  days: WeatherDay[];
  today_hours: WeatherHour[];
  /** Ближайший хороший день (даты), null — на неделе такого нет. */
  best: { wash: string | null; coat: string | null };
};

export type WeatherPlace = {
  name: string;
  admin1?: string | null;
  country?: string | null;
  lat: number;
  lon: number;
};

export type WeatherPoint = { name: string; lat: number; lon: number };

/** Параметры запроса города: ?lat=&lon=&name= (их же держит адрес экрана погоды). */
export function weatherPointQuery(p: WeatherPoint): string {
  return new URLSearchParams({ lat: String(p.lat), lon: String(p.lon), name: p.name }).toString();
}

export const weather = {
  /** Прогноз места по умолчанию; с point — любого города (место по умолчанию не меняется). */
  get: (signal?: AbortSignal, point?: WeatherPoint) =>
    request<Weather>(point ? `/api/weather?${weatherPointQuery(point)}` : '/api/weather', { signal }),
  places: async (q: string, signal?: AbortSignal) =>
    (await request<{ places: WeatherPlace[] }>(`/api/weather/places?q=${encodeURIComponent(q)}`, { signal })).places,
  setLocation: (point: WeatherPoint) => request<WeatherLocation>('/api/weather/location', { method: 'PUT', body: point }),
  resetLocation: () => request<undefined>('/api/weather/location', { method: 'DELETE' }),
};
