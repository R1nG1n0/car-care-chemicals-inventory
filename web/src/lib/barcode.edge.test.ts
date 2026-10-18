import { describe, expect, it } from 'vitest';
import { bindPlan, BARCODE_MAX, cropRect, findExact, NOT_PRODUCT_CODE, normalizeCode, parseManual, parseScanned, sameBarcode, scanOutcome, searchKey } from './barcode';

const p = (id: number, barcode: string, archived = false, name = `П${id}`) => ({ id, barcode, archived, name });

describe('нормализация: пробелы и дефисы', () => {
  it('любые пробельные символы убираются: таб, перевод строки, NBSP, узкий пробел', () => {
    expect(normalizeCode('4\t600\n000\u00a0000\u202f000')).toBe('4600000000000');
  });
  it('дефисы подряд и по краям', () => {
    expect(normalizeCode('--4--6--')).toBe('46');
    expect(normalizeCode(' - - ')).toBe('');
  });
  it('только пробелы и дефисы — пусто', () => {
    expect(parseScanned(' - ')).toEqual({ ok: false, error: NOT_PRODUCT_CODE });
    expect(parseManual(' - ')).toEqual({ ok: false, error: 'Введите цифры под штрихкодом' });
  });
  it('длинное тире и минус считаются дефисом: клавиатура iOS подставляет их сама', () => {
    expect(parseManual('460–000')).toEqual({ ok: true, code: '460000' });
    expect(parseManual('460−000')).toEqual({ ok: true, code: '460000' });
    expect(parseManual('460—000')).toEqual({ ok: true, code: '460000' });
  });
});

describe('валидация: буквы, ноль, длина', () => {
  it('буквы, латинская O вместо нуля, кириллическая О', () => {
    expect(parseManual('46OO').ok).toBe(false);
    expect(parseManual('46ОО').ok).toBe(false);
    expect(parseScanned('46OO')).toEqual({ ok: false, error: NOT_PRODUCT_CODE });
  });
  it('арабо-индийские и полноширинные цифры — не цифры (хранятся только ASCII)', () => {
    expect(parseScanned('٤٦٠٠').ok).toBe(false);
    expect(parseManual('４６００').ok).toBe(false);
  });
  it('«0» и ноли подряд — допустимый код, ведущие нули сохраняются', () => {
    expect(parseManual('0')).toEqual({ ok: true, code: '0' });
    expect(parseScanned('00000000')).toEqual({ ok: true, code: '00000000' });
    expect(parseManual('000123')).toEqual({ ok: true, code: '000123' });
  });
  it('32 цифры проходят, 33 нет — и в сканере, и вручную', () => {
    expect(parseManual('7'.repeat(32))).toEqual({ ok: true, code: '7'.repeat(32) });
    expect(parseManual('7'.repeat(33))).toEqual({ ok: false, error: `До ${BARCODE_MAX} цифр` });
    expect(parseScanned('7'.repeat(33)).ok).toBe(false);
  });
  it('длина считается после удаления пробелов и дефисов', () => {
    const spaced = '7 '.repeat(32).trim(); // 32 цифры с пробелами
    expect(parseManual(spaced).ok).toBe(true);
    expect(parseManual('7-'.repeat(33)).ok).toBe(false);
  });
  it('цифры вперемешку с буквами: приоритет ошибки «Только цифры» над длиной', () => {
    expect(parseManual('a' + '7'.repeat(40))).toEqual({ ok: false, error: 'Только цифры' });
  });
  it('десятичная точка, плюс, запятая, скобки не проходят', () => {
    for (const s of ['4.6', '+46', '4,6', '(46)', '4/6']) expect(parseManual(s).ok).toBe(false);
  });
  it('инъекция в тексте QR не мешает: просто не код товара', () => {
    expect(parseScanned('<script>alert(1)</script>')).toEqual({ ok: false, error: NOT_PRODUCT_CODE });
    expect(parseScanned("1'; DROP TABLE products;--").ok).toBe(false);
  });
});

describe('UPC-A ↔ EAN-13, EAN-8', () => {
  it('UPC-A и EAN-13 с ведущим нулём — один код в обе стороны', () => {
    expect(sameBarcode('012345678905', '0012345678905')).toBe(true);
    expect(sameBarcode('0012345678905', '012345678905')).toBe(true);
  });
  it('нуль не в начале и другой префикс — разные коды', () => {
    expect(sameBarcode('012345678905', '1012345678905')).toBe(false);
    expect(sameBarcode('012345678905', '0012345678906')).toBe(false);
  });
  it('12 цифр против 13 — только при ровно одном ведущем нуле, не при двух', () => {
    expect(sameBarcode('12345678905', '0012345678905')).toBe(false); // 11 против 13
    expect(sameBarcode('96385074', '0000096385074')).toBe(false); // EAN-8 не приравнивается к EAN-13
    expect(sameBarcode('96385074', '000096385074')).toBe(false); // и к 12-значному тоже
  });
  it('EAN-8 сравнивается только с собой', () => {
    expect(sameBarcode('96385074', '96385074')).toBe(true);
    expect(sameBarcode('96385074', '96385075')).toBe(false);
  });
  it('пустые коды никогда не равны, даже друг другу', () => {
    expect(sameBarcode('', '')).toBe(false);
    expect(sameBarcode('', '4600000000000')).toBe(false);
    expect(sameBarcode('0', '')).toBe(false);
  });
  it('13 цифр без нуля и 12 цифр с суффиксом — не совпадают', () => {
    expect(sameBarcode('460000000000', '4600000000000')).toBe(false);
  });
  it('ключ поиска: только EAN-13 с нулём урезается; EAN-8 и UPC-A как есть', () => {
    expect(searchKey('0000096385074')).toBe('000096385074');
    expect(searchKey('96385074')).toBe('96385074');
    expect(searchKey('0')).toBe('0');
    expect(searchKey('')).toBe('');
    expect(searchKey('0'.repeat(13))).toBe('0'.repeat(12));
  });
  it('ключ поиска находит запись в обоих видах (подстрока на сервере)', () => {
    const stored = ['012345678905', '0012345678905'];
    for (const scanned of ['012345678905', '0012345678905']) {
      const key = searchKey(scanned);
      expect(stored.filter((s) => s.includes(key))).toHaveLength(2);
    }
  });
  it('findExact: UPC-A в базе находится по EAN-13 со сканера и наоборот, чужой код с подстрокой — нет', () => {
    const list = [p(1, '012345678905'), p(2, '0012345678905'), p(3, '9012345678905'), p(4, '')];
    expect(findExact(list, '0012345678905').map((x) => x.id)).toEqual([1, 2]);
    expect(findExact(list, '012345678905').map((x) => x.id)).toEqual([1, 2]);
  });
  it('findExact: пустой код и пустой список', () => {
    expect(findExact([p(1, '')], '')).toEqual([]);
    expect(findExact([], '4600000000000')).toEqual([]);
  });
});

describe('выбор действия по числу найденных', () => {
  it('одна архивная — открыть её', () => {
    expect(scanOutcome([p(1, 'x', true)])).toEqual({ kind: 'open', product: p(1, 'x', true) });
  });
  it('архивные + одна действующая — действующая; архивные + две действующих — выбор только из действующих', () => {
    const r = scanOutcome([p(1, 'x', true, 'А'), p(2, 'x', false, 'Б'), p(3, 'x', false, 'В')]);
    expect(r.kind === 'choose' && r.products.map((x) => x.id)).toEqual([2, 3]);
  });
  it('сортировка по-русски: ё, регистр, латиница', () => {
    const r = scanOutcome([p(1, 'x', false, 'яблоко'), p(2, 'x', false, 'Ёлка'), p(3, 'x', false, 'Арбуз')]);
    expect(r.kind === 'choose' && r.products.map((x) => x.id)).toEqual([3, 2, 1]);
  });
  it('исходный массив не меняется', () => {
    const src = [p(1, 'x', false, 'Я'), p(2, 'x', false, 'А')];
    scanOutcome(src);
    expect(src.map((x) => x.id)).toEqual([1, 2]);
  });
  it('одинаковые имена — порядок стабилен', () => {
    const r = scanOutcome([p(1, 'x', false, 'Пена'), p(2, 'x', false, 'Пена')]);
    expect(r.kind === 'choose' && r.products.map((x) => x.id)).toEqual([1, 2]);
  });
  it('все архивные, две — выбор, не открытие', () => {
    expect(scanOutcome([p(1, 'x', true), p(2, 'x', true)]).kind).toBe('choose');
  });
});

describe('план привязки', () => {
  it('нет кода — привязать, независимо от вида нового кода', () => {
    expect(bindPlan({ barcode: '' }, '0')).toBe('bind');
  });
  it('тот же код, в том числе UPC-A ↔ EAN-13 — «уже стоит»', () => {
    expect(bindPlan({ barcode: '4600000000008' }, '4600000000008')).toBe('already');
    expect(bindPlan({ barcode: '0012345678905' }, '012345678905')).toBe('already');
  });
  it('другой код — заменить; код-подстрока тоже другой', () => {
    expect(bindPlan({ barcode: '4600000000008' }, '600000000008')).toBe('replace');
    expect(bindPlan({ barcode: '96385074' }, '0000096385074')).toBe('replace');
  });
  it('пустой отсканированный код при заполненном — «заменить» (на практике недостижимо: parse*)', () => {
    expect(bindPlan({ barcode: '4600000000008' }, '')).toBe('replace');
  });
});

describe('область кадра: крайние случаи', () => {
  it('отрицательные и NaN размеры — весь кадр без падения', () => {
    const full = { x: 0, y: 0, w: 640, h: 480 };
    expect(cropRect({ w: 640, h: 480 }, { w: -1, h: 100 }, { x: 0, y: 0, w: 10, h: 10 })).toEqual(full);
    expect(cropRect({ w: 0, h: 0 }, { w: 100, h: 100 }, { x: 0, y: 0, w: 10, h: 10 })).toEqual({ x: 0, y: 0, w: 0, h: 0 });
  });
  it('рамка за пределами элемента не даёт отрицательных координат и выхода за кадр', () => {
    const r = cropRect({ w: 640, h: 480 }, { w: 320, h: 240 }, { x: -200, y: -200, w: 1000, h: 1000 });
    expect(r.x).toBeGreaterThanOrEqual(0);
    expect(r.y).toBeGreaterThanOrEqual(0);
    expect(r.x + r.w).toBeLessThanOrEqual(640);
    expect(r.y + r.h).toBeLessThanOrEqual(480);
  });
  it('видео выше экрана (портрет в узком окне) — обрезка по высоте', () => {
    const r = cropRect({ w: 480, h: 640 }, { w: 640, h: 360 }, { x: 0, y: 0, w: 640, h: 360 }, 0);
    expect(r.x).toBe(0);
    expect(r.w).toBe(480);
    expect(r.h).toBeLessThan(640);
  });
});
