// Неотправленное (ФТ-12.5.1): записи расхода без связи — время, позиция, количество,
// состояние (ждёт / ошибка с причиной); изменить количество, списать сколько есть,
// удалить (с подтверждением), «Отправить сейчас».
import { IconCloudCheck } from '@tabler/icons-react';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { isOnline } from '../../api/connection';
import { flushOffline, queueOf } from '../../app/offline';
import { useToast } from '../../app/toast';
import { usePendingOps } from '../../app/useOffline';
import { BackLink } from '../../components/BackLink';
import { QtyInput } from '../../components/QtyInput';
import { busyProps, Empty, HeadRow } from '../../components/ui';
import { opQtyText, opTitle, type ApplyItem, type QueueOp } from '../../lib/offlineQueue';
import { clampQty, MAX_QTY, parseQty } from '../../lib/qty';
import { formatDay, formatTime } from '../../lib/text';

/** Редактор количества одной строки: число, «Сохранить», «Отмена». */
function QtyEdit({ id, item, onSave, onCancel }: { id: string; item: { qty: number; unit: ApplyItem['unit']; name: string }; onSave: (qty: number) => void; onCancel: () => void }) {
  const [v, setV] = useState(String(item.qty));
  const n = parseQty(v);
  const bad = n === null || n <= 0 || n > MAX_QTY;
  return (
    <div className="app-stack">
      <QtyInput id={id} label={item.name} value={v} onChange={setV} unit={item.unit} min={1} max={MAX_QTY} error={bad ? 'Введите количество' : undefined} />
      <div className="g-invite__actions">
        <button type="button" className="g-btn g-btn--sm g-press" aria-disabled={bad || undefined} onClick={() => !bad && onSave(clampQty(n, 1, MAX_QTY))}>
          Сохранить
        </button>
        <button type="button" className="g-btn g-btn--sm g-btn--ghost" onClick={onCancel}>
          Отмена
        </button>
      </div>
    </div>
  );
}

function OpRow({ op }: { op: QueueOp }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [edit, setEdit] = useState<number | 'one' | null>(null);
  const [ask, setAsk] = useState(false);
  const queue = queueOf(qc);
  const when = `${formatDay(op.occurred_at)} ${formatTime(op.occurred_at * 1000)}`;
  const failed = op.status === 'error';
  // Поправленная запись уходит сразу, если связь есть
  const kick = () => {
    if (isOnline()) void flushOffline(qc, toast);
  };
  const canTakeAll = op.kind === 'consume' && op.available !== undefined && op.available > 0;

  return (
    <div className={`g-row shop-tpl-row${failed ? ' g-row--warn' : ''}`}>
      <div className="g-row__main">
        <div className="g-row__title">{opTitle(op)}</div>
        <div className="g-row__sub">
          {when} · {opQtyText(op)}
        </div>
        <div className={failed ? 'shop-short' : 'g-row__sub'}>{failed ? (op.error ?? 'Не принято сервером') : 'Ждёт отправки'}</div>
      </div>
      {edit !== null && op.kind === 'consume' && (
        <QtyEdit
          id={`pend-q-${op.op_id}`}
          item={{ qty: op.qty, unit: op.unit, name: op.name }}
          onSave={(n) => {
            queue?.setQty(op.op_id, n);
            setEdit(null);
            kick();
          }}
          onCancel={() => setEdit(null)}
        />
      )}
      {typeof edit === 'number' && op.kind === 'apply' && (
        <QtyEdit
          id={`pend-q-${op.op_id}`}
          item={op.items.find((i) => i.product_id === edit) ?? { qty: 1, unit: 'ml', name: '' }}
          onSave={(n) => {
            queue?.setQty(op.op_id, n, edit);
            setEdit(null);
            kick();
          }}
          onCancel={() => setEdit(null)}
        />
      )}
      {edit === null && !ask && (
        <div className="g-invite__actions">
          {!failed ? null : op.kind === 'consume' ? (
            <button type="button" className="g-btn g-btn--sm g-press" onClick={() => setEdit('one')}>
              Изменить количество
            </button>
          ) : (
            op.items.map((i) => (
              <button key={i.product_id} type="button" className="g-btn g-btn--sm g-press" onClick={() => setEdit(i.product_id)}>
                {i.name}: изменить
              </button>
            ))
          )}
          {canTakeAll && op.kind === 'consume' && (
            <button type="button" className="g-btn g-btn--sm g-press" onClick={() => (queue?.setQty(op.op_id, op.available!), kick())}>
              Списать сколько есть
            </button>
          )}
          <button type="button" className="g-btn g-btn--sm g-btn--ghost g-btn--danger-text" onClick={() => setAsk(true)}>
            Удалить
          </button>
        </div>
      )}
      {ask && (
        <div className="g-invite" role="group" aria-label="Удалить запись">
          <b>Удалить запись?</b>
          <span className="g-invite__meta">Остаток на сервере не изменится — эта запись так и не будет списана.</span>
          <div className="g-invite__actions">
            <button type="button" className="g-btn g-btn--sm g-btn--danger g-press" autoFocus onClick={() => queue?.remove(op.op_id)}>
              Удалить
            </button>
            <button type="button" className="g-btn g-btn--sm g-press" onClick={() => setAsk(false)}>
              Отмена
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export function PendingScreen() {
  const qc = useQueryClient();
  const toast = useToast();
  const ops = usePendingOps();
  const [sending, setSending] = useState(false);

  const send = async () => {
    if (sending) return;
    setSending(true);
    try {
      const r = await flushOffline(qc, toast, { retryErrors: true });
      if (r && r.sent === 0 && r.failed === 0 && r.stopped) toast.show({ text: 'Нет связи — отправим, когда появится VPN' });
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="app-stack">
      <HeadRow>
        <BackLink to="/more" label="Ещё" />
      </HeadRow>
      <h1 className="g-title">Неотправленное</h1>
      {ops.length === 0 ? (
        <Empty id="pend-none" icon={IconCloudCheck} title="Всё отправлено" text="Записей, ожидающих связи, нет." />
      ) : (
        <>
          <p className="shop-hint">Эти записи сделаны без связи. Они уйдут на сервер сами, когда появится VPN.</p>
          <div className="g-list">
            {ops.map((op) => (
              <OpRow key={op.op_id} op={op} />
            ))}
          </div>
          <button type="button" className="g-btn g-btn--cta g-btn--block g-press" onClick={() => void send()} {...busyProps(sending)}>
            {sending ? 'Отправляю…' : 'Отправить сейчас'}
          </button>
        </>
      )}
    </div>
  );
}
