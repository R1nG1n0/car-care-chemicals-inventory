import { describe, expect, it } from 'vitest';
import {
  countLines,
  downloadHref,
  exportedLabel,
  fileSizeError,
  importedText,
  isWarehouseEmpty,
  MAX_IMPORT_BYTES,
  parseExportFile,
  type TransferCounts,
} from './transfer';

const file = (extra: Record<string, unknown> = {}) =>
  JSON.stringify({ format: 'garage-export', version: 1, exported_at: '2026-10-14T17:46:40Z', user: { login: 'ringino' }, ...extra });

describe('parseExportFile', () => {
  it('считает записи по видам, строки покупок суммирует', () => {
    const r = parseExportFile(
      file({
        products: [{}, {}, {}],
        purchases: [{ lines: [{}, {}] }, { lines: [{}] }],
        movements: [{}],
        categories: [{}, {}],
      }),
    );
    expect(r.ok).toBe(true);
    if (!r.ok) return;
    expect(r.summary.login).toBe('ringino');
    expect(r.summary.exportedAt).toBe('2026-10-14T17:46:40Z');
    expect(r.summary.counts).toEqual({
      categories: 2,
      locations: 0,
      stores: 0,
      products: 3,
      units: 0,
      purchases: 2,
      purchase_lines: 3,
      movements: 1,
      recipes: 0,
      shopping: 0,
    });
  });

  it('отсутствующие и null-разделы — пустые', () => {
    const r = parseExportFile(file({ products: null }));
    expect(r.ok && r.summary.counts.products).toBe(0);
  });

  it('BOM в начале файла не мешает', () => {
    expect(parseExportFile('﻿' + file()).ok).toBe(true);
  });

  it('не JSON', () => {
    const r = parseExportFile('это не json');
    expect(r).toEqual({ ok: false, error: 'Файл повреждён или это не JSON.' });
    expect(parseExportFile('').ok).toBe(false);
  });

  it('чужой JSON, массив, примитив', () => {
    for (const text of ['{"a":1}', '[]', '5', 'null', '{"format":"other","version":1}']) {
      const r = parseExportFile(text);
      expect(r.ok).toBe(false);
      if (!r.ok) expect(r.error).toContain('не файл экспорта');
    }
  });

  it('версия: другая и отсутствующая', () => {
    const v2 = parseExportFile(JSON.stringify({ format: 'garage-export', version: 2 }));
    expect(v2).toEqual({ ok: false, error: 'Версия файла 2 не поддерживается: нужна версия 1.' });
    const none = parseExportFile(JSON.stringify({ format: 'garage-export' }));
    expect(none).toEqual({ ok: false, error: 'Версия файла не поддерживается: нужна версия 1.' });
  });

  it('раздел не массив — ошибка с названием раздела', () => {
    const r = parseExportFile(file({ products: { a: 1 } }));
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.error).toContain('«products»');
  });

  it('логин и дата необязательны', () => {
    const r = parseExportFile(JSON.stringify({ format: 'garage-export', version: 1 }));
    expect(r.ok && r.summary.login).toBe('');
    expect(r.ok && r.summary.exportedAt).toBeNull();
  });
});

describe('предпросмотр', () => {
  const zero: TransferCounts = {
    categories: 0, locations: 0, stores: 0, products: 0, units: 0, purchases: 0,
    purchase_lines: 0, movements: 0, recipes: 0, shopping: 0,
  };

  it('склонения и порядок, пустые виды пропускаются', () => {
    expect(countLines({ ...zero, products: 21, purchases: 2, movements: 5, recipes: 1, locations: 11 })).toEqual([
      '21 позиция', '2 покупки', '5 движений', '1 шаблон', '11 мест',
    ]);
    expect(countLines(zero)).toEqual([]);
  });

  it('текст уведомления: позиции, покупки, движения', () => {
    expect(importedText({ ...zero, products: 12, purchases: 3, movements: 40, units: 9, locations: 2 })).toBe('12 позиций, 3 покупки, 40 движений');
    expect(importedText(zero)).toBe('файл был пустым');
  });

  it('дата экспорта', () => {
    expect(exportedLabel('2026-10-14T12:00:00Z')).toBe('14.10.2026');
    expect(exportedLabel(null)).toBeNull();
    expect(exportedLabel('вчера')).toBeNull();
  });
});

describe('размер файла', () => {
  it('до 25 МБ включительно проходит', () => {
    expect(fileSizeError(1)).toBeNull();
    expect(fileSizeError(MAX_IMPORT_BYTES)).toBeNull();
  });
  it('больше предела и пустой — отказ', () => {
    expect(fileSizeError(MAX_IMPORT_BYTES + 1)).toContain('25 МБ');
    expect(fileSizeError(0)).toBe('Файл пустой.');
  });
});

describe('скачивание и пустой склад', () => {
  it('адреса с часовым поясом', () => {
    expect(downloadHref('json', 180)).toBe('/api/export.json?tz=180');
    expect(downloadHref('products', 0)).toBe('/api/export/products.csv');
    expect(downloadHref('movements', -300)).toBe('/api/export/movements.csv?tz=-300');
    expect(downloadHref('purchases', 180)).toBe('/api/export/purchases.csv?tz=180');
  });
  it('склад пуст, только если нет позиций, в том числе архивных', () => {
    expect(isWarehouseEmpty(0, 0)).toBe(true);
    expect(isWarehouseEmpty(1, 0)).toBe(false);
    expect(isWarehouseEmpty(0, 2)).toBe(false);
  });
});
