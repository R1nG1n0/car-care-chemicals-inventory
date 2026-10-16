// Экраны до входа в приложение: подложка загрузки и «Сервер недоступен» (И-3а).
import { IconAlertTriangle, IconRefresh, IconWifiOff } from '@tabler/icons-react';
import { isNetworkError } from '../api';
import { formatTime } from '../lib/text';
import { busyProps, Empty, Ico, Logo } from '../components/ui';

/** Первый запрос /api/auth/me ещё идёт: подложки по форме карточек. */
export function BootScreen() {
  return (
    <main className="app-plain">
      <Logo />
      <div className="app-stack" aria-busy="true" aria-label="Загрузка">
        {[0, 1, 2].map((k) => (
          <div className="g-skel" aria-hidden="true" key={k}>
            <div className="g-skel__strip" />
            <div className="g-skel__body">
              <div className="g-skel__line" />
              <div className="g-skel__num" />
              <div className="g-skel__bar" />
              <div className="g-skel__line g-skel__line--short" />
            </div>
          </div>
        ))}
      </div>
    </main>
  );
}

type OfflineProps = { error: unknown; lastAttempt: number; retrying: boolean; onRetry: () => void };

/** Сервер не ответил при запуске: VPN выключен или сервер лежит. */
export function OfflineScreen({ error, lastAttempt, retrying, onRetry }: OfflineProps) {
  const network = isNetworkError(error);
  return (
    <main className="app-plain">
      <Logo />
      <Empty
        id="offline-title"
        error
        icon={network ? IconWifiOff : IconAlertTriangle}
        title={network ? 'Сервер недоступен' : 'Ошибка сервера'}
        text={
          network
            ? 'Сайт работает только через VPN. Проверьте, что AmneziaWG включён.'
            : error instanceof Error
              ? error.message
              : 'Не удалось загрузить данные'
        }
        actions={
          <button type="button" className="g-btn g-btn--cta g-btn--block g-press" onClick={() => !retrying && onRetry()} {...busyProps(retrying)}>
            <Ico icon={IconRefresh} /> {retrying ? 'Проверяю…' : 'Повторить'}
          </button>
        }
        meta={lastAttempt ? `Последняя попытка: ${formatTime(lastAttempt)}` : undefined}
      />
    </main>
  );
}
