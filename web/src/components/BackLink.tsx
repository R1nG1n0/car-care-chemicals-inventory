// «Назад» в шапке экрана и переход назад по истории.
import { IconChevronLeft } from '@tabler/icons-react';
import { useCallback } from 'react';
import { Link, useNavigate } from 'react-router';
import { Ico } from './ui';

/**
 * Назад по истории, если пришли из приложения (history.state.idx > 0),
 * иначе — на запасной адрес (открыли ссылку напрямую).
 */
export function useGoBack(fallback: string): () => void {
  const navigate = useNavigate();
  return useCallback(() => {
    const idx = (window.history.state as { idx?: number } | null)?.idx ?? 0;
    if (idx > 0) void navigate(-1);
    else void navigate(fallback, { replace: true });
  }, [navigate, fallback]);
}

type Props = {
  /** Куда ведёт ссылка; с history — запасной адрес. */
  to: string;
  label: string;
  /** Назад по истории: экран открывается из разных разделов («История покупок» из «Ещё» и «Купить»). */
  history?: boolean;
};

export function BackLink({ to, label, history }: Props) {
  const goBack = useGoBack(to);
  return (
    <Link
      to={to}
      className="g-btn g-btn--ghost g-btn--sm app-back"
      onClick={
        history
          ? (e) => {
              if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
              e.preventDefault();
              goBack();
            }
          : undefined
      }
    >
      <Ico icon={IconChevronLeft} />
      {label}
    </Link>
  );
}
