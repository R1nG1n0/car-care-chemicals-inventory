// Хуки для экранов: очередь без связи, метка «не отправлено», «Нужна связь».
import { useQueryClient } from '@tanstack/react-query';
import { useMemo, useSyncExternalStore } from 'react';
import { opProductIds, type QueueOp } from '../lib/offlineQueue';
import { queueOf } from './offline';
import { useOnline, useSession } from './session';

const NONE: readonly QueueOp[] = [];

/** Неотправленные записи текущего пользователя. */
export function usePendingOps(): readonly QueueOp[] {
  const qc = useQueryClient();
  const uid = useSession().data?.user.id;
  const q = uid === undefined ? null : queueOf(qc);
  return useSyncExternalStore(
    (cb) => (q ? q.subscribe(cb) : () => undefined),
    () => q?.list() ?? NONE,
    () => NONE,
  );
}

/** Позиции, по которым есть неотправленный расход (метка на карточках). */
export function usePendingProductIds(): ReadonlySet<number> {
  const ops = usePendingOps();
  return useMemo(() => opProductIds(ops), [ops]);
}

export const NEEDS_ONLINE = 'Нужна связь';

/**
 * Действия, недоступные без связи (ФТ-12.2.3): offline — связи нет;
 * guard() не пускает действие и подсказывает «Нужна связь».
 */
export function useNeedsOnline() {
  const offline = !useOnline();
  return {
    offline,
    /** Свойства кнопки: неактивна и с подсказкой. */
    props: offline ? ({ 'aria-disabled': true, title: NEEDS_ONLINE } as const) : {},
    hint: offline ? NEEDS_ONLINE : undefined,
    guard:
      <A extends unknown[]>(fn: (...a: A) => void) =>
      (...a: A) => {
        if (!offline) fn(...a);
      },
  };
}
