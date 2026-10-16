import { describe, expect, it } from 'vitest';
import { charCount, loginError, passwordError, validateLogin, validatePasswordChange, validateSignup, validateSteps } from './validate';

describe('логин', () => {
  it.each(['abc', 'ringino', 'a.b-c_d', 'A1'.padEnd(32, 'x')])('подходит: %s', (s) => expect(loginError(s)).toBeUndefined());
  it.each(['', 'ab', 'x'.repeat(33), 'с кириллицей', 'a b', 'a@b'])('не подходит: «%s»', (s) => expect(loginError(s)).toBeDefined());
});

describe('пароль', () => {
  it('8–128 символов', () => {
    expect(passwordError('1234567')).toBe('Не короче 8 символов');
    expect(passwordError('12345678')).toBeUndefined();
    expect(passwordError('x'.repeat(128))).toBeUndefined();
    expect(passwordError('x'.repeat(129))).toBe('Не длиннее 128 символов');
    expect(passwordError('')).toBe('Введите пароль');
  });
  it('считаются символы, а не байты и не UTF-16', () => {
    expect(charCount('пароль😀')).toBe(7);
    expect(passwordError('пароль😀')).toBe('Не короче 8 символов');
    expect(passwordError('пароль😀😀')).toBeUndefined();
  });
});

describe('формы', () => {
  it('вход — только непустые поля', () => {
    expect(validateLogin({ login: '', password: '' })).toEqual({ login: 'Введите логин', password: 'Введите пароль' });
    expect(validateLogin({ login: 'x', password: '1' })).toEqual({});
  });

  it('приглашение — повтор пароля', () => {
    expect(validateSignup({ login: 'friend', password: 'secret123', repeat: 'secret124' })).toEqual({ repeat: 'Пароли не совпадают' });
    expect(validateSignup({ login: 'friend', password: 'secret123', repeat: 'secret123' })).toEqual({});
    // Короткий пароль — ошибка у пароля, повтор не ругаем.
    expect(validateSignup({ login: 'fr', password: '1', repeat: '2' })).toEqual({
      login: 'От 3 до 32 символов: латиница, цифры, _ . -',
      password: 'Не короче 8 символов',
    });
  });

  it('смена пароля — ключи совпадают с полями API (current, new)', () => {
    expect(validatePasswordChange({ current: '', next: 'short', repeat: '' })).toEqual({
      current: 'Введите текущий пароль',
      new: 'Не короче 8 символов',
    });
    expect(validatePasswordChange({ current: 'old', next: 'newpass12', repeat: 'newpass1' })).toEqual({ repeat: 'Пароли не совпадают' });
    expect(validatePasswordChange({ current: 'old', next: 'newpass12', repeat: 'newpass12' })).toEqual({});
  });
});

describe('шаги кнопок количества', () => {
  it('корректные', () => expect(validateSteps('10', ' 50 ')).toEqual({ ok: true, value: { step_small: 10, step_large: 50 } }));
  it('не числа и дроби', () => {
    expect(validateSteps('', '5.5')).toEqual({ ok: false, errors: { step_small: 'Целое число', step_large: 'Целое число' } });
    expect(validateSteps('-1', '50')).toEqual({ ok: false, errors: { step_small: 'Целое число' } });
  });
  it('границы', () => {
    expect(validateSteps('0', '5001')).toEqual({ ok: false, errors: { step_small: 'От 1 до 1000', step_large: 'От 1 до 5000' } });
    expect(validateSteps('1000', '5000')).toEqual({ ok: true, value: { step_small: 1000, step_large: 5000 } });
  });
  it('малый должен быть меньше большого', () => {
    expect(validateSteps('50', '50')).toEqual({ ok: false, errors: { step_large: 'Больше малого шага' } });
  });
});
