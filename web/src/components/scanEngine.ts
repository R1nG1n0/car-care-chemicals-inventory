// Камера и распознавание для сканера штрихкодов (ФТ-9.1, 9.6).
// zxing-wasm подгружается динамическим import() только при первом открытии
// сканера — в основной бандл попадает лишь адрес .wasm. Сам .wasm Vite кладёт
// в assets/ рядом со сборкой: библиотека по умолчанию тянет его с jsDelivr,
// поэтому locateFile переопределён (CSP запрещает внешние адреса). Строка с
// адресом jsDelivr в reader-*.js остаётся мёртвым кодом библиотеки: locateFile
// её подменяет, обращений к внешним адресам нет.
import wasmUrl from 'zxing-wasm/reader/zxing_reader.wasm?url';
import type { ReaderOptions } from 'zxing-wasm/reader';
import { cropRect, type CameraFailure, type Rect } from '../lib/barcode';

export class CameraError extends Error {
  readonly kind: CameraFailure;
  constructor(kind: CameraFailure) {
    super(kind);
    this.name = 'CameraError';
    this.kind = kind;
  }
}

/** Задняя камера. Без HTTPS (кроме localhost) браузер не даёт mediaDevices. */
export async function openCamera(): Promise<MediaStream> {
  if (!window.isSecureContext) throw new CameraError('insecure');
  if (!navigator.mediaDevices?.getUserMedia) throw new CameraError('unsupported');
  return navigator.mediaDevices.getUserMedia({
    audio: false,
    video: { facingMode: { ideal: 'environment' }, width: { ideal: 1280 }, height: { ideal: 720 } },
  });
}

export function stopStream(stream: MediaStream | null | undefined): void {
  stream?.getTracks().forEach((t) => t.stop());
}

/** Фонарик есть в capabilities не везде: Chrome на Android — да, Safari на iPhone — пока нет. */
export function torchSupported(track: MediaStreamTrack | undefined): boolean {
  const caps = track?.getCapabilities?.() as (MediaTrackCapabilities & { torch?: boolean }) | undefined;
  return caps?.torch === true;
}

export function setTorch(track: MediaStreamTrack, on: boolean): Promise<void> {
  return track.applyConstraints({ advanced: [{ torch: on } as MediaTrackConstraintSet] });
}

const READER: ReaderOptions = {
  formats: ['EAN13', 'EAN8', 'UPCA', 'UPCE', 'Code128', 'QRCode'],
  maxNumberOfSymbols: 1,
  // Штрихкод в рамке всегда горизонтальный и тёмный на светлом: поворот, инверсия и
  // шумоподавление только тратят время (распознавание идёт в основном потоке).
  // tryHarder и tryDownscale оставлены: нужны для мелких и размытых EAN.
  tryHarder: true,
  tryRotate: false,
  tryInvert: false,
  tryDenoise: false,
  tryDownscale: true,
};

export type Reader = (img: ImageData) => Promise<string[]>;

let reader: Promise<Reader> | null = null;

/** Модуль распознавания: грузится один раз; ошибка загрузки — можно повторить. */
export function loadReader(): Promise<Reader> {
  reader ??= import('zxing-wasm/reader')
    .then(async (zx) => {
      await zx.prepareZXingModule({
        overrides: { locateFile: (path: string, prefix: string) => (path.endsWith('.wasm') ? wasmUrl : prefix + path) },
        fireImmediately: true,
      });
      return async (img: ImageData) => (await zx.readBarcodes(img, READER)).filter((r) => r.isValid).map((r) => r.text);
    })
    .catch((err: unknown) => {
      reader = null;
      throw err;
    });
  return reader;
}

/** Самая длинная сторона кадра для распознавания: больше — медленнее без выигрыша. */
const MAX_SIDE = 960;

/**
 * Снимок области под рамкой видоискателя. null — видео ещё не готово.
 * canvas переиспользуется между кадрами. getImageData каждый раз создаёт новый
 * буфер (API canvas не умеет писать в готовый), поэтому кадр ограничен MAX_SIDE.
 */
export function grabFrame(video: HTMLVideoElement, frame: HTMLElement, canvas: HTMLCanvasElement): ImageData | null {
  if (video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA || video.videoWidth === 0) return null;
  const v = video.getBoundingClientRect();
  const f = frame.getBoundingClientRect();
  const box: Rect = { x: f.left - v.left, y: f.top - v.top, w: f.width, h: f.height };
  // По высоте запас больше: рамка низкая, под штрихкод, а QR квадратный
  const src = cropRect({ w: video.videoWidth, h: video.videoHeight }, { w: v.width, h: v.height }, box, 0.2, 0.5);
  const k = Math.min(1, MAX_SIDE / Math.max(src.w, src.h));
  const w = Math.max(1, Math.round(src.w * k));
  const h = Math.max(1, Math.round(src.h * k));
  if (canvas.width !== w) canvas.width = w;
  if (canvas.height !== h) canvas.height = h;
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  if (!ctx) return null;
  ctx.drawImage(video, src.x, src.y, src.w, src.h, 0, 0, w, h);
  return ctx.getImageData(0, 0, w, h);
}

/** Короткий отклик «поймал» там, где есть вибромотор (не на iPhone). */
export function buzz(): void {
  try {
    navigator.vibrate?.(40);
  } catch {
    // vibrate может быть запрещён политикой страницы — не важно
  }
}
