// Тряпки в работе (ФТ-19): у каждой свой счётчик стирок.
import { request } from './client';
import type { ProductSummary } from './inventory';

export type RagLevel = 'ok' | 'soon' | 'due';
export type RagStatus = 'active' | 'retired';
export type RetireReason = 'worn' | 'lost' | 'damaged';

export type Rag = {
  id: number;
  product_id: number;
  /** Метка на бирке: «№ 7» или своя (до 20 символов). */
  label: string;
  number: number;
  washes: number;
  /** Ресурс позиции сейчас; null — у позиции учёт снят. */
  wear_limit: number | null;
  level: RagLevel;
  status: RagStatus;
  retire_reason: RetireReason | null;
  created_at: number;
  retired_at: number | null;
  last_washed_at: number | null;
};

/** Ответ «В работу»: тряпка, свежая сводка позиции и движения списания (для «Отменить»). */
export type RagStart = { rag: Rag; product: ProductSummary; movement_ids: number[] };

export const rags = {
  list: async (productId: number, all = false, signal?: AbortSignal) =>
    (await request<{ rags: Rag[] }>(`/api/products/${productId}/rags${all ? '?all=1' : ''}`, { signal })).rags,
  start: (productId: number, b: { label?: string; unit_id?: number; client_op_id?: string } = {}) =>
    request<RagStart>(`/api/products/${productId}/rags`, { method: 'POST', body: b }),
  /** Отмена «В работу» (ФТ-19.2): тряпка убирается, 1 шт возвращается; 409 — её уже стирали или списали. */
  unstart: (id: number) => request<{ product: ProductSummary }>(`/api/rags/${id}/unstart`, { method: 'POST' }),
  rename: (id: number, label: string) => request<Rag>(`/api/rags/${id}`, { method: 'PATCH', body: { label } }),
  wash: async (ids: number[]) => (await request<{ rags: Rag[] }>('/api/rags/wash', { method: 'POST', body: { ids } })).rags,
  unwash: async (ids: number[]) => (await request<{ rags: Rag[] }>('/api/rags/unwash', { method: 'POST', body: { ids } })).rags,
  retire: (id: number, reason: RetireReason) => request<Rag>(`/api/rags/${id}/retire`, { method: 'POST', body: { reason } }),
};
