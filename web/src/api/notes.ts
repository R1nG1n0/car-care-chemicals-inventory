// Заметки (ФТ-18): список с поиском и фильтрами, чтение, создание, правка, удаление.
import { request } from './client';

export type Note = {
  id: number;
  title: string;
  body: string;
  product_ids: number[];
  recipe_ids: number[];
  created_at: number;
  updated_at: number;
};

/** Фильтры списка; q ищет по заголовку и тексту. */
export type NoteQuery = { q?: string; product_id?: number; recipe_id?: number };

/** Тело создания; при правке передаются только изменяемые поля, списки заменяются целиком. */
export type NoteInput = { title: string; body: string; product_ids?: number[]; recipe_ids?: number[] };

function query(q: NoteQuery): string {
  const p = new URLSearchParams();
  if (q.q) p.set('q', q.q);
  if (q.product_id) p.set('product_id', String(q.product_id));
  if (q.recipe_id) p.set('recipe_id', String(q.recipe_id));
  const s = p.toString();
  return s ? `?${s}` : '';
}

export const notes = {
  list: async (q: NoteQuery = {}, signal?: AbortSignal) => (await request<{ notes: Note[] }>(`/api/notes${query(q)}`, { signal })).notes,
  get: (id: number, signal?: AbortSignal) => request<Note>(`/api/notes/${id}`, { signal }),
  create: (b: NoteInput) => request<Note>('/api/notes', { method: 'POST', body: b }),
  update: (id: number, b: Partial<NoteInput>) => request<Note>(`/api/notes/${id}`, { method: 'PATCH', body: b }),
  remove: (id: number) => request<undefined>(`/api/notes/${id}`, { method: 'DELETE' }),
};
