import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import type { Location } from '../api';
import {
  checkedLabel,
  clearState,
  goneMarks,
  mergeOrder,
  isStale,
  loadState,
  newState,
  parseState,
  progressOf,
  saveState,
  serializeState,
  shortDate,
  sortLocations,
  storageKey,
  summarize,
  summaryLines,
  withMark,
  withoutMark,
  type Mark,
} from './inventory';

const loc = (id: number, sort: number, checked_at: number | null, archived = false): Location => ({
  id,
  name: `Место ${id}`,
  sort,
  archived,
  units_count: 1,
  checked_at,
});
const mark = (verdict: Mark['verdict'], name = 'Шампунь'): Mark => ({ verdict, name, detail: '' });

describe('sortLocations', () => {
  it('никогда не проверявшиеся сверху, затем самые давние; архивные скрыты', () => {
    const list = [loc(1, 0, 3000), loc(2, 1, null), loc(3, 2, 1000), loc(4, 3, null, true), loc(5, 4, null)];
    expect(sortLocations(list).map((l) => l.id)).toEqual([2, 5, 3, 1]);
  });
  it('при равенстве — по порядку справочника и не меняет исходный массив', () => {
    const list = [loc(2, 5, 100), loc(1, 5, 100), loc(3, 0, 100)];
    expect(sortLocations(list).map((l) => l.id)).toEqual([3, 1, 2]);
    expect(list.map((l) => l.id)).toEqual([2, 1, 3]);
  });
});

describe('подписи', () => {
  it('«не проверялось» и «проверено ДД.ММ»', () => {
    expect(checkedLabel(null)).toBe('не проверялось');
    const t = Math.floor(new Date(2026, 9, 3, 12).getTime() / 1000);
    expect(shortDate(t)).toBe('03.10');
    expect(checkedLabel(t)).toBe('проверено 03.10');
  });
  it('давно — больше 30 дней или никогда', () => {
    const now = 100 * 86_400;
    expect(isStale(null, now)).toBe(true);
    expect(isStale(now - 31 * 86_400, now)).toBe(true);
    expect(isStale(now - 30 * 86_400, now)).toBe(false);
  });
});

describe('progressOf', () => {
  it('считает отметки по текущим экземплярам', () => {
    let s = newState(1, 0);
    s = withMark(s, 10, mark('ok'));
    expect(progressOf([10, 11, 12], s.marks)).toEqual({ done: 1, total: 3, level: 1 / 3 });
  });
  it('ушедший из места экземпляр с отметкой остаётся в M и N', () => {
    const marks = { '10': mark('moved'), '11': mark('ok') };
    expect(progressOf([11, 12], marks)).toEqual({ done: 2, total: 3, level: 2 / 3 });
    expect(goneMarks([11, 12], marks).map((g) => g.id)).toEqual([10]);
  });
  it('пустое место — 0 из 0 без деления на ноль', () => {
    expect(progressOf([], {})).toEqual({ done: 0, total: 0, level: 0 });
  });
});

describe('mergeOrder', () => {
  it('сохраняет прежний порядок, новые — в конец', () => {
    expect(mergeOrder([3, 1, 2], [1, 2, 3, 4], {})).toEqual([3, 1, 2, 4]);
  });
  it('ушедший без отметки пропадает, с отметкой остаётся на месте', () => {
    expect(mergeOrder([3, 1, 2], [1, 3], { '2': mark('moved') })).toEqual([3, 1, 2]);
    expect(mergeOrder([3, 1, 2], [1, 3], {})).toEqual([3, 1]);
  });
  it('после перезагрузки отметки ушедших идут в конец по id', () => {
    expect(mergeOrder([], [5, 6], { '9': mark('missing'), '4': mark('moved'), '5': mark('ok') })).toEqual([5, 6, 4, 9]);
  });
});

describe('summarize', () => {
  it('считает исходы и непроверенные', () => {
    const marks = { '1': mark('ok'), '2': mark('ok'), '3': mark('fixed'), '4': mark('missing'), '5': mark('moved') };
    const s = summarize([1, 2, 3, 6, 7], marks);
    expect(s).toEqual({ ok: 2, fixed: 1, missing: 1, moved: 1, unchecked: 2 });
    expect(summaryLines(s).map((l) => [l.label, l.count])).toEqual([
      ['Подтверждено', 2],
      ['Исправлено', 1],
      ['Не найдено', 1],
      ['Перемещено', 1],
    ]);
  });
  it('нулевые строки не показываются', () => {
    expect(summaryLines({ ok: 0, fixed: 0, missing: 0, moved: 0, unchecked: 3 })).toEqual([]);
  });
});

describe('withMark / withoutMark', () => {
  it('не мутируют состояние и заменяют отметку', () => {
    const s0 = newState(1, 5);
    const s1 = withMark(s0, 7, mark('ok'));
    const s2 = withMark(s1, 7, mark('fixed'));
    expect(s0.marks).toEqual({});
    expect(s2.marks['7']?.verdict).toBe('fixed');
    expect(withoutMark(s2, 7).marks).toEqual({});
    expect(s2.marks['7']).toBeDefined();
  });
});

describe('сериализация', () => {
  it('туда и обратно', () => {
    const s = withMark(newState(4, 123), 9, { verdict: 'moved', name: 'Воск', detail: 'в Коробка 3' });
    expect(parseState(serializeState(s), 4)).toEqual(s);
  });
  it('чужое место, битый JSON, неверная форма — null', () => {
    const raw = serializeState(newState(4, 1));
    expect(parseState(raw, 5)).toBeNull();
    expect(parseState(null, 4)).toBeNull();
    expect(parseState('{', 4)).toBeNull();
    expect(parseState('[]', 4)).toBeNull();
    expect(parseState(JSON.stringify({ v: 2, locationId: 4, startedAt: 1, marks: {} }), 4)).toBeNull();
    expect(parseState(JSON.stringify({ v: 1, locationId: 4, startedAt: 1, marks: { x: mark('ok') } }), 4)).toBeNull();
    expect(parseState(JSON.stringify({ v: 1, locationId: 4, startedAt: 1, marks: { '1': { verdict: 'bad', name: '', detail: '' } } }), 4)).toBeNull();
  });
});

/** Тесты идут в node: подставляем простое хранилище на память. */
function fakeStorage(): Storage {
  const m = new Map<string, string>();
  return {
    get length() {
      return m.size;
    },
    clear: () => m.clear(),
    getItem: (k) => m.get(k) ?? null,
    key: (i) => [...m.keys()][i] ?? null,
    removeItem: (k) => void m.delete(k),
    setItem: (k, v) => void m.set(k, v),
  };
}

describe('localStorage', () => {
  const had = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
  beforeEach(() => Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: fakeStorage() }));
  afterEach(() => {
    if (had) Object.defineProperty(globalThis, 'localStorage', had);
    else Reflect.deleteProperty(globalThis, 'localStorage');
  });

  it('сохраняет, читает и очищает; ключ зависит от пользователя', () => {
    const s = withMark(newState(2, 1), 3, mark('ok'));
    saveState(10, s);
    expect(globalThis.localStorage.getItem(storageKey(10, 2))).not.toBeNull();
    expect(loadState(10, 2)).toEqual(s);
    expect(loadState(11, 2)).toBeNull();
    clearState(10, 2);
    expect(loadState(10, 2)).toBeNull();
  });
  it('состояние без отметок удаляет ключ', () => {
    saveState(1, withMark(newState(2, 1), 3, mark('ok')));
    saveState(1, newState(2, 1));
    expect(loadState(1, 2)).toBeNull();
  });
  it('недоступное хранилище не роняет', () => {
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      get() {
        throw new Error('закрыто');
      },
    });
    expect(loadState(1, 2)).toBeNull();
    expect(() => saveState(1, withMark(newState(2, 1), 3, mark('ok')))).not.toThrow();
    expect(() => clearState(1, 2)).not.toThrow();
  });
});
