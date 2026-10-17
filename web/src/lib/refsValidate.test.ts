import { describe, expect, it } from 'vitest';
import { refNameError, storeUrlError } from './refsValidate';

describe('refNameError', () => {
  it('пусто и длинно — ошибка', () => {
    expect(refNameError('  ')).toBeTruthy();
    expect(refNameError('я'.repeat(41))).toBeTruthy();
  });
  it('обычное имя, пробелы по краям не считаются', () => {
    expect(refNameError(' Коробка 3 ')).toBeUndefined();
    expect(refNameError(' ' + 'я'.repeat(40) + ' ')).toBeUndefined();
  });
});

describe('storeUrlError', () => {
  it('пусто — можно', () => expect(storeUrlError('')).toBeUndefined());
  it('http и https', () => {
    expect(storeUrlError('https://ozon.ru/x')).toBeUndefined();
    expect(storeUrlError('HTTP://a.b')).toBeUndefined();
  });
  it('без схемы и чужие схемы — ошибка', () => {
    expect(storeUrlError('ozon.ru')).toBeTruthy();
    expect(storeUrlError('javascript:alert(1)')).toBeTruthy();
    expect(storeUrlError('https://a b')).toBeTruthy();
  });
});
