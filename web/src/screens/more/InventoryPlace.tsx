// Проверка одного места (ФТ-10.1.2–10.1.5): экземпляры, прогресс «проверено N из M»,
// действия над экземпляром, сводка при завершении. Состояние проверки живёт в localStorage
// до завершения (lib/inventory). Все изменения — обычные движения с отменой.
import { IconClipboardCheck } from '@tabler/icons-react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { errorText, inventory, type InventoryUnit } from '../../api';
import { invalidateStock, keys } from '../../app/queries';
import { useSession } from '../../app/session';
import { useToast } from '../../app/toast';
import { BackLink } from '../../components/BackLink';
import { Meter } from '../../components/ProductCard';
import { Empty, LoadError, SkelList } from '../../components/ui';
import { buildRows, clearState, loadState, mergeOrder, newState, progressOf, saveState, shortDate, summarize, type CheckState } from '../../lib/inventory';
import { InventoryFinish } from './InventoryFinish';
import { InventoryGroupRow } from './InventoryGroupRow';
import { InventoryGroupSheet } from './InventoryGroupSheet';
import { InventoryRow } from './InventoryRow';
import { InventorySheet } from './InventorySheet';
import { useInventoryActions } from './useInventoryActions';

const nowSec = () => Math.floor(Date.now() / 1000);

export function InventoryPlace() {
  const id = Number(useParams()['id']);
  const userId = useSession().data?.user.id;
  if (!Number.isInteger(id) || id <= 0) return <Missing />;
  // Состояние проверки привязано к пользователю: пока сессия не известна, хранилище не трогаем
  if (userId === undefined) return <div className="app-stack"><SkelList label="Загрузка" /></div>;
  return <PlaceBody key={`${userId}:${id}`} userId={userId} locationId={id} />;
}

function Missing() {
  return (
    <div className="app-stack">
      <BackLink to="/more/inventory" label="Инвентаризация" />
      <Empty id="inv-nf" icon={IconClipboardCheck} title="Место не найдено" text="Вернитесь к списку и выберите место." />
    </div>
  );
}

function PlaceBody({ userId, locationId }: { userId: number; locationId: number }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const toast = useToast();

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
  const flash = (unitId: number) => {
    setResumed(false);
    setFresh(unitId);
    clearTimeout(freshTimer.current);
    freshTimer.current = setTimeout(() => setFresh(null), 900);
  };
  const { actions, groupActions, undo } = useInventoryActions({ locationId, places: places.data ?? [], state, setState, flash });

  const [picked, setPicked] = useState<InventoryUnit | null>(null);
  const [pickedGroup, setPickedGroup] = useState<InventoryUnit[] | null>(null);
  const [undoing, setUndoing] = useState<string | null>(null);
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
  const rows = useMemo(() => buildRows(merged, byId, state.marks), [merged, byId, state.marks]);

  const progress = progressOf(ids, state.marks);
  const summary = summarize(ids, state.marks);

  const runUndo = async (key: string, unitIds: readonly number[]) => {
    setUndoing(key);
    try {
      await undo(unitIds);
    } finally {
      setUndoing(null);
    }
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
            {rows.map((row) => {
              if (row.kind === 'group') {
                const members = row.ids.flatMap((i) => byId.get(i) ?? []);
                const marks = row.done ? row.ids.flatMap((i) => state.marks[String(i)] ?? []) : [];
                return (
                  <InventoryGroupRow
                    key={row.key}
                    units={members}
                    marks={marks}
                    count={row.ids.length}
                    fresh={fresh !== null && row.ids.includes(fresh)}
                    busy={undoing === row.key}
                    onConfirm={() => groupActions.confirm(members)}
                    onShortage={() => setPickedGroup(members)}
                    onUndo={() => void runUndo(row.key, row.ids)}
                  />
                );
              }
              const u = byId.get(row.id) ?? null;
              return (
                <InventoryRow
                  key={row.id}
                  unit={u}
                  mark={state.marks[String(row.id)] ?? null}
                  fresh={fresh === row.id}
                  busy={undoing === String(row.id)}
                  onConfirm={() => u && actions.confirm(u)}
                  onMore={() => setPicked(u)}
                  onUndo={() => void runUndo(String(row.id), [row.id])}
                />
              );
            })}
          </ul>
        </>
      )}

      {!finishing && (
        <div className="inv-foot" data-toast-anchor>
          <button type="button" className="g-btn g-btn--cta g-btn--block g-press" onClick={() => (setFinishError(undefined), setFinishing(true))}>
            Завершить проверку
          </button>
        </div>
      )}

      <InventorySheet unit={picked} places={places.data ?? []} actions={actions} onClose={() => setPicked(null)} />
      <InventoryGroupSheet units={pickedGroup} places={places.data ?? []} actions={groupActions} onClose={() => setPickedGroup(null)} />
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
