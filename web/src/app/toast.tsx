// Уведомления внизу экрана (.g-toast, motion.css). Одно за раз: новое заменяет старое.
// С действием «Отменить» — полоска-таймер 10 с (ФТ-5.6). Пауза, пока вкладка скрыта,
// и пока курсор или фокус внутри уведомления (WCAG 2.2.1).
//
// Где показывать: над нижним меню, а если у экрана есть закреплённая панель действий
// ([data-toast-anchor] — «Списать всё», «Сохранить покупку») — над ней, чтобы не
// закрыть ни кнопку, ни отчёт в панели. Положение пересчитывается при прокрутке.
import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react';

export type ToastOptions = {
  text: string;
  /** Кнопка в уведомлении, обычно «Отменить». */
  action?: { label: string; onClick: () => void };
  /** Сколько показывать, мс. С действием по умолчанию 10 000 (--dur-undo), без — 3000. */
  duration?: number;
};

type ToastState = ToastOptions & { id: number; leaving: boolean };

type ToastApi = { show: (t: ToastOptions) => void; hide: () => void };

const ToastContext = createContext<ToastApi | null>(null);

const LEAVE_MS = 200; // --dur-fast + запас
const GAP = 8; // --sp-2 между панелью и уведомлением

/** Панель действий текущего (не уходящего) экрана, если она видна. */
function anchorTop(): number | null {
  const el = document.querySelector<HTMLElement>('.g-view:not(.is-leaving) [data-toast-anchor]');
  if (!el) return null;
  const r = el.getBoundingClientRect();
  if (r.height === 0 || r.top >= window.innerHeight) return null;
  return r.top;
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<ToastState | null>(null);
  const [hidden, setHidden] = useState(false);
  const [hold, setHold] = useState(false);
  const paused = hidden || hold;
  const seq = useRef(0);
  const left = useRef(0);
  const startedAt = useRef(0);
  const slot = useRef<HTMLDivElement>(null);

  const hide = useCallback(() => setToast((t) => (t ? { ...t, leaving: true } : t)), []);
  const show = useCallback((t: ToastOptions) => {
    seq.current += 1;
    left.current = t.duration ?? (t.action ? 10_000 : 3000);
    setHold(false);
    setToast({ ...t, id: seq.current, leaving: false });
  }, []);

  // Пауза, пока вкладка скрыта: время на отмену не утекает в фоне.
  useEffect(() => {
    const onVis = () => setHidden(document.hidden);
    document.addEventListener('visibilitychange', onVis);
    return () => document.removeEventListener('visibilitychange', onVis);
  }, []);

  useEffect(() => {
    if (!toast || toast.leaving || paused) return;
    startedAt.current = Date.now();
    const t = setTimeout(hide, left.current);
    return () => {
      clearTimeout(t);
      left.current -= Date.now() - startedAt.current;
    };
  }, [toast, paused, hide]);

  useEffect(() => {
    if (!toast?.leaving) return;
    const t = setTimeout(() => setToast((cur) => (cur && cur.leaving ? null : cur)), LEAVE_MS);
    return () => clearTimeout(t);
  }, [toast]);

  // Над панелью действий экрана: --toast-bottom, пока уведомление на экране
  const visible = toast !== null;
  useLayoutEffect(() => {
    const el = slot.current;
    if (!el || !visible) return;
    let frame = 0;
    const place = () => {
      frame = 0;
      const top = anchorTop();
      if (top === null) el.style.removeProperty('--toast-bottom');
      else el.style.setProperty('--toast-bottom', `${Math.round(window.innerHeight - top + GAP)}px`);
    };
    const later = () => {
      if (!frame) frame = requestAnimationFrame(place);
    };
    place();
    window.addEventListener('scroll', later, { passive: true });
    window.addEventListener('resize', later);
    return () => {
      if (frame) cancelAnimationFrame(frame);
      window.removeEventListener('scroll', later);
      window.removeEventListener('resize', later);
    };
  }, [visible, toast?.id]);

  const value = useMemo(() => ({ show, hide }), [show, hide]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div ref={slot} className="app-toast-slot" role="status" aria-live="polite">
        {toast && (
          <div
            key={toast.id}
            className={`g-toast${toast.action ? '' : ' app-toast--plain'}${toast.leaving ? ' is-leaving' : ''}`}
            data-paused={paused || undefined}
            onPointerEnter={() => setHold(true)}
            onPointerLeave={() => setHold(false)}
            onFocus={() => setHold(true)}
            onBlur={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setHold(false);
            }}
          >
            <span className="app-toast__text">{toast.text}</span>
            {toast.action && (
              <button
                type="button"
                className="g-btn g-btn--sm g-press"
                onClick={() => {
                  toast.action?.onClick();
                  hide();
                }}
              >
                {toast.action.label}
              </button>
            )}
          </div>
        )}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast вне ToastProvider');
  return ctx;
}
