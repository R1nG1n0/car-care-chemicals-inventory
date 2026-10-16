// Уведомления внизу экрана (.g-toast, motion.css). Одно за раз: новое заменяет старое.
// С действием «Отменить» — полоска-таймер 10 с (ФТ-5.6), пауза, пока вкладка скрыта.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

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

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<ToastState | null>(null);
  const [paused, setPaused] = useState(false);
  const seq = useRef(0);
  const left = useRef(0);
  const startedAt = useRef(0);

  const hide = useCallback(() => setToast((t) => (t ? { ...t, leaving: true } : t)), []);
  const show = useCallback((t: ToastOptions) => {
    seq.current += 1;
    left.current = t.duration ?? (t.action ? 10_000 : 3000);
    setToast({ ...t, id: seq.current, leaving: false });
  }, []);

  // Пауза, пока вкладка скрыта: время на отмену не утекает в фоне.
  useEffect(() => {
    const onVis = () => setPaused(document.hidden);
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

  const value = useMemo(() => ({ show, hide }), [show, hide]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="app-toast-slot" role="status" aria-live="polite">
        {toast && (
          <div
            key={toast.id}
            className={`g-toast${toast.action ? '' : ' app-toast--plain'}${toast.leaving ? ' is-leaving' : ''}`}
            data-paused={paused || undefined}
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
