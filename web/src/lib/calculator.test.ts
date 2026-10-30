import { describe, expect, it } from 'vitest';
import { calcErrors, calculate } from './calculator';

const f = (volume: string, ratio: string) => ({ mode: 'forward' as const, volume, concentrate: '', ratio });
const r = (concentrate: string, ratio: string) => ({ mode: 'reverse' as const, volume: '', concentrate, ratio });

describe('калькулятор: сколько концентрата', () => {
  it('концентрат и вода на объём', () => {
    expect(calculate(f('1000', '10'))).toMatchObject({ concentrate: 91, water: 909 });
    expect(calculate(f('5 000', '20'))).toMatchObject({ concentrate: 238, water: 4762 });
    expect(calculate(f('500', '1'))).toMatchObject({ concentrate: 250, water: 250 });
  });
  it('пока поля не заполнены — нет результата', () => {
    expect(calculate(f('', '10'))).toBeNull();
    expect(calculate(f('1000', ''))).toBeNull();
    expect(calculate(f('1000', '0'))).toBeNull();
  });
});

describe('калькулятор: сколько раствора получится', () => {
  it('раствор и вода из концентрата', () => {
    expect(calculate(r('100', '10'))).toMatchObject({ solution: 1100, water: 1000 });
    expect(calculate(r('50', '20'))).toMatchObject({ solution: 1050, water: 1000 });
  });
  it('границы', () => {
    expect(calculate(r('0', '10'))).toBeNull();
    expect(calculate(r('100001', '10'))).toBeNull();
    expect(calculate(r('10', '1001'))).toBeNull();
  });
  it('прямой и обратный расчёты согласованы там, где деление без остатка', () => {
    const back = calculate(r('100', '10'));
    expect(back?.mode === 'reverse' && calculate(f(String(back.solution), '10'))).toMatchObject({ concentrate: 100 });
  });
});

describe('ошибки полей', () => {
  it('пустое поле — не ошибка, неверное — ошибка', () => {
    expect(calcErrors(f('', ''))).toEqual({});
    expect(calcErrors(f('0', '10')).volume).toBeTruthy();
    expect(calcErrors(f('500', '2000')).ratio).toBeTruthy();
    expect(calcErrors(r('0', '10')).concentrate).toBeTruthy();
    expect(calcErrors(r('', '10'))).toEqual({});
  });
});
