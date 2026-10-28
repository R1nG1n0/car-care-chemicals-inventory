// Оценка и «Не брать больше» прямо в карточке позиции (ФТ-15.4): PATCH /rating без формы,
// оптимистично — экран меняется сразу, при ошибке возвращается прежнее и показывается уведомление.
// Без связи (ФТ-12.2.3) кнопки неактивны с подсказкой «Нужна связь».
import { IconBan } from '@tabler/icons-react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { errorText, inventory, type ProductDetail, type ProductSummary } from '../../api';
import { invalidateStock, keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { useNeedsOnline } from '../../app/useOffline';
import { RatingStars } from '../../components/Rating';
import { Ico } from '../../components/ui';
import { applyRatingToDetail, type RatingChange } from '../../lib/ratingPatch';

export function ProductRating({ product: p }: { product: ProductSummary }) {
  const qc = useQueryClient();
  const toast = useToast();
  const online = useNeedsOnline();

  const save = useMutation({
    mutationFn: (c: RatingChange) => inventory.setRating(p.id, c),
    onMutate: async (c) => {
      await qc.cancelQueries({ queryKey: keys.product(p.id) });
      const before = qc.getQueryData<ProductDetail>(keys.product(p.id));
      qc.setQueryData<ProductDetail | undefined>(keys.product(p.id), (d) => applyRatingToDetail(d, c));
      return { before };
    },
    onError: (err, _c, ctx) => {
      if (ctx?.before) qc.setQueryData(keys.product(p.id), ctx.before);
      toast.show({ text: errorText(err) });
    },
    onSettled: () => invalidateStock(qc),
  });

  return (
    <section className="about-rate" aria-labelledby="item-rate-h">
      <h2 className="g-label" id="item-rate-h">
        Оценка
      </h2>
      <div className="about-rate__row">
        <RatingStars value={p.rating} disabled={online.offline} hint={online.hint} onChange={(rating) => save.mutate({ rating })} />
        <button
          type="button"
          className={`g-btn g-btn--sm g-press${p.do_not_buy ? ' g-btn--danger' : ''}`}
          aria-pressed={p.do_not_buy}
          {...online.props}
          onClick={online.guard(() => save.mutate({ do_not_buy: !p.do_not_buy }))}
        >
          <Ico icon={IconBan} /> Не брать больше
        </button>
      </div>
      {p.do_not_buy && <p className="g-field__hint">Не попадёт в список покупок и не предлагается как аналог.</p>}
      {online.hint && <p className="g-field__hint">{online.hint}: оценку можно поставить только онлайн.</p>}
    </section>
  );
}
