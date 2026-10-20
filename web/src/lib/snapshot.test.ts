import { beforeEach, describe, expect, it } from 'vitest';
import type { Session } from '../api';
import { memoryStorage } from './testStorage';
import { isPersistableKey, MAX_PRODUCT_CARDS, SnapshotStore } from './snapshot';

const session = (id: number): Session => ({ user: { id, login: `u${id}`, is_owner: false }, settings: { step_small: 10, step_large: 50 } });

describe('снимки данных', () => {
  let st: ReturnType<typeof memoryStorage>;
  let s: SnapshotStore;
  beforeEach(() => {
    st = memoryStorage();
    s = new SnapshotStore(st);
  });

  it('что сохраняется', () => {
    expect(isPersistableKey(['products', {}])).toBe(true);
    expect(isPersistableKey(['products', { q: 'abc' }])).toBe(false);
    expect(isPersistableKey(['product', 3])).toBe(true);
    expect(isPersistableKey(['recipes'])).toBe(true);
    expect(isPersistableKey(['categories', true])).toBe(true);
    expect(isPersistableKey(['locations', false])).toBe(true);
    expect(isPersistableKey(['movements', null])).toBe(false);
    expect(isPersistableKey(['report', 'spending', 'a', 'b'])).toBe(false);
  });

  it('сессия: сохранение, чтение, очистка', () => {
    expect(s.loadMe()).toBeNull();
    s.saveMe(session(1), 123);
    expect(s.loadMe()).toEqual({ session: session(1), at: 123 });
    s.clearMe();
    expect(s.loadMe()).toBeNull();
  });

  it('данные возвращаются с временем снимка', () => {
    s.save(1, ['products', {}], [{ id: 1 }], 1000);
    s.save(1, ['product', 1], { product: { id: 1 } }, 2000);
    const all = s.loadAll(1).sort((a, b) => a.at - b.at);
    expect(all).toEqual([
      { key: ['products', {}], at: 1000, data: [{ id: 1 }] },
      { key: ['product', 1], at: 2000, data: { product: { id: 1 } } },
    ]);
    expect(s.lastSync(1)).toBe(2000);
  });

  it('пользователи не видят данные друг друга', () => {
    s.save(1, ['products', {}], ['мои'], 1000);
    s.save(2, ['products', {}], ['чужие'], 2000);
    expect(s.loadAll(1).map((e) => e.data)).toEqual([['мои']]);
    expect(s.lastSync(3)).toBeNull();
    s.forget(1);
    expect(s.loadAll(1)).toEqual([]);
    expect(s.loadAll(2)).toHaveLength(1);
  });

  it('выход стирает и сессию этого пользователя, но не чужую', () => {
    s.saveMe(session(1), 1);
    s.forget(2);
    expect(s.loadMe()).not.toBeNull();
    s.forget(1);
    expect(s.loadMe()).toBeNull();
  });

  it('карточек позиций не больше предела: остаются самые свежие', () => {
    for (let i = 1; i <= MAX_PRODUCT_CARDS + 5; i++) s.save(1, ['product', i], { i }, i);
    const cards = s.loadAll(1).filter((e) => e.key[0] === 'product');
    expect(cards).toHaveLength(MAX_PRODUCT_CARDS);
    expect(Math.min(...cards.map((c) => c.at))).toBe(6);
  });

  it('повреждённые записи и сессия пропускаются', () => {
    st.setItem('garage:session', '{oops');
    expect(s.loadMe()).toBeNull();
    st.setItem('garage:session', JSON.stringify({ at: 1, session: { user: {} } }));
    expect(s.loadMe()).toBeNull();
    st.setItem('garage:snap:1:["product",1]', 'мусор');
    st.setItem('garage:snap:1:не-ключ', '{"at":1,"data":1}');
    s.save(1, ['product', 2], { ok: true }, 5);
    expect(s.loadAll(1).map((e) => e.key)).toEqual([['product', 2]]);
  });

  it('без хранилища и при ошибках записи ничего не падает', () => {
    const none = new SnapshotStore(null);
    none.save(1, ['product', 1], {}, 1);
    none.saveMe(session(1), 1);
    expect(none.loadAll(1)).toEqual([]);
    expect(none.loadMe()).toBeNull();
    const full = memoryStorage();
    full.setItem = () => {
      throw new Error('quota');
    };
    expect(() => new SnapshotStore(full).save(1, ['product', 1], {}, 1)).not.toThrow();
  });
});
