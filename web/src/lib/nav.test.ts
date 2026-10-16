import { describe, expect, it } from 'vitest';
import { returnPath } from './nav';

describe('returnPath', () => {
  it('свой путь', () => expect(returnPath({ from: '/profile' })).toBe('/profile'));
  it.each([null, undefined, {}, { from: 5 }, { from: 'https://evil.example' }, { from: '//evil.example' }, { from: '/\\evil' }, { from: 'profile' }])(
    'иначе — главная: %j',
    (s) => expect(returnPath(s)).toBe('/'),
  );
});
