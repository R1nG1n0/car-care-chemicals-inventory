// Тексты интерфейса, зависящие от чисел и времени: склонения, сроки, «через N минут».

/** Склонение по числу: plural(5, ['минуту', 'минуты', 'минут']) → «минут». */
export function plural(n: number, forms: readonly [string, string, string]): string {
  const abs = Math.abs(Math.trunc(n));
  const d10 = abs % 10;
  const d100 = abs % 100;
  if (d10 === 1 && d100 !== 11) return forms[0];
  if (d10 >= 2 && d10 <= 4 && (d100 < 12 || d100 > 14)) return forms[1];
  return forms[2];
}

const MINUTES = ['минуту', 'минуты', 'минут'] as const;

/** Формы для plural: «2 упаковки», «5 позиций». */
export const PACKAGES = ['упаковка', 'упаковки', 'упаковок'] as const;
export const POSITIONS = ['позиция', 'позиции', 'позиций'] as const;
const DAYS = ['день', 'дня', 'дней'] as const;

/** «через 12 минут» по числу секунд из retry_after; меньше минуты округляется до минуты. */
export function retryAfterText(seconds: number): string {
  const min = Math.max(1, Math.ceil(seconds / 60));
  return `через ${min} ${plural(min, MINUTES)}`;
}

/** Текст ошибки 429 для формы входа. */
export function tooManyAttemptsText(seconds: number | undefined): string {
  if (seconds === undefined) return 'Слишком много попыток. Повторите позже';
  return `Слишком много попыток. Повторите ${retryAfterText(seconds)}`;
}

const dayFmt = new Intl.DateTimeFormat('ru-RU', { day: '2-digit', month: '2-digit' });
const timeFmt = new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit' });

/** «14.10» из секунд Unix. */
export function formatDay(unix: number): string {
  return dayFmt.format(new Date(unix * 1000));
}

/** Дата покупки «2026-10-02» → «02.10»; другой год — «02.10.2025». */
export function purchaseDay(date: string, nowYear = new Date().getFullYear()): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/u.exec(date);
  if (!m) return date;
  const dm = `${m[3]}.${m[2]}`;
  return Number(m[1]) === nowYear ? dm : `${dm}.${m[1]}`;
}

/** «12:04» из миллисекунд (Date.now()). */
export function formatTime(ms: number): string {
  return timeFmt.format(new Date(ms));
}

/** «действует ещё 6 дней» — сколько осталось до expires_at; неполные сутки — целый день. */
export function validForText(expiresAt: number, nowSec: number): string {
  const days = Math.max(1, Math.ceil((expiresAt - nowSec) / 86_400));
  return `действует ещё ${days} ${plural(days, DAYS)}`;
}

export type InviteLike = { created_at: number; expires_at: number; status: 'active' | 'used' | 'expired' | 'revoked' };

export const INVITE_STATUS: Record<InviteLike['status'], string> = {
  active: 'активно',
  used: 'использовано',
  expired: 'истекло',
  revoked: 'отозвано',
};

/** Строка под приглашением: «Создано 14.10 · действует ещё 6 дней». */
export function inviteMeta(inv: InviteLike, nowSec: number): string {
  const created = `Создано ${formatDay(inv.created_at)}`;
  if (inv.status === 'active' && inv.expires_at > nowSec) return `${created} · ${validForText(inv.expires_at, nowSec)}`;
  if (inv.status === 'used') return `${created} · использовано`;
  if (inv.status === 'revoked') return `${created} · отозвано`;
  return `${created} · истекло ${formatDay(inv.expires_at)}`;
}
