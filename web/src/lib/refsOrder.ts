// Порядок записей справочника (места, категории, шаблоны): перестановка
// кнопками «Выше/Ниже» и перетаскиванием за ручку.

/** Новый массив, где элемент from стоит на месте to. */
export function moveItem<T>(list: readonly T[], from: number, to: number): T[] {
  const out = [...list];
  if (from < 0 || from >= out.length) return out;
  const t = Math.max(0, Math.min(out.length - 1, to));
  const [item] = out.splice(from, 1);
  out.splice(t, 0, item as T);
  return out;
}

/** Порядок изменился? (чтобы не слать reorder впустую) */
export function sameOrder(a: readonly number[], b: readonly number[]): boolean {
  return a.length === b.length && a.every((v, i) => v === b[i]);
}

/**
 * Куда встанет перетаскиваемая строка: from — её индекс, dy — сдвиг пальца,
 * heights — высоты строк по порядку. Сосед уступает место, когда сдвиг
 * заходит за его середину.
 */
export function dropIndex(from: number, dy: number, heights: readonly number[]): number {
  const dist = Math.abs(dy);
  const dir = dy > 0 ? 1 : -1;
  let to = from;
  let passed = 0;
  for (let i = from + dir; dy !== 0 && i >= 0 && i < heights.length; i += dir) {
    const hi = heights[i]!;
    if (dist <= passed + hi / 2) break;
    to = i;
    passed += hi;
  }
  return to;
}

/** Сдвиг соседней строки i, пока строка from «висит» над местом to: соседи расступаются. */
export function neighbourShift(i: number, from: number, to: number, draggedHeight: number): number {
  if (i === from) return 0;
  if (from < to && i > from && i <= to) return -draggedHeight;
  if (from > to && i >= to && i < from) return draggedHeight;
  return 0;
}
