// Нижняя шторка .g-sheet с затемнением .g-scrim (motion.css, прототип docs/design/prototype.js):
// выезжает снизу, уходит туда же быстрее; смахивается вниз за ручку (дальше четверти
// высоты или рывком), закрывается Esc и нажатием на фон. Шторки складываются стопкой:
// Esc и фокус относятся к верхней. Рисуется в document.body.
import { useEffect, useId, useRef, type PointerEvent as ReactPointerEvent, type ReactNode, type RefObject } from 'react';
import { createPortal } from 'react-dom';
import { FLICK, REDUCED_MOTION, usePresence } from './hooks';

export type SheetProps = {
  open: boolean;
  onClose: () => void;
  /** Подпись диалога для экранного диктора (aria-label), если нет видимого заголовка. */
  label?: string;
  /** Видимый заголовок: рисуется в шапке шторки и связывается через aria-labelledby. */
  title?: ReactNode;
  children: ReactNode;
  /** Закреплённый низ: главная кнопка, не прокручивается вместе с телом. */
  footer?: ReactNode;
  className?: string;
};

const LEAVE_MS = 240; // 200 мс ухода + запас
const stack: { id: symbol; el: HTMLElement | null }[] = [];
let locks = 0;
/** Куда вернуть фокус после слоя — чтобы следующий слой, открытый из него, знал адрес. */
const backs = new WeakMap<HTMLElement, HTMLElement | null>();

const FOCUSABLE = 'button:not([disabled]), [href], input:not([disabled]), select, textarea, [tabindex]:not([tabindex="-1"])';

export function Sheet({ open, onClose, label, title, children, footer, className }: SheetProps) {
  const { mounted, leaving } = usePresence(open, LEAVE_MS);
  if (!mounted) return null;
  return createPortal(
    <SheetBody leaving={leaving} onClose={onClose} label={label} title={title} footer={footer} className={className}>
      {children}
    </SheetBody>,
    document.body,
  );
}

type BodyProps = Omit<SheetProps, 'open'> & { leaving: boolean };

function SheetBody({ leaving, onClose, label, title, children, footer, className }: BodyProps) {
  const sheet = useRef<HTMLElement>(null);
  const scrim = useRef<HTMLDivElement>(null);
  const drag = useRef<{ id: number; y: number; t: number; dy: number } | null>(null);
  const titleId = useId();
  const close = useModalLayer(sheet, onClose);

  // Уход: шторка доезжает вниз с того места, где её отпустили.
  // При уменьшенном движении гаснет на месте.
  useEffect(() => {
    if (!leaving) return;
    const el = sheet.current;
    const sc = scrim.current;
    el?.classList.remove('is-dragging');
    sc?.classList.remove('is-dragging');
    if (el && !window.matchMedia(REDUCED_MOTION).matches) el.style.transform = '';
    if (sc) sc.style.opacity = '';
  }, [leaving]);

  const onDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (e.button !== 0 || leaving) return;
    drag.current = { id: e.pointerId, y: e.clientY, t: e.timeStamp, dy: 0 };
    e.currentTarget.setPointerCapture(e.pointerId);
    sheet.current?.classList.add('is-dragging');
    scrim.current?.classList.add('is-dragging');
  };
  const onMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    const g = drag.current;
    const el = sheet.current;
    if (!g || e.pointerId !== g.id || !el) return;
    g.dy = e.clientY - g.y;
    // Вниз — за пальцем, вверх — с сильным сопротивлением
    const y = g.dy < 0 ? g.dy * 0.2 : g.dy;
    el.style.transform = `translateY(${y}px)`;
    if (scrim.current) scrim.current.style.opacity = String(Math.max(0, 1 - Math.max(0, y) / el.offsetHeight));
  };
  const onEnd = (e: ReactPointerEvent<HTMLDivElement>) => {
    const g = drag.current;
    const el = sheet.current;
    if (!g || e.pointerId !== g.id || !el) return;
    drag.current = null;
    const v = g.dy / Math.max(1, e.timeStamp - g.t);
    if (e.type === 'pointerup' && (g.dy > el.offsetHeight * 0.25 || (v > FLICK && g.dy > 10))) {
      close.current();
      return;
    }
    el.classList.remove('is-dragging');
    scrim.current?.classList.remove('is-dragging');
    el.style.transform = '';
    if (scrim.current) scrim.current.style.opacity = '';
  };

  const cls = (base: string) => `${base}${leaving ? ' is-leaving' : ''}`;

  return (
    <div className="app-layer">
      <div ref={scrim} className={cls('g-scrim')} onClick={() => close.current()} aria-hidden="true" />
      <section
        ref={sheet}
        className={cls(`g-sheet app-sheet${className ? ` ${className}` : ''}`)}
        role="dialog"
        aria-modal="true"
        aria-label={title ? undefined : label}
        aria-labelledby={title ? titleId : undefined}
        tabIndex={-1}
        inert={leaving || undefined}
      >
        <div className="app-sheet__grab" onPointerDown={onDown} onPointerMove={onMove} onPointerUp={onEnd} onPointerCancel={onEnd}>
          <div className="g-sheet__grab" />
        </div>
        {title && (
          <h2 className="app-sheet__title" id={titleId}>
            {title}
          </h2>
        )}
        <div className="app-sheet__body">{children}</div>
        {footer && <div className="app-sheet__foot">{footer}</div>}
      </section>
    </div>
  );
}

/**
 * Модальный слой (шторка, сканер): стопка слоёв — Esc и Tab относятся к верхнему;
 * блокировка прокрутки страницы; фокус внутрь ([data-autofocus] или первый
 * фокусируемый) и обратно при закрытии. Возвращает ref на актуальный onClose.
 */
export function useModalLayer(ref: RefObject<HTMLElement | null>, onClose: () => void): RefObject<() => void> {
  const close = useRef(onClose);
  useEffect(() => {
    close.current = onClose;
  }, [onClose]);

  useEffect(() => {
    const me = Symbol('layer');
    const el = ref.current;
    let back = document.activeElement as HTMLElement | null;
    // Фокус в уходящем слое или потерян (сканер закрылся → открылась шторка):
    // вернуть туда, куда вернул бы его предыдущий слой
    if (!back || back === document.body || back.closest('[inert]')) {
      const prev = stack[stack.length - 1]?.el;
      back = prev ? (backs.get(prev) ?? null) : null;
    }
    // Слой открыт из другого, который закроется раньше (шторка → сканер):
    // запасной адрес — куда вернулся бы фокус из того слоя
    const owner = back?.closest<HTMLElement>('[aria-modal="true"]');
    const outerBack = owner ? backs.get(owner) : undefined;
    if (el) backs.set(el, back);
    stack.push({ id: me, el });
    locks += 1;
    document.documentElement.classList.add('app-sheet-open');
    const first = el?.querySelector<HTMLElement>('[data-autofocus]') ?? el?.querySelector<HTMLElement>(FOCUSABLE) ?? el;
    first?.focus({ preventScroll: true });

    const onKey = (e: KeyboardEvent) => {
      if (stack[stack.length - 1]?.id !== me) return;
      if (e.key === 'Escape') {
        e.preventDefault();
        close.current();
      } else if (e.key === 'Tab' && el) trapTab(e, el);
    };
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('keydown', onKey);
      const i = stack.findIndex((x) => x.id === me);
      if (i >= 0) stack.splice(i, 1);
      locks -= 1;
      if (locks === 0) document.documentElement.classList.remove('app-sheet-open');
      // Фокус назад — только если он ещё у этого слоя (или потерян): если поверх
      // уже открыт следующий слой (шторка → сканер), не уводим фокус из него
      const now = document.activeElement;
      const target = back?.isConnected ? back : outerBack?.isConnected ? outerBack : null;
      if (target && (!now || now === document.body || el?.contains(now))) target.focus({ preventScroll: true });
    };
  }, [ref]);
  return close;
}

/** Tab не уходит из открытой шторки. */
function trapTab(e: KeyboardEvent, root: HTMLElement) {
  const items = [...root.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((x) => x.offsetParent !== null);
  if (items.length === 0) return;
  const first = items[0]!;
  const last = items[items.length - 1]!;
  if (e.shiftKey && (document.activeElement === first || document.activeElement === root)) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault();
    first.focus();
  }
}
