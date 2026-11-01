import { describe, expect, it } from 'vitest';
import type { Rag } from '../api';
import { keepActive, levelText, mergeRags, parseWearLimit, ragsLine, retireReasonText, sortRags, startLabel, toggleAll, toggleSelected, validateRagLabel, wearInput, washedText, washLabel, wearLevel, wearShare, wearText } from './rags';
import { validateProduct, type ProductFields } from './product';

const rag = (p: Partial<Rag> = {}): Rag => ({ id: 1, product_id: 5, label: '№ 1', number: 1, washes: 0, wear_limit: 100, level: 'ok', status: 'active', retire_reason: null, created_at: 1, retired_at: null, last_washed_at: null, ...p });

describe('уровень износа', () => {
  it('до 90 % — ok, от 90 % — скоро, на 100 % и больше — пора', () => {
    expect(wearLevel(0, 100)).toBe('ok');
    expect(wearLevel(89, 100)).toBe('ok');
    expect(wearLevel(90, 100)).toBe('soon');
    expect(wearLevel(99, 100)).toBe('soon');
    expect(wearLevel(100, 100)).toBe('due');
    expect(wearLevel(120, 100)).toBe('due');
  });
  it('малый ресурс: 10 стирок — скоро на 9-й', () => {
    expect(wearLevel(8, 10)).toBe('ok');
    expect(wearLevel(9, 10)).toBe('soon');
    expect(wearLevel(1, 1)).toBe('due');
  });
  it('нулевой ресурс не ломает расчёт', () => {
    expect(wearLevel(5, 0)).toBe('ok');
    expect(wearShare(5, 0)).toBe(0);
  });
  it('доля шкалы в границах 0…1', () => {
    expect(wearShare(0, 100)).toBe(0);
    expect(wearShare(37, 100)).toBeCloseTo(0.37);
    expect(wearShare(150, 100)).toBe(1);
  });
  it('подписи', () => {
    expect(wearText(37, 100)).toBe('37 из 100');
    expect(wearText(37, null)).toBe('37');
    expect(levelText('ok')).toBeNull();
    expect(levelText('soon')).toBe('скоро менять');
    expect(levelText('due')).toBe('пора менять');
  });
});

describe('строка позиции на главной', () => {
  it('нет тряпок — ничего', () => {
    expect(ragsLine(null)).toBeNull();
    expect(ragsLine(undefined)).toBeNull();
    expect(ragsLine({ active: 0, soon: 0, due: 0 })).toBeNull();
  });
  it('в работе и пора менять', () => {
    expect(ragsLine({ active: 5, soon: 0, due: 0 })).toEqual({ text: 'в работе 5', urgent: false });
    expect(ragsLine({ active: 5, soon: 2, due: 1 })).toEqual({ text: 'в работе 5 · пора менять 1', urgent: true });
  });
  it('только скоро менять — не срочно', () => {
    expect(ragsLine({ active: 4, soon: 2, due: 0 })).toEqual({ text: 'в работе 4 · скоро менять 2', urgent: false });
  });
});

describe('ресурс стирок в форме', () => {
  it('пусто — учёт выключен (null)', () => {
    expect(parseWearLimit('')).toEqual({ ok: true, value: null });
    expect(parseWearLimit('  ')).toEqual({ ok: true, value: null });
  });
  it('1 и 1000 можно, 0 и 1001 — нельзя', () => {
    expect(parseWearLimit('1')).toEqual({ ok: true, value: 1 });
    expect(parseWearLimit('1000')).toEqual({ ok: true, value: 1000 });
    expect(parseWearLimit('0').ok).toBe(false);
    expect(parseWearLimit('1001').ok).toBe(false);
  });
  it('не число — ошибка', () => {
    for (const bad of ['abc', '1.5', '-3', '10 стирок', '12345']) expect(parseWearLimit(bad).ok, bad).toBe(false);
  });
  it('пробел-разряд допустим', () => {
    expect(parseWearLimit('1 00')).toEqual({ ok: true, value: 100 });
  });

  const fields = (f: Partial<ProductFields>): ProductFields => ({
    name: 'Микрофибра', brand: '', category_id: 1, unit: 'pcs', package_size: '1', min_stock: '', barcode: '', note: '', purpose_ids: [], ph: '', incompatible: '', instruction: '', ...f,
  });
  it('позиция в шт: ресурс уходит в запрос, пусто — null', () => {
    const a = validateProduct(fields({ wear_limit: '100' }));
    expect(a.ok && a.value.wear_limit).toBe(100);
    const b = validateProduct(fields({ wear_limit: '' }));
    expect(b.ok && b.value.wear_limit).toBeNull();
  });
  it('ошибка ресурса — под полем', () => {
    const r = validateProduct(fields({ wear_limit: '5000' }));
    expect(!r.ok && r.errors.wear_limit).toBeTruthy();
  });
  it('позиция загружена без поля (старый снимок) — ресурс не отправляется', () => {
    expect(wearInput(undefined)).toBeUndefined();
    expect(wearInput(null)).toBe('');
    expect(wearInput(100)).toBe('100');
    const r = validateProduct(fields({ wear_limit: wearInput(undefined) }));
    expect(r.ok && 'wear_limit' in r.value).toBe(false);
  });
  it('в мл ресурса нет, даже если поле заполнено; без поля формы ключа тоже нет', () => {
    const a = validateProduct(fields({ unit: 'ml', wear_limit: '100' }));
    expect(a.ok && 'wear_limit' in a.value).toBe(false);
    const b = validateProduct(fields({}));
    expect(b.ok && 'wear_limit' in b.value).toBe(false);
  });
});

describe('метка тряпки', () => {
  it('до 20 символов, не пустая', () => {
    expect(validateRagLabel('  Красная  ')).toEqual({ ok: true, value: 'Красная' });
    expect(validateRagLabel('')).toMatchObject({ ok: false });
    expect(validateRagLabel('   ')).toMatchObject({ ok: false });
    expect(validateRagLabel('я'.repeat(20)).ok).toBe(true);
    expect(validateRagLabel('я'.repeat(21)).ok).toBe(false);
  });
});

describe('выбор нескольких', () => {
  it('toggleSelected добавляет и убирает', () => {
    expect(toggleSelected([], 3)).toEqual([3]);
    expect(toggleSelected([3, 4], 3)).toEqual([4]);
    expect(toggleSelected([3], 4)).toEqual([3, 4]);
  });
  it('toggleAll: выбрать все или снять', () => {
    const list = [rag({ id: 1 }), rag({ id: 2 })];
    expect(toggleAll([], list)).toEqual([1, 2]);
    expect(toggleAll([1], list)).toEqual([1, 2]);
    expect(toggleAll([1, 2], list)).toEqual([]);
  });
  it('keepActive отбрасывает списанные и пропавшие', () => {
    const list = [rag({ id: 1 }), rag({ id: 2, status: 'retired' })];
    expect(keepActive([1, 2, 9], list)).toEqual([1]);
  });
});

describe('тексты', () => {
  it('кнопки и уведомление', () => {
    expect(washLabel(3)).toBe('Постирал (3)');
    expect(washedText(3)).toBe('Постирано: 3');
    expect(startLabel(7)).toBe('В работу · в запасе 7');
    expect(retireReasonText('worn')).toBe('износилась');
    expect(retireReasonText('lost')).toBe('потеряна');
    expect(retireReasonText('damaged')).toBe('испорчена');
    expect(retireReasonText(null)).toBe('');
  });
  it('mergeRags заменяет только обновлённые', () => {
    const list = [rag({ id: 1, washes: 1 }), rag({ id: 2, washes: 2 })];
    const out = mergeRags(list, [rag({ id: 2, washes: 3 })]);
    expect(out.map((r) => r.washes)).toEqual([1, 3]);
  });
  it('sortRags — по номеру', () => {
    expect(sortRags([rag({ id: 5, number: 3 }), rag({ id: 6, number: 1 })]).map((r) => r.number)).toEqual([1, 3]);
  });
});
