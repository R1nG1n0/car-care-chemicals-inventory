// Ключи кэша React Query для данных этапа 2 и общая инвалидация.
// Любое изменение склада (покупка, расход, отмена, корректировка…) меняет
// сводки позиций, список покупок и шаблоны — поэтому инвалидация одна на всех.
import type { QueryClient } from '@tanstack/react-query';
import type { ProductQuery } from '../api';

export const keys = {
  categories: (archived = false) => ['categories', archived] as const,
  purposes: (archived = false) => ['purposes', archived] as const,
  analogs: (id: number) => ['analogs', id] as const,
  locations: (archived = false) => ['locations', archived] as const,
  stores: (archived = false) => ['stores', archived] as const,
  locationUnits: (id: number) => ['locationUnits', id] as const,
  products: (q: ProductQuery = {}) => ['products', q] as const,
  product: (id: number) => ['product', id] as const,
  brands: ['brands'] as const,
  shopping: ['shopping'] as const,
  purchases: ['purchases'] as const,
  purchase: (id: number) => ['purchase', id] as const,
  movements: (productId?: number) => ['movements', productId ?? null] as const,
  report: (kind: 'spending' | 'consumption', from: string, to: string) => ['report', kind, from, to] as const,
  recipes: ['recipes'] as const,
  recipe: (id: number) => ['recipe', id] as const,
};

/** После любого изменения остатков. */
export function invalidateStock(qc: QueryClient): Promise<void> {
  return qc.invalidateQueries({
    predicate: (q) =>
      ['products', 'product', 'analogs', 'brands', 'shopping', 'purchases', 'purchase', 'movements', 'recipes', 'recipe', 'locations', 'locationUnits', 'report'].includes(
        q.queryKey[0] as string,
      ),
  });
}

/** После изменения справочников (имя, цвет, архив, порядок): имена магазинов и мест видны и в покупках. */
export function invalidateRefs(qc: QueryClient): Promise<void> {
  return qc.invalidateQueries({
    predicate: (q) =>
      ['categories', 'purposes', 'analogs', 'locations', 'locationUnits', 'stores', 'products', 'product', 'shopping', 'purchases', 'purchase'].includes(q.queryKey[0] as string),
  });
}
