// Объекты API — по договору docs/API.md. Время — секунды Unix (UTC).

export type User = {
  id: number;
  login: string;
  is_owner: boolean;
};

/** Шаги кнопок количества (ФТ-5.8). */
export type Settings = {
  step_small: number;
  step_large: number;
};

/** Ответ входа, принятия приглашения и /api/auth/me. */
export type Session = {
  user: User;
  settings: Settings;
};

export type InviteStatus = 'active' | 'used' | 'expired' | 'revoked';

/** Приглашение без токена: ссылка видна только в ответе на создание. */
export type Invite = {
  id: number;
  created_at: number;
  expires_at: number;
  status: InviteStatus;
};

export type CreatedInvite = {
  invite: Invite;
  url: string;
};

export type InviteCheck = {
  expires_at: number;
};

export type Health = {
  status: string;
  version: string;
};
