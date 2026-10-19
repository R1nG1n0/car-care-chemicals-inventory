// Экспорт и импорт (ФТ-10.3): разбор файла на клиенте до отправки, счётчики для
// предпросмотра, адреса скачивания. Сервер всё равно проверяет файл заново.
import { plural } from './text';

/** Предел файла импорта, как на сервере (docs/API.md). */
export const MAX_IMPORT_BYTES = 100 * 1024 * 1024;

export const EXPORT_FORMAT = 'garage-export';
export const EXPORT_VERSION = 1;

/** Сколько записей каждого вида в файле или импортировано (ответ POST /api/import). */
export type TransferCounts = {
  categories: number;
  locations: number;
  stores: number;
  products: number;
  units: number;
  purchases: number;
  purchase_lines: number;
  movements: number;
  recipes: number;
  shopping: number;
};

export type ExportSummary = {
  login: string;
  /** Дата экспорта (RFC 3339) или null, если в файле её нет. */
  exportedAt: string | null;
  counts: TransferCounts;
};

export type ParseResult = { ok: true; summary: ExportSummary } | { ok: false; error: string };

const NOT_EXPORT = 'Это не файл экспорта garage. Нужен файл, скачанный кнопкой «Скачать всё с фото» или «Скачать без фото».';

/** Списки файла, которые считаются. Отсутствующий список — пустой, не массив — ошибка. */
const LISTS = ['categories', 'locations', 'stores', 'products', 'units', 'purchases', 'movements', 'recipes', 'shopping'] as const;

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);

/** Проверка размера до чтения файла; null — размер подходит. */
export function fileSizeError(size: number): string | null {
  if (size > MAX_IMPORT_BYTES) return 'Файл больше 100 МБ. Загрузить его нельзя. Скачайте копию «без фото» или уменьшите число фото.';
  if (size === 0) return 'Файл пустой.';
  return null;
}

/** Разбор текста файла: формат, версия, счётчики. Ничего не отправляет. */
export function parseExportFile(text: string): ParseResult {
  let data: unknown;
  try {
    data = JSON.parse(text.charCodeAt(0) === 0xfeff ? text.slice(1) : text);
  } catch {
    return { ok: false, error: 'Файл повреждён или это не JSON.' };
  }
  if (!isObject(data) || data.format !== EXPORT_FORMAT) return { ok: false, error: NOT_EXPORT };
  if (data.version !== EXPORT_VERSION) {
    const v = typeof data.version === 'number' ? ` ${data.version}` : '';
    return { ok: false, error: `Версия файла${v} не поддерживается: нужна версия ${EXPORT_VERSION}.` };
  }

  const lists: Record<string, unknown[]> = {};
  for (const name of LISTS) {
    const v = data[name];
    if (v === undefined || v === null) lists[name] = [];
    else if (Array.isArray(v)) lists[name] = v;
    else return { ok: false, error: `В файле повреждён раздел «${name}».` };
  }
  let lines = 0;
  for (const p of lists.purchases ?? []) {
    const l = isObject(p) ? p.lines : undefined;
    if (Array.isArray(l)) lines += l.length;
  }
  const n = (name: string) => lists[name]?.length ?? 0;
  const user = isObject(data.user) && typeof data.user.login === 'string' ? data.user.login : '';
  return {
    ok: true,
    summary: {
      login: user,
      exportedAt: typeof data.exported_at === 'string' ? data.exported_at : null,
      counts: {
        categories: n('categories'),
        locations: n('locations'),
        stores: n('stores'),
        products: n('products'),
        units: n('units'),
        purchases: n('purchases'),
        purchase_lines: lines,
        movements: n('movements'),
        recipes: n('recipes'),
        shopping: n('shopping'),
      },
    },
  };
}

const LABELS: { key: keyof TransferCounts; forms: readonly [string, string, string] }[] = [
  { key: 'products', forms: ['позиция', 'позиции', 'позиций'] },
  { key: 'units', forms: ['экземпляр', 'экземпляра', 'экземпляров'] },
  { key: 'purchases', forms: ['покупка', 'покупки', 'покупок'] },
  { key: 'movements', forms: ['движение', 'движения', 'движений'] },
  { key: 'recipes', forms: ['шаблон', 'шаблона', 'шаблонов'] },
  { key: 'locations', forms: ['место', 'места', 'мест'] },
  { key: 'stores', forms: ['магазин', 'магазина', 'магазинов'] },
  { key: 'categories', forms: ['категория', 'категории', 'категорий'] },
  { key: 'shopping', forms: ['пункт списка покупок', 'пункта списка покупок', 'пунктов списка покупок'] },
];

/** Строки предпросмотра «12 позиций»; виды без записей не показываются. */
export function countLines(c: TransferCounts): string[] {
  return LABELS.filter((l) => c[l.key] > 0).map((l) => `${c[l.key]} ${plural(c[l.key], l.forms)}`);
}

/** Короткая сводка для уведомления после импорта: «12 позиций, 3 покупки, 40 движений». */
export function importedText(c: TransferCounts): string {
  const main = countLines({ ...c, units: 0, locations: 0, stores: 0, categories: 0, shopping: 0, recipes: 0 });
  return main.length > 0 ? main.join(', ') : 'файл был пустым';
}

/** Дата экспорта «ДД.ММ.ГГГГ» из RFC 3339; null — даты нет или она не разобралась. */
export function exportedLabel(iso: string | null): string | null {
  if (!iso) return null;
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return null;
  return new Date(t).toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric' });
}

/** json — полная копия с фото, json-nophoto — та же копия без фото (?photos=0). */
export type DownloadKind = 'json' | 'json-nophoto' | 'products' | 'purchases' | 'movements';

/** Адрес скачивания. tz — смещение часового пояса в минутах к востоку от UTC (МСК = 180). */
export function downloadHref(kind: DownloadKind, tz = -new Date().getTimezoneOffset()): string {
  const json = kind === 'json' || kind === 'json-nophoto';
  const path = json ? '/api/export.json' : `/api/export/${kind}.csv`;
  const query = [kind === 'json-nophoto' ? 'photos=0' : '', tz === 0 ? '' : `tz=${tz}`].filter(Boolean).join('&');
  return query ? `${path}?${query}` : path;
}

/**
 * Склад пуст, если нет ни одной позиции, в том числе архивной: покупки и
 * движения без позиций не бывают. Решает сервер, это только подсказка кнопке.
 */
export function isWarehouseEmpty(activeProducts: number, archivedProducts: number): boolean {
  return activeProducts === 0 && archivedProducts === 0;
}
