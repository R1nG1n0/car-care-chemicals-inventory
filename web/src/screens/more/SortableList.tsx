// Список с ручкой перетаскивания (.g-handle, motion.css «Перетаскивание строк»):
// строка приподнимается и следует за пальцем, соседи расступаются.
// Pointer events — одинаково для мыши, пальца и пера. Без жеста порядок
// меняется стрелками на ручке и кнопками «Выше/Ниже» в меню строки.
// После перестановки стрелкой фокус возвращается на ручку той же строки
// (React переносит узел, и фокус иначе теряется), новое место объявляется вслух.
import { IconGripVertical } from '@tabler/icons-react';
import { useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type ReactNode } from 'react';
import { Ico } from '../../components/ui';
import { dropIndex, neighbourShift } from '../../lib/refsOrder';

type Drag = { from: number; to: number; startY: number; heights: number[]; pointerId: number };

type Props<T extends { id: number }> = {
  items: T[];
  label: string;
  nameOf: (item: T) => string;
  rowClass?: (item: T) => string;
  onMove: (from: number, to: number) => void;
  children: (item: T, index: number) => ReactNode;
};

export function SortableList<T extends { id: number }>({ items, label, nameOf, rowClass, onMove, children }: Props<T>) {
  const rows = useRef<(HTMLDivElement | null)[]>([]);
  const drag = useRef<Drag | null>(null);
  const [lifted, setLifted] = useState<number | null>(null);
  const handles = useRef(new Map<number, HTMLButtonElement>());
  const refocus = useRef<number | null>(null);
  const [said, setSaid] = useState('');

  // Строка, переставленная с клавиатуры, — снова в фокусе на новом месте
  useLayoutEffect(() => {
    const id = refocus.current;
    if (id === null) return;
    const el = handles.current.get(id);
    if (el && document.activeElement !== el) el.focus({ preventScroll: false });
    const at = items.findIndex((x) => x.id === id);
    if (at >= 0) {
      refocus.current = null;
      setSaid(`${nameOf(items[at]!)}: место ${at + 1} из ${items.length}`);
    }
  }, [items, nameOf]);

  const paint = (d: Drag, dy: number) => {
    rows.current.forEach((el, i) => {
      if (!el) return;
      if (i === d.from) el.style.transform = `translateY(${dy}px) scale(1.02)`;
      else {
        const s = neighbourShift(i, d.from, d.to, d.heights[d.from]!);
        el.style.transform = s ? `translateY(${s}px)` : '';
      }
    });
  };

  const clear = () => {
    rows.current.forEach((el) => {
      if (el) {
        el.style.transform = '';
        el.classList.remove('is-dragging');
      }
    });
  };

  const down = (e: PointerEvent<HTMLButtonElement>, index: number) => {
    if (e.button !== 0 || items.length < 2) return;
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    const heights = rows.current.slice(0, items.length).map((el) => el?.getBoundingClientRect().height ?? 0);
    drag.current = { from: index, to: index, startY: e.clientY, heights, pointerId: e.pointerId };
    setLifted(index);
  };

  const move = (e: PointerEvent<HTMLButtonElement>) => {
    const d = drag.current;
    if (!d || e.pointerId !== d.pointerId) return;
    // Не дальше краёв списка: строка не уезжает за рамку
    const above = d.heights.slice(0, d.from).reduce((s, h) => s + h, 0);
    const below = d.heights.slice(d.from + 1).reduce((s, h) => s + h, 0);
    const dy = Math.max(-above, Math.min(below, e.clientY - d.startY));
    d.to = dropIndex(d.from, dy, d.heights);
    rows.current[d.from]?.classList.add('is-dragging');
    paint(d, dy);
  };

  const up = (e: PointerEvent<HTMLButtonElement>) => {
    const d = drag.current;
    if (!d || e.pointerId !== d.pointerId) return;
    drag.current = null;
    clear();
    setLifted(null);
    if (d.to !== d.from && e.type === 'pointerup') onMove(d.from, d.to);
  };

  const key = (e: KeyboardEvent, index: number) => {
    const to = e.key === 'ArrowUp' ? index - 1 : e.key === 'ArrowDown' ? index + 1 : -1;
    if (to < 0 || to >= items.length) return;
    e.preventDefault();
    refocus.current = items[index]!.id;
    onMove(index, to);
  };

  return (
    <>
    <div className="g-list" aria-label={label} role="list">
      {items.map((item, i) => (
        <div
          key={item.id}
          role="listitem"
          ref={(el) => {
            rows.current[i] = el;
          }}
          className={`g-row${rowClass ? ` ${rowClass(item)}` : ''}${lifted === i ? ' is-lifted' : ''}`}
        >
          <button
            ref={(el) => {
              if (el) handles.current.set(item.id, el);
              else handles.current.delete(item.id);
            }}
            type="button"
            className="g-handle"
            aria-label={`Перетащить ${nameOf(item)}. Стрелки вверх и вниз меняют порядок`}
            onPointerDown={(e) => down(e, i)}
            onPointerMove={move}
            onPointerUp={up}
            onPointerCancel={up}
            onKeyDown={(e) => key(e, i)}
          >
            <Ico icon={IconGripVertical} />
          </button>
          {children(item, i)}
        </div>
      ))}
    </div>
    <span className="g-sr" aria-live="polite">
      {said}
    </span>
    </>
  );
}
