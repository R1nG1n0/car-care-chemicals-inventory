// Сервис-воркер (ФТ-12.1): только в собранном приложении, не в режиме разработки.
// Файл sw.js — тот же источник ('self'), встроенных скриптов нет (ТБ-6).
export function registerServiceWorker(): void {
  if (!import.meta.env.PROD || !('serviceWorker' in navigator)) return;
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js', { scope: '/' }).catch(() => undefined);
  });
}
