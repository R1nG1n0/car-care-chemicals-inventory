// Карточка средства (ФТ-15): назначения, оценка, аналоги. Чистые функции — покрыты тестами.
import type { ProductDetail, ProductSummary, Purpose } from '../api';

export const MAX_RATING = 5;
/** Инструкция длиннее этого сворачивается («Показать полностью»). */
export const INSTRUCTION_FOLD = 280;

/** Назначения, доступные для выбора: действующие и уже выбранные архивные (ФТ-15.1.3). */
export function purposeChoices(all: readonly Purpose[], selected: readonly number[]): Purpose[] {
  return all.filter((p) => !p.archived || selected.includes(p.id));
}

/** Выбор чипа: добавляет id или убирает; порядок — как выбирали. */
export function toggleId(ids: readonly number[], id: number): number[] {
  return ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id];
}

/** Имена назначений позиции в порядке справочника; неизвестные id пропускаются. */
export function purposesOf(ids: readonly number[] | undefined, byId: ReadonlyMap<number, Purpose>): Purpose[] {
  const set = new Set(ids ?? []);
  return [...byId.values()].filter((p) => set.has(p.id)).sort((a, b) => a.sort - b.sort || a.id - b.id);
}

/** Оценка после нажатия на звезду: повторное нажатие на текущую снимает оценку (ФТ-15.4.1). */
export function nextRating(current: number | null, clicked: number): number | null {
  return current === clicked ? null : clicked;
}

/** Подпись оценки для диктора: «Оценка 4 из 5» / «Без оценки». */
export function ratingLabel(rating: number | null | undefined): string {
  return rating ? `Оценка ${rating} из ${MAX_RATING}` : 'Без оценки';
}

/** Текстовые поля карточки: сервер может положить их в product или рядом с ним. */
export function aboutOf(d: ProductDetail): { incompatible: string; instruction: string } {
  return {
    incompatible: (d.product.incompatible ?? d.incompatible ?? '').trim(),
    instruction: (d.product.instruction ?? d.instruction ?? '').trim(),
  };
}

/** pH для показа: «7,5»; не указан — null. */
export function phText(ph: number | null | undefined): string | null {
  return ph === null || ph === undefined ? null : String(ph).replace('.', ',');
}

/** Блок «Есть дома для того же» нужен, когда позиции нет или она ниже минимума (ФТ-15.5.3). */
export function wantsAnalogs(p: Pick<ProductSummary, 'low' | 'stock'>): boolean {
  return p.low || p.stock.total === 0;
}

/** Пометка «Не брать больше»: заметна и в карточке, и в списке. */
export const DO_NOT_BUY_TEXT = 'не брать';

/** Нужен ли сворачиваемый показ инструкции. */
export const isLongText = (s: string): boolean => [...s].length > INSTRUCTION_FOLD || s.split('\n').length > 5;
