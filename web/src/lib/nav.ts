// Куда вернуться после входа: защищённый экран кладёт свой адрес в state.from.
// Принимаются только пути этого же сайта — не «//чужой.сайт» и не полные адреса.
export function returnPath(state: unknown): string {
  const from = (state as { from?: unknown } | null)?.from;
  return typeof from === 'string' && from.startsWith('/') && !from.startsWith('//') && !from.startsWith('/\\') ? from : '/';
}
