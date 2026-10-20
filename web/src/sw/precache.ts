// Состав предкэша сервис-воркера (ФТ-12.1) и подстановка в шаблон sw.js.
// Чистые функции: файлы сборки и их хеши приходят из vite.config.ts.

export type BuiltFile = { path: string; hash: string };

/** Файлы из public/, которые нужны оболочке (остальное, например «icon 2.png», не берём). */
const PUBLIC_SHELL = /^(favicon\.svg|manifest\.webmanifest|icon-(192|512)\.png|apple-touch-icon\.png)$/u;

/**
 * Что сохраняется при установке: страница, все файлы assets/ кроме больших
 * wasm сканера (они кладутся в кэш при первом запросе), иконки и манифест.
 */
export function precachePaths(files: readonly string[]): string[] {
  return files
    .filter((p) => {
      if (p === 'index.html') return true;
      if (p.startsWith('assets/')) {
        // .woff — запасной формат для старых браузеров, берётся из сети при нужде
        return !(p.endsWith('.wasm') || p.endsWith('.map') || p.endsWith('.woff') || /\s/u.test(p));
      }
      return PUBLIC_SHELL.test(p);
    })
    .sort();
}

/** Адреса для кэша: абсолютные, страница — «/index.html» (запасной ответ навигации). */
export function precacheUrls(files: readonly string[]): string[] {
  return precachePaths(files).map((p) => `/${p}`);
}

/** Версия сборки: меняется, если изменился состав или содержимое хотя бы одного файла оболочки. */
export function swVersion(files: readonly BuiltFile[], digest: (s: string) => string): string {
  const keep = new Set(precachePaths(files.map((f) => f.path)));
  const key = files
    .filter((f) => keep.has(f.path))
    .sort((a, b) => (a.path < b.path ? -1 : 1))
    .map((f) => `${f.path}:${f.hash}`)
    .join('\n');
  return digest(key).slice(0, 12);
}

/** Подставляет версию и список в шаблон; метки должны быть в нём ровно по одному разу. */
export function renderSw(template: string, urls: readonly string[], version: string): string {
  const versionMark = "'__SW_VERSION__'";
  const listMark = '[/*__SW_PRECACHE__*/]';
  if (template.split(versionMark).length !== 2 || template.split(listMark).length !== 2) {
    throw new Error('sw.js: в шаблоне нет меток версии или списка файлов');
  }
  return template.replace(versionMark, JSON.stringify(version)).replace(listMark, JSON.stringify(urls));
}
