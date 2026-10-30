// Значки погоды (Tabler) по коду WMO и уровни оценки: форма разная, не только цвет.
import { IconAlertTriangle, IconCircleCheck, IconCircleX, IconCloud, IconCloudFog, IconCloudRain, IconCloudSnow, IconCloudStorm, IconDroplet, IconSun, type TablerIcon } from '@tabler/icons-react';
import type { WeatherLevel } from '../api';
import { weatherKind, type WeatherKind } from '../lib/weather';
import { Ico } from './ui';

const ICONS: Record<WeatherKind, TablerIcon> = {
  clear: IconSun,
  partly: IconCloud,
  cloud: IconCloud,
  fog: IconCloudFog,
  drizzle: IconDroplet,
  rain: IconCloudRain,
  snow: IconCloudSnow,
  storm: IconCloudStorm,
};

const LEVEL_ICONS: Record<WeatherLevel, TablerIcon> = { good: IconCircleCheck, meh: IconAlertTriangle, bad: IconCircleX };

/** Значок погоды; подпись — для экранных дикторов. */
export function WeatherIcon({ code, size = 24 }: { code: number; size?: number }) {
  const { kind, label } = weatherKind(code);
  return (
    <span role="img" aria-label={label} title={label} className="wx-ico">
      <Ico icon={ICONS[kind]} size={size} />
    </span>
  );
}

/** Значок уровня оценки: галочка / треугольник / крест. Подпись рядом с ним. */
export function LevelIcon({ level, size = 20 }: { level: WeatherLevel; size?: number }) {
  return <Ico icon={LEVEL_ICONS[level]} size={size} />;
}
