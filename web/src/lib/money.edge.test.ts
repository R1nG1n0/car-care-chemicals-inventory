// Граничные случаи денег: рубли ↔ копейки в форме покупки,
// предел цены договора API (0–100 000 000 копеек), формат «2 450,50 ₽».
import { describe, expect, it } from 'vitest';
import { formatMoney } from './format';
import { MAX_PRICE_KOPECKS, kopecksToInput, parseRubles } from './money';

const sp = (s: string) => s.replace(/[\u00a0\u202f]/gu, ' ');

describe('parseRubles — ввод цены в покупке', () => {
  it('«2 450,50» с обычным и неразрывным пробелом', () => {
    expect(parseRubles('2 450,50')).toBe(245_050);
    expect(parseRubles('2\u00a0450,50')).toBe(245_050);
    expect(parseRubles('2\u202f450,50')).toBe(245_050);
  });
  it('пробелы по краям и знак рубля с пробелом', () => {
    expect(parseRubles('  1190  ')).toBe(119_000);
    expect(parseRubles('1 190 ₽')).toBe(119_000);
    expect(parseRubles('1190 руб.')).toBe(119_000);
    expect(parseRubles('1190 РУБ')).toBe(119_000);
  });
  it('одна цифра копеек — десятки копеек', () => {
    expect(parseRubles('0,5')).toBe(50);
    expect(parseRubles('0.05')).toBe(5);
  });
  it('отрицательные, со знаком плюс, экспонента, валюта в начале — не цена', () => {
    for (const s of ['-1', '−1', '+1', '1e3', '₽100', '1 000,000', 'Infinity', 'NaN', '0x10']) expect(parseRubles(s)).toBeNull();
  });
  it('запятая без рублей — не цена', () => expect(parseRubles(',50')).toBeNull());
  it('слишком длинное число — null, а не неточные копейки', () => {
    expect(parseRubles('9'.repeat(20))).toBeNull();
  });
  it('предел API: 1 000 000 ₽ ровно — это MAX_PRICE_KOPECKS', () => {
    expect(parseRubles('1 000 000')).toBe(MAX_PRICE_KOPECKS);
    expect(parseRubles('1 000 000,01')).toBe(MAX_PRICE_KOPECKS + 1);
  });
});

describe('kopecksToInput — цена обратно в поле', () => {
  it('предел и малые суммы', () => {
    expect(kopecksToInput(MAX_PRICE_KOPECKS)).toBe('1000000');
    expect(kopecksToInput(1)).toBe('0,01');
    expect(kopecksToInput(10)).toBe('0,10');
    expect(kopecksToInput(0)).toBe('0');
  });
});

describe('formatMoney', () => {
  it('«2 450,50 ₽» и «1 190 ₽», знак рубля через неразрывный пробел', () => {
    expect(sp(formatMoney(245_050))).toBe('2 450,50 ₽');
    expect(sp(formatMoney(119_000))).toBe('1 190 ₽');
    expect(formatMoney(119_000)).toMatch(/\u00a0₽$/u);
  });
  it('копейки всегда двумя знаками', () => {
    expect(sp(formatMoney(1))).toBe('0,01 ₽');
    expect(sp(formatMoney(10))).toBe('0,10 ₽');
    expect(sp(formatMoney(0))).toBe('0 ₽');
  });
  it('предел API', () => expect(sp(formatMoney(MAX_PRICE_KOPECKS))).toBe('1 000 000 ₽'));
  it('обратимость ввод → копейки → текст для поля', () => {
    for (const k of [1, 50, 99, 245_050, 99_999_999]) expect(parseRubles(sp(formatMoney(k)))).toBe(k);
  });
});
