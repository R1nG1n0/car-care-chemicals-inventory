import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { createHash } from 'node:crypto';
import { readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import type { Plugin } from 'vite';
import { precacheUrls, renderSw, swVersion } from './src/sw/precache.ts';

const sha = (data: string | Buffer) => createHash('sha256').update(data).digest('hex');

// Сервис-воркер (ФТ-12.1): после сборки в dist/sw.js подставляются версия сборки
// и список файлов оболочки (страница, assets/ без wasm сканера, иконки, манифест).
function serviceWorker(): Plugin {
  const out = 'dist';
  return {
    name: 'garage-sw',
    apply: 'build',
    enforce: 'post',
    closeBundle() {
      const files: { path: string; hash: string }[] = [];
      const walk = (dir: string) => {
        for (const name of readdirSync(dir)) {
          const p = join(dir, name);
          if (statSync(p).isDirectory()) walk(p);
          else files.push({ path: relative(out, p).split(sep).join('/'), hash: sha(readFileSync(p)) });
        }
      };
      walk(out);
      const template = readFileSync('src/sw/sw.js', 'utf8');
      writeFileSync(join(out, 'sw.js'), renderSw(template, precacheUrls(files.map((f) => f.path)), swVersion(files, sha)));
    },
  };
}

// Политика безопасности (ТБ-6) запрещает встроенные скрипты и стили, поэтому
// в итоговом index.html — только ссылки на файлы из assets/; это проверяет
// scripts/check-dist.mjs.
export default defineConfig({
  plugins: [react(), serviceWorker()],
  build: {
    outDir: 'dist',
    // Каталог чистит scripts/clean-dist.mjs: файл dist/.keep нужен go:embed.
    emptyOutDir: false,
    target: 'es2022',
    // Полифил modulepreload Vite вставляет кодом в страницу — не нужен.
    modulePreload: { polyfill: false },
    // Никаких data:-адресов для шрифтов и скриптов: всё — файлами.
    assetsInlineLimit: 0,
    sourcemap: false,
  },
  server: {
    port: 5173,
    strictPort: true,
    // Сервер garage на тестовой базе (make dev, config/dev.json).
    proxy: { '/api': { target: 'http://127.0.0.1:8090' } },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
