// Список карточек главной (motion.css): появление очередью при входе (.g-stagger),
// перестроение при поиске и фильтрах (.g-flip): ушедшие гаснут на своём месте,
// оставшиеся переезжают (WAAPI, 200 мс, --ease-in-out), новые проявляются.
// Положение карточек запоминается после каждой отрисовки — это «First» для следующей.
import { useEffect, useLayoutEffect, useRef, useState, type MouseEvent } from 'react';
import type { ProductSummary } from '../../api';
import { ProductCard } from '../../components/ProductCard';
import { REDUCED_MOTION } from '../../components/StockHooks';
import { StockSwipe } from './StockSwipe';

type Props = {
  items: ProductSummary[];
  selectedId?: number | null | undefined;
  onSwipe: (p: ProductSummary) => void;
  onCardClick?: ((p: ProductSummary, e: MouseEvent<HTMLAnchorElement>) => void) | undefined;
};

type Box = { top: number; left: number; width: number };

const STAGGER_MS = 300 + 8 * 40; // --dur-enter + 8 × --stagger
const EXIT_MS = 200;

export function StockList({ items, selectedId, onSwipe, onCardClick }: Props) {
  const list = useRef<HTMLDivElement>(null);
  const boxes = useRef(new Map<string, Box>());
  const flips = useRef(new Map<string, Animation>());
  const mounted = useRef(false);
  const [stagger, setStagger] = useState(true);

  // Ушедшие карточки остаются ещё на 200 мс, чтобы погаснуть
  const ids = items.map((p) => p.id).join(',');
  const [prev, setPrev] = useState({ ids, items });
  const [exiting, setExiting] = useState<ProductSummary[]>([]);
  if (ids !== prev.ids) {
    const now = new Set(items.map((p) => p.id));
    const gone = prev.items.filter((p) => !now.has(p.id));
    setExiting((ex) => [...ex.filter((p) => !now.has(p.id) && !gone.some((g) => g.id === p.id)), ...gone]);
    setPrev({ ids, items });
  }
  useEffect(() => {
    if (exiting.length === 0) return;
    const t = setTimeout(() => setExiting([]), EXIT_MS);
    return () => clearTimeout(t);
  }, [exiting]);

  // Очередь играет только при входе: потом класс снимается, чтобы карточки
  // из поиска появлялись по правилам FLIP, а не очередью
  useEffect(() => {
    const t = setTimeout(() => setStagger(false), STAGGER_MS);
    return () => clearTimeout(t);
  }, []);

  useLayoutEffect(() => {
    const root = list.current;
    if (!root) return;
    const base = root.getBoundingClientRect();
    const rm = window.matchMedia(REDUCED_MOTION).matches;
    const kids = [...root.querySelectorAll<HTMLElement>(':scope > [data-key]')];

    // Уходящие: на прежнее место поверх потока, затем гаснут
    for (const el of kids.filter((k) => k.dataset.exit && !k.classList.contains('is-exiting'))) {
      const b = boxes.current.get(el.dataset.key!);
      if (b) {
        el.style.top = `${b.top}px`;
        el.style.left = `${b.left}px`;
        el.style.width = `${b.width}px`;
      }
      void el.offsetWidth;
      el.classList.add('is-exiting');
    }

    const next = new Map<string, Box>();
    const ease = getComputedStyle(document.documentElement).getPropertyValue('--ease-in-out').trim() || 'ease-in-out';
    for (const el of kids.filter((k) => !k.dataset.exit)) {
      const key = el.dataset.key!;
      flips.current.get(key)?.cancel();
      const r = el.getBoundingClientRect();
      const box = { top: r.top - base.top, left: r.left - base.left, width: r.width };
      next.set(key, box);
      const was = boxes.current.get(key);
      if (!mounted.current) continue;
      if (!was) {
        // Новая: проявляется на своём месте
        el.classList.add('is-entering');
        void el.offsetWidth;
        el.classList.remove('is-entering');
      } else if (!rm) {
        const dx = was.left - box.left;
        const dy = was.top - box.top;
        if (Math.abs(dx) + Math.abs(dy) >= 0.5) {
          flips.current.set(key, el.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'none' }], { duration: 200, easing: ease }));
        }
      }
    }
    boxes.current = next;
    mounted.current = true;
  });

  return (
    <div ref={list} className={`stock-list g-flip${stagger ? ' g-stagger' : ''}`}>
      {items.map((p) => (
        <StockSwipe key={p.id} onSwipe={() => onSwipe(p)} dataKey={String(p.id)}>
          <ProductCard product={p} current={selectedId === p.id} onClick={onCardClick ? (e) => onCardClick(p, e) : undefined} />
        </StockSwipe>
      ))}
      {exiting.map((p) => (
        <div key={`x${p.id}`} className="g-swipe" data-key={String(p.id)} data-exit="1" aria-hidden="true" inert>
          <ProductCard product={p} />
        </div>
      ))}
    </div>
  );
}
