import { describe, expect, it } from 'vitest';
import { clampQty, MAX_QTY, parseQty, validateConsume } from './qty';

const sp = (s: string | null) => s?.replace(/[\u00a0\u202f]/gu, ' ') ?? null;

describe('parseQty и clampQty', () => {
  it('пусто — null, разряды — можно', () => {
    expect(parseQty('')).toBeNull();
    expect(parseQty('1 000')).toBe(1000);
  });
  it('clampQty: NaN — нижняя граница', () => expect(clampQty(Number.NaN, 1, 10)).toBe(1));
});

describe('validateConsume — расход одной позиции', () => {
  it('пусто и ноль — «Введите количество»', () => {
    expect(validateConsume(null, 100, 'ml')).toBe('Введите количество');
    expect(validateConsume(0, 100, 'ml')).toBe('Введите количество');
  });
  it('больше остатка — «Не хватает»', () => expect(sp(validateConsume(101, 100, 'ml'))).toBe('Не хватает: есть 100 мл'));
  it('ровно остаток — можно', () => expect(validateConsume(100, 100, 'ml')).toBeNull());
  it('больше 100 000 за раз — ошибка даже при большом остатке', () => {
    expect(validateConsume(MAX_QTY, 250_000, 'ml')).toBeNull();
    expect(sp(validateConsume(MAX_QTY + 1, 250_000, 'ml'))).toBe('За раз — не больше 100 000 мл');
  });
  it('шт', () => expect(sp(validateConsume(3, 2, 'pcs'))).toBe('Не хватает: есть 2 шт'));
});
