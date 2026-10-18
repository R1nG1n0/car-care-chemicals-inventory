// Штрихкоды (ФТ-9): проверка кода со сканера и из ручного ввода, точное сравнение,
// что делать с результатом поиска, привязка кода к позиции, область кадра для
// распознавания. Без React и браузера — проверяется тестами.
import type { ProductSummary } from '../api';

/** У позиции хранятся только цифры, до 32 (сервер: «Штрихкод — до 32 цифр»). */
export const BARCODE_MAX = 32;

export const NOT_PRODUCT_CODE = 'Это не штрихкод товара';

/** Убирает пробелы и дефисы (в том числе тире U+2010–2015 и минус U+2212, как их подставляет клавиатура): «4 600000 000000», «460–000». */
export function normalizeCode(raw: string): string {
  return raw.replace(/[\s\-\u2010-\u2015\u2212]/gu, '');
}

export type ScanParse = { ok: true; code: string } | { ok: false; error: string };

/**
 * Текст, распознанный камерой. QR и Code 128 бывают с буквами (адрес сайта,
 * серийный номер) — такой код товару не присвоить, сканирование продолжается.
 */
export function parseScanned(text: string): ScanParse {
  const code = normalizeCode(text);
  if (!/^\d+$/u.test(code) || code.length > BARCODE_MAX) return { ok: false, error: NOT_PRODUCT_CODE };
  return { ok: true, code };
}

/** Код, набранный вручную: ошибки — для поля ввода. */
export function parseManual(text: string): ScanParse {
  const code = normalizeCode(text);
  if (code === '') return { ok: false, error: 'Введите цифры под штрихкодом' };
  if (!/^\d+$/u.test(code)) return { ok: false, error: 'Только цифры' };
  if (code.length > BARCODE_MAX) return { ok: false, error: `До ${BARCODE_MAX} цифр` };
  return { ok: true, code };
}

/**
 * Один и тот же товар: коды совпадают или это UPC-A (12 цифр) и тот же код
 * в виде EAN-13 с ведущим нулём — сканер читает UPC-A без нуля, а под
 * штрихкодом бывает напечатано 13 цифр.
 */
export function sameBarcode(a: string, b: string): boolean {
  if (a === '' || b === '') return false;
  if (a === b) return true;
  const [short, long] = a.length < b.length ? [a, b] : [b, a];
  return short.length === 12 && long.length === 13 && long === `0${short}`;
}

/**
 * Что передать в поиск GET /api/products?q=: сервер ищет подстроку, поэтому
 * для EAN-13 с ведущим нулём ищем без нуля — найдутся обе записи кода.
 */
export function searchKey(code: string): string {
  return code.length === 13 && code.startsWith('0') ? code.slice(1) : code;
}

/** Позиции с точно таким кодом (поиск сервера — по подстроке). Без повторов по id. */
export function findExact<P extends Pick<ProductSummary, 'id' | 'barcode'>>(products: readonly P[], code: string): P[] {
  const seen = new Set<number>();
  return products.filter((p) => {
    if (seen.has(p.id) || !sameBarcode(p.barcode, code)) return false;
    seen.add(p.id);
    return true;
  });
}

export type ScanOutcome<P> = { kind: 'none' } | { kind: 'open'; product: P } | { kind: 'choose'; products: P[] };

/**
 * Что делать после сканирования на главной (ФТ-9.2, 9.4, 9.5). Архивные
 * позиции учитываются, только если действующих с этим кодом нет: если товар
 * снова куплен и заведён заново, старая запись не мешает.
 */
export function scanOutcome<P extends Pick<ProductSummary, 'archived' | 'name'>>(matches: readonly P[]): ScanOutcome<P> {
  const active = matches.filter((p) => !p.archived);
  const pool = active.length > 0 ? active : [...matches];
  if (pool.length === 0) return { kind: 'none' };
  if (pool.length === 1) return { kind: 'open', product: pool[0]! };
  return { kind: 'choose', products: [...pool].sort((a, b) => a.name.localeCompare(b.name, 'ru')) };
}

/** Привязка кода к выбранной позиции: просто записать, уже стоит или заменить чужой код (спросить). */
export type BindPlan = 'bind' | 'already' | 'replace';

export function bindPlan(p: Pick<ProductSummary, 'barcode'>, code: string): BindPlan {
  if (p.barcode === '') return 'bind';
  return sameBarcode(p.barcode, code) ? 'already' : 'replace';
}

/** Ошибка getUserMedia → понятная причина для пользователя. */
export type CameraFailure = 'insecure' | 'unsupported' | 'denied' | 'nocamera' | 'busy' | 'error';

export function cameraFailure(err: unknown): CameraFailure {
  // DOMException в части браузеров не instanceof Error — смотрим только на name
  const name = typeof err === 'object' && err !== null && 'name' in err ? String(err.name) : '';
  switch (name) {
    case 'NotAllowedError':
    case 'PermissionDeniedError':
    case 'SecurityError':
      return 'denied';
    case 'NotFoundError':
    case 'DevicesNotFoundError':
    case 'OverconstrainedError':
      return 'nocamera';
    case 'NotSupportedError':
      return 'unsupported';
    case 'NotReadableError':
    case 'TrackStartError':
    case 'AbortError':
      return 'busy';
    default:
      return 'error';
  }
}

export type Rect = { x: number; y: number; w: number; h: number };

/**
 * Область кадра камеры (в пикселях видео), которая видна в рамке видоискателя.
 * Видео растянуто по элементу с object-fit: cover; рамку берём с запасом
 * `margin` (по ширине) и `marginY` (по высоте) с каждой стороны — доля её
 * размера: код редко ровно в рамке, а QR выше низкой рамки для штрихкода.
 * view — размер элемента видео, frame — рамка в координатах элемента.
 */
export function cropRect(video: { w: number; h: number }, view: { w: number; h: number }, frame: Rect, margin = 0.2, marginY = margin): Rect {
  const full = { x: 0, y: 0, w: video.w, h: video.h };
  if (video.w <= 0 || video.h <= 0 || view.w <= 0 || view.h <= 0 || frame.w <= 0 || frame.h <= 0) return full;
  const scale = Math.max(view.w / video.w, view.h / video.h);
  const ox = (view.w - video.w * scale) / 2;
  const oy = (view.h - video.h * scale) / 2;
  const mx = frame.w * margin;
  const my = frame.h * marginY;
  const x0 = Math.max(0, Math.floor((frame.x - mx - ox) / scale));
  const y0 = Math.max(0, Math.floor((frame.y - my - oy) / scale));
  const x1 = Math.min(video.w, Math.ceil((frame.x + frame.w + mx - ox) / scale));
  const y1 = Math.min(video.h, Math.ceil((frame.y + frame.h + my - oy) / scale));
  if (x1 - x0 < 16 || y1 - y0 < 16) return full;
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
}
