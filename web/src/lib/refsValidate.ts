// Проверки справочников до отправки (ФТ-2): те же правила, что у сервера.

/** Имя записи справочника: 1–40 символов после обрезки пробелов. */
export function refNameError(name: string): string | undefined {
  const n = name.trim();
  if (n === '') return 'Введите название';
  if ([...n].length > 40) return 'Не длиннее 40 символов';
  return undefined;
}

/** Ссылка магазина: пусто или http(s)://… */
export function storeUrlError(url: string): string | undefined {
  const u = url.trim();
  if (u === '') return undefined;
  return /^https?:\/\/\S+$/i.test(u) ? undefined : 'Ссылка начинается с http:// или https://';
}
