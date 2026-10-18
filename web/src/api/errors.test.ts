import { describe, expect, it } from 'vitest';
import { ApiError, NetworkError } from './client';
import { errorText, fieldErrors, MAYBE_SAVED_TEXT, NOT_SENT_TEXT } from './errors';

describe('errorText', () => {
  it('не дошло — можно повторить; ответ не пришёл — неизвестно, сохранилось ли', () => {
    expect(errorText(new NetworkError('offline'))).toBe(NOT_SENT_TEXT);
    expect(errorText(new NetworkError('timeout'))).toBe(MAYBE_SAVED_TEXT);
    expect(MAYBE_SAVED_TEXT).toMatch(/неизвестно, сохранилось ли/u);
  });
  it('ошибка сервера — её текст, прочее — «Не получилось»', () => {
    expect(errorText(new ApiError(409, 'Не хватает'))).toBe('Не хватает');
    expect(errorText('строка')).toBe('Не получилось');
  });
});

describe('fieldErrors', () => {
  const known = ['name', 'package_size'] as const;
  it('известное поле — под поле', () => {
    expect(fieldErrors(new ApiError(400, 'Занято', 'name'), known)).toEqual({ name: 'Занято' });
  });
  it('неизвестное поле не теряется — в form', () => {
    expect(fieldErrors(new ApiError(400, 'Не больше 32 цифр', 'barcode'), known)).toEqual({ form: 'Не больше 32 цифр' });
  });
  it('без field и «нет связи» — в form', () => {
    expect(fieldErrors(new ApiError(500, 'Сбой'), known)).toEqual({ form: 'Сбой' });
    expect(fieldErrors(new NetworkError('timeout'), known)).toEqual({ form: MAYBE_SAVED_TEXT });
  });
});
