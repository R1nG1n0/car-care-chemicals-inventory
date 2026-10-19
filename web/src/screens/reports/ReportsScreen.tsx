// «Ещё → Отчёты» (ФТ-10.2): период, вкладки «Покупки» и «Расход».
import { useState } from 'react';
import { BackLink } from '../../components/BackLink';
import { customError, periodOf, ymdOf, type Period } from '../../lib/reports';
import { ReportsConsumption } from './ReportsConsumption';
import { ReportsPeriod } from './ReportsPeriod';
import { ReportsSpending } from './ReportsSpending';

type Tab = 'spending' | 'consumption';

const TABS: { id: Tab; label: string }[] = [
  { id: 'spending', label: 'Покупки' },
  { id: 'consumption', label: 'Расход' },
];

export function ReportsScreen() {
  const [today] = useState(() => ymdOf(new Date()));
  const [period, setPeriod] = useState<Period>(() => periodOf('month', today));
  const [tab, setTab] = useState<Tab>('spending');
  const valid = period.kind !== 'custom' || customError(period.from, period.to) === '';

  return (
    <div className="app-stack">
      <BackLink to="/more" label="Ещё" />
      <h1 className="g-title">Отчёты</h1>
      <div className="g-seg" role="group" aria-label="Отчёт">
        {TABS.map((t) => (
          <button key={t.id} type="button" aria-pressed={tab === t.id} onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </div>
      <ReportsPeriod period={period} today={today} onChange={setPeriod} />
      {valid && (
        <div key={tab} className="rep-panel">
          {tab === 'spending' ? <ReportsSpending from={period.from} to={period.to} /> : <ReportsConsumption from={period.from} to={period.to} />}
        </div>
      )}
    </div>
  );
}
