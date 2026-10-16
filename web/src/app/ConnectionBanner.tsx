// Полоса «Нет связи» вверху экрана, если связь пропала во время работы (И-3а).
// Пока её видно, сервер опрашивается раз в 10 секунд: вернулась связь —
// полоса исчезает сама, а данные экранов перезапрашиваются.
import { IconWifiOff } from '@tabler/icons-react';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import { busyProps, Ico } from '../components/ui';
import { useOnline } from './session';

const PING_MS = 10_000;

export function ConnectionBanner() {
  const online = useOnline();
  const client = useQueryClient();
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

  if (online) return null;
  return (
    <div className="g-banner g-banner--danger app-banner" role="alert">
      <Ico icon={IconWifiOff} />
      <span className="g-banner__text">Нет связи — изменения не сохранятся</span>
      <button type="button" className="g-btn g-btn--sm g-press" onClick={() => !checking && void check()} {...busyProps(checking)}>
        Повторить
      </button>
    </div>
  );
}
