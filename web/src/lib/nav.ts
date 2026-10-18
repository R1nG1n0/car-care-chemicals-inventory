// Куда вернуться после входа: защищённый экран кладёт свой адрес в state.from.
// Принимаются только пути этого же сайта — не «//чужой.сайт» и не полные адреса.
// Управляющие символы (\t, \n, \r…) браузер выбрасывает из адреса, и «/\t/evil»
// стал бы «//evil», поэтому такие адреса отбрасываются целиком.
// eslint-disable-next-line no-control-regex -- ищем именно управляющие символы
const CONTROL = /[\u0000-\u001f\u007f]/u;

export function returnPath(state: unknown): string {
  const from = (state as { from?: unknown } | null)?.from;
  if (typeof from !== 'string' || CONTROL.test(from)) return '/';
  return from.startsWith('/') && !from.startsWith('//') && !from.startsWith('/\\') ? from : '/';
}
