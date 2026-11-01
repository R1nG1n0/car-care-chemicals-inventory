// Шторка тряпки (ФТ-19.2, ФТ-19.4): сменить метку на бирке и списать с причиной.
// Действия — только со связью (ФТ-19.6), ответ сервера сразу кладётся в список позиции.
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { errorText, rags, type Rag, type RetireReason } from '../../api';
import { invalidateStock } from '../../app/queries';
import { useToast } from '../../app/toast';
import { useNeedsOnline } from '../../app/useOffline';
import { Sheet } from '../../components/Sheet';
import { busyProps, Field } from '../../components/ui';
import { MAX_RAG_LABEL, RETIRE_REASONS, validateRagLabel } from '../../lib/rags';

export function RagSheet({ rag, onClose }: { rag: Rag | null; onClose: () => void }) {
  // Шторка закрывается с анимацией: пока она уходит, тряпка нужна для содержимого
  const [last, setLast] = useState(rag);
  if (rag && rag !== last) setLast(rag);
  const shown = rag ?? last;
  return (
    <Sheet open={rag !== null} onClose={onClose} title={shown ? shown.label : ''}>
      {shown && <Body key={shown.id} rag={shown} onClose={onClose} />}
    </Sheet>
  );
}

function Body({ rag, onClose }: { rag: Rag; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const online = useNeedsOnline();
  const [label, setLabel] = useState(rag.label);
  const [error, setError] = useState('');
  const [reason, setReason] = useState<RetireReason | null>(null);

  const done = async (text: string) => {
    await invalidateStock(qc);
    toast.show({ text });
    onClose();
  };
  const rename = useMutation({
    mutationFn: (v: string) => rags.rename(rag.id, v),
    onSuccess: (r) => done(`Метка: ${r.label}`),
    onError: (err) => setError(errorText(err)),
  });
  const retire = useMutation({
    mutationFn: (r: RetireReason) => rags.retire(rag.id, r),
    onSuccess: (r) => done(`Списана: ${r.label}`),
    onError: (err) => setError(errorText(err)),
  });

  const submitRename = () => {
    if (rename.isPending || online.offline) return;
    const r = validateRagLabel(label);
    if (!r.ok) return setError(r.error);
    setError('');
    if (r.value === rag.label) return onClose();
    rename.mutate(r.value);
  };

  return (
    <div className="app-stack rag-sheet">
      <form
        className="app-stack"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          submitRename();
        }}
      >
        <Field id="rag-label" label="Метка на бирке" maxLength={MAX_RAG_LABEL} autoComplete="off" value={label} error={error} hint="Эту метку пишете маркером на бирке" onChange={(e) => (setLabel(e.target.value), setError(''))} />
        <button type="submit" className="g-btn g-btn--block g-press" {...online.props} {...busyProps(rename.isPending)}>
          Переименовать
        </button>
      </form>

      <div className="g-field">
        <span className="g-label" id="rag-reason-l">
          Списать тряпку
        </span>
        <div className="stock-reasons" role="group" aria-labelledby="rag-reason-l">
          {RETIRE_REASONS.map((r) => (
            <button key={r.value} type="button" className="g-btn g-btn--sm g-press" aria-pressed={reason === r.value} onClick={() => setReason(r.value)}>
              {r.label}
            </button>
          ))}
        </div>
        <button
          type="button"
          className="g-btn g-btn--block g-btn--danger g-press"
          aria-disabled={reason === null || online.offline || undefined}
          title={online.hint}
          {...busyProps(retire.isPending)}
          onClick={() => reason && !online.offline && !retire.isPending && retire.mutate(reason)}
        >
          Списать
        </button>
        {reason === null && <span className="g-field__hint">Выберите причину: износ, потеряна или испорчена</span>}
      </div>
      {online.hint && <p className="g-field__hint">{online.hint}: тряпки меняются только онлайн.</p>}
    </div>
  );
}
