// Раздел «Telegram» в профиле: какое состояние показывать, опрос и тексты.
import type { TelegramStatus } from '../api';

/** Как часто спрашивать статус, пока владелец переходит в бота. */
export const TELEGRAM_POLL_MS = 3000;

/** Как часто спрашивать статус, пока бот ещё не дозвонился до Telegram. */
export const TELEGRAM_PENDING_POLL_MS = 10_000;

/** pending — бот настроен, но имя ещё не получено от Telegram. */
export type TelegramView = 'disabled' | 'pending' | 'unlinked' | 'linked';

export function telegramView(status: TelegramStatus): TelegramView {
  if (!status.enabled) return 'disabled';
  if (!status.linked && status.bot_username === null) return 'pending';
  return status.linked ? 'linked' : 'unlinked';
}

/** Через сколько мс перезапросить статус; false — не надо. */
export function pollInterval(expiresAt: number | null, status: TelegramStatus | undefined, nowSec: number): number | false {
  if (status && telegramView(status) === 'pending') return TELEGRAM_PENDING_POLL_MS;
  return shouldPoll(expiresAt, status, nowSec) ? TELEGRAM_POLL_MS : false;
}

/** Ссылка ещё действует. */
export function linkAlive(expiresAt: number, nowSec: number): boolean {
  return expiresAt > nowSec;
}

/** Опрашивать статус: ссылка выдана и жива, привязки ещё нет. */
export function shouldPoll(expiresAt: number | null, status: TelegramStatus | undefined, nowSec: number): boolean {
  if (expiresAt === null || !status || status.linked) return false;
  return linkAlive(expiresAt, nowSec);
}

const dateFmt = new Intl.DateTimeFormat('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric' });

/** «19.10.2026» из секунд Unix. */
export function formatDate(unix: number): string {
  return dateFmt.format(new Date(unix * 1000));
}

/** «Имя · с 19.10.2026». */
export function linkedText(status: TelegramStatus): string {
  const name = status.telegram_name?.trim() || 'Telegram';
  return status.linked_at === null ? name : `${name} · с ${formatDate(status.linked_at)}`;
}

export const TELEGRAM_OFF_TEXT = 'Бот пока не настроен';
export const TELEGRAM_PITCH = 'Остатки, расход и список покупок прямо в Telegram';
export const TELEGRAM_LINK_HINT = 'Ссылка действует 10 минут';
export const TELEGRAM_PENDING_TEXT = 'Бот подключается к Telegram…';
export const TELEGRAM_UNLINKED_TEXT = 'Telegram отключён';
