// Фото позиции (ФТ-10.4): чистая логика без DOM — расчёт размеров, подбор качества,
// проверка файла и типа. Работа с canvas и createImageBitmap — в lib/photoEncode.ts.

/** Длинная сторона после сжатия, px (ФТ-10.4.2). */
export const PHOTO_MAX_SIDE = 1024;
/** Размер файла после сжатия, байт: не больше 300 КБ (ФТ-10.4.2). */
export const PHOTO_MAX_BYTES = 300 * 1024;
/** Предел сервера (docs/API.md): сжатое фото всегда укладывается с запасом. */
export const PHOTO_SERVER_MAX_BYTES = 400 * 1024;
/** Исходный снимок больше этого не читаем: телефонные фото — единицы МБ. */
export const PHOTO_SOURCE_MAX_BYTES = 60 * 1024 * 1024;

export const QUALITY_START = 0.85;
export const QUALITY_MIN = 0.5;
const QUALITY_STEP = 0.05;
/** Если на 1024 px даже качество 0,5 не уложилось в 300 КБ — уменьшаем сторону. */
const FALLBACK_SIDES = [800, 640];

export type PhotoSize = { width: number; height: number };

/**
 * Размер после масштабирования: длинная сторона не больше max, пропорции сохраняются,
 * маленькое не увеличивается. Стороны целые и не меньше 1.
 */
export function fitSize(width: number, height: number, max: number = PHOTO_MAX_SIDE): PhotoSize {
  const w = Math.max(1, Math.floor(width));
  const h = Math.max(1, Math.floor(height));
  const long = Math.max(w, h);
  if (!Number.isFinite(long) || long <= max) return { width: w, height: h };
  const k = max / long;
  return { width: Math.max(1, Math.round(w * k)), height: Math.max(1, Math.round(h * k)) };
}

/** Качества JPEG по убыванию: 0,85 → 0,5 с шагом 0,05. */
export function qualitySteps(start: number = QUALITY_START, min: number = QUALITY_MIN): number[] {
  const out: number[] = [];
  for (let i = 0; ; i++) {
    const q = Math.round((start - i * QUALITY_STEP) * 100) / 100;
    if (q < min - 1e-9) break;
    out.push(q);
  }
  return out;
}

/** Фото не удалось подготовить; text можно показывать как есть. */
export class PhotoError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'PhotoError';
  }
}

export type Rendered = { size: number };

/**
 * Подбор качества: кодирует с убывающим качеством, пока результат не уложится в maxBytes.
 * render(maxSide, quality) кодирует картинку (в браузере — canvas.toBlob) и возвращает результат
 * с полем size. Сначала 1024 px, затем при необходимости 800 и 640. Ничего не подошло — PhotoError.
 */
export async function encodeWithin<T extends Rendered>(
  render: (maxSide: number, quality: number) => Promise<T | null>,
  maxBytes: number = PHOTO_MAX_BYTES,
): Promise<T> {
  for (const side of [PHOTO_MAX_SIDE, ...FALLBACK_SIDES]) {
    for (const q of qualitySteps()) {
      const out = await render(side, q);
      if (!out) throw new PhotoError('Не удалось сжать снимок. Попробуйте другое фото.');
      if (out.size <= maxBytes) return out;
    }
  }
  throw new PhotoError('Фото не удалось сжать до 300 КБ. Попробуйте другое.');
}

const IMAGE_EXT = /\.(jpe?g|png|webp|heic|heif|gif|bmp|avif|tiff?)$/i;

/** Проверка выбранного файла до чтения; null — подходит. Тип у HEIC бывает пустым — смотрим расширение. */
export function pickedFileError(file: { type: string; name: string; size: number }): string | null {
  if (file.size === 0) return 'Файл пустой.';
  const typed = file.type.startsWith('image/');
  if (!typed && !(file.type === '' && IMAGE_EXT.test(file.name))) return 'Нужна картинка: JPEG, PNG, WebP или снимок с камеры.';
  if (file.type === 'image/svg+xml') return 'SVG не подходит: нужен снимок или картинка JPEG, PNG, WebP.';
  if (file.size > PHOTO_SOURCE_MAX_BYTES) return 'Файл слишком большой. Выберите снимок поменьше.';
  return null;
}

export type PhotoType = 'image/jpeg' | 'image/png' | 'image/webp';

/** Тип картинки по первым байтам, как делает сервер (internal/photo); null — не JPEG, PNG и не WebP. */
export function sniffImageType(b: Uint8Array): PhotoType | null {
  if (b.length >= 3 && b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff) return 'image/jpeg';
  const png = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
  if (b.length >= 8 && png.every((v, i) => b[i] === v)) return 'image/png';
  const at = (i: number, s: string) => [...s].every((c, k) => b[i + k] === c.charCodeAt(0));
  if (b.length >= 12 && at(0, 'RIFF') && at(8, 'WEBP')) return 'image/webp';
  return null;
}

/** Адрес фото: ?v=<photo_updated_at> сбрасывает кэш при замене. null — у позиции нет фото. */
export function photoUrl(productId: number, updatedAt: number | null | undefined): string | null {
  if (updatedAt === null || updatedAt === undefined) return null;
  return `/api/products/${productId}/photo?v=${updatedAt}`;
}
