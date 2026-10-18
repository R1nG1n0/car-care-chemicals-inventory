// Расход (ФТ-5.1, ФТ-5.6): POST /api/consume, уведомление «Списано 40 мл · …»
// с «Отменить» на 10 секунд (общий useRevert), обновление кэша.
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { inventory, type ProductSummary } from '../../api';
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
    mutationFn: (v: ConsumeVars) => inventory.consume({ product_id: v.product.id, qty: v.qty, unit_id: v.unitId }),
    onSuccess: (r, v) => {
      onDone?.();
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
