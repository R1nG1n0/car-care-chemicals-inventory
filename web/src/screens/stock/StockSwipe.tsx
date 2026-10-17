// Свайп влево по карточке открывает расход (.g-swipe, прототип docs/design/prototype.js):
// карточка следует за пальцем, после 112 px — сопротивление ×0,3, вправо почти не тянется;
// отпустили дальше порога или резким рывком — расход, иначе карточка возвращается
// переходом из motion.css. Вертикальный жест остаётся прокруткой.
import { IconDroplet } from '@tabler/icons-react';
import { useRef, type MouseEvent, type PointerEvent, type ReactNode } from 'react';
import { Ico } from '../../components/ui';
import { FLICK, SWIPE_AT } from '../../components/StockHooks';

const offset = (dx: number) => (dx > 0 ? dx * 0.15 : dx < -SWIPE_AT ? -SWIPE_AT + (dx + SWIPE_AT) * 0.3 : dx);

type Gesture = { id: number; x: number; y: number; t: number; on: boolean; dx: number; card: HTMLElement };

type Props = { onSwipe: () => void; children: ReactNode; className?: string; dataKey?: string };

export function StockSwipe({ onSwipe, children, className, dataKey }: Props) {
  const g = useRef<Gesture | null>(null);
  const dragged = useRef(0);

  const down = (e: PointerEvent<HTMLDivElement>) => {
    const card = e.currentTarget.querySelector<HTMLElement>('.g-card');
    if (e.button !== 0 || !card || e.currentTarget.classList.contains('is-exiting')) return;
    g.current = { id: e.pointerId, x: e.clientX, y: e.clientY, t: e.timeStamp, on: false, dx: 0, card };
  };
  const move = (e: PointerEvent<HTMLDivElement>) => {
    const s = g.current;
    if (!s || e.pointerId !== s.id) return;
    const dx = e.clientX - s.x;
    const dy = e.clientY - s.y;
    if (!s.on) {
      if (Math.hypot(dx, dy) < 6) return;
      if (Math.abs(dx) <= Math.abs(dy)) {
        g.current = null; // вертикальный жест — прокрутка
        return;
      }
      s.on = true;
      s.card.setPointerCapture(e.pointerId);
      s.card.classList.add('is-dragging');
    }
    s.dx = dx;
    s.card.style.transform = `translateX(${offset(dx)}px)`;
  };
  const end = (e: PointerEvent<HTMLDivElement>) => {
    const s = g.current;
    if (!s || e.pointerId !== s.id) return;
    g.current = null;
    if (!s.on) return;
    dragged.current = e.timeStamp;
    s.card.classList.remove('is-dragging');
    s.card.style.transform = ''; // доводка до места — переходом из motion.css
    const v = -s.dx / Math.max(1, e.timeStamp - s.t);
    if (e.type === 'pointerup' && (-s.dx > SWIPE_AT || (v > FLICK && -s.dx > 24))) onSwipe();
  };
  // После жеста отпускание не должно сработать как нажатие на карточку
  const click = (e: MouseEvent<HTMLDivElement>) => {
    if (e.timeStamp - dragged.current < 400) {
      e.preventDefault();
      e.stopPropagation();
    }
  };

  return (
    <div
      className={`g-swipe${className ? ` ${className}` : ''}`}
      data-key={dataKey}
      onPointerDown={down}
      onPointerMove={move}
      onPointerUp={end}
      onPointerCancel={end}
      onClickCapture={click}
      onDragStart={(e) => e.preventDefault()}
    >
      <div className="g-swipe__action" aria-hidden="true">
        <Ico icon={IconDroplet} /> Расход
      </div>
      {children}
    </div>
  );
}
