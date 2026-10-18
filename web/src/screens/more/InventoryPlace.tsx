// Проверка одного места (ФТ-10.1.2–10.1.5): экземпляры, прогресс «проверено N из M»,
// действия над экземпляром, сводка при завершении. Состояние проверки живёт в localStorage
// до завершения (lib/inventory). Все изменения — обычные движения с отменой.
import { IconClipboardCheck } from '@tabler/icons-react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { errorText, inventory, type InventoryUnit, type Location } from '../../api';
import { invalidateStock, keys } from '../../app/queries';
import { useSession } from '../../app/session';
import { useToast } from '../../app/toast';
import { useRevert } from '../../app/useRevert';
import { BackLink } from '../../components/BackLink';
import { Meter } from '../../components/ProductCard';
import { Empty, LoadError, SkelList } from '../../components/ui';
import {
  clearState,
  loadState,
  mergeOrder,
  newState,
  progressOf,
  REASON_FIX,
  REASON_MISSING,
  saveState,
  shortDate,
  summarize,
  withMark,
  withoutMark,
  type CheckState,
  type Mark,
} from '../../lib/inventory';
import { qtyText } from '../../lib/stock';
import { InventoryFinish } from './InventoryFinish';
import { InventoryRow } from './InventoryRow';
import { InventorySheet, type After, type SheetActions } from './InventorySheet';

const nowSec = () => Math.floor(Date.now() / 1000);

export function InventoryPlace() {
  const id = Number(useParams()['id']);
  return Number.isInteger(id) && id > 0 ? <PlaceBody key={id} locationId={id} /> : <Missing />;
}

function Missing() {
  return (
    <div className="app-stack">
      <BackLink to="/more/inventory" label="Инвентаризация" />
      <Empty id="inv-nf" icon={IconClipboardCheck} title="Место не найдено" text="Вернитесь к списку и выберите место." />
    </div>
  );
}

function PlaceBody({ locationId }: { locationId: number }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const toast = useToast();
  const revert = useRevert();
  const userId = useSession().data?.user.id ?? 0;

  const places = useQuery({ queryKey: keys.locations(false), queryFn: ({ signal }) => inventory.locations(false, signal) });
  const units = useQuery({
    queryKey: keys.locationUnits(locationId),
    queryFn: ({ signal }) => inventory.locationUnits(locationId, signal),
    refetchOnWindowFocus: false,
  });
  const place = places.data?.find((l) => l.id === locationId);

  // Состояние проверки: из хранилища или новое; пустое не пишется
  const [state, setState] = useState<CheckState>(() => loadState(userId, locationId) ?? newState(locationId, nowSec()));
  const [resumed, setResumed] = useState(() => Object.keys(state.marks).length > 0);
  useEffect(() => saveState(userId, state), [userId, state]);

  const [fresh, setFresh] = useState<number | null>(null);
  const freshTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(freshTimer.current), []);

  const [picked, setPicked] = useState<InventoryUnit | null>(null);
  const [finishing, setFinishing] = useState(false);
  const [finishBusy, setFinishBusy] = useState(false);
  const [finishError, setFinishError] = useState<string>();

  // Порядок строк не прыгает при обновлении списка
  const [order, setOrder] = useState<number[]>([]);
  const list = units.data;
  const ids = useMemo(() => (list ?? []).map((u) => u.id), [list]);
  const merged = list ? mergeOrder(order, ids, state.marks) : order;
  if (merged.length !== order.length || merged.some((v, i) => v !== order[i])) setOrder(merged);
  const byId = useMemo(() => new Map((list ?? []).map((u) => [u.id, u])), [list]);

  const progress = progressOf(ids, state.marks);
  const summary = summarize(ids, state.marks);

  const mark = useCallback((unit: InventoryUnit, m: Omit<Mark, 'name'>) => {
    setState((s) => withMark(s, unit.id, { ...m, name: unit.product.name }));
    setResumed(false);
    setFresh(unit.id);
    clearTimeout(freshTimer.current);
    freshTimer.current = setTimeout(() => setFresh(null), 900);
  }, []);
  const unmark = (unitId: number) => setState((s) => withoutMark(s, unitId));
  const refresh = () => Promise.all([invalidateStock(qc), qc.invalidateQueries({ queryKey: keys.locationUnits(locationId) })]);

  /** После отмены: сначала экземпляр возвращается в список, потом снимается отметка — строка остаётся на своём месте. */
  const back = async (unitId: number) => {
    await qc.refetchQueries({ queryKey: keys.locationUnits(locationId) });
    unmark(unitId);
    void invalidateStock(qc);
  };

  const placeName = (lid: number) => places.data?.find((l) => l.id === lid)?.name ?? 'другое место';

  const undoable = (unit: InventoryUnit, movementId: number | undefined, text: string, undone: string): After => () => {
    toast.show(
      movementId === undefined
        ? { text }
        : {
            text,
            action: {
              label: 'Отменить',
              onClick: () =>
                void revert([movementId], undone)
                  .then(() => back(unit.id))
                  .catch(() => undefined),
            },
          },
    );
  };

  const actions: SheetActions = {
    confirm: (u) => mark(u, { verdict: 'ok', detail: 'Остаток верный' }),
    adjust: async (u, n) => {
      const r = await inventory.adjustUnit(u.id, n, REASON_FIX);
      mark(u, { verdict: 'fixed', detail: `${qtyText(u.remaining_qty, u.product.unit)} → ${qtyText(n, u.product.unit)}` });
      void refresh();
      return undoable(u, r.movement?.id, `Остаток исправлен: ${qtyText(n, u.product.unit)}`, 'Корректировка отменена');
    },
    missing: async (u) => {
      const r = await inventory.writeoffUnit(u.id, REASON_MISSING);
      mark(u, { verdict: 'missing', detail: `Списано ${qtyText(u.remaining_qty, u.product.unit)}` });
      void refresh();
      return undoable(u, r.movement?.id, `Списано ${qtyText(u.remaining_qty, u.product.unit)} · не найдено`, 'Списание отменено');
    },
    move: async (u, to: Location) => {
      await inventory.moveUnit(u.id, to.id);
      mark(u, { verdict: 'moved', detail: `Лежит в месте «${to.name}»` });
      void refresh();
      // Перемещение отмена API не умеет: возвращаем обратным перемещением
      return () => toast.show({
        text: `Перемещено: ${to.name}`,
        action: {
          label: 'Отменить',
          onClick: () =>
            void inventory
              .moveUnit(u.id, locationId)
              .then(() => back(u.id))
              .then(() => toast.show({ text: `Возвращено: ${placeName(locationId)}` }))
              .catch((err: unknown) => toast.show({ text: `Не отменено: ${errorText(err)}` })),
        },
      });
    },
  };

  const restart = () => {
    setState(newState(locationId, nowSec()));
    setResumed(false);
  };

  const finish = async () => {
    setFinishBusy(true);
    setFinishError(undefined);
    try {
      const l = await inventory.markLocationChecked(locationId);
      clearState(userId, locationId);
      await invalidateStock(qc);
      toast.show({ text: `${l.name}: проверено ${shortDate(l.checked_at ?? nowSec())}` });
      void navigate('/more/inventory', { replace: true });
    } catch (err) {
      setFinishError(errorText(err));
      setFinishBusy(false);
    }
  };

  const header = (
    <>
      <BackLink to="/more/inventory" label="Инвентаризация" />
      <h1 className="g-title">{place?.name ?? 'Место'}</h1>
    </>
  );

  if (units.isPending) {
    return (
      <div className="app-stack">
        {header}
        <SkelList label="Экземпляры загружаются" />
      </div>
    );
  }
  if (units.isError && !list) {
    return (
      <div className="app-stack">
        {header}
        <LoadError id="inv-units-err" error={units.error} what="Экземпляры" onRetry={() => void units.refetch()} />
      </div>
    );
  }

  const rows = merged;
  const empty = rows.length === 0;
  const left = progress.total - progress.done;

  return (
    <div className="app-stack inv-place">
      {header}
      {resumed && (
        <div className="g-banner" role="status">
          <span className="g-banner__text">
            Проверка не закончена с {shortDate(state.startedAt)}: отмечено {progress.done} из {progress.total}. Продолжайте с места остановки.
          </span>
          <button type="button" className="g-btn g-btn--sm g-btn--ghost" onClick={restart}>
            Начать заново
          </button>
        </div>
      )}

      {empty ? (
        <Empty
          id="inv-empty"
          icon={IconClipboardCheck}
          title="В этом месте ничего нет"
          text="Если что-то лежит здесь на самом деле, переложите это в разделе «Остатки». Пустое место тоже можно отметить проверенным."
        />
      ) : (
        <>
          <div className="inv-progress">
            <div className="stock-between">
              <b>
                Проверено {progress.done} из {progress.total}
              </b>
              <span className="g-muted">{left > 0 ? `осталось ${left}` : 'всё проверено'}</span>
            </div>
            <Meter level={progress.level} low={false} now={progress.done} max={Math.max(progress.total, 1)} label="Проверено экземпляров" className="inv-meter" />
          </div>
          <ul className="g-list inv-list" aria-label="Экземпляры места">
            {rows.map((uid) => {
              const u = byId.get(uid) ?? null;
              return (
                <InventoryRow
                  key={uid}
                  unit={u}
                  mark={state.marks[String(uid)] ?? null}
                  fresh={fresh === uid}
                  onConfirm={() => u && actions.confirm(u)}
                  onMore={() => setPicked(u)}
                />
              );
            })}
          </ul>
        </>
      )}

      <div className="inv-foot" data-toast-anchor>
        <button type="button" className="g-btn g-btn--cta g-btn--block g-press" onClick={() => (setFinishError(undefined), setFinishing(true))}>
          Завершить проверку
        </button>
      </div>

      <InventorySheet unit={picked} places={places.data ?? []} actions={actions} onClose={() => setPicked(null)} />
      <InventoryFinish
        open={finishing}
        placeName={place?.name ?? 'Место'}
        summary={summary}
        busy={finishBusy}
        error={finishError}
        onFinish={() => void finish()}
        onClose={() => setFinishing(false)}
      />
    </div>
  );
}
