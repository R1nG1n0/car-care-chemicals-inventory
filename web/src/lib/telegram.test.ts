import { describe, expect, it } from 'vitest';
import type { TelegramStatus } from '../api';
import { formatDate, linkAlive, linkedText, pollInterval, shouldPoll, telegramView } from './telegram';

const st = (over: Partial<TelegramStatus> = {}): TelegramStatus => ({ enabled: true, bot_username: 'garage_bot', linked: false, telegram_name: null, linked_at: null, ...over });

describe('telegramView', () => {
  it('три состояния', () => {
    expect(telegramView(st({ enabled: false, bot_username: null }))).toBe('disabled');
    expect(telegramView(st())).toBe('unlinked');
    expect(telegramView(st({ linked: true, telegram_name: 'Данил' }))).toBe('linked');
  });
});

describe('бот ещё подключается', () => {
  it('enabled без имени бота — pending, опрос раз в 10 с', () => {
    const s = st({ bot_username: null });
    expect(telegramView(s)).toBe('pending');
    expect(pollInterval(null, s, 0)).toBe(10_000);
    expect(pollInterval(null, st({ bot_username: 'b' }), 0)).toBe(false);
  });
  it('выключенный бот — disabled, даже без имени', () => {
    expect(telegramView(st({ enabled: false, bot_username: null }))).toBe('disabled');
  });
  it('пока ссылка жива, опрос — 3 с', () => {
    expect(pollInterval(100, st(), 0)).toBe(3000);
  });
});

describe('опрос', () => {
  const now = 1_000;
  it('идёт, пока ссылка жива и привязки нет', () => {
    expect(shouldPoll(now + 600, st(), now)).toBe(true);
  });
  it('стоит без ссылки, после срока и после привязки', () => {
    expect(shouldPoll(null, st(), now)).toBe(false);
    expect(shouldPoll(now, st(), now)).toBe(false);
    expect(shouldPoll(now + 600, st({ linked: true }), now)).toBe(false);
    expect(shouldPoll(now + 600, undefined, now)).toBe(false);
  });
  it('linkAlive: ровно в срок — уже нет', () => {
    expect(linkAlive(now + 1, now)).toBe(true);
    expect(linkAlive(now, now)).toBe(false);
  });
});

describe('тексты', () => {
  const at = Date.UTC(2026, 9, 19, 12) / 1000;
  it('дата ДД.ММ.ГГГГ', () => {
    expect(formatDate(at)).toBe('19.10.2026');
  });
  it('«имя · с даты»', () => {
    expect(linkedText(st({ linked: true, telegram_name: 'Данил', linked_at: at }))).toBe('Данил · с 19.10.2026');
    expect(linkedText(st({ linked: true, telegram_name: null, linked_at: null }))).toBe('Telegram');
  });
});
