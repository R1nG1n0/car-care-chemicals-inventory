// Справочники для карточек, форм и выбора позиции: категории и места по id,
// общий список позиций, цвет категории классом. Запросы общие со всеми экранами
// (ключи app/queries.ts).
import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { inventory, type CatColor, type Category, type Location, type ProductSummary } from '../api';
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

/**
 * Цвет категории — один способ на весь сайт: класс .cat--{color} (components.css)
 * задаёт --cat (полоса, шапка, точка) и --sw (образец цвета).
 */
export const catClass = (c: CatColor | undefined): string => `cat--${c ?? 'slate'}`;

/** Класс цвета категории позиции; категории нет в справочнике — сланцевый. */
export function catClassOf(categories: ReadonlyMap<number, Category>, p: Pick<ProductSummary, 'category_id'>): string {
  return catClass(categories.get(p.category_id)?.color);
}
