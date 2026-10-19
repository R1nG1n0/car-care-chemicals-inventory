// Отчёт «Покупки» (ФТ-10.2.2): итог, по месяцам, по категориям, по магазинам.
import { IconReceipt } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { inventory } from '../../api';
import { keys } from '../../app/queries';
import { catClass } from '../../components/refs';
import { Empty, LoadError, SkelList } from '../../components/ui';
import { formatMoney } from '../../lib/format';
import { plural } from '../../lib/text';
import { MonthBars, ShareList } from './ReportBars';

const PURCHASES = ['покупка', 'покупки', 'покупок'] as const;

export function ReportsSpending({ from, to }: { from: string; to: string }) {
  const q = useQuery({
    queryKey: keys.report('spending', from, to),
    queryFn: ({ signal }) => inventory.spending(from, to, signal),
    placeholderData: (prev) => prev,
  });
  if (q.isPending) return <SkelList rows={3} label="Отчёт загружается" />;
  if (q.isError && !q.data) return <LoadError id="rep-sp-err" error={q.error} what="Отчёт" onRetry={() => void q.refetch()} />;
  const r = q.data;
  if (!r) return null;
  if (r.purchases_count === 0) {
    return <Empty id="rep-sp-empty" icon={IconReceipt} title="Покупок нет" text="За этот период покупок не было. Выберите другой период или добавьте покупку." />;
  }
  return (
    <div className="app-stack" aria-busy={q.isFetching && q.isPlaceholderData ? true : undefined}>
      <div className="rep-total">
        <span className="g-label">Потрачено</span>
        <span className="g-num rep-total__num">{formatMoney(r.total)}</span>
        <span className="g-muted">
          {r.purchases_count} {plural(r.purchases_count, PURCHASES)}
        </span>
      </div>

      {r.by_month.length > 1 && (
        <section className="app-stack" aria-labelledby="rep-sp-months">
          <h2 className="g-label" id="rep-sp-months">
            По месяцам
          </h2>
          <MonthBars label="Расходы на покупки по месяцам" items={r.by_month.map((m) => ({ month: m.month, value: m.total }))} />
        </section>
      )}

      <section className="app-stack" aria-labelledby="rep-sp-cats">
        <h2 className="g-label" id="rep-sp-cats">
          По категориям
        </h2>
        <ShareList
          label="Расходы по категориям"
          total={r.total}
          rows={r.by_category.map((c) => ({
            key: c.category_id,
            name: c.name,
            value: c.total,
            valueText: formatMoney(c.total),
            catClass: catClass(c.color),
          }))}
        />
      </section>

      <section className="app-stack" aria-labelledby="rep-sp-stores">
        <h2 className="g-label" id="rep-sp-stores">
          По магазинам
        </h2>
        <ShareList
          label="Расходы по магазинам"
          total={r.total}
          rows={r.by_store.map((s) => ({ key: s.store_id ?? 'none', name: s.name, value: s.total, valueText: formatMoney(s.total) }))}
        />
      </section>
    </div>
  );
}
