// Тряпки в работе (ФТ-19): уровень износа, подписи, выбор нескольких, проверка ресурса и метки.
import type { Rag, RagLevel, RagsSummary, RetireReason } from '../api';
import { charCount } from './validate';

export const MAX_WEAR_LIMIT = 1000;
export const MAX_RAG_LABEL = 20;
/** От какой доли ресурса тряпка «скоро менять» (ФТ-19.4). */
export const SOON_SHARE = 0.9;

/** Уровень износа: от 90 % ресурса — «скоро», на 100 % и больше — «пора». */
export function wearLevel(washes: number, limit: number): RagLevel {
  if (limit <= 0) return 'ok';
  if (washes >= limit) return 'due';
  return washes >= limit * SOON_SHARE ? 'soon' : 'ok';
}

/** Доля износа 0…1 для шкалы. */
export function wearShare(washes: number, limit: number): number {
  if (limit <= 0 || washes <= 0) return 0;
  return Math.min(1, washes / limit);
}

/** «37 из 100»; ресурса нет (учёт снят) — просто число стирок. */
export const wearText = (washes: number, limit: number | null): string => (limit ? `${washes} из ${limit}` : String(washes));

/** Подпись уровня; у целой тряпки — null (цвет не единственный признак: подпись есть всегда у soon и due). */
export function levelText(level: RagLevel): string | null {
  return level === 'due' ? 'пора менять' : level === 'soon' ? 'скоро менять' : null;
}

/** Строка позиции на главной: «в работе 5 · пора менять 1». Нет тряпок в работе — null. */
export function ragsLine(r: RagsSummary | null | undefined): { text: string; urgent: boolean } | null {
  if (!r || r.active <= 0) return null;
  const parts = [`в работе ${r.active}`];
  if (r.due > 0) parts.push(`пора менять ${r.due}`);
  else if (r.soon > 0) parts.push(`скоро менять ${r.soon}`);
  return { text: parts.join(' · '), urgent: r.due > 0 };
}

export const RETIRE_REASONS: { value: RetireReason; label: string }[] = [
  { value: 'worn', label: 'Износилась' },
  { value: 'lost', label: 'Потеряна' },
  { value: 'damaged', label: 'Испорчена' },
];

export const retireReasonText = (r: RetireReason | null): string => RETIRE_REASONS.find((x) => x.value === r)?.label.toLowerCase() ?? '';

/** Ресурс стирок из поля формы: пусто — null (учёт выключен), иначе целое 1–1000. */
export function parseWearLimit(raw: string): { ok: true; value: number | null } | { ok: false; error: string } {
  const t = raw.replace(/[\s\u00a0]/gu, '');
  if (t === '') return { ok: true, value: null };
  if (!/^\d{1,4}$/u.test(t)) return { ok: false, error: `Целое число от 1 до ${MAX_WEAR_LIMIT} или пусто` };
  const n = Number(t);
  if (n < 1 || n > MAX_WEAR_LIMIT) return { ok: false, error: `Целое число от 1 до ${MAX_WEAR_LIMIT} или пусто` };
  return { ok: true, value: n };
}

/** Ресурс в поле формы: undefined — позиция загружена без поля (старый снимок), форма ресурс не правит и не отправляет. */
export function wearInput(limit: number | null | undefined): string | undefined {
  return limit === undefined ? undefined : limit ? String(limit) : '';
}

/** Метка тряпки: непустая, до 20 символов. */
export function validateRagLabel(raw: string): { ok: true; value: string } | { ok: false; error: string } {
  const t = raw.trim();
  if (t === '') return { ok: false, error: 'Введите метку' };
  if (charCount(t) > MAX_RAG_LABEL) return { ok: false, error: `Не длиннее ${MAX_RAG_LABEL} символов` };
  return { ok: true, value: t };
}

/** Выбор нескольких: добавляет или убирает id, порядок выбора сохраняется. */
export function toggleSelected(selected: readonly number[], id: number): number[] {
  return selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id];
}

/** Оставляет в выборе только тряпки, которые ещё в работе (после списания или обновления списка). */
export function keepActive(selected: readonly number[], rags: readonly Pick<Rag, 'id' | 'status'>[]): number[] {
  const live = new Set(rags.filter((r) => r.status === 'active').map((r) => r.id));
  return selected.filter((id) => live.has(id));
}

/** Все активные: если все уже выбраны — снять выбор. */
export function toggleAll(selected: readonly number[], rags: readonly Pick<Rag, 'id'>[]): number[] {
  return selected.length === rags.length ? [] : rags.map((r) => r.id);
}

/** Подпись кнопки стирки: «Постирал (3)». */
export const washLabel = (n: number): string => `Постирал (${n})`;
export const washedText = (n: number): string => `Постирано: ${n}`;

/** Кнопка «В работу»: остаток запаса в подписи; нет запаса — подсказка. */
export function startLabel(total: number): string {
  return `В работу · в запасе ${total}`;
}
export const NO_STOCK_HINT = 'Нет запаса: добавьте покупку или остаток';

/** Заменяет в списке обновлённые тряпки (ответ стирки и отмены), остальные не трогает. */
export function mergeRags(list: readonly Rag[], fresh: readonly Rag[]): Rag[] {
  const by = new Map(fresh.map((r) => [r.id, r]));
  return list.map((r) => by.get(r.id) ?? r);
}

/** Активные тряпки по номеру, затем по id. */
export function sortRags(list: readonly Rag[]): Rag[] {
  return [...list].sort((a, b) => a.number - b.number || a.id - b.id);
}
