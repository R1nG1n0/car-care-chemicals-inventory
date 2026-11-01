import { describe, expect, it } from 'vitest';
import type { Note } from '../api';
import { excerpt, filterNotes, linkParam, newNoteUrl, noteChanged, sortNotes, validateNote } from './notes';
import { isPersistableKey, MAX_NOTE_CARDS, SnapshotStore } from './snapshot';
import { memoryStorage } from './testStorage';

const note = (p: Partial<Note> = {}): Note => ({ id: 1, title: 'Рецепт для салона', body: 'Шампунь 1:10\nМикрофибра', product_ids: [], recipe_ids: [], created_at: 1, updated_at: 1, ...p });

describe('проверка заметки', () => {
  const d = { title: ' Заголовок ', body: 'текст', product_ids: [3, 3, 4], recipe_ids: [] };
  it('заголовок обрезается, привязки без повторов', () => {
    const r = validateNote(d);
    expect(r).toEqual({ ok: true, value: { title: 'Заголовок', body: 'текст', product_ids: [3, 4], recipe_ids: [] } });
  });
  it('заголовок 120 — можно, 121 — нельзя, пустой — нельзя', () => {
    expect(validateNote({ ...d, title: 'я'.repeat(120) }).ok).toBe(true);
    expect(validateNote({ ...d, title: 'я'.repeat(121) }).ok).toBe(false);
    expect(validateNote({ ...d, title: '  ' })).toMatchObject({ ok: false, errors: { title: expect.any(String) } });
  });
  it('текст 10 000 — можно, 10 001 — нельзя, пустой — можно', () => {
    expect(validateNote({ ...d, body: 'я'.repeat(10_000) }).ok).toBe(true);
    expect(validateNote({ ...d, body: 'я'.repeat(10_001) })).toMatchObject({ ok: false, errors: { body: expect.any(String) } });
    expect(validateNote({ ...d, body: '' }).ok).toBe(true);
  });
  it('переносы строк в тексте сохраняются как есть', () => {
    const r = validateNote({ ...d, body: '  строка 1\n\nстрока 3 ' });
    expect(r.ok && r.value.body).toBe('  строка 1\n\nстрока 3 ');
  });
});

describe('фильтр без связи', () => {
  const list = [
    note({ id: 1, title: 'Салон', body: 'Кислотный очиститель', product_ids: [5] }),
    note({ id: 2, title: 'Кузов', body: 'Двухфазная мойка ёлка', recipe_ids: [7], product_ids: [5, 6] }),
    note({ id: 3, title: 'Стекло', body: '' }),
  ];
  it('без условий — всё', () => expect(filterNotes(list)).toHaveLength(3));
  it('поиск по заголовку и тексту без учёта регистра и ё/е', () => {
    expect(filterNotes(list, { q: 'салон' }).map((n) => n.id)).toEqual([1]);
    expect(filterNotes(list, { q: 'КИСЛОТН' }).map((n) => n.id)).toEqual([1]);
    expect(filterNotes(list, { q: 'елка' }).map((n) => n.id)).toEqual([2]);
    expect(filterNotes(list, { q: 'нет такого' })).toEqual([]);
  });
  it('по позиции и по шаблону', () => {
    expect(filterNotes(list, { product_id: 5 }).map((n) => n.id)).toEqual([1, 2]);
    expect(filterNotes(list, { product_id: 6 }).map((n) => n.id)).toEqual([2]);
    expect(filterNotes(list, { recipe_id: 7 }).map((n) => n.id)).toEqual([2]);
    expect(filterNotes(list, { product_id: 5, q: 'кузов' }).map((n) => n.id)).toEqual([2]);
  });
  it('новые и изменённые сверху', () => {
    const s = sortNotes([note({ id: 1, updated_at: 5 }), note({ id: 2, updated_at: 9 }), note({ id: 3, updated_at: 9 })]);
    expect(s.map((n) => n.id)).toEqual([3, 2, 1]);
  });
});

describe('выдержка и привязки', () => {
  it('первые непустые строки', () => {
    expect(excerpt('\n  первая \n\nвторая\nтретья')).toBe('первая\nвторая');
    expect(excerpt('', 2)).toBe('');
    expect(excerpt('а\nб\nв', 3)).toBe('а\nб\nв');
  });
  it('?product= и ?recipe=: только положительное целое', () => {
    expect(linkParam('12')).toEqual([12]);
    for (const bad of [null, '', '0', '-1', 'abc', '1.5', '12abc']) expect(linkParam(bad), String(bad)).toEqual([]);
  });
  it('адрес «Новая заметка»', () => {
    expect(newNoteUrl({ product_id: 4 })).toBe('/more/notes/new?product=4');
    expect(newNoteUrl({ recipe_id: 9 })).toBe('/more/notes/new?recipe=9');
    expect(newNoteUrl({})).toBe('/more/notes/new');
  });
  it('noteChanged: новая пустая — нет, правка текста — да, порядок привязок не важен', () => {
    expect(noteChanged(null, { title: '', body: '', product_ids: [], recipe_ids: [] })).toBe(false);
    expect(noteChanged(null, { title: 'а', body: '', product_ids: [], recipe_ids: [] })).toBe(true);
    const n = note({ product_ids: [1, 2] });
    expect(noteChanged(n, { title: n.title, body: n.body, product_ids: [2, 1], recipe_ids: [] })).toBe(false);
    expect(noteChanged(n, { title: n.title, body: n.body + '!', product_ids: [2, 1], recipe_ids: [] })).toBe(true);
  });
});

describe('сохранение на телефоне', () => {
  it('список без поиска и карточки заметок сохраняются, поиск — нет', () => {
    expect(isPersistableKey(['notes', {}])).toBe(true);
    expect(isPersistableKey(['notes', { q: 'салон' }])).toBe(false);
    expect(isPersistableKey(['note', 3])).toBe(true);
    expect(isPersistableKey(['rags', 3, true])).toBe(false);
  });
});

describe('карточки заметок на телефоне', () => {
  it('сохраняются не больше MAX_NOTE_CARDS свежих', () => {
    const st = new SnapshotStore(memoryStorage());
    for (let i = 1; i <= MAX_NOTE_CARDS + 5; i++) st.save(1, ['note', i], note({ id: i }), i);
    const ids = st.loadAll(1).filter((e) => e.key[0] === 'note').map((e) => e.key[1] as number);
    expect(ids).toHaveLength(MAX_NOTE_CARDS);
    expect(ids).not.toContain(1);
    expect(ids).toContain(MAX_NOTE_CARDS + 5);
  });
  it('список заметок prune не затрагивает', () => {
    const st = new SnapshotStore(memoryStorage());
    st.save(1, ['notes', {}], [], 1);
    for (let i = 1; i <= MAX_NOTE_CARDS + 1; i++) st.save(1, ['note', i], note({ id: i }), i + 1);
    expect(st.loadAll(1).some((e) => e.key[0] === 'notes')).toBe(true);
  });
});
