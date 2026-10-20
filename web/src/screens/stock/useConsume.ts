// Расход (ФТ-5.1, ФТ-5.6): POST /api/consume, уведомление «Списано 40 мл · …»
// с «Отменить» на 10 секунд (общий useRevert), обновление кэша.
import { useMutation, useQueryClient } from '@tanstack/react-query';
import type { ProductSummary } from '../../api';
import { consumeOrQueue, toastQueued } from '../../app/offline';
import { invalidateStock } from '../../app/queries';
import { useToast } from '../../app/toast';
import { applyProducts, useRevert } from '../../app/useRevert';
import { consumeToastText } from '../../lib/stockQty';

/** Пауза перед сменой шкалы и наклейки: шторка уже уходит (прототип, --dur-fast). */
const PAINT_DELAY = 160;

export type ConsumeVars = { product: ProductSummary; qty: number; unitId?: number | undefined };

export function useConsume(onDone?: () => void) {
  const qc = useQueryClient();
  const toast = useToast();
  const revert = useRevert();
  return useMutation({
    mutationFn: (v: ConsumeVars) => consumeOrQueue(qc, v),
    onSuccess: (res, v) => {
      onDone?.();
      // Нет связи: запись ушла в очередь, остаток на экране — оценка (ФТ-12.3)
      if ('queued' in res) return toastQueued(qc, toast, res.queued);
      const r = res.sent;
      setTimeout(() => {
        applyProducts(qc, [r.product]);
        void invalidateStock(qc);
      }, PAINT_DELAY);
      const ids = r.movements.map((m) => m.id);
      toast.show({
        text: consumeToastText(v.product.name, v.product.unit, v.qty, r),
        action: { label: 'Отменить', onClick: () => void revert(ids, 'Расход отменён').catch(() => undefined) },
      });
    },
  });
}
