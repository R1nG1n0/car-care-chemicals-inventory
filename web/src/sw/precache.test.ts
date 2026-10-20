import { describe, expect, it } from 'vitest';
import { precachePaths, precacheUrls, renderSw, swVersion } from './precache';

const FILES = [
  'index.html',
  'assets/index-AAA.js',
  'assets/index-BBB.css',
  'assets/zxing_reader-CCC.wasm',
  'assets/manrope-latin-500-normal-DDD.woff2',
  'assets/manrope-latin-500-normal-EEE.woff',
  'assets/reader-FFF.js',
  'favicon.svg',
  'manifest.webmanifest',
  'icon-192.png',
  'icon-512.png',
  'icon-192 2.png',
  'apple-touch-icon.png',
  'icon.svg',
  'sw.js',
  '.keep',
];

describe('состав предкэша', () => {
  it('страница, assets без wasm и запасных шрифтов, иконки и манифест', () => {
    expect(precachePaths(FILES)).toEqual([
      'apple-touch-icon.png',
      'assets/index-AAA.js',
      'assets/index-BBB.css',
      'assets/manrope-latin-500-normal-DDD.woff2',
      'assets/reader-FFF.js',
      'favicon.svg',
      'icon-192.png',
      'icon-512.png',
      'index.html',
      'manifest.webmanifest',
    ]);
  });

  it('адреса абсолютные; sw.js, wasm и файлы с пробелами не входят', () => {
    const urls = precacheUrls(FILES);
    expect(urls).toContain('/index.html');
    expect(urls.every((u) => u.startsWith('/'))).toBe(true);
    expect(urls.some((u) => u.endsWith('.wasm') || u.includes('sw.js') || u.includes(' ') || u.startsWith('/api'))).toBe(false);
  });
});

describe('версия сборки', () => {
  // Подставной «хеш»: учитывает всю строку
  const digest = (s: string) => {
    let h = 7;
    for (const c of s) h = (Math.imul(h, 31) + c.charCodeAt(0)) >>> 0;
    return `${h.toString(16).padStart(8, '0')}${s.length.toString(16).padStart(8, '0')}`;
  };
  const files = (h: string) => FILES.map((path) => ({ path, hash: path === 'index.html' ? h : 'x' }));

  it('меняется вместе с содержимым оболочки, не зависит от порядка и от sw.js', () => {
    const a = swVersion(files('1'), digest);
    expect(swVersion(files('2'), digest)).not.toBe(a);
    expect(swVersion([...files('1')].reverse(), digest)).toBe(a);
    expect(swVersion([...files('1'), { path: 'extra.txt', hash: 'q' }], digest)).toBe(a);
    expect(a).toHaveLength(12);
  });
});

describe('подстановка в шаблон', () => {
  const tpl = "const VERSION = '__SW_VERSION__';\nconst PRECACHE = [/*__SW_PRECACHE__*/];";

  it('версия и список подставляются', () => {
    const out = renderSw(tpl, ['/index.html', '/assets/a.js'], 'abc');
    expect(out).toContain('const VERSION = "abc";');
    expect(out).toContain('const PRECACHE = ["/index.html","/assets/a.js"];');
    expect(out).not.toContain('__SW_');
  });

  it('шаблон без меток — ошибка', () => {
    expect(() => renderSw('const x = 1;', [], 'v')).toThrow();
  });
});
