// Плашка погоды на главной (ФТ-16.3.1): «Сегодня: мойка ✓ · покрытие ✗», причина и «Лучше: …».
// Нажатие — экран погоды; кнопка рядом сворачивает плашку (запоминается).
// Нет данных, погода выключена или сервер ответил ошибкой — плашки нет, главную не трогаем.
import { IconChevronDown, IconChevronUp } from '@tabler/icons-react';
import { useState } from 'react';
import { Link } from 'react-router';
import type { WeatherLevel } from '../../api';
import { Ico } from '../../components/ui';
import { LevelIcon } from '../../components/WeatherIcon';
import { bannerLine, levelText, readCollapsed, tempText, VERDICT_TITLE, writeCollapsed, type VerdictKind } from '../../lib/weather';
import { useWeather } from './useWeather';

function Chip({ kind, level }: { kind: VerdictKind; level: WeatherLevel }) {
  return (
    <span className={`wx-chip wx-chip--${level}`}>
      <LevelIcon level={level} size={18} />
      <span aria-hidden="true">{VERDICT_TITLE[kind]}</span>
      <span className="wx-sr">{levelText(kind, level)}</span>
    </span>
  );
}

export function WeatherBanner() {
  const { data } = useWeather();
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const line = bannerLine(data);
  if (!line) return null;
  const { today } = line;
  const toggle = () => {
    writeCollapsed(!collapsed);
    setCollapsed(!collapsed);
  };
  return (
    <section className="wx-banner g-enter" aria-label="Погода для мойки и покрытий">
      <Link to="/weather" className="wx-banner__main">
        <span className="wx-banner__line">
          <span className="wx-banner__lead">{line.lead}:</span>
          <Chip kind="wash" level={today.wash.level} />
          <Chip kind="coat" level={today.coat.level} />
          {collapsed && <span className="wx-banner__temp">{tempText(today.t_max)}</span>}
        </span>
        {!collapsed && (line.reason || line.better || line.note) && (
          <span className="wx-banner__more g-enter">
            {line.reason && <span className="wx-banner__reason">{line.reason}</span>}
            {line.better && <span className="wx-banner__better">{line.better}</span>}
            {line.note && <span className="wx-banner__note">{line.note}</span>}
          </span>
        )}
      </Link>
      <button type="button" className="g-btn g-btn--icon g-btn--ghost wx-banner__toggle" aria-expanded={!collapsed} aria-label={collapsed ? 'Развернуть плашку погоды' : 'Свернуть плашку погоды'} onClick={toggle}>
        <Ico icon={collapsed ? IconChevronDown : IconChevronUp} />
      </button>
    </section>
  );
}
