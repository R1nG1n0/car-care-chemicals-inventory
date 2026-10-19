// Инвентаризация (ФТ-10.1.1, ФТ-10.1.4): выбор места. Давно не проверенные и ни разу
// не проверенные — сверху, под названием «проверено ДД.ММ» или «не проверялось».
import { IconChevronRight, IconClipboardCheck, IconMapPin } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { inventory } from '../../api';
import { keys } from '../../app/queries';
import { useSession } from '../../app/session';
import { BackLink } from '../../components/BackLink';
import { Empty, Ico, LoadError, SkelList } from '../../components/ui';
import { checkedLabel, exemplars, isStale, loadState, sortLocations } from '../../lib/inventory';

export function InventoryScreen() {
  const userId = useSession().data?.user.id;
  const q = useQuery({ queryKey: keys.locations(false), queryFn: ({ signal }) => inventory.locations(false, signal) });
  const places = useMemo(() => sortLocations(q.data ?? []), [q.data]);
  const [now] = useState(() => Math.floor(Date.now() / 1000));
  // Незавершённые проверки читаются из хранилища один раз на набор мест, а не на каждый рендер
  const pending = useMemo(() => {
    const m = new Map<number, number>();
    if (userId === undefined) return m;
    for (const l of places) {
      const saved = loadState(userId, l.id, now);
      if (saved) m.set(l.id, Object.keys(saved.marks).length);
    }
    return m;
  }, [places, userId, now]);

  return (
    <div className="app-stack">
      <BackLink to="/more" label="Ещё" />
      <h1 className="g-title">Инвентаризация</h1>
      <p className="g-muted">Выберите место и сверьте, что в нём лежит. Давно не проверенные — сверху.</p>
      {q.isPending && <SkelList label="Места загружаются" />}
      {q.isError && !q.data && <LoadError id="inv-err" error={q.error} what="Места" onRetry={() => void q.refetch()} />}
      {q.data && places.length === 0 && (
        <Empty id="inv-empty" icon={IconMapPin} title="Мест пока нет" text="Добавьте места хранения в разделе «Ещё → Места», тогда их можно будет проверять." />
      )}
      {places.length > 0 && (
        <nav className="g-list" aria-label="Места для проверки">
          {places.map((l) => {
            const marked = pending.get(l.id) ?? 0;
            const stale = l.checked_at !== null && isStale(l.checked_at, now);
            return (
              <Link key={l.id} to={`/more/inventory/${l.id}`} className="g-row g-row--link">
                <Ico icon={IconClipboardCheck} />
                <span className="g-row__main">
                  <span className="g-row__title inv-block">{l.name}</span>
                  <span className="g-row__sub inv-block">
                    {checkedLabel(l.checked_at)} · {exemplars(l.units_count)}
                    {marked > 0 && <b className="inv-pending"> · идёт проверка, отмечено {marked}</b>}
                  </span>
                </span>
                {stale && <span className="g-chip">давно</span>}
                <span className="g-row__go">
                  <Ico icon={IconChevronRight} />
                </span>
              </Link>
            );
          })}
        </nav>
      )}
    </div>
  );
}
