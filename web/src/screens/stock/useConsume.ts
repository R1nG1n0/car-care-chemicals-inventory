// Расход и отмена (ФТ-5.1, ФТ-5.6): POST /api/consume, уведомление «Списано 40 мл · …»
// с «Отменить» на 10 секунд (POST /api/movements/revert), обновление кэша.
import { useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { inventory, type ProductDetail, type ProductSummary } from '../../api';
import { invalidateStock, keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { replaceProducts } from '../../lib/stock';
import { consumeToastText } from '../../lib/stockQty';

/** Пауза перед сменой шкалы и наклейки: шторка уже уходит (прототип, --dur-fast). */
const PAINT_DELAY = 160;

/** Свежие сводки позиций — сразу в список и в карточки, затем перезапрос. */
export function applyProducts(qc: QueryClient, fresh: readonly ProductSummary[]): void {
  qc.setQueryData<ProductSummary[]>(keys.products(), (old) => (old ? replaceProducts(old, fresh) : old));
  for (const p of fresh) {
    qc.setQueryData<ProductDetail>(keys.product(p.id), (old) => (old ? { ...old, product: p } : old));
  }
}

export type ConsumeVars = { product: ProductSummary; qty: number; unitId?: number | undefined };

/** Отмена движений по id с уведомлением; общая для «Отменить» и журнала. */
export function useRevert() {
  const qc = useQueryClient();
  const toast = useToast();
  return (ids: number[], done = 'Отменено') =>
    inventory
      .revert(ids)
      .then((products) => {
        applyProducts(qc, products);
        void invalidateStock(qc);
        toast.show({ text: done });
      })
      .catch((err: unknown) => {
        toast.show({ text: err instanceof Error ? `Не отменено: ${err.message}` : 'Не отменено' });
        throw err;
      });
}

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
