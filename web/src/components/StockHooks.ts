// Общие хуки раздела «Остатки»: присутствие с анимацией ухода, медиазапрос.
import { useEffect, useState, useSyncExternalStore } from 'react';

/**
 * Держит элемент смонтированным ещё `leaveMs` после show=false, чтобы успела
 * сыграть анимация ухода (.is-leaving в motion.css).
 */
export function usePresence(show: boolean, leaveMs: number): { mounted: boolean; leaving: boolean } {
  const [mounted, setMounted] = useState(show);
  if (show && !mounted) setMounted(true);
  useEffect(() => {
    if (show || !mounted) return;
    const t = setTimeout(() => setMounted(false), leaveMs);
    return () => clearTimeout(t);
  }, [show, mounted, leaveMs]);
  return { mounted: mounted || show, leaving: mounted && !show };
}

const subscribers = new Map<string, (cb: () => void) => () => void>();

/** Одна функция подписки на запрос: иначе useSyncExternalStore переподписывается каждый рендер. */
function subscribeMedia(query: string) {
  const known = subscribers.get(query);
  if (known) return known;
  const fn = (cb: () => void) => {
    const mq = window.matchMedia(query);
    mq.addEventListener('change', cb);
    return () => mq.removeEventListener('change', cb);
  };
  subscribers.set(query, fn);
  return fn;
}

/** true, пока выполняется медиазапрос: «(min-width: 1024px)», «(prefers-reduced-motion: reduce)». */
export function useMedia(query: string): boolean {
  return useSyncExternalStore(
    subscribeMedia(query),
    () => window.matchMedia(query).matches,
    () => false,
  );
}

export const DESKTOP = '(min-width: 1024px)';
export const REDUCED_MOTION = '(prefers-reduced-motion: reduce)';

/** Порог свайпа и «рывка» — как в прототипе (docs/design/prototype.js). */
export const SWIPE_AT = 112;
export const FLICK = 0.11;
