import { describe, expect, it } from 'vitest';
import type { Weather, WeatherDay } from '../api';
import { memoryStorage } from './testStorage';
import {
  allGood, bannerLine, BANNER_COLLAPSE_KEY, betterText, dayMonth, dayName, firstReason, hasForecast, levelText, mmText, placeLabel,
  placeQuery, placesErrorText, precipText, readCollapsed, tempText, updatedText, weatherKind, weekdayOf, writeCollapsed, upcomingHours, placeNow, liveWeather, isOutdated, hoursToday, dataNote } from './weather';

const day = (date: string, wash: 'good' | 'meh' | 'bad', coat: 'good' | 'meh' | 'bad', over: Partial<WeatherDay> = {}): WeatherDay => ({
  date, weekday: weekdayOf(date), t_min: 1, t_max: 9, precip_mm: 0, precip_prob: 10, code: 3,
  wash: { level: wash, reasons: wash === 'good' ? [] : ['дождь в 15:00'] },
  coat: { level: coat, reasons: coat === 'good' ? [] : ['завтра дождь'] },
  ...over,
});
const forecast = (days: WeatherDay[], best: Weather['best'] = { wash: null, coat: null }): Weather => ({
  enabled: true, location: { name: 'Донецк', lat: 48, lon: 37.8, custom: false }, fetched_at: 1_700_000_000, stale: false, days, today_hours: [], best,
});

describe('weatherKind', () => {
  it('группы кодов WMO', () => {
    expect(weatherKind(0).kind).toBe('clear');
    expect(weatherKind(2).kind).toBe('partly');
    expect(weatherKind(3).kind).toBe('cloud');
    expect(weatherKind(45).kind).toBe('fog');
    expect(weatherKind(53).kind).toBe('drizzle');
    expect(weatherKind(63).kind).toBe('rain');
    expect(weatherKind(81).kind).toBe('rain');
    expect(weatherKind(73).kind).toBe('snow');
    expect(weatherKind(86).kind).toBe('snow');
    expect(weatherKind(95).kind).toBe('storm');
  });
  it('неизвестный код — облачно', () => {
    expect(weatherKind(1234)).toEqual({ kind: 'cloud', label: 'Облачно' });
  });
});

describe('тексты', () => {
  it('levelText', () => {
    expect(levelText('wash', 'good')).toBe('мойка: хорошо');
    expect(levelText('coat', 'meh')).toBe('покрытие: так себе');
    expect(levelText('coat', 'bad')).toBe('покрытие: плохо');
  });
  it('день недели и дата', () => {
    expect(weekdayOf('2026-10-30')).toBe('пт');
    expect(weekdayOf('мусор')).toBe('');
    expect(dayMonth('2026-10-30')).toBe('30.10');
    expect(dayMonth('x')).toBe('x');
  });
  it('температура с настоящим минусом, без «−0»', () => {
    expect(tempText(-3.4)).toBe('−3°');
    expect(tempText(4.6)).toBe('5°');
    expect(tempText(-0.2)).toBe('0°');
  });
  it('осадки', () => {
    expect(mmText(2.54)).toBe('2,5');
    expect(mmText(3)).toBe('3');
    expect(precipText(0, 0)).toBe('без осадков');
    expect(precipText(2.5, 60)).toBe('2,5\u00a0мм\u00a0·\u00a060\u00a0%');
  });
  it('место', () => {
    expect(placeLabel({ name: 'Донецк', admin1: 'Донецкая область', country: 'Украина', lat: 0, lon: 0 })).toBe('Донецк — Донецкая область, Украина');
    expect(placeLabel({ name: 'Москва', admin1: 'Москва', country: 'Россия', lat: 0, lon: 0 })).toBe('Москва — Россия');
    expect(placeLabel({ name: 'Х', lat: 0, lon: 0 })).toBe('Х');
  });
  it('обновлено', () => {
    expect(updatedText(null, false)).toBe('');
    expect(updatedText(null, true)).toBe('Прогноз мог устареть');
    expect(updatedText(1_700_000_000, false)).toMatch(/^обновлено \d{2}:\d{2}$/u);
    expect(updatedText(1_700_000_000, true)).toMatch(/прогноз мог устареть$/u);
  });
});

describe('плашка', () => {
  it('выключено или нет дней — плашки нет', () => {
    expect(bannerLine(undefined)).toBeNull();
    expect(bannerLine({ ...forecast([day('2026-10-30', 'good', 'good')]), enabled: false })).toBeNull();
    expect(hasForecast(forecast([]))).toBe(false);
  });
  it('сегодня всё хорошо — без причины и «Лучше»', () => {
    const w = forecast([day('2026-10-30', 'good', 'good'), day('2026-10-31', 'good', 'good')], { wash: '2026-10-30', coat: '2026-10-30' });
    expect(bannerLine(w)).toMatchObject({ reason: '', better: '' });
    expect(allGood(w.days[0]!)).toBe(true);
  });
  it('мойка плохо, покрытие хорошо: причина мойки, «Лучше» только мойка', () => {
    const w = forecast([day('2026-10-30', 'bad', 'good'), day('2026-11-01', 'good', 'good')], { wash: '2026-11-01', coat: '2026-10-30' });
    const b = bannerLine(w)!;
    expect(b.reason).toBe('дождь в 15:00');
    expect(b.better).toBe('Лучше: мойка — вс');
  });
  it('оба плохо — обе части, дни из ответа сервера', () => {
    const days = [day('2026-10-30', 'bad', 'meh'), { ...day('2026-11-03', 'good', 'good'), weekday: 'вт' }, day('2026-11-01', 'good', 'good')];
    const w = forecast(days, { wash: '2026-11-01', coat: '2026-11-03' });
    expect(betterText(w)).toBe('мойка — вс, покрытие — вт');
  });
  it('хорошего дня нет — «Лучше» пусто', () => {
    expect(bannerLine(forecast([day('2026-10-30', 'bad', 'bad')]))!.better).toBe('');
  });
  it('первая причина: мойка раньше покрытия, пропуская «хорошо»', () => {
    expect(firstReason(day('2026-10-30', 'good', 'bad'))).toBe('завтра дождь');
    expect(firstReason(day('2026-10-30', 'bad', 'bad'))).toBe('дождь в 15:00');
    expect(firstReason(day('2026-10-30', 'good', 'good'))).toBe('');
  });
  it('dayName: запасной путь без дня в списке', () => {
    expect(dayName([], '2026-11-01')).toBe('вс');
  });
});

describe('свёрнутая плашка', () => {
  it('запоминается', () => {
    const s = memoryStorage();
    expect(readCollapsed(s)).toBe(false);
    writeCollapsed(true, s);
    expect(s.data.get(BANNER_COLLAPSE_KEY)).toBe('1');
    expect(readCollapsed(s)).toBe(true);
    writeCollapsed(false, s);
    expect(readCollapsed(s)).toBe(false);
  });
  it('хранилище бросает или отсутствует — не падаем', () => {
    const broken = { getItem: () => { throw new Error('x'); }, setItem: () => { throw new Error('x'); } };
    expect(readCollapsed(broken)).toBe(false);
    expect(() => writeCollapsed(true, broken)).not.toThrow();
    expect(readCollapsed(null)).toBe(false);
    expect(() => writeCollapsed(true, null)).not.toThrow();
  });
});

describe('поиск города', () => {
  it('от двух символов, пробелы обрезаются', () => {
    expect(placeQuery('д')).toBe('');
    expect(placeQuery('  д ')).toBe('');
    expect(placeQuery(' До ')).toBe('До');
  });
  it('429 — понятное сообщение', () => {
    expect(placesErrorText(429, 7.2, 'x')).toBe('Слишком частые запросы. Повторите через 8\u00a0с.');
    expect(placesErrorText(429, undefined, 'x')).toMatch(/Слишком частые/u);
    expect(placesErrorText(500, undefined, 'сбой')).toBe('сбой');
  });
});

describe('лента часов', () => {
  const h = (t: string) => ({ time: t });
  const day = ['00:00', '09:00', '14:00', '15:00', '23:00'].map(h);
  it('с текущего часа, прошедшие убираются', () => {
    expect(upcomingHours(day, 14).map((x) => x.time)).toEqual(['14:00', '15:00', '23:00']);
    expect(upcomingHours(day, 0)).toHaveLength(5);
  });
  it('все прошли — последние три', () => {
    expect(upcomingHours(day.slice(0, 4), 23).map((x) => x.time)).toEqual(['09:00', '14:00', '15:00']);
  });
});

describe('время места', () => {
  const at = (iso: string) => Date.parse(iso);
  it('placeNow считает по смещению места', () => {
    expect(placeNow(3 * 3600, at('2026-10-30T22:30:00Z'))).toEqual({ date: '2026-10-31', hour: 1 });
    expect(placeNow(-5 * 3600, at('2026-10-30T02:00:00Z'))).toEqual({ date: '2026-10-29', hour: 21 });
    expect(placeNow(0, at('2026-10-30T12:00:00Z'))).toEqual({ date: '2026-10-30', hour: 12 });
  });
  it('нет смещения — часы устройства', () => {
    const now = new Date(2026, 9, 30, 15, 5).getTime();
    expect(placeNow(undefined, now)).toEqual({ date: '2026-10-30', hour: 15 });
  });
  const w = (over: Partial<Weather> = {}) => ({ ...forecast([day('2026-10-29', 'good', 'good'), day('2026-10-30', 'bad', 'good'), day('2026-11-01', 'good', 'good')], { wash: '2026-10-29', coat: '2026-11-01' }), utc_offset: 0, ...over });
  it('старые дни отбрасываются, best на ушедший день сбрасывается', () => {
    const live = liveWeather(w(), at('2026-10-30T10:00:00Z'))!;
    expect(live.days.map((d) => d.date)).toEqual(['2026-10-30', '2026-11-01']);
    expect(live.best).toEqual({ wash: null, coat: '2026-11-01' });
    expect(bannerLine(w(), at('2026-10-30T10:00:00Z'))!.lead).toBe('Сегодня');
  });
  it('сегодняшнего дня нет — подпись дня недели', () => {
    expect(bannerLine(w(), at('2026-10-31T10:00:00Z'))!.lead).toBe('вс');
  });
  it('всё устарело — плашки нет', () => {
    const now = at('2026-11-05T10:00:00Z');
    expect(liveWeather(w(), now)!.days).toEqual([]);
    expect(isOutdated(w(), now)).toBe(true);
    expect(bannerLine(w(), now)).toBeNull();
    expect(isOutdated(forecast([]), now)).toBe(false);
  });
  it('часы — только за сегодня места', () => {
    const hrs = ['09:00', '14:00', '20:00'].map((t) => ({ time: t, temp: 1, precip_mm: 0, precip_prob: 0, wind: 1, humidity: 1, code: 0 }));
    const base = { ...w(), today_hours: hrs };
    // первый день снимка — 29.10, сейчас 30.10: часы вчерашние
    expect(hoursToday(base, at('2026-10-30T10:00:00Z'))).toEqual([]);
    const fresh = { ...base, days: base.days.slice(1) };
    expect(hoursToday(fresh, at('2026-10-30T12:00:00Z')).map((x) => x.time)).toEqual(['14:00', '20:00']);
    // пояс места: в UTC ещё 30.10 22:00, у места +3 уже 31.10
    expect(hoursToday({ ...fresh, utc_offset: 3 * 3600 }, at('2026-10-30T22:00:00Z'))).toEqual([]);
  });
  it('приписка «данные на …» при stale или старше 3 часов', () => {
    const fetched = at('2026-10-30T08:00:00Z') / 1000;
    const base = { ...w(), fetched_at: fetched };
    expect(dataNote(base, at('2026-10-30T10:00:00Z'))).toBe('');
    expect(dataNote(base, at('2026-10-30T11:30:00Z'))).toMatch(/^данные на \d{2}\.\d{2} \d{2}:\d{2}$/u);
    expect(dataNote({ ...base, stale: true }, at('2026-10-30T08:10:00Z'))).toMatch(/^данные на/u);
    expect(dataNote({ ...base, fetched_at: null, stale: true }, 0)).toBe('');
  });
});
