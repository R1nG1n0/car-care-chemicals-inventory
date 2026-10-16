import { describe, expect, it } from 'vitest';
import { inviteMeta, plural, retryAfterText, tooManyAttemptsText, validForText } from './text';

const M = ['минуту', 'минуты', 'минут'] as const;

describe('plural', () => {
  it.each([
    [1, 'минуту'], [2, 'минуты'], [4, 'минуты'], [5, 'минут'], [11, 'минут'], [12, 'минут'],
    [14, 'минут'], [21, 'минуту'], [22, 'минуты'], [25, 'минут'], [101, 'минуту'], [111, 'минут'], [0, 'минут'],
  ])('%i → %s', (n, want) => expect(plural(n, M)).toBe(want));
});

describe('retryAfterText', () => {
  it('720 секунд — через 12 минут', () => expect(retryAfterText(720)).toBe('через 12 минут'));
  it('неполная минута округляется вверх', () => expect(retryAfterText(61)).toBe('через 2 минуты'));
  it('меньше минуты — через 1 минуту', () => expect(retryAfterText(5)).toBe('через 1 минуту'));
  it('0 — тоже 1 минута', () => expect(retryAfterText(0)).toBe('через 1 минуту'));
  it('21 минута', () => expect(retryAfterText(21 * 60)).toBe('через 21 минуту'));
  it('текст ошибки 429', () => {
    expect(tooManyAttemptsText(900)).toBe('Слишком много попыток. Повторите через 15 минут');
    expect(tooManyAttemptsText(undefined)).toBe('Слишком много попыток. Повторите позже');
  });
});

describe('сроки приглашений', () => {
  const now = 1_792_140_000;
  const day = 86_400;
  it('ровно 7 дней', () => expect(validForText(now + 7 * day, now)).toBe('действует ещё 7 дней'));
  it('неполные сутки — 1 день', () => expect(validForText(now + 3600, now)).toBe('действует ещё 1 день'));
  it('2,5 дня — 3 дня', () => expect(validForText(now + 2.5 * day, now)).toBe('действует ещё 3 дня'));

  it('строка активного', () => {
    expect(inviteMeta({ created_at: now - day, expires_at: now + 6 * day, status: 'active' }, now)).toMatch(/^Создано \d\d\.\d\d · действует ещё 6 дней$/);
  });
  it('активное, но срок прошёл — истекло', () => {
    expect(inviteMeta({ created_at: now - 8 * day, expires_at: now - day, status: 'active' }, now)).toMatch(/· истекло \d\d\.\d\d$/);
  });
  it('использовано и отозвано', () => {
    expect(inviteMeta({ created_at: now, expires_at: now + day, status: 'used' }, now)).toMatch(/· использовано$/);
    expect(inviteMeta({ created_at: now, expires_at: now + day, status: 'revoked' }, now)).toMatch(/· отозвано$/);
  });
});
