import { describe, expect, it } from 'vitest';
import {
  BARCODE_MAX,
  bindPlan,
  cameraFailure,
  cropRect,
  findExact,
  NOT_PRODUCT_CODE,
  normalizeCode,
  parseManual,
  parseScanned,
  sameBarcode,
  scanOutcome,
  searchKey,
} from './barcode';

const p = (id: number, barcode: string, archived = false, name = `П${id}`) => ({ id, barcode, archived, name });

describe('код со сканера', () => {
  it('EAN-13, EAN-8, UPC — цифры проходят', () => {
    expect(parseScanned('4600000000000')).toEqual({ ok: true, code: '4600000000000' });
    expect(parseScanned('96385074')).toEqual({ ok: true, code: '96385074' });
    expect(parseScanned('012345678905')).toEqual({ ok: true, code: '012345678905' });
  });
  it('QR с адресом и Code 128 с буквами — не товар', () => {
    expect(parseScanned('https://example.com/p/1')).toEqual({ ok: false, error: NOT_PRODUCT_CODE });
    expect(parseScanned('SN-AB12')).toEqual({ ok: false, error: NOT_PRODUCT_CODE });
    expect(parseScanned('')).toEqual({ ok: false, error: NOT_PRODUCT_CODE });
  });
  it('цифр больше 32 — не товар', () => {
    expect(parseScanned('1'.repeat(BARCODE_MAX)).ok).toBe(true);
    expect(parseScanned('1'.repeat(BARCODE_MAX + 1))).toEqual({ ok: false, error: NOT_PRODUCT_CODE });
  });
  it('пробелы и дефисы убираются', () => {
    expect(normalizeCode(' 4 600000-000000 ')).toBe('4600000000000');
    expect(parseScanned('4 600000 000000')).toEqual({ ok: true, code: '4600000000000' });
  });
});

describe('ручной ввод', () => {
  it('ошибки для поля', () => {
    expect(parseManual('  ')).toEqual({ ok: false, error: 'Введите цифры под штрихкодом' });
    expect(parseManual('46OO')).toEqual({ ok: false, error: 'Только цифры' });
    expect(parseManual('9'.repeat(33))).toEqual({ ok: false, error: 'До 32 цифр' });
    expect(parseManual('4 600000 000000')).toEqual({ ok: true, code: '4600000000000' });
  });
});

describe('точное сравнение', () => {
  it('совпадение целиком, а не подстрока', () => {
    expect(sameBarcode('4600000000000', '4600000000000')).toBe(true);
    expect(sameBarcode('46000000000001', '4600000000000')).toBe(false);
    expect(sameBarcode('460000000000', '4600000000000')).toBe(false);
    expect(sameBarcode('', '')).toBe(false);
  });
  it('UPC-A и тот же код в EAN-13 с ведущим нулём', () => {
    expect(sameBarcode('012345678905', '0012345678905')).toBe(true);
    expect(sameBarcode('0012345678905', '012345678905')).toBe(true);
    expect(sameBarcode('012345678905', '1012345678905')).toBe(false);
  });
  it('поиск на сервере — без ведущего нуля EAN-13', () => {
    expect(searchKey('0012345678905')).toBe('012345678905');
    expect(searchKey('4600000000000')).toBe('4600000000000');
    expect(searchKey('012345678905')).toBe('012345678905');
  });
  it('из ответа сервера (подстрока) остаются только точные, без повторов', () => {
    const list = [p(1, '4600000000000'), p(2, '14600000000000'), p(3, ''), p(1, '4600000000000'), p(4, '4600000000000', true)];
    expect(findExact(list, '4600000000000').map((x) => x.id)).toEqual([1, 4]);
    expect(findExact(list, '0000')).toEqual([]);
  });
});

describe('что делать после сканирования', () => {
  it('ни одной — предложить создать или привязать', () => {
    expect(scanOutcome([])).toEqual({ kind: 'none' });
  });
  it('одна — открыть карточку', () => {
    expect(scanOutcome([p(1, 'x')])).toEqual({ kind: 'open', product: p(1, 'x') });
  });
  it('несколько — выбор по алфавиту', () => {
    const r = scanOutcome([p(1, 'x', false, 'Шампунь'), p(2, 'x', false, 'Активная пена')]);
    expect(r.kind).toBe('choose');
    expect(r.kind === 'choose' && r.products.map((x) => x.id)).toEqual([2, 1]);
  });
  it('архивные — только если действующих нет', () => {
    expect(scanOutcome([p(1, 'x', true), p(2, 'x')])).toEqual({ kind: 'open', product: p(2, 'x') });
    expect(scanOutcome([p(1, 'x', true)])).toEqual({ kind: 'open', product: p(1, 'x', true) });
    expect(scanOutcome([p(1, 'x', true), p(2, 'x', true)]).kind).toBe('choose');
  });
});

describe('привязка кода', () => {
  it('без кода — записать, с тем же — уже стоит, с другим — спросить', () => {
    expect(bindPlan({ barcode: '' }, '4600000000000')).toBe('bind');
    expect(bindPlan({ barcode: '4600000000000' }, '4600000000000')).toBe('already');
    expect(bindPlan({ barcode: '012345678905' }, '0012345678905')).toBe('already');
    expect(bindPlan({ barcode: '4007218000123' }, '4600000000000')).toBe('replace');
  });
});

describe('ошибки камеры', () => {
  const e = (name: string) => Object.assign(new Error('x'), { name });
  it('по имени ошибки getUserMedia', () => {
    expect(cameraFailure(e('NotAllowedError'))).toBe('denied');
    expect(cameraFailure({ name: 'SecurityError' })).toBe('denied');
    expect(cameraFailure(e('NotFoundError'))).toBe('nocamera');
    expect(cameraFailure(e('OverconstrainedError'))).toBe('nocamera');
    expect(cameraFailure(e('NotReadableError'))).toBe('busy');
    expect(cameraFailure(e('NotSupportedError'))).toBe('unsupported');
    expect(cameraFailure(e('TypeError'))).toBe('error');
    expect(cameraFailure('строка')).toBe('error');
    expect(cameraFailure(null)).toBe('error');
  });
});

describe('область кадра под рамкой', () => {
  it('видео шире экрана: обрезано по бокам (object-fit: cover)', () => {
    // 1280×720 в элементе 360×640: масштаб 640/720, видно 568 px из 1280 по ширине
    const r = cropRect({ w: 1280, h: 720 }, { w: 360, h: 640 }, { x: 30, y: 220, w: 300, h: 200 }, 0);
    // центр рамки — центр кадра: 471 + 338 / 2 = 640
    expect(r).toEqual({ x: 471, y: 247, w: 338, h: 226 });
  });
  it('запас вокруг рамки и границы кадра', () => {
    const r = cropRect({ w: 1280, h: 720 }, { w: 1280, h: 720 }, { x: 0, y: 260, w: 1280, h: 200 }, 0.2);
    expect(r).toEqual({ x: 0, y: 220, w: 1280, h: 280 });
  });
  it('по высоте запас больше — для QR', () => {
    const r = cropRect({ w: 1000, h: 1000 }, { w: 1000, h: 1000 }, { x: 300, y: 400, w: 400, h: 200 }, 0.1, 0.5);
    expect(r).toEqual({ x: 260, y: 300, w: 480, h: 400 });
  });
  it('нет размеров — весь кадр', () => {
    expect(cropRect({ w: 640, h: 480 }, { w: 0, h: 0 }, { x: 0, y: 0, w: 10, h: 10 })).toEqual({ x: 0, y: 0, w: 640, h: 480 });
    expect(cropRect({ w: 640, h: 480 }, { w: 100, h: 100 }, { x: 500, y: 500, w: 10, h: 10 })).toEqual({ x: 0, y: 0, w: 640, h: 480 });
  });
});
