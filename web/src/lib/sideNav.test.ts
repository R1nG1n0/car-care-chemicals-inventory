import { describe, expect, it } from 'vitest';
import type { ProductSummary } from '../api';
import { countText, lowCount, toBuyCount, versionText } from './sideNav';

const p = (low: boolean) => ({ low }) as ProductSummary;

describe('счётчики бокового меню', () => {
  it('«мало» — позиции ниже минимума', () => {
    expect(lowCount([p(true), p(false), p(true)])).toBe(2);
    expect(lowCount(undefined)).toBe(0);
  });

  it('к покупке — ниже минимума и невычеркнутые ручные', () => {
    expect(toBuyCount({ low: [1, 2], manual: [{ done_at: null }, { done_at: 100 }, { done_at: null }] })).toBe(4);
    expect(toBuyCount(undefined)).toBe(0);
  });

  it('текст: пусто при нуле, 99+ для больших', () => {
    expect(countText(0)).toBe('');
    expect(countText(7)).toBe('7');
    expect(countText(100)).toBe('99+');
  });

  it('версия — короткий коммит или «для разработки»', () => {
    expect(versionText('507c0b6d1234')).toBe('версия 507c0b6');
    expect(versionText('dev')).toBe('версия для разработки');
    expect(versionText(undefined)).toBe('');
  });
});
