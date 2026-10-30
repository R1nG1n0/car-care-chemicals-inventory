// Боковое меню на компьютере (от 1024 px): разделы со счётчиками («мало»,
// к покупке, неотправленное), «Добавить», затем разделы «Ещё» группами (на
// компьютере пункта «Ещё» нет), внизу — профиль, состояние связи и версия. Данные для счётчиков запрашиваются только на широком экране: на
// телефоне меню скрыто, лишних запросов нет.
import {
  IconBox,
  IconBuildingStore,
  IconChartBar,
  IconClipboardCheck,
  IconCloud,
  IconCloudUpload,
  IconDatabase,
  IconPackage,
  IconPlus,
  IconReceipt,
  IconShoppingCart,
  IconStack2,
  IconTags,
  IconTargetArrow,
  IconUserCircle,
  type TablerIcon,
} from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useSyncExternalStore } from 'react';
import { Link, NavLink, useLocation } from 'react-router';
import { api, inventory } from '../api';
import { Ico, Logo } from '../components/ui';
import { countText, lowCount, toBuyCount, versionText } from '../lib/sideNav';
import { keys } from './queries';
import { useOnline, useSession } from './session';
import { usePendingOps } from './useOffline';
import type { SectionKey } from './Shell';

const WIDE = '(min-width: 1024px)';

type Item = { to: string; icon: TablerIcon; title: string };

/** Разделы «Ещё» группами — на компьютере они видны сразу, без лишнего перехода. */
const GROUPS: { title: string; items: Item[] }[] = [
  {
    title: 'Учёт',
    items: [
      { to: '/more/inventory', icon: IconClipboardCheck, title: 'Инвентаризация' },
      { to: '/more/reports', icon: IconChartBar, title: 'Отчёты' },
      { to: '/purchases', icon: IconReceipt, title: 'История покупок' },
      { to: '/weather', icon: IconCloud, title: 'Погода' },
    ],
  },
  {
    title: 'Справочники',
    items: [
      { to: '/more/places', icon: IconBox, title: 'Места' },
      { to: '/more/categories', icon: IconTags, title: 'Категории' },
      { to: '/more/purposes', icon: IconTargetArrow, title: 'Назначения' },
      { to: '/more/stores', icon: IconBuildingStore, title: 'Магазины' },
    ],
  },
  { title: 'Данные', items: [{ to: '/more/data', icon: IconDatabase, title: 'Экспорт и импорт' }] },
];

/** Подпункт открыт: сам адрес или вложенный экран (/more/inventory/3). /purchases/new — это «Купить». */
function isOpen(pathname: string, to: string): boolean {
  if (to === '/purchases') return pathname === '/purchases' || /^\/purchases\/\d+/.test(pathname);
  return pathname === to || pathname.startsWith(`${to}/`);
}

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
  const { pathname } = useLocation();
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
      <NavLink to="/shopping" aria-current={isOpen(pathname, '/purchases') ? undefined : current('shopping')}>
        <Ico icon={IconShoppingCart} />
        Купить
        <Count n={toBuy} label="к покупке" />
      </NavLink>
      <NavLink to="/templates" aria-current={current('templates')}>
        <Ico icon={IconStack2} />
        Шаблоны
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

      {GROUPS.map((g) => (
        <div key={g.title} className="app-side__group" role="group" aria-label={g.title}>
          <span className="app-side__label" aria-hidden="true">
            {g.title}
          </span>
          {g.items.map((it) => (
            <Link key={it.to} to={it.to} className="app-side__sub" aria-current={isOpen(pathname, it.to) ? 'page' : undefined}>
              <Ico icon={it.icon} />
              {it.title}
            </Link>
          ))}
        </div>
      ))}

      <div className="app-side__foot">
        <span className={`app-side__net${online ? '' : ' is-off'}`} role="status">
          <span className="app-side__dot" aria-hidden="true" />
          {online ? 'В сети' : 'Нет связи'}
        </span>
        {me && (
          <Link to="/profile" className="app-side__me" aria-current={pathname === '/profile' ? 'page' : undefined}>
            <Ico icon={IconUserCircle} />
            <span className="app-side__login">{me.user.login}</span>
          </Link>
        )}
        {health.data && <span className="app-side__ver">{versionText(health.data.version)}</span>}
      </div>
    </nav>
  );
}
