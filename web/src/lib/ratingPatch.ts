// Оптимистичная правка оценки (ФТ-15.4.1): экран меняется сразу, при ошибке возвращается прежнее.
import type { ProductDetail, ProductSummary } from '../api';

export type RatingChange = { rating?: number | null; do_not_buy?: boolean };

/** Сводка позиции с применённой правкой; поля, которых в правке нет, не трогаются. */
export function applyRating<T extends Pick<ProductSummary, 'rating' | 'do_not_buy'>>(p: T, c: RatingChange): T {
  return {
    ...p,
    ...(c.rating !== undefined ? { rating: c.rating } : {}),
    ...(c.do_not_buy !== undefined ? { do_not_buy: c.do_not_buy } : {}),
  };
}

export function applyRatingToDetail(d: ProductDetail | undefined, c: RatingChange): ProductDetail | undefined {
  return d ? { ...d, product: applyRating(d.product, c) } : d;
}
