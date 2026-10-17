// Справочники для карточек и выбора позиции: категории и места по id.
// Запросы общие с остальными экранами (ключи app/queries.ts).
import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { inventory, type Category, type Location, type ProductSummary } from '../api';
import { keys } from '../app/queries';

export type StockRefs = {
  categories: ReadonlyMap<number, Category>;
  locations: ReadonlyMap<number, Location>;
  categoryList: Category[];
  locationList: Location[];
};

export function useStockRefs(): StockRefs {
  // С архивными: у старой позиции категория или место могут быть в архиве
  const cats = useQuery({ queryKey: keys.categories(true), queryFn: ({ signal }) => inventory.categories(true, signal), staleTime: 60_000 });
  const locs = useQuery({ queryKey: keys.locations(true), queryFn: ({ signal }) => inventory.locations(true, signal), staleTime: 60_000 });
  return useMemo(() => {
    const categoryList = cats.data ?? [];
    const locationList = locs.data ?? [];
    return {
      categories: new Map(categoryList.map((c) => [c.id, c])),
      locations: new Map(locationList.map((l) => [l.id, l])),
      categoryList,
      locationList,
    };
  }, [cats.data, locs.data]);
}

/** Список позиций (без архивных) — один кэш на главную, выбор позиции и шторку расхода. */
export function useProducts() {
  return useQuery({ queryKey: keys.products(), queryFn: ({ signal }) => inventory.products({}, signal) });
}

/** Цвет категории для --cat: «var(--cat-blue)». */
export function catVar(categories: ReadonlyMap<number, Category>, p: Pick<ProductSummary, 'category_id'>): string {
  return `var(--cat-${categories.get(p.category_id)?.color ?? 'slate'})`;
}
