// Перестроение списка (motion.css, «FLIP-список»): строка, сменившая место,
// переезжает к новому положению трансформацией, 200 мс, кривая --ease-in-out
// из tokens.css. При prefers-reduced-motion — сразу на месте.
import { useCallback, useLayoutEffect, useRef } from 'react';
import { REDUCED_MOTION } from './hooks';

/** Кривая перестроения из токена --ease-in-out (одна на все FLIP-списки). */
export function easeInOut(): string {
  return getComputedStyle(document.documentElement).getPropertyValue('--ease-in-out').trim() || 'ease-in-out';
}

export const FLIP_MS = 200;

export function useFlip(order: readonly (string | number)[]) {
  const els = useRef(new Map<string | number, HTMLElement>());
  const prev = useRef(new Map<string | number, number>());
  const sig = order.join(',');

  useLayoutEffect(() => {
    const reduce = window.matchMedia(REDUCED_MOTION).matches;
    const ease = easeInOut();
    const next = new Map<string | number, number>();
    els.current.forEach((el, k) => {
      // offsetTop — относительно списка: прокрутка страницы не даёт ложного сдвига
      const top = el.offsetTop;
      next.set(k, top);
      const was = prev.current.get(k);
      if (!reduce && was !== undefined && was !== top) {
        el.animate([{ transform: `translateY(${was - top}px)` }, { transform: 'none' }], { duration: FLIP_MS, easing: ease });
      }
    });
    prev.current = next;
  }, [sig]);

  return useCallback(
    (k: string | number) => (el: HTMLElement | null) => {
      if (el) els.current.set(k, el);
      else els.current.delete(k);
    },
    [],
  );
}
