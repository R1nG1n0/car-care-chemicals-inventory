// Заметки (ФТ-18): проверка полей, поиск без связи, выдержка для списков, привязки.
import type { Note, NoteInput } from '../api';
import { normalize } from './stock';
import { charCount } from './validate';

export const MAX_NOTE_TITLE = 120;
export const MAX_NOTE_BODY = 10_000;

export type NoteDraft = { title: string; body: string; product_ids: number[]; recipe_ids: number[] };
export type NoteErrors = { title?: string; body?: string };

/** Проверка заметки по правилам сервера; значение — без лишних пробелов по краям заголовка. */
export function validateNote(d: NoteDraft): { ok: true; value: Required<NoteInput> } | { ok: false; errors: NoteErrors } {
  const e: NoteErrors = {};
  const title = d.title.trim();
  if (title === '') e.title = 'Введите заголовок';
  else if (charCount(title) > MAX_NOTE_TITLE) e.title = `Не длиннее ${MAX_NOTE_TITLE} символов`;
  if (charCount(d.body) > MAX_NOTE_BODY) e.body = `Не длиннее ${MAX_NOTE_BODY} символов`;
  if (e.title || e.body) return { ok: false, errors: e };
  return { ok: true, value: { title, body: d.body, product_ids: [...new Set(d.product_ids)], recipe_ids: [...new Set(d.recipe_ids)] } };
}

/**
 * Фильтр списка на телефоне: тот же смысл, что у сервера (заголовок и текст без учёта регистра,
 * привязка к позиции или шаблону). Нужен без связи и для блоков в карточках.
 */
export function filterNotes(list: readonly Note[], f: { q?: string; product_id?: number; recipe_id?: number } = {}): Note[] {
  const q = normalize(f.q ?? '');
  return list.filter((n) => {
    if (f.product_id !== undefined && !n.product_ids.includes(f.product_id)) return false;
    if (f.recipe_id !== undefined && !n.recipe_ids.includes(f.recipe_id)) return false;
    return q === '' || normalize(n.title).includes(q) || normalize(n.body).includes(q);
  });
}

/** Новые и изменённые сверху (как отдаёт сервер) — для списка из снимка. */
export function sortNotes(list: readonly Note[]): Note[] {
  return [...list].sort((a, b) => b.updated_at - a.updated_at || b.id - a.id);
}

/** Первые непустые строки текста для списка: не больше `lines` строк. */
export function excerpt(body: string, lines = 2): string {
  return body
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
    .slice(0, lines)
    .join('\n');
}

/** Есть ли что сохранять: изменился ли черновик относительно заметки (или заметка новая). */
export function noteChanged(note: Note | null, d: NoteDraft): boolean {
  if (!note) return d.title.trim() !== '' || d.body !== '' || d.product_ids.length > 0 || d.recipe_ids.length > 0;
  const same = (a: readonly number[], b: readonly number[]) => a.length === b.length && [...a].sort().join() === [...b].sort().join();
  return note.title !== d.title.trim() || note.body !== d.body || !same(note.product_ids, d.product_ids) || !same(note.recipe_ids, d.recipe_ids);
}

/** Привязка из ссылки /more/notes/new?product=ID&recipe=ID: мусор отбрасывается. */
export function linkParam(raw: string | null): number[] {
  const n = Number(raw);
  return raw !== null && /^\d{1,9}$/u.test(raw) && n > 0 ? [n] : [];
}

/** Куда вести «Новая заметка» из карточки позиции или шаблона. */
export function newNoteUrl(link: { product_id?: number; recipe_id?: number }): string {
  if (link.product_id) return `/more/notes/new?product=${link.product_id}`;
  if (link.recipe_id) return `/more/notes/new?recipe=${link.recipe_id}`;
  return '/more/notes/new';
}
