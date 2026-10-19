// Привязка Telegram-бота (ФТ-11.2): статус, одноразовая ссылка на бота, отвязка.
import { request } from './client';

export type TelegramStatus = {
  /** Бот настроен на сервере. */
  enabled: boolean;
  bot_username: string | null;
  linked: boolean;
  telegram_name: string | null;
  /** Unix-секунды. */
  linked_at: number | null;
};

export type TelegramLink = {
  url: string;
  /** Unix-секунды: когда ссылка перестанет работать. */
  expires_at: number;
};

export const telegram = {
  status: (signal?: AbortSignal) => request<TelegramStatus>('/api/telegram', { signal }),
  link: () => request<TelegramLink>('/api/telegram/link', { method: 'POST' }),
  unlink: () => request<undefined>('/api/telegram', { method: 'DELETE' }),
};
