// Шаблоны расхода (ФТ-8): строки применения, нехватка, «из последнего расхода».
import type { Movement, RecipeInput, Unit } from '../api';
import { formatQty } from './format';
import { unitLabel } from './stock';

export const MAX_QTY = 100_000;

/** Шаг кнопок ± в строке шаблона: для мл — малый шаг из профиля, для шт — 1. */
export function rowStep(unit: Unit, stepSmall: number): number {
  return unit === 'pcs' ? 1 : stepSmall;
}

export function clampQty(n: number): number {
  if (!Number.isFinite(n)) return 0;
  return Math.min(MAX_QTY, Math.max(0, Math.trunc(n)));
}

/** Текст поля → количество; пусто — 0, мусор — null. */
export function parseQty(s: string): number | null {
  const t = s.replace(/\s/g, '');
  if (t === '') return 0;
  if (!/^\d+$/.test(t)) return null;
  return clampQty(Number(t));
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
 * Одно нажатие «Списать» — движения consume с одинаковым created_at без recipe_run_id
 * (batch_id в JSON не отдаётся). Количества одной позиции складываются,
 * порядок — как в журнале снизу вверх (как списывали).
 * Возвращает null, если ручного расхода в этой странице журнала нет.
 */
export function lastManualConsume(movements: Movement[]): ApplyRow[] | null {
  const first = movements.find((m) => m.type === 'consume' && m.recipe_run_id === null);
  if (!first) return null;
  const group = movements.filter((m) => m.type === 'consume' && m.recipe_run_id === null && m.created_at === first.created_at);
  const byProduct = new Map<number, number>();
  for (const m of [...group].reverse()) byProduct.set(m.product_id, (byProduct.get(m.product_id) ?? 0) - m.qty_delta);
  return [...byProduct].map(([product_id, qty]) => ({ product_id, qty })).filter((r) => r.qty > 0);
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
    const q = parseQty(r.qty);
    if (q === null || q < 1) re.qty = 'От 1 до 100 000';
    if (re.product || re.qty) e.byRow[r.key] = re;
  }
  if (e.name || e.rows || Object.keys(e.byRow).length > 0) return { ok: false, errors: e };
  return { ok: true, value: { name: n, items: rows.map((r) => ({ product_id: r.product_id!, qty: parseQty(r.qty)! })) } };
}
