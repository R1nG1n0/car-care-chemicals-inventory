import { describe, expect, it } from 'vitest';
import { kopecksToInput, parseRubles } from './shopMoney';

describe('parseRubles', () => {
  it('целые рубли и разряды через пробел', () => {
    expect(parseRubles('2450')).toBe(245_000);
    expect(parseRubles('2 450')).toBe(245_000);
    expect(parseRubles('2 450 ₽')).toBe(245_000);
  });
  it('копейки через запятую или точку', () => {
    expect(parseRubles('2450,5')).toBe(245_050);
    expect(parseRubles('2450.05')).toBe(245_005);
    expect(parseRubles('0,99')).toBe(99);
    expect(parseRubles('12,')).toBe(1200);
  });
  it('ноль — допустимая цена', () => {
    expect(parseRubles('0')).toBe(0);
  });
  it('пусто и мусор — null', () => {
    expect(parseRubles('')).toBeNull();
    expect(parseRubles('  ')).toBeNull();
    expect(parseRubles('abc')).toBeNull();
    expect(parseRubles('-5')).toBeNull();
    expect(parseRubles('1,234')).toBeNull();
    expect(parseRubles('1,2,3')).toBeNull();
  });
});

describe('kopecksToInput', () => {
  it('без копеек — только рубли', () => expect(kopecksToInput(245_000)).toBe('2450'));
  it('с копейками — два знака', () => {
    expect(kopecksToInput(245_050)).toBe('2450,50');
    expect(kopecksToInput(5)).toBe('0,05');
  });
  it('обратимо с parseRubles', () => {
    for (const k of [0, 1, 99, 100, 12_345, 100_000_000]) expect(parseRubles(kopecksToInput(k))).toBe(k);
  });
});
