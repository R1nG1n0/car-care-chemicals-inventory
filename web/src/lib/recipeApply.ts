// Шаблоны расхода (ФТ-8): строки применения, нехватка, «из последнего расхода».
import type { ApplyResult, Movement, Recipe, RecipeInput, Unit } from '../api';
import { formatMoney, formatQty } from './format';
import { MAX_QTY, parseQty } from './qty';
import { qtyText, unitLabel } from './stock';
import { plural, POSITIONS } from './text';

export { MAX_QTY };

/**
 * Текст поля строки шаблона → количество; пусто — 0, мусор — null.
 * Число больше 100 000 не обрезается: форма покажет ошибку, а не тихую подмену.
 */
export function parseRowQty(s: string): number | null {
  if (s.replace(/\s/gu, '') === '') return 0;
  return parseQty(s);
}

/** «Есть только 80 мл — спишется 80» или null, если хватает. */
export function shortageText(qty: number, available: number, unit: Unit): string | null {
  if (qty <= available) return null;
  if (available <= 0) return 'Нет в наличии — строка пропустится';
  return `Есть только ${formatQty(available)} ${unitLabel(unit)} — спишется ${formatQty(available)}`;
}

export type ApplyRow = { product_id: number; qty: number };

/** Тело применения: строки с нулём пропускаются (их можно было и не слать — сервер пропустит). */
export function applyItems(rows: ApplyRow[]): ApplyRow[] {
  return rows.filter((r) => r.qty > 0).map((r) => ({ product_id: r.product_id, qty: r.qty }));
}

/** Количества поменяли относительно шаблона — оценка себестоимости сервера уже неточна. */
export function qtyChanged(rows: ApplyRow[], original: ApplyRow[]): boolean {
  if (rows.length !== original.length) return true;
  return rows.some((r, i) => r.product_id !== original[i]!.product_id || r.qty !== original[i]!.qty);
}

/**
 * Последний ручной расход по журналу (ФТ-8.5). Журнал — новые сверху.
 * Одно нажатие «Списать» — движения consume с одним batch_id без recipe_run_id
 * (добор из второй бутылки — то же нажатие). Количества одной позиции складываются,
 * порядок — как в журнале снизу вверх (как списывали).
 * Возвращает null, если ручного расхода в этой странице журнала нет.
 */
export function lastManualConsume(movements: Movement[]): ApplyRow[] | null {
  const manual = (m: Movement) => m.type === 'consume' && m.recipe_run_id === null;
  const first = movements.find(manual);
  if (!first) return null;
  const group = movements.filter((m) => manual(m) && m.batch_id === first.batch_id);
  const byProduct = new Map<number, number>();
  for (const m of [...group].reverse()) byProduct.set(m.product_id, (byProduct.get(m.product_id) ?? 0) - m.qty_delta);
  return [...byProduct].map(([product_id, qty]) => ({ product_id, qty })).filter((r) => r.qty > 0);
}

/** «Koch Gsf — списано 400 мл из 600 мл; Позиция» — отчёт о нехватке после применения. */
export function shortageReport(shortages: ApplyResult['shortages'], items: readonly Pick<Recipe['items'][number], 'product_id' | 'product'>[]): string {
  return shortages
    .map((s) => {
      const it = items.find((i) => i.product_id === s.product_id);
      if (!it) return 'Позиция';
      const u = it.product.unit;
      return `${it.product.name} — списано ${qtyText(s.consumed, u)} из ${qtyText(s.requested, u)}`;
    })
    .join('; ');
}

/** Подпись строки шаблона в списке: «4 позиции · ≈ 1 272,20 ₽». */
export function recipeSub(r: Pick<Recipe, 'items' | 'cost'>): string {
  const n = r.items.length;
  return [`${n} ${plural(n, POSITIONS)}`, r.cost !== null ? `≈ ${formatMoney(r.cost)}` : ''].filter(Boolean).join(' · ');
}

export type RecipeRowDraft = { key: number; product_id: number | null; qty: string };
export type RecipeErrors = { name?: string; rows?: string; byRow: Record<number, { product?: string; qty?: string }> };

/** Проверка формы шаблона: имя 1–60, 1–30 строк, позиции без повторов, количество 1–100 000. */
export function validateRecipe(name: string, rows: RecipeRowDraft[]): { ok: true; value: RecipeInput } | { ok: false; errors: RecipeErrors } {
  const e: RecipeErrors = { byRow: {} };
  const n = name.trim();
  if (n === '') e.name = 'Введите название';
  else if ([...n].length > 60) e.name = 'Не длиннее 60 символов';
  if (rows.length === 0) e.rows = 'Добавьте хотя бы одну позицию';
  if (rows.length > 30) e.rows = 'В шаблоне — не больше 30 строк';
  const seen = new Set<number>();
  for (const r of rows) {
    const re: { product?: string; qty?: string } = {};
    if (r.product_id === null) re.product = 'Выберите позицию';
    else if (seen.has(r.product_id)) re.product = 'Позиция уже есть в шаблоне';
    if (r.product_id !== null) seen.add(r.product_id);
    const q = parseRowQty(r.qty);
    if (q === null || q < 1 || q > MAX_QTY) re.qty = 'От 1 до 100 000';
    if (re.product || re.qty) e.byRow[r.key] = re;
  }
  if (e.name || e.rows || Object.keys(e.byRow).length > 0) return { ok: false, errors: e };
  return { ok: true, value: { name: n, items: rows.map((r) => ({ product_id: r.product_id!, qty: parseRowQty(r.qty)! })) } };
}
