// Сжатие снимка в браузере (ФТ-10.4.2): декодирование с учётом поворота камеры,
// масштаб до 1024 px, JPEG ≤ 300 КБ. Работает с DOM, поэтому без тестов Vitest;
// вся логика выбора размера и качества — в lib/photo.ts и проверена там.
import { encodeWithin, fitSize, PhotoError, pickedFileError, sniffImageType } from './photo';

type Source = { source: CanvasImageSource; width: number; height: number; release: () => void };

const CANT_READ = 'Не удалось прочитать снимок. Попробуйте другое фото.';

/** Декодирует файл: createImageBitmap, при неудаче — <img> (blob: разрешён в img-src). */
async function decode(file: File): Promise<Source> {
  if (typeof createImageBitmap === 'function') {
    try {
      // Поворот по EXIF: снимок с iPhone иначе окажется на боку
      const bmp = await createImageBitmap(file, { imageOrientation: 'from-image' });
      return { source: bmp, width: bmp.width, height: bmp.height, release: () => bmp.close() };
    } catch {
      try {
        const bmp = await createImageBitmap(file);
        return { source: bmp, width: bmp.width, height: bmp.height, release: () => bmp.close() };
      } catch {
        // дальше — запасной путь через <img>
      }
    }
  }
  const url = URL.createObjectURL(file);
  try {
    // onload, а не img.decode(): decode() в ряде встроенных браузеров не завершается
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const el = new Image();
      el.onload = () => resolve(el);
      el.onerror = () => reject(new PhotoError(CANT_READ));
      el.src = url;
    });
    return { source: img, width: img.naturalWidth, height: img.naturalHeight, release: () => undefined };
  } finally {
    // Картинка уже загружена и держится в памяти — адрес больше не нужен
    URL.revokeObjectURL(url);
  }
}

function toBlob(canvas: HTMLCanvasElement, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, 'image/jpeg', quality));
}

/** Сжатое фото: JPEG не больше 300 КБ, длинная сторона не больше 1024 px. */
export async function compressPhoto(file: File): Promise<Blob> {
  const bad = pickedFileError(file);
  if (bad) throw new PhotoError(bad);
  const src = await decode(file);
  try {
    if (src.width < 1 || src.height < 1) throw new PhotoError(CANT_READ);
    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new PhotoError(CANT_READ);
    let drawn = 0;
    const blob = await encodeWithin(async (side, quality) => {
      const { width, height } = fitSize(src.width, src.height, side);
      if (drawn !== side) {
        canvas.width = width;
        canvas.height = height;
        // JPEG без прозрачности: PNG с прозрачным фоном иначе станет чёрным
        ctx.fillStyle = '#fff';
        ctx.fillRect(0, 0, width, height);
        ctx.drawImage(src.source, 0, 0, width, height);
        drawn = side;
      }
      const out = await toBlob(canvas, quality);
      return out ? { blob: out, size: out.size } : null;
    });
    // Браузер без JPEG в canvas вернул бы PNG: сервер принял бы, но размер не тот
    const head = new Uint8Array(await blob.blob.slice(0, 12).arrayBuffer());
    if (sniffImageType(head) !== 'image/jpeg') throw new PhotoError('Браузер не смог сжать снимок в JPEG.');
    return blob.blob;
  } finally {
    src.release();
  }
}
