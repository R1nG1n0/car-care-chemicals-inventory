// Сервис-воркер garage (ФТ-12.1). Шаблон: версия и список файлов подставляются при
// сборке (vite.config.ts → dist/sw.js). Хранит только файлы интерфейса; ответы API
// не хранит и не перехватывает. Без внешних адресов и eval — политика безопасности ТБ-6.
const VERSION = '__SW_VERSION__';
const PRECACHE = [/*__SW_PRECACHE__*/];
const CACHE = `garage-shell-${VERSION}`;
const NAV_TIMEOUT_MS = 3000;

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE);
      // cache: 'reload' — мимо HTTP-кэша, иначе в предкэш мог бы попасть файл прошлой сборки
      await cache.addAll(PRECACHE.map((url) => new Request(url, { cache: 'reload' })));
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      // Новая сборка заменяет старые файлы целиком (ФТ-12.1.2)
      for (const name of await caches.keys()) {
        if (name.startsWith('garage-shell-') && name !== CACHE) await caches.delete(name);
      }
      await self.clients.claim();
    })(),
  );
});

/** Сеть с тайм-аутом; не ответила вовремя или упала — сохранённая страница. */
async function navigate(request) {
  const cache = await caches.open(CACHE);
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), NAV_TIMEOUT_MS);
  try {
    const res = await fetch(request, { signal: ctrl.signal });
    // 5xx от прокси при выключенном VPN — тоже «нет связи»
    if (res.status >= 502 && res.status <= 504) throw new Error('gateway');
    return res;
  } catch {
    const shell = await cache.match('/index.html');
    return shell ?? Response.error();
  } finally {
    clearTimeout(timer);
  }
}

/** assets/: сначала кэш; чего нет (модуль сканера) — из сети и в кэш. */
async function asset(request) {
  const cache = await caches.open(CACHE);
  const hit = await cache.match(request);
  if (hit) return hit;
  const res = await fetch(request);
  if (res.ok) await cache.put(request, res.clone());
  return res;
}

self.addEventListener('fetch', (event) => {
  const request = event.request;
  if (request.method !== 'GET') return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  // API никогда не кэшируется и не перехватывается; сам воркер — тоже
  if (url.pathname.startsWith('/api/') || url.pathname === '/sw.js') return;

  if (request.mode === 'navigate') {
    event.respondWith(navigate(request));
  } else if (url.pathname.startsWith('/assets/')) {
    event.respondWith(asset(request));
  } else if (PRECACHE.includes(url.pathname)) {
    // Иконки и манифест (адреса с ?v=…): из кэша, иначе из сети
    event.respondWith(caches.match(url.pathname).then((hit) => hit ?? fetch(request)));
  }
});
