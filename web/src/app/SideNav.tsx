// Боковое меню на компьютере (от 1024 px): разделы со счётчиками («мало»,
// к покупке, неотправленное), «Добавить», внизу — профиль, состояние связи и
// версия. Данные для счётчиков запрашиваются только на широком экране: на
// телефоне меню скрыто, лишних запросов нет.
import { IconCloudUpload, IconDots, IconPackage, IconPlus, IconShoppingCart, IconStack2, IconUserCircle } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useSyncExternalStore } from 'react';
import { Link, NavLink } from 'react-router';
import { api, inventory } from '../api';
import { Ico, Logo } from '../components/ui';
import { countText, lowCount, toBuyCount, versionText } from '../lib/sideNav';
import { keys } from './queries';
import { useOnline, useSession } from './session';
import { usePendingOps } from './useOffline';
import type { SectionKey } from './Shell';

const WIDE = '(min-width: 1024px)';

/** Широкий экран: меню видно. Без matchMedia (тесты, старые браузеры) — нет. */
function useWide(): boolean {
  return useSyncExternalStore(
    (cb) => {
      if (typeof window === 'undefined' || !window.matchMedia) return () => undefined;
      const mq = window.matchMedia(WIDE);
      mq.addEventListener('change', cb);
      return () => mq.removeEventListener('change', cb);
    },
    () => typeof window !== 'undefined' && !!window.matchMedia && window.matchMedia(WIDE).matches,
    () => false,
  );
}

/** Счётчик справа от пункта; danger — красный («мало»). Для чтения с экрана — подпись. */
function Count({ n, label, danger = false }: { n: number; label: string; danger?: boolean }) {
  const text = countText(n);
  if (!text) return null;
  return (
    <span className={`app-side__count${danger ? ' app-side__count--danger' : ''}`} aria-label={`${n} ${label}`}>
      {text}
    </span>
  );
}

export function SideNav({ section, onAdd }: { section: SectionKey; onAdd: () => void }) {
  const wide = useWide();
  const online = useOnline();
  const me = useSession().data;
  const pending = usePendingOps().length;
  // Тот же ключ, что у главной: кэш общий, отдельного запроса нет
  const products = useQuery({ queryKey: keys.products(), queryFn: ({ signal }) => inventory.products({}, signal), enabled: wide });
  const shopping = useQuery({ queryKey: keys.shopping, queryFn: ({ signal }) => inventory.shopping(signal), enabled: wide });
  const health = useQuery({ queryKey: ['health'], queryFn: ({ signal }) => api.health(signal), enabled: wide, staleTime: Infinity, retry: false });

  const current = (key: SectionKey) => (section === key ? 'page' : undefined);
  const low = wide ? lowCount(products.data) : 0;
  const toBuy = wide ? toBuyCount(shopping.data) : 0;

  return (
    <nav className="g-side app-side" aria-label="Разделы">
      <div className="app-side__logo">
        <Logo />
      </div>
      <Link to="/" aria-current={current('stock')}>
        <Ico icon={IconPackage} />
        Остатки
        <Count n={low} label="ниже минимума" danger />
      </Link>
      <NavLink to="/shopping" aria-current={current('shopping')}>
        <Ico icon={IconShoppingCart} />
        Купить
        <Count n={toBuy} label="к покупке" />
      </NavLink>
      <NavLink to="/templates" aria-current={current('templates')}>
        <Ico icon={IconStack2} />
        Шаблоны
      </NavLink>
      <NavLink to="/more" aria-current={current('more')}>
        <Ico icon={IconDots} />
        Ещё
      </NavLink>
      {pending > 0 && (
        <NavLink to="/more/pending" className="app-side__pending">
          <Ico icon={IconCloudUpload} />
          Неотправленное
          <Count n={pending} label="не отправлено" danger />
        </NavLink>
      )}
      <button type="button" className="g-btn g-press app-side__plus" onClick={onAdd}>
        <Ico icon={IconPlus} /> Добавить
      </button>

      <div className="app-side__foot">
        <span className={`app-side__net${online ? '' : ' is-off'}`} role="status">
          <span className="app-side__dot" aria-hidden="true" />
          {online ? 'В сети' : 'Нет связи'}
        </span>
        {me && (
          <Link to="/profile" className="app-side__me">
            <Ico icon={IconUserCircle} />
            <span className="app-side__login">{me.user.login}</span>
          </Link>
        )}
        {health.data && <span className="app-side__ver">{versionText(health.data.version)}</span>}
      </div>
    </nav>
  );
}
