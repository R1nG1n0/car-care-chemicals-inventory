// Ещё → Данные (ФТ-10.3): скачать всё одним файлом, таблицы CSV для Excel, загрузить из файла.
// Скачивание — обычные ссылки: cookie сессии уходит сама, файл сохраняет браузер.
import { IconDownload, IconFileSpreadsheet, IconFileExport } from '@tabler/icons-react';
import { BackLink } from '../../components/BackLink';
import { Ico } from '../../components/ui';
import { downloadHref, type DownloadKind } from '../../lib/transfer';
import { DataImport } from './DataImport';

const FILES: { kind: DownloadKind; title: string; sub: string; csv: boolean }[] = [
  { kind: 'json', title: 'Скачать всё (JSON)', sub: 'Полная копия: для переноса и восстановления', csv: false },
  { kind: 'products', title: 'Позиции с остатками (CSV)', sub: 'Для Excel', csv: true },
  { kind: 'purchases', title: 'Покупки (CSV)', sub: 'Строки покупок, для Excel', csv: true },
  { kind: 'movements', title: 'Движения (CSV)', sub: 'Журнал прихода и расхода, для Excel', csv: true },
];

export function DataScreen() {
  return (
    <div className="app-stack">
      <BackLink to="/more" label="Ещё" />
      <h1 className="g-title">Данные</h1>
      <section className="g-section" aria-labelledby="data-down">
        <span className="g-label" id="data-down">
          Скачать
        </span>
        <div className="g-list">
          {FILES.map((f) => (
            <a key={f.kind} href={downloadHref(f.kind)} download className="g-row g-row--link" id={`data-${f.kind}`}>
              <Ico icon={f.csv ? IconFileSpreadsheet : IconFileExport} />
              <span className="g-row__main">
                <span className="g-row__title data-block">{f.title}</span>
                <span className="g-row__sub data-block">{f.sub}</span>
              </span>
              <span className="g-row__go">
                <Ico icon={IconDownload} />
              </span>
            </a>
          ))}
        </div>
      </section>
      <DataImport />
    </div>
  );
}
