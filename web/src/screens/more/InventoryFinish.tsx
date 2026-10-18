// Завершение проверки (ФТ-10.1.4): сводка и отметка места «проверено ДД.ММ».
import { Sheet } from '../../components/Sheet';
import { busyProps } from '../../components/ui';
import { exemplars, summaryLines, type Summary } from '../../lib/inventory';

type Props = {
  open: boolean;
  placeName: string;
  summary: Summary;
  busy: boolean;
  error: string | undefined;
  onFinish: () => void;
  onClose: () => void;
};

export function InventoryFinish({ open, placeName, summary, busy, error, onFinish, onClose }: Props) {
  const lines = summaryLines(summary);
  return (
    <Sheet open={open} onClose={onClose} title={`Итоги: ${placeName}`}>
      <div className="stock-gap">
        {lines.length > 0 ? (
          <ul className="g-list inv-sum" aria-label="Что изменилось">
            {lines.map((l) => (
              <li key={l.verdict} className={`g-row inv-sum__row inv-sum__row--${l.verdict}`}>
                <span className="g-row__main g-row__title">{l.label}</span>
                <span className="g-num">{l.count}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="g-muted">Ничего не отмечено.</p>
        )}
        {summary.unchecked > 0 && (
          <p className="g-muted">Не проверено: {exemplars(summary.unchecked)} — они останутся как есть.</p>
        )}
        {error && (
          <span className="g-field__error" role="alert">
            {error}
          </span>
        )}
        <button type="button" className="g-btn g-btn--cta g-btn--block g-press" onClick={onFinish} {...busyProps(busy)}>
          {busy ? 'Сохраняю…' : 'Отметить место проверенным'}
        </button>
        <button type="button" className="g-btn g-btn--block g-btn--ghost" onClick={onClose}>
          Вернуться к проверке
        </button>
      </div>
    </Sheet>
  );
}
