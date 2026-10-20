// Отправка очереди без связи (ФТ-12.4.3): при возврате связи, возврате в приложение
// и при запуске; без связи — проверка сервера (полоса связи опрашивает раз в 15 с).
import { useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { api } from '../api';
import { isOnline, subscribeConnection } from '../api/connection';
import { flushOffline, queueOf } from './offline';
import { useSession } from './session';
import { useToast } from './toast';

export function OfflineSync() {
  const qc = useQueryClient();
  const toast = useToast();
  const uid = useSession().data?.user.id;

  useEffect(() => {
    if (uid === undefined) return;
    const hasPending = () => (queueOf(qc)?.pending().length ?? 0) > 0;
    const run = () => {
      if (isOnline() && hasPending()) void flushOffline(qc, toast);
    };
    // Связь только что вернулась (ответ сервера или проверка здоровья)
    const unsub = subscribeConnection(run);
    const probe = () => {
      if (!hasPending()) return;
      if (isOnline()) run();
      else void api.health().catch(() => undefined);
    };
    const onVisible = () => {
      if (document.visibilityState === 'visible') probe();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('online', probe);
    run();
    return () => {
      unsub();
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('online', probe);
    };
  }, [qc, toast, uid]);

  return null;
}
