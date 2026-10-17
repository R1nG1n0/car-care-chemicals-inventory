// Перестроение списка (motion.css, «FLIP-список»): строка, сменившая место,
// переезжает к новому положению трансформацией, 200 мс, --ease-in-out.
// При prefers-reduced-motion — сразу на месте.
import { useCallback, useLayoutEffect, useRef } from 'react';

const EASE_IN_OUT = 'cubic-bezier(0.77, 0, 0.175, 1)';

export function useFlip(order: readonly (string | number)[]) {
  const els = useRef(new Map<string | number, HTMLElement>());
  const prev = useRef(new Map<string | number, number>());
  const sig = order.join(',');

  useLayoutEffect(() => {
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const next = new Map<string | number, number>();
    els.current.forEach((el, k) => {
      // offsetTop — относительно списка: прокрутка страницы не даёт ложного сдвига
      const top = el.offsetTop;
      next.set(k, top);
      const was = prev.current.get(k);
      if (!reduce && was !== undefined && was !== top) {
        el.animate([{ transform: `translateY(${was - top}px)` }, { transform: 'none' }], { duration: 200, easing: EASE_IN_OUT });
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
