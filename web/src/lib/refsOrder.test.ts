import { describe, expect, it } from 'vitest';
import { dropIndex, moveItem, neighbourShift, sameOrder } from './refsOrder';

describe('moveItem', () => {
  it('вниз и вверх', () => {
    expect(moveItem([1, 2, 3, 4], 0, 2)).toEqual([2, 3, 1, 4]);
    expect(moveItem([1, 2, 3, 4], 3, 1)).toEqual([1, 4, 2, 3]);
  });
  it('за краями — к краю, исходный массив не меняется', () => {
    const a = [1, 2, 3];
    expect(moveItem(a, 0, 10)).toEqual([2, 3, 1]);
    expect(moveItem(a, 2, -1)).toEqual([3, 1, 2]);
    expect(moveItem(a, 5, 0)).toEqual([1, 2, 3]);
    expect(a).toEqual([1, 2, 3]);
  });
});

describe('sameOrder', () => {
  it('сравнивает поэлементно', () => {
    expect(sameOrder([1, 2], [1, 2])).toBe(true);
    expect(sameOrder([1, 2], [2, 1])).toBe(false);
    expect(sameOrder([1], [1, 2])).toBe(false);
  });
});

describe('dropIndex', () => {
  const h = [50, 50, 50, 50];
  it('малый сдвиг — на месте', () => {
    expect(dropIndex(1, 20, h)).toBe(1);
    expect(dropIndex(1, -20, h)).toBe(1);
    expect(dropIndex(1, 0, h)).toBe(1);
  });
  it('за середину соседа — на его место', () => {
    expect(dropIndex(1, 30, h)).toBe(2);
    expect(dropIndex(1, 80, h)).toBe(3);
    expect(dropIndex(1, 500, h)).toBe(3);
    expect(dropIndex(2, -30, h)).toBe(1);
    expect(dropIndex(2, -500, h)).toBe(0);
  });
  it('строки разной высоты', () => {
    expect(dropIndex(0, 40, [40, 100, 40])).toBe(0);
    expect(dropIndex(0, 71, [40, 100, 40])).toBe(1);
  });
});

describe('neighbourShift', () => {
  it('соседи между from и to сдвигаются на высоту строки', () => {
    expect(neighbourShift(2, 1, 3, 50)).toBe(-50);
    expect(neighbourShift(4, 1, 3, 50)).toBe(0);
    expect(neighbourShift(1, 3, 1, 50)).toBe(50);
    expect(neighbourShift(3, 3, 1, 50)).toBe(0);
  });
});
