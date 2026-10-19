// Действия инвентаризации над экземплярами и группами и их отмена (ФТ-10.1.2–10.1.3).
// Все изменения — обычные движения; отметки хранятся в состоянии проверки (lib/inventory).
import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef, type Dispatch, type SetStateAction } from 'react';
import { errorText, inventory, isApiError, type InventoryUnit, type Location } from '../../api';
import { invalidateStock, keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { useRevert } from '../../app/useRevert';
import { REASON_FIX, REASON_MISSING, withMarks, withoutMarks, type CheckState, type Mark } from '../../lib/inventory';
import { qtyText } from '../../lib/stock';
import { plural } from '../../lib/text';
import type { After, SheetActions } from './InventorySheet';

/** Сервер принимает до 100 движений в одной отмене. */
const REVERT_CHUNK = 100;

export type MissingHow = { kind: 'lost' } | { kind: 'moved'; to: Location };

export type GroupActions = {
  confirm: (units: InventoryUnit[]) => void;
  /** present экземпляров на месте, остальные — не найдены или лежат в другом месте. */
  shortage: (units: InventoryUnit[], present: number, how: MissingHow) => Promise<After>;
};

type Deps = {
  locationId: number;
  places: readonly Location[];
  state: CheckState;
  setState: Dispatch<SetStateAction<CheckState>>;
  /** Подсветить строку, только что получившую отметку. */
  flash: (unitId: number) => void;
};

const chunks = <T,>(a: readonly T[], n: number): T[][] => Array.from({ length: Math.ceil(a.length / n) }, (_, i) => a.slice(i * n, (i + 1) * n));

export function useInventoryActions({ locationId, places, state, setState, flash }: Deps) {
  const qc = useQueryClient();
  const toast = useToast();
  const revert = useRevert();
  const latest = useRef(state);
  useEffect(() => {
    latest.current = state;
  }, [state]);

  const refresh = () => Promise.all([invalidateStock(qc), qc.invalidateQueries({ queryKey: keys.locationUnits(locationId) })]);
  const put = (entries: (readonly [number, Mark])[]) => {
    setState((s) => withMarks(s, entries));
    const first = entries[0];
    if (first) flash(first[0]);
  };

  /** После отмены: сначала экземпляры возвращаются в список, потом снимаются отметки — строки остаются на местах. */
  const drop = async (ids: readonly number[]) => {
    await qc.refetchQueries({ queryKey: keys.locationUnits(locationId) });
    setState((s) => withoutMarks(s, ids));
    void invalidateStock(qc);
  };

  /** Снимает отметки / отменяет движения. Что не удалось — остаётся с отметкой, причина в уведомлении. */
  const undo = async (ids: readonly number[]): Promise<void> => {
    const items = ids.flatMap((id) => {
      const m = latest.current.marks[String(id)];
      return m ? [{ id, m }] : [];
    });
    const undone: number[] = items.filter((x) => x.m.verdict === 'ok').map((x) => x.id);
    let failed = 0;
    let note = '';

    const byMovement = items.filter((x) => x.m.verdict === 'fixed' || x.m.verdict === 'missing');
    const withMv = byMovement.filter((x) => x.m.mv !== undefined);
    failed += byMovement.length - withMv.length;
    if (byMovement.length > withMv.length) note = 'Движение не найдено — отмена невозможна';
    for (const part of chunks(withMv, REVERT_CHUNK)) {
      try {
        await revert(
          part.map((x) => x.m.mv as number),
          part.every((x) => x.m.verdict === 'fixed') ? 'Корректировка отменена' : 'Списание отменено',
        );
        undone.push(...part.map((x) => x.id));
      } catch (err) {
        failed += part.length;
        note = isApiError(err, 404)
          ? 'Движение уже отменено или удалено — отметка осталась'
          : isApiError(err, 409)
            ? `Нельзя отменить: ${errorText(err)}`
            : `Не отменено: ${errorText(err)}`;
      }
    }

    const moved = items.filter((x) => x.m.verdict === 'moved');
    if (moved.length > 0) {
      const r = await undoMoves(moved);
      undone.push(...r.undone);
      failed += r.failed;
      note = r.note || note;
    }

    if (undone.length > 0) await drop(undone);
    if (failed > 0) {
      const all = failed === items.length;
      toast.show({ text: all ? note : `Отменено ${undone.length} из ${items.length}. ${note}` });
    } else if (moved.length > 0) {
      toast.show({ text: `Возвращено: ${placeName(locationId)}` });
    } else if (undone.length > 0 && byMovement.length === 0) {
      toast.show({ text: 'Отметка снята' });
    }
  };

  /** Обратный перенос: только то, что всё ещё лежит там, куда переложили, и активно. */
  const undoMoves = async (moved: { id: number; m: Mark }[]) => {
    const out = { undone: [] as number[], failed: 0, note: '' };
    const target = moved[0]?.m.to;
    let there: Set<number> | null = null;
    if (target !== undefined) {
      try {
        there = new Set((await inventory.locationUnits(target)).map((u) => u.id));
      } catch (err) {
        out.failed = moved.length;
        out.note = `Не отменено: ${errorText(err)}`;
        return out;
      }
    }
    for (const x of moved) {
      if (!there || x.m.to !== target || !there.has(x.id)) {
        out.failed += 1;
        out.note = moved.length === 1 ? 'Экземпляр уже изменён — отмена не выполнена' : 'Часть экземпляров уже изменена — они не возвращены';
        continue;
      }
      try {
        await inventory.moveUnit(x.id, locationId);
        out.undone.push(x.id);
      } catch (err) {
        out.failed += 1;
        out.note = `Не отменено: ${errorText(err)}`;
      }
    }
    return out;
  };

  const placeName = (lid: number) => places.find((l) => l.id === lid)?.name ?? 'другое место';

  const undoable = (ids: number[], text: string): After => () => {
    toast.show({ text, action: { label: 'Отменить', onClick: () => void undo(ids) } });
  };

  const actions: SheetActions = {
    confirm: (u) => put([[u.id, { verdict: 'ok', name: u.product.name, detail: 'Остаток верный' }]]),
    adjust: async (u, n) => {
      const r = await inventory.adjustUnit(u.id, n, REASON_FIX);
      put([[u.id, { verdict: 'fixed', name: u.product.name, detail: `${qtyText(u.remaining_qty, u.product.unit)} → ${qtyText(n, u.product.unit)}`, ...(r.movement ? { mv: r.movement.id } : {}) }]]);
      void refresh();
      return undoable([u.id], `Остаток исправлен: ${qtyText(n, u.product.unit)}`);
    },
    missing: async (u) => {
      const r = await inventory.writeoffUnit(u.id, REASON_MISSING);
      put([[u.id, { verdict: 'missing', name: u.product.name, detail: `Списано ${qtyText(u.remaining_qty, u.product.unit)}`, ...(r.movement ? { mv: r.movement.id } : {}) }]]);
      void refresh();
      return undoable([u.id], `Списано ${qtyText(u.remaining_qty, u.product.unit)} · не найдено`);
    },
    move: async (u, to) => {
      await inventory.moveUnit(u.id, to.id);
      put([[u.id, { verdict: 'moved', name: u.product.name, detail: `Лежит в месте «${to.name}»`, to: to.id }]]);
      void refresh();
      return undoable([u.id], `Перемещено: ${to.name}`);
    },
  };

  const groupActions: GroupActions = {
    confirm: (units) => {
      const group = Math.min(...units.map((u) => u.id));
      put(units.map((u) => [u.id, { verdict: 'ok', name: u.product.name, detail: 'Остаток верный', group }] as const));
    },
    shortage: async (units, present, how) => {
      const sorted = [...units].sort((a, b) => a.id - b.id);
      const group = sorted[0]?.id ?? 0;
      const gone = sorted.slice(present);
      const done: (readonly [number, Mark])[] = [];
      let failure: unknown;
      for (const u of gone) {
        try {
          if (how.kind === 'lost') {
            const r = await inventory.writeoffUnit(u.id, REASON_MISSING);
            done.push([u.id, { verdict: 'missing', name: u.product.name, detail: `Списано ${qtyText(u.remaining_qty, u.product.unit)}`, group, ...(r.movement ? { mv: r.movement.id } : {}) }]);
          } else {
            await inventory.moveUnit(u.id, how.to.id);
            done.push([u.id, { verdict: 'moved', name: u.product.name, detail: `Лежит в месте «${how.to.name}»`, group, to: how.to.id }]);
          }
        } catch (err) {
          failure = err;
          break;
        }
      }
      // Остальные отмечаются «на месте» только когда все недостающие обработаны
      if (failure === undefined) {
        for (const u of sorted.slice(0, present)) done.push([u.id, { verdict: 'ok', name: u.product.name, detail: 'Остаток верный', group }]);
      }
      if (done.length > 0) {
        put(done);
        void refresh();
      }
      if (failure !== undefined) {
        throw new Error(done.length > 0 ? `Обработано ${done.length} из ${gone.length}, дальше ошибка: ${errorText(failure)}` : errorText(failure));
      }
      const n = gone.length;
      const word = plural(n, ['экземпляр', 'экземпляра', 'экземпляров']);
      const text = how.kind === 'lost' ? `Списано ${n} ${word} · не найдено` : `Перемещено ${n} ${word}: ${how.to.name}`;
      return undoable(
        done.map(([id]) => id),
        text,
      );
    },
  };

  return { actions, groupActions, undo };
}
