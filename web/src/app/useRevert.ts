// Отмена движений (ФТ-5.6, POST /api/movements/revert) — одна на весь сайт:
// «Отменить» в уведомлении после расхода, списания, корректировки, применения
// шаблона и отмена из журнала. Свежие сводки — сразу в кэш, затем перезапрос.
import { useQueryClient, type QueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';
import { errorText, inventory, isNetworkError, type ProductDetail, type ProductSummary } from '../api';
import { replaceProducts } from '../lib/stock';
import { applyFreshWithQueue } from './offline';
import { invalidateStock, keys } from './queries';
import { useToast } from './toast';

/** Свежие сводки позиций — сразу в список и в карточки. */
export function applyProducts(qc: QueryClient, fresh: readonly ProductSummary[]): void {
  if (applyFreshWithQueue(qc, fresh)) return;
  qc.setQueryData<ProductSummary[]>(keys.products(), (old) => (old ? replaceProducts(old, fresh) : old));
  for (const p of fresh) {
    qc.setQueryData<ProductDetail>(keys.product(p.id), (old) => (old ? { ...old, product: p } : old));
  }
}

/** Отменяет движения по id и показывает уведомление; при ошибке — «Не отменено: …» и throw. */
export function useRevert() {
  const qc = useQueryClient();
  const toast = useToast();
  return useCallback(
    (ids: number[], done = 'Отменено') =>
      inventory
        .revert(ids)
        .then((products) => {
          applyProducts(qc, products);
          void invalidateStock(qc);
          toast.show({ text: done });
        })
        .catch((err: unknown) => {
          // Ответ не пришёл — отмена могла пройти: перечитываем, а не повторяем
          if (isNetworkError(err) && err.maybeSaved) void invalidateStock(qc);
          toast.show({ text: `Не отменено: ${errorText(err)}` });
          throw err;
        }),
    [qc, toast],
  );
}
