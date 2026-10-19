// Отчёт «Расход» (ФТ-10.2.3): списано по позициям в мл/шт и в деньгах
// (себестоимость), по месяцам; списания (испорчено, не найдено) и корректировки
// остатка вниз (потери при инвентаризации) — отдельными блоками, в расход не входят.
import { IconDroplet } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { inventory, type ProductUse } from '../../api';
import { keys } from '../../app/queries';
import { Empty, LoadError, SkelList } from '../../components/ui';
import { formatMoney } from '../../lib/format';
import { tzOffset } from '../../lib/reports';
import { qtyText } from '../../lib/stock';
import { plural, POSITIONS } from '../../lib/text';
import { MonthBars, ShareList, type ShareRow } from './ReportBars';

const costText = (c: number | null) => (c === null ? 'без цены' : formatMoney(c));

function useRows(list: readonly ProductUse[]): ShareRow[] {
  return list.map((u) => ({
    key: u.product_id,
    name: u.name,
    value: u.cost ?? 0,
    valueText: costText(u.cost),
    sub: qtyText(u.qty, u.unit),
  }));
}

export function ReportsConsumption({ from, to }: { from: string; to: string }) {
  const tz = tzOffset();
  const q = useQuery({
    queryKey: keys.report('consumption', from, to),
    queryFn: ({ signal }) => inventory.consumption(from, to, tz, signal),
    placeholderData: (prev) => prev,
  });
  const used = useRows(q.data?.by_product ?? []);
  const lost = useRows(q.data?.writeoffs.by_product ?? []);
  const fixed = useRows(q.data?.corrections.by_product ?? []);
  if (q.isPending) return <SkelList rows={3} label="Отчёт загружается" />;
  if (q.isError && !q.data) return <LoadError id="rep-co-err" error={q.error} what="Отчёт" onRetry={() => void q.refetch()} />;
  const r = q.data;
  if (!r) return null;
  if (r.by_product.length === 0 && r.writeoffs.by_product.length === 0 && r.corrections.by_product.length === 0) {
    return <Empty id="rep-co-empty" icon={IconDroplet} title="Расхода нет" text="За этот период ничего не списывали. Выберите другой период." />;
  }
  return (
    <div className="app-stack" aria-busy={q.isFetching && q.isPlaceholderData ? true : undefined}>
      <div className="rep-total">
        <span className="g-label">Списано на сумму</span>
        <span className="g-num rep-total__num">{formatMoney(r.total_cost)}</span>
        <span className="g-muted">
          {r.by_product.length} {plural(r.by_product.length, POSITIONS)} · по цене покупки
        </span>
      </div>

      {r.by_month.length > 1 && (
        <section className="app-stack" aria-labelledby="rep-co-months">
          <h2 className="g-label" id="rep-co-months">
            По месяцам
          </h2>
          <MonthBars label="Стоимость расхода по месяцам" items={r.by_month.map((m) => ({ month: m.month, value: m.cost }))} />
        </section>
      )}

      {r.by_product.length > 0 && (
        <section className="app-stack" aria-labelledby="rep-co-items">
          <h2 className="g-label" id="rep-co-items">
            По позициям
          </h2>
          <ShareList label="Расход по позициям" total={r.total_cost} rows={used} />
        </section>
      )}

      {r.writeoffs.by_product.length > 0 && (
        <section className="app-stack" aria-labelledby="rep-co-lost">
          <h2 className="g-label" id="rep-co-lost">
            Списания: {formatMoney(r.writeoffs.cost)}
          </h2>
          <p className="g-muted">Испорчено, не найдено, потеряно — в расход не входит.</p>
          <ShareList label="Списания по позициям" total={r.writeoffs.cost} rows={lost} />
        </section>
      )}

      {r.corrections.by_product.length > 0 && (
        <section className="app-stack" aria-labelledby="rep-co-fix">
          <h2 className="g-label" id="rep-co-fix">
            Корректировки: {formatMoney(r.corrections.cost)}
          </h2>
          <p className="g-muted">Остаток уменьшили при инвентаризации или вручную — в расход не входит.</p>
          <ShareList label="Корректировки по позициям" total={r.corrections.cost} rows={fixed} />
        </section>
      )}
    </div>
  );
}
