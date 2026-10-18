// Тексты ошибок для показа человеку — одни на весь сайт — и разбор поля ошибки
// сервера (field) под поле формы.
import { isApiError, isNetworkError } from './client';

/** «Нет связи» при загрузке экрана (LoadError). */
export const OFFLINE_TITLE = 'Сервер недоступен';
export const OFFLINE_TEXT = 'Сайт работает только через VPN. Проверьте, что AmneziaWG включён.';

/** Запрос не дошёл: ничего не изменилось, можно повторить. */
export const NOT_SENT_TEXT = 'Нет связи — не сохранилось. Проверьте VPN и повторите';
/** Ответ не пришёл: изменение могло сохраниться, повтор может задвоить. */
export const MAYBE_SAVED_TEXT = 'Ответ не пришёл — неизвестно, сохранилось ли. Проверьте журнал или историю, прежде чем повторять';

/** Текст ошибки мутации для показа под полем или кнопкой. */
export function errorText(err: unknown): string {
  if (isNetworkError(err)) return err.maybeSaved ? MAYBE_SAVED_TEXT : NOT_SENT_TEXT;
  return err instanceof Error ? err.message : 'Не получилось';
}

export type FieldErrorMap<F extends string> = Partial<Record<F | 'form', string>>;

/**
 * Ошибка мутации → под поле формы. Поле из known — под это поле, всё остальное
 * (неизвестное поле, «нет связи», ошибка без field) — в form, чтобы ничего не терялось.
 */
export function fieldErrors<F extends string>(err: unknown, known: readonly F[]): FieldErrorMap<F> {
  if (isApiError(err) && err.field && (known as readonly string[]).includes(err.field)) {
    return { [err.field as F]: err.message } as FieldErrorMap<F>;
  }
  return { form: errorText(err) } as FieldErrorMap<F>;
}
