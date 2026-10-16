// Состояние связи с сервером: любой полученный ответ — связь есть,
// упавший запрос или 502–504 от nginx — связи нет. Подписка — для
// useSyncExternalStore (полоса «Нет связи» вверху экрана, И-3а).

type Listener = () => void;

let online = true;
const listeners = new Set<Listener>();

export function isOnline(): boolean {
  return online;
}

export function reportConnection(ok: boolean): void {
  if (online === ok) return;
  online = ok;
  for (const l of listeners) l();
}

export function subscribeConnection(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
