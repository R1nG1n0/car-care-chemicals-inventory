import { describe, expect, it } from 'vitest';
import { formatMoney, formatQty } from './format';

// Разделитель разрядов в ru-RU — неразрывный пробел; сравниваем через \s,
// чтобы тест не зависел от того, какой именно пробел даёт ICU.
const norm = (s: string) => s.replace(/\s/gu, ' ');

describe('formatQty', () => {
  it('разделяет разряды', () => {
    expect(norm(formatQty(2320))).toBe('2 320');
    expect(norm(formatQty(1000000))).toBe('1 000 000');
  });
  it('не трогает малые числа и ноль', () => {
    expect(formatQty(40)).toBe('40');
    expect(formatQty(0)).toBe('0');
  });
});

describe('formatMoney', () => {
  it('целые рубли — без копеек', () => {
    expect(norm(formatMoney(119000))).toBe('1 190 ₽');
  });
  it('копейки — двумя знаками', () => {
    expect(norm(formatMoney(11950))).toBe('119,50 ₽');
  });
});
