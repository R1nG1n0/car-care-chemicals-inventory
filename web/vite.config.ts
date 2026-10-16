import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// Политика безопасности (ТБ-6) запрещает встроенные скрипты и стили, поэтому
// в итоговом index.html — только ссылки на файлы из assets/; это проверяет
// scripts/check-dist.mjs.
export default defineConfig({
  plugins: [react()],
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
