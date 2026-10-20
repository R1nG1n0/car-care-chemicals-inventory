// Полоса связи вверху экрана (И-3а, ФТ-12.2.2, ФТ-12.5.1).
// Нет связи: «Нет связи · данные на 20.10 14:05» и счётчик неотправленного.
// Связь есть, но часть записей сервер не принял, — полоса напоминает о них.
// Пока связи нет, сервер опрашивается раз в 15 секунд: ответил — полоса
// исчезает сама, данные экранов перезапрашиваются, очередь уходит (OfflineSync).
import { IconAlertTriangle, IconWifiOff } from '@tabler/icons-react';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router';
import { api } from '../api';
import { busyProps, Ico } from '../components/ui';
import { lastSyncOf } from './offline';
import { useOnline } from './session';
import { usePendingOps } from './useOffline';
import { formatDay, formatTime, plural } from '../lib/text';

export const PING_MS = 15_000;
const RECORDS = ['запись', 'записи', 'записей'] as const;

export function ConnectionBanner() {
  const online = useOnline();
  const client = useQueryClient();
  const ops = usePendingOps();
  const [checking, setChecking] = useState(false);

  const check = useCallback(async () => {
    setChecking(true);
    try {
      await api.health();
      await client.invalidateQueries();
    } catch {
      // Связи всё ещё нет — полоса остаётся.
    } finally {
      setChecking(false);
    }
  }, [client]);

  useEffect(() => {
    if (online) return;
    const t = setInterval(() => void api.health().then(() => client.invalidateQueries()).catch(() => undefined), PING_MS);
    return () => clearInterval(t);
  }, [online, client]);

  const failed = ops.filter((o) => o.status === 'error').length;
  const count = (
    <Link to="/more/pending" className="g-btn g-btn--sm g-press">
      Не отправлено: {ops.length}
    </Link>
  );

  if (online) {
    if (failed === 0) return null;
    return (
      <div className="g-banner app-banner" role="status">
        <Ico icon={IconAlertTriangle} />
        <span className="g-banner__text">
          Сервер не принял {failed} {plural(failed, RECORDS)}
        </span>
        <span className="g-banner__actions">{count}</span>
      </div>
    );
  }
  const at = lastSyncOf(client);
  return (
    <div className="g-banner app-banner" role="alert">
      <Ico icon={IconWifiOff} />
      <span className="g-banner__text">Нет связи{at ? ` · данные на ${formatDay(at / 1000)} ${formatTime(at)}` : ''}</span>
      <span className="g-banner__actions">
        {ops.length > 0 && count}
        <button type="button" className="g-btn g-btn--sm g-press" onClick={() => !checking && void check()} {...busyProps(checking)}>
          Повторить
        </button>
      </span>
    </div>
  );
}
