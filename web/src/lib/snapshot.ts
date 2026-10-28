// Последние данные на телефоне (ФТ-12.2): после каждой успешной загрузки
// сессия, список позиций, карточки, шаблоны, категории, назначения и места сохраняются
// в localStorage; без связи экраны показывают их. Ключи содержат id пользователя,
// данные одного пользователя другому не показываются.
import type { Session } from '../api';
import { browserStorage, type StorageLike } from './offlineQueue';

const ME_KEY = 'garage:session';
const PREFIX = 'garage:snap:';
const SYNC = '__sync';
/** Карточек позиций на телефоне не больше: localStorage мал (около 5 МБ). */
export const MAX_PRODUCT_CARDS = 80;

export type SnapEntry = { key: readonly unknown[]; at: number; data: unknown };

/** Какие запросы сохраняются: общий список позиций (без поиска), карточки, шаблоны, справочники. */
export function isPersistableKey(key: readonly unknown[]): boolean {
  switch (key[0]) {
    case 'products':
      return JSON.stringify(key[1] ?? {}) === '{}';
    case 'product':
    case 'recipes':
    case 'recipe':
    case 'categories':
    case 'purposes':
    case 'locations':
      return true;
    default:
      return false;
  }
}

export class SnapshotStore {
  constructor(private readonly storage: StorageLike | null = browserStorage()) {}

  private prefix(uid: number): string {
    return `${PREFIX}${uid}:`;
  }

  private set(key: string, value: string): boolean {
    try {
      this.storage?.setItem(key, value);
      return this.storage !== null;
    } catch {
      return false;
    }
  }

  private get(key: string): string | null {
    try {
      return this.storage?.getItem(key) ?? null;
    } catch {
      return null;
    }
  }

  private keysWith(prefix: string): string[] {
    const s = this.storage;
    if (!s || s.length === undefined || !s.key) return [];
    const out: string[] = [];
    try {
      for (let i = 0; i < s.length; i++) {
        const k = s.key(i);
        if (k?.startsWith(prefix)) out.push(k);
      }
    } catch {
      return out;
    }
    return out;
  }

  // ----- сессия -----

  saveMe(session: Session, at: number): void {
    this.set(ME_KEY, JSON.stringify({ at, session }));
  }

  loadMe(): { session: Session; at: number } | null {
    const raw = this.get(ME_KEY);
    if (!raw) return null;
    try {
      const d = JSON.parse(raw) as { at?: unknown; session?: Session };
      const u = d.session?.user;
      if (typeof d.at !== 'number' || !u || typeof u.id !== 'number' || !d.session?.settings) return null;
      return { session: d.session, at: d.at };
    } catch {
      return null;
    }
  }

  clearMe(): void {
    try {
      this.storage?.removeItem(ME_KEY);
    } catch {
      // нечего удалять
    }
  }

  // ----- данные -----

  save(uid: number, key: readonly unknown[], data: unknown, at: number): void {
    const k = this.prefix(uid) + JSON.stringify(key);
    const value = JSON.stringify({ at, data });
    if (!this.set(k, value)) {
      // Места не хватило: убираем половину карточек позиций и пробуем ещё раз
      this.prune(uid, Math.floor(MAX_PRODUCT_CARDS / 2));
      if (!this.set(k, value)) return;
    }
    this.set(this.prefix(uid) + SYNC, String(at));
    if (key[0] === 'product') this.prune(uid, MAX_PRODUCT_CARDS);
  }

  /** Оставляет `keep` самых свежих карточек позиций. */
  prune(uid: number, keep: number): void {
    const cards = this.keysWith(`${this.prefix(uid)}["product",`)
      .map((k) => ({ k, at: this.entryAt(k) }))
      .sort((a, b) => b.at - a.at);
    for (const c of cards.slice(keep)) this.remove(c.k);
  }

  private entryAt(k: string): number {
    try {
      return (JSON.parse(this.get(k) ?? '{}') as { at?: number }).at ?? 0;
    } catch {
      return 0;
    }
  }

  private remove(k: string): void {
    try {
      this.storage?.removeItem(k);
    } catch {
      // нечего удалять
    }
  }

  loadAll(uid: number): SnapEntry[] {
    const p = this.prefix(uid);
    const out: SnapEntry[] = [];
    for (const k of this.keysWith(p)) {
      const name = k.slice(p.length);
      if (name === SYNC) continue;
      try {
        const key = JSON.parse(name) as unknown;
        const v = JSON.parse(this.get(k) ?? 'null') as { at?: unknown; data?: unknown } | null;
        if (Array.isArray(key) && v && typeof v.at === 'number' && 'data' in v) out.push({ key, at: v.at, data: v.data });
      } catch {
        // повреждённая запись пропускается
      }
    }
    return out;
  }

  /** Когда данные в последний раз успешно загружались, мс; нет данных — null. */
  lastSync(uid: number): number | null {
    const n = Number(this.get(this.prefix(uid) + SYNC));
    return Number.isFinite(n) && n > 0 ? n : null;
  }

  /** Выход: данные пользователя стираются. */
  forget(uid: number): void {
    for (const k of this.keysWith(this.prefix(uid))) this.remove(k);
    const me = this.loadMe();
    if (me?.session.user.id === uid) this.clearMe();
  }
}
