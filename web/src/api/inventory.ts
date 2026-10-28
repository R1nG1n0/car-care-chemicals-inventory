// Этап 2 docs/API.md: справочники, позиции, склад, покупки, расход, шаблоны.
// Количество — целое (мл или шт), деньги — копейки, время — секунды Unix.
import { request } from './client';

export type CatColor = 'blue' | 'berry' | 'olive' | 'ochre' | 'petrol' | 'plum' | 'slate' | 'umber';
export const CAT_COLORS: CatColor[] = ['blue', 'berry', 'olive', 'ochre', 'petrol', 'plum', 'slate', 'umber'];

export type Category = { id: number; name: string; color: CatColor; sort: number; archived: boolean; products_count: number };
export type Location = {
  id: number;
  name: string;
  sort: number;
  archived: boolean;
  units_count: number;
  /** Когда место проверяли инвентаризацией (секунды Unix); null — не проверялось. */
  checked_at: number | null;
};
/** Назначение средства (ФТ-15.1): «ЛКП», «стекло»… Свой справочник у каждого пользователя. */
export type Purpose = { id: number; name: string; sort: number; archived: boolean; products_count?: number };
export type Store = { id: number; name: string; url: string; archived: boolean };

export type Unit = 'ml' | 'pcs';

export type Stock = {
  total: number;
  open_qty: number | null;
  open_capacity: number | null;
  sealed_count: number;
  units_count: number;
};

export type ProductSummary = {
  id: number;
  category_id: number;
  name: string;
  brand: string;
  unit: Unit;
  package_size: number;
  barcode: string;
  min_stock: number | null;
  note: string;
  archived: boolean;
  stock: Stock;
  low: boolean;
  location_ids: number[];
  last_consume_qty: number | null;
  /** Метка фото (секунды Unix) для адреса ?v=; null — фото нет (ФТ-10.4). */
  photo_updated_at: number | null;
  /** Назначения позиции (ФТ-15.1.3), в том числе архивные. */
  purpose_ids: number[];
  /** Кислотность 0–14 с одним знаком (ФТ-15.2.1); null — не указана. */
  ph: number | null;
  /** Оценка 1–5 (ФТ-15.4.1); null — не оценена. */
  rating: number | null;
  /** «Не брать больше» (ФТ-15.4.2). */
  do_not_buy: boolean;
};

/** Аналог из имеющегося (ФТ-15.5): позиция в наличии с общим назначением. */
export type AnalogBrief = { id: number; name: string; brand: string; unit: Unit; total: number };

export type UnitStatus = 'sealed' | 'open' | 'finished' | 'written_off';

export type StockUnit = {
  id: number;
  product_id: number;
  location_id: number | null;
  initial_qty: number;
  remaining_qty: number;
  status: UnitStatus;
  opened_at: number | null;
  created_at: number;
  unit_price: number | null;
};

/** Экземпляр места для инвентаризации: данные позиции приходят вместе с ним. */
export type InventoryUnit = StockUnit & {
  product: { id: number; name: string; brand: string; unit: Unit; package_size: number };
};

/** Ответ корректировки и списания: экземпляр и созданное движение (null — без изменений). */
export type UnitChange = StockUnit & { movement: Movement | null };

export type PurchaseRow = {
  purchase_id: number;
  date: string;
  store: string | null;
  packages: number;
  price_per_package: number;
  price_per_100?: number;
  price_per_unit?: number;
};

export type MovementType = 'purchase' | 'consume' | 'writeoff' | 'move' | 'adjust';

export type Movement = {
  id: number;
  unit_id: number;
  product_id: number;
  type: MovementType;
  qty_delta: number;
  from_location_id: number | null;
  to_location_id: number | null;
  reason: string;
  recipe_run_id: number | null;
  /** Группа запроса: id первого движения; одиночное — своё id. */
  batch_id: number;
  cost: number | null;
  created_at: number;
};

export type ProductDetail = {
  product: ProductSummary & { incompatible?: string; instruction?: string };
  /** Если сервер кладёт текстовые поля рядом с product, а не внутрь. */
  incompatible?: string;
  instruction?: string;
  units: StockUnit[];
  purchases: PurchaseRow[];
  movements: Movement[];
};

export type ProductInput = {
  category_id: number;
  name: string;
  brand: string;
  unit: Unit;
  package_size: number;
  barcode: string;
  min_stock: number | null;
  note: string;
  purpose_ids: number[];
  ph: number | null;
  incompatible: string;
  instruction: string;
};

export type ProductFilter = 'all' | 'low' | 'out' | 'in';
export type ProductQuery = { q?: string; category_id?: number; location_id?: number; purpose_id?: number; filter?: ProductFilter; archived?: boolean };

export type LowItem = {
  product: ProductSummary;
  last_price: number | null;
  last_store: string | null;
  cheapest_store: string | null;
  /** Есть дома для того же (ФТ-15.5.3), до 3. */
  analogs?: AnalogBrief[];
};
export type ShoppingItem = {
  id: number;
  product_id: number | null;
  title: string;
  product: ProductSummary | null;
  done_at: number | null;
  created_at: number;
};

export type PurchaseLine = {
  id: number;
  product_id: number;
  product: string;
  packages: number;
  price_per_package: number;
  unit_ids: number[];
};
export type Purchase = {
  id: number;
  date: string;
  store_id: number | null;
  store: string | null;
  note: string;
  total: number;
  lines: PurchaseLine[];
  created_at: number;
};
export type PurchaseShort = { id: number; date: string; store: string | null; total: number; lines_count: number };
export type PurchaseInput = {
  date: string;
  store_id: number | null;
  note: string;
  lines: { product_id: number; packages: number; price_per_package: number; location_id: number | null }[];
};

export type ConsumeResult = {
  movements: Movement[];
  product: ProductSummary;
  opened_unit_ids: number[];
  finished_unit_ids: number[];
  cost: number | null;
};

export type RecipeItem = { product_id: number; qty: number; product: ProductSummary; available: number; short: boolean };
export type Recipe = { id: number; name: string; sort: number; last_used_at: number | null; cost: number | null; items: RecipeItem[] };
export type RecipeInput = { name: string; items: { product_id: number; qty: number }[] };
export type ApplyResult = {
  run_id: number;
  movements: Movement[];
  products: ProductSummary[];
  shortages: { product_id: number; requested: number; consumed: number }[];
  cost: number | null;
};

// Отчёты (ФТ-10.2): деньги — копейки, month — «ГГГГ-ММ».
export type SpendingReport = {
  total: number;
  purchases_count: number;
  by_category: { category_id: number; name: string; color: CatColor; total: number }[];
  by_store: { store_id: number | null; name: string; total: number }[];
  by_month: { month: string; total: number }[];
};
export type ProductUse = { product_id: number; name: string; unit: Unit; qty: number; cost: number | null };
export type ConsumptionReport = {
  total_cost: number;
  by_product: ProductUse[];
  by_month: { month: string; cost: number; qty_ml: number; qty_pcs: number }[];
  writeoffs: { by_product: ProductUse[]; cost: number };
  /** Корректировки остатка вниз (потери из инвентаризации): в расход не входят. */
  corrections: { by_product: ProductUse[]; cost: number };
};

function qs(params: Record<string, string | number | boolean | undefined>): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '' || v === false) continue;
    p.set(k, v === true ? '1' : String(v));
  }
  const s = p.toString();
  return s ? `?${s}` : '';
}

/** Фото до 300 КБ по мобильной сети грузится дольше обычного запроса. */
const PHOTO_TIMEOUT_MS = 60_000;

const get = <T>(path: string, signal?: AbortSignal) => request<T>(path, { signal });
const send = <T>(method: 'POST' | 'PATCH' | 'PUT' | 'DELETE', path: string, body?: unknown) =>
  request<T>(path, { method, body });

export const inventory = {
  // Справочники
  categories: async (archived = false, signal?: AbortSignal) =>
    (await get<{ categories: Category[] }>(`/api/categories${qs({ archived })}`, signal)).categories,
  createCategory: (b: { name: string; color: CatColor }) => send<Category>('POST', '/api/categories', b),
  updateCategory: (id: number, b: Partial<{ name: string; color: CatColor; archived: boolean }>) =>
    send<Category>('PATCH', `/api/categories/${id}`, b),
  reorderCategories: (ids: number[]) => send<undefined>('POST', '/api/categories/reorder', { ids }),

  purposes: async (archived = false, signal?: AbortSignal) =>
    (await get<{ purposes: Purpose[] }>(`/api/purposes${qs({ archived })}`, signal)).purposes,
  createPurpose: (b: { name: string }) => send<Purpose>('POST', '/api/purposes', b),
  updatePurpose: (id: number, b: Partial<{ name: string; archived: boolean }>) => send<Purpose>('PATCH', `/api/purposes/${id}`, b),
  reorderPurposes: (ids: number[]) => send<undefined>('POST', '/api/purposes/reorder', { ids }),

  locations: async (archived = false, signal?: AbortSignal) =>
    (await get<{ locations: Location[] }>(`/api/locations${qs({ archived })}`, signal)).locations,
  createLocation: (b: { name: string }) => send<Location>('POST', '/api/locations', b),
  updateLocation: (id: number, b: Partial<{ name: string; archived: boolean }>) =>
    send<Location>('PATCH', `/api/locations/${id}`, b),
  reorderLocations: (ids: number[]) => send<undefined>('POST', '/api/locations/reorder', { ids }),
  locationUnits: async (id: number, signal?: AbortSignal) =>
    (await get<{ units: InventoryUnit[] }>(`/api/locations/${id}/units`, signal)).units,
  markLocationChecked: (id: number) => send<Location>('POST', `/api/locations/${id}/checked`),

  stores: async (archived = false, signal?: AbortSignal) =>
    (await get<{ stores: Store[] }>(`/api/stores${qs({ archived })}`, signal)).stores,
  createStore: (b: { name: string; url: string }) => send<Store>('POST', '/api/stores', b),
  updateStore: (id: number, b: Partial<{ name: string; url: string; archived: boolean }>) =>
    send<Store>('PATCH', `/api/stores/${id}`, b),

  // Позиции
  products: async (q: ProductQuery = {}, signal?: AbortSignal) =>
    (await get<{ products: ProductSummary[] }>(`/api/products${qs(q)}`, signal)).products,
  product: (id: number, signal?: AbortSignal) => get<ProductDetail>(`/api/products/${id}`, signal),
  createProduct: (b: ProductInput) => send<ProductSummary>('POST', '/api/products', b),
  updateProduct: (id: number, b: Partial<ProductInput & { archived: boolean }>) =>
    send<ProductSummary>('PATCH', `/api/products/${id}`, b),
  /** Оценка и «Не брать больше» (ФТ-15.4): передаются только изменяемые поля; rating: null снимает оценку. */
  setRating: (id: number, b: { rating?: number | null; do_not_buy?: boolean }) =>
    send<ProductSummary>('PATCH', `/api/products/${id}/rating`, b),
  /** Аналоги из имеющегося (ФТ-15.5), до 5. */
  analogs: async (id: number, signal?: AbortSignal) =>
    (await get<{ analogs: ProductSummary[] }>(`/api/products/${id}/analogs`, signal)).analogs,
  /** Фото позиции (ФТ-10.4): тело — сырые байты JPEG; ответ — сводка с новым photo_updated_at. */
  setPhoto: (id: number, photo: Blob) =>
    request<ProductSummary>(`/api/products/${id}/photo`, { method: 'PUT', binary: photo, timeoutMs: PHOTO_TIMEOUT_MS }),
  deletePhoto: (id: number) => send<ProductSummary>('DELETE', `/api/products/${id}/photo`),
  brands: async (signal?: AbortSignal) => (await get<{ brands: string[] }>('/api/brands', signal)).brands,

  // Список покупок
  shopping: (signal?: AbortSignal) => get<{ low: LowItem[]; manual: ShoppingItem[] }>('/api/shopping', signal),
  addShopping: (b: { product_id?: number; title?: string }) => send<ShoppingItem>('POST', '/api/shopping', b),
  setShoppingDone: (id: number, done: boolean) => send<ShoppingItem>('PATCH', `/api/shopping/${id}`, { done }),
  deleteShopping: (id: number) => send<undefined>('DELETE', `/api/shopping/${id}`),

  // Покупки и экземпляры
  createPurchase: async (b: PurchaseInput) => (await send<{ purchase: Purchase }>('POST', '/api/purchases', b)).purchase,
  purchases: async (before?: number, signal?: AbortSignal) =>
    (await get<{ purchases: PurchaseShort[] }>(`/api/purchases${qs({ before })}`, signal)).purchases,
  purchase: async (id: number, signal?: AbortSignal) =>
    (await get<{ purchase: Purchase }>(`/api/purchases/${id}`, signal)).purchase,
  addUnit: (b: { product_id: number; remaining_qty: number; initial_qty?: number; location_id?: number | null }) =>
    send<StockUnit>('POST', '/api/units', b),
  moveUnit: (id: number, location_id: number) => send<StockUnit>('POST', `/api/units/${id}/move`, { location_id }),
  adjustUnit: (id: number, remaining_qty: number, reason = '') =>
    send<UnitChange>('POST', `/api/units/${id}/adjust`, { remaining_qty, reason }),
  writeoffUnit: (id: number, reason: string, qty?: number) =>
    send<UnitChange>('POST', `/api/units/${id}/writeoff`, { qty, reason }),

  // Расход и журнал
  consume: (b: { product_id: number; qty: number; unit_id?: number; client_op_id?: string; occurred_at?: number }) =>
    request<ConsumeResult>('/api/consume', { method: 'POST', body: b, offlineOk: true }),
  revert: async (ids: number[]) => (await send<{ products: ProductSummary[] }>('POST', '/api/movements/revert', { ids })).products,
  movements: async (q: { product_id?: number; before?: number } = {}, signal?: AbortSignal) =>
    (await get<{ movements: Movement[] }>(`/api/movements${qs(q)}`, signal)).movements,

  // Отчёты: from, to — «ГГГГ-ММ-ДД» включительно; tz — минуты к востоку от UTC
  spending: (from: string, to: string, signal?: AbortSignal) =>
    get<SpendingReport>(`/api/reports/spending${qs({ from, to })}`, signal),
  consumption: (from: string, to: string, tz: number, signal?: AbortSignal) =>
    get<ConsumptionReport>(`/api/reports/consumption${qs({ from, to, tz: tz === 0 ? undefined : tz })}`, signal),

  // Шаблоны
  recipes: async (signal?: AbortSignal) => (await get<{ recipes: Recipe[] }>('/api/recipes', signal)).recipes,
  recipe: async (id: number, signal?: AbortSignal) => (await get<{ recipe: Recipe }>(`/api/recipes/${id}`, signal)).recipe,
  createRecipe: (b: RecipeInput) => send<Recipe>('POST', '/api/recipes', b),
  updateRecipe: (id: number, b: RecipeInput) => send<Recipe>('PUT', `/api/recipes/${id}`, b),
  deleteRecipe: (id: number) => send<undefined>('DELETE', `/api/recipes/${id}`),
  reorderRecipes: (ids: number[]) => send<undefined>('POST', '/api/recipes/reorder', { ids }),
  /** client_op_id и occurred_at — очередь без связи (ФТ-12.4.1): повтор с тем же номером не списывает второй раз. */
  applyRecipe: (id: number, items: { product_id: number; qty: number }[], op?: { client_op_id: string; occurred_at: number }) =>
    request<ApplyResult>(`/api/recipes/${id}/apply`, { method: 'POST', body: { items, ...op }, offlineOk: true }),
};
