// Безопасный адрес возврата после входа и перестановка порядка справочников —
// граничные случаи.
import { describe, expect, it } from 'vitest';
import { returnPath } from './nav';
import { dropIndex, moveItem, neighbourShift, sameOrder } from './refsOrder';

describe('returnPath', () => {
  it('путь с параметрами и якорем сохраняется', () => {
    expect(returnPath({ from: '/?q=koch&f=low' })).toBe('/?q=koch&f=low');
    expect(returnPath({ from: '/products/12#history' })).toBe('/products/12#history');
  });
  it.each(['', 'javascript:alert(1)', 'data:text/html,x', '\\\\evil.example', ' /profile'])('чужое и кривое — главная: %j', (from) =>
    expect(returnPath({ from })).toBe('/'),
  );
  it('state — строка или массив, а не объект — главная', () => {
    expect(returnPath('/profile')).toBe('/');
    expect(returnPath(['/profile'])).toBe('/');
  });
  // Было: браузер выбрасывает из адреса табуляцию и переводы строк, и «/\t/evil»
  // превращался в «//evil». Теперь адрес с управляющими символами отбрасывается.
  it.each(['/\t/evil.example', '/\n/evil.example', '/\r\\evil.example', '/profile\u0000'])('управляющие символы — главная: %j', (from) =>
    expect(returnPath({ from })).toBe('/'),
  );
});

describe('порядок справочника', () => {
  it('moveItem: на то же место — копия того же порядка', () => {
    const a = [1, 2, 3];
    const b = moveItem(a, 1, 1);
    expect(b).toEqual(a);
    expect(b).not.toBe(a);
  });
  it('moveItem: from за пределами — без изменений', () => {
    expect(moveItem([1, 2, 3], -1, 0)).toEqual([1, 2, 3]);
    expect(moveItem([1, 2, 3], 3, 0)).toEqual([1, 2, 3]);
  });
  it('moveItem: первый в конец и последний в начало', () => {
    expect(moveItem([1, 2, 3, 4], 0, 3)).toEqual([2, 3, 4, 1]);
    expect(moveItem([1, 2, 3, 4], 3, 0)).toEqual([4, 1, 2, 3]);
  });
  it('moveItem + sameOrder: «Выше» у первого ничего не шлёт', () => {
    const ids = [5, 6, 7];
    expect(sameOrder(moveItem(ids, 0, -1), ids)).toBe(true);
    expect(sameOrder(moveItem(ids, 2, 3), ids)).toBe(true);
  });
  it('sameOrder: разная длина', () => expect(sameOrder([1, 2], [1, 2, 3])).toBe(false));
  it('dropIndex: ровно на середине соседа — ещё на месте, на пиксель дальше — на его месте', () => {
    const h = [50, 50, 50];
    expect(dropIndex(0, 25, h)).toBe(0);
    expect(dropIndex(0, 26, h)).toBe(1);
    expect(dropIndex(2, -26, h)).toBe(1);
  });
  it('dropIndex: далеко за краем — к краю; одна строка — на месте', () => {
    expect(dropIndex(1, 9999, [50, 50, 50])).toBe(2);
    expect(dropIndex(1, -9999, [50, 50, 50])).toBe(0);
    expect(dropIndex(0, 500, [50])).toBe(0);
  });
  it('neighbourShift: строки вне диапазона не двигаются', () => {
    expect(neighbourShift(3, 0, 2, 50)).toBe(0);
    expect(neighbourShift(0, 1, 1, 50)).toBe(0);
  });
});
