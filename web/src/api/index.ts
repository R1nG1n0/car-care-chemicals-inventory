// Функции для всех адресов docs/API.md (этап 1).
import { request } from './client';
import type { CreatedInvite, Health, Invite, InviteCheck, Session, Settings } from './types';

export * from './client';
export * from './types';

const tokenPath = (token: string) => `/api/invites/by-token/${encodeURIComponent(token)}`;

export const api = {
  health: (signal?: AbortSignal) => request<Health>('/api/health', { signal }),

  login: (login: string, password: string) =>
    request<Session>('/api/auth/login', { method: 'POST', body: { login, password } }),
  me: (signal?: AbortSignal) => request<Session>('/api/auth/me', { signal }),
  logout: () => request<undefined>('/api/auth/logout', { method: 'POST' }),
  logoutAll: () => request<undefined>('/api/auth/logout-all', { method: 'POST' }),
  changePassword: (current: string, next: string) =>
    request<undefined>('/api/auth/password', { method: 'POST', body: { current, new: next } }),
  saveSettings: (settings: Settings) => request<Settings>('/api/auth/settings', { method: 'PUT', body: settings }),

  invites: async (signal?: AbortSignal) =>
    (await request<{ invites: Invite[] }>('/api/invites', { signal })).invites,
  createInvite: () => request<CreatedInvite>('/api/invites', { method: 'POST' }),
  revokeInvite: (id: number) => request<undefined>(`/api/invites/${id}`, { method: 'DELETE' }),
  checkInvite: (token: string, signal?: AbortSignal) => request<InviteCheck>(tokenPath(token), { signal }),
  acceptInvite: (token: string, login: string, password: string) =>
    request<Session>(`${tokenPath(token)}/accept`, { method: 'POST', body: { login, password } }),
};
