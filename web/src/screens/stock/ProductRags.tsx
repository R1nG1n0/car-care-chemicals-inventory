// Блок «В работе» в карточке позиции с ресурсом стирок (ФТ-19.5): активные тряпки со шкалой
// износа, «В работу» (списывает 1 шт), выбор нескольких и «Постирал (N)» с «Отменить» на 10 с,
// метка и списание в шторке, «Списанные (N)» свёрнуты. Всё — только со связью (ФТ-19.6).
import { IconAlertTriangle, IconCheck, IconChevronDown } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type CSSProperties } from 'react';
import { errorText, rags, type Rag, type ProductSummary } from '../../api';
import { invalidateStock, keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { useNeedsOnline } from '../../app/useOffline';
import { applyProducts } from '../../app/useRevert';
import { newOpId } from '../../lib/offlineQueue';
import { Ico } from '../../components/ui';
import { keepActive, levelText, mergeRags, NO_STOCK_HINT, retireReasonText, sortRags, startLabel, toggleAll, toggleSelected, washedText, washLabel, wearLevel, wearShare, wearText } from '../../lib/rags';
import { formatDay } from '../../lib/text';
import { RagSheet } from './RagSheet';

function RagRow({ rag, limit, selecting, selected, onPick }: { rag: Rag; limit: number; selecting: boolean; selected: boolean; onPick: () => void }) {
  const level = rag.level ?? wearLevel(rag.washes, limit);
  const lt = levelText(level);
  const label = [rag.label, `стирок ${wearText(rag.washes, limit)}`, lt].filter(Boolean).join(', ');
  return (
    <button type="button" className={`rag-row${selected ? ' is-selected' : ''}`} aria-label={label} aria-pressed={selecting ? selected : undefined} onClick={onPick}>
      {selecting && (
        <span className="rag-row__check" aria-hidden="true">
          {selected && <Ico icon={IconCheck} size={18} />}
        </span>
      )}
      <span className="rag-row__body">
        <span className="rag-row__top">
          <b className="rag-row__name">{rag.label}</b>
          <span className="rag-row__count">стирок {wearText(rag.washes, limit)}</span>
        </span>
        <span className={`g-wear g-wear--${level}`} aria-hidden="true">
          <span className="g-wear__fill" style={{ '--lv': wearShare(rag.washes, limit).toFixed(3) } as CSSProperties} />
        </span>
        {lt && (
          <span className={`rag-row__level rag-row__level--${level}`}>
            <Ico icon={IconAlertTriangle} size={14} /> {lt}
          </span>
        )}
      </span>
    </button>
  );
}

export function ProductRags({ product: p }: { product: ProductSummary }) {
  const qc = useQueryClient();
  const toast = useToast();
  const online = useNeedsOnline();
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<number[]>([]);
  const [open, setOpen] = useState<Rag | null>(null);
  const [showOld, setShowOld] = useState(false);
  const key = keys.rags(p.id, true);
  const list = useQuery({ queryKey: key, queryFn: ({ signal }) => rags.list(p.id, true, signal), enabled: Boolean(p.wear_limit) });
  const all = list.data ?? [];
  const active = sortRags(all.filter((r) => r.status === 'active'));
  const retired = all.filter((r) => r.status === 'retired').sort((a, b) => (b.retired_at ?? 0) - (a.retired_at ?? 0));
  const picked = keepActive(selected, all);

  const unstart = useMutation({
    mutationFn: (id: number) => rags.unstart(id),
    onSuccess: async (r) => {
      applyProducts(qc, [r.product]);
      await Promise.all([invalidateStock(qc), qc.invalidateQueries({ queryKey: key })]);
      toast.show({ text: 'Отменено: тряпка убрана, 1 шт возвращена' });
    },
    onError: (err) => toast.show({ text: `Не отменено: ${errorText(err)}` }),
  });
  const start = useMutation({
    mutationFn: () => rags.start(p.id, { client_op_id: newOpId() }),
    onSuccess: async (r) => {
      applyProducts(qc, [r.product]);
      await Promise.all([invalidateStock(qc), qc.invalidateQueries({ queryKey: key })]);
      toast.show({ text: `${r.rag.label} в работе`, action: { label: 'Отменить', onClick: () => unstart.mutate(r.rag.id) } });
    },
    onError: (err) => toast.show({ text: errorText(err) }),
  });

  const unwash = useMutation({
    mutationFn: (ids: number[]) => rags.unwash(ids),
    onSuccess: (fresh) => {
      qc.setQueryData<Rag[]>(key, (old) => (old ? mergeRags(old, fresh) : old));
      void invalidateStock(qc);
      toast.show({ text: 'Стирка отменена' });
    },
    onError: (err) => toast.show({ text: `Не отменено: ${errorText(err)}` }),
  });
  const wash = useMutation({
    mutationFn: (ids: number[]) => rags.wash(ids),
    onSuccess: (fresh, ids) => {
      // Шкалы сдвигаются сразу (transform), затем сводка позиции перечитывается
      qc.setQueryData<Rag[]>(key, (old) => (old ? mergeRags(old, fresh) : old));
      void invalidateStock(qc);
      setSelected([]);
      setSelecting(false);
      toast.show({ text: washedText(fresh.length), action: { label: 'Отменить', onClick: () => !unwash.isPending && !wash.isPending && unwash.mutate(ids) } });
    },
    onError: (err) => toast.show({ text: errorText(err) }),
  });

  const limit = p.wear_limit;
  if (!limit) return null;
  const noStock = p.stock.total <= 0;
  const blocked = online.offline;

  return (
    <section className="rags" aria-labelledby="item-rags-h">
      <div className="rags__head">
        <h2 className="g-label" id="item-rags-h">
          В работе{active.length > 0 ? ` · ${active.length}` : ''}
        </h2>
        {active.length > 0 && (
          <button
            type="button"
            className="g-btn g-btn--sm g-btn--ghost g-press"
            aria-pressed={selecting}
            {...online.props}
            onClick={online.guard(() => {
              setSelecting((v) => !v);
              setSelected([]);
            })}
          >
            {selecting ? 'Готово' : 'Выбрать'}
          </button>
        )}
      </div>
      <p className="g-field__hint">Ресурс — {limit} стирок на тряпку</p>

      {list.isError && !list.data && <p className="g-field__hint">Список тряпок не загрузился{online.offline ? ': нужна связь' : ''}.</p>}
      {list.data && active.length === 0 && <p className="g-muted">Тряпок в работе нет. «В работу» спишет 1 шт из запаса и заведёт новую тряпку.</p>}
      {active.length > 0 && (
        <div className="rags__list">
          {active.map((r) => (
            <RagRow key={r.id} rag={r} limit={r.wear_limit ?? limit} selecting={selecting} selected={picked.includes(r.id)} onPick={() => (selecting ? setSelected(toggleSelected(picked, r.id)) : !blocked && setOpen(r))} />
          ))}
        </div>
      )}

      {selecting && active.length > 0 ? (
        <div className="rags__actions">
          <button type="button" className="g-btn g-press" onClick={() => setSelected(toggleAll(picked, active))}>
            {picked.length === active.length ? 'Снять выбор' : 'Выбрать все'}
          </button>
          <button
            type="button"
            className="g-btn g-press"
            aria-disabled={picked.length === 0 || blocked || undefined}
            title={online.hint}
            aria-busy={wash.isPending || undefined}
            onClick={() => picked.length > 0 && !blocked && !wash.isPending && wash.mutate(picked)}
          >
            {washLabel(picked.length)}
          </button>
        </div>
      ) : (
        <button
          type="button"
          className="g-btn g-btn--block g-press"
          aria-disabled={noStock || blocked || undefined}
          aria-describedby={noStock ? 'item-rags-nostock' : undefined}
          title={blocked ? online.hint : noStock ? NO_STOCK_HINT : undefined}
          aria-busy={start.isPending || undefined}
          onClick={() => !noStock && !blocked && !start.isPending && start.mutate()}
        >
          {startLabel(p.stock.total)}
        </button>
      )}
      {noStock && !selecting && (
        <p className="g-field__hint" id="item-rags-nostock">
          {NO_STOCK_HINT}
        </p>
      )}
      {online.hint && <p className="g-field__hint">{online.hint}: тряпки меняются только онлайн.</p>}

      {retired.length > 0 && (
        <div className="rags__old">
          <button type="button" className="g-btn g-btn--sm g-btn--ghost g-press" aria-expanded={showOld} aria-controls="item-rags-old" onClick={() => setShowOld((v) => !v)}>
            Списанные ({retired.length}) <Ico icon={IconChevronDown} size={16} />
          </button>
          {showOld && (
            <ul className="rags__old-list" id="item-rags-old">
              {retired.map((r) => (
                <li key={r.id} className="g-muted">
                  {r.label} · стирок {wearText(r.washes, r.wear_limit ?? limit)} · {retireReasonText(r.retire_reason)}
                  {r.retired_at ? `, ${formatDay(r.retired_at)}` : ''}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
      <RagSheet rag={open} onClose={() => setOpen(null)} />
    </section>
  );
}
