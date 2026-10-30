// Каркас приложения: нижнее меню на телефоне (.g-tabbar), боковое на ширине
// от 1024 px (.g-side, app/SideNav.tsx — со счётчиками и профилем внизу), смена раздела с анимацией .g-view (motion.css):
// новый экран въезжает по порядку разделов, старый гаснет поверх.
//
// Раздел рисует свой компонент с вложенным <Routes>. Уходящему экрану
// передаётся «замороженный» адрес, чтобы он не перерисовался под новый.
import {
  IconCirclePlus,
  IconDots,
  IconDroplet,
  IconPackage,
  IconPlus,
  IconShoppingCart,
  IconStack2,
  IconX,
} from '@tabler/icons-react';
import { useEffect, useState, type ReactNode } from 'react';
import { Link, NavLink, useLocation, useNavigate, type Location } from 'react-router';
import { Sheet } from '../components/Sheet';
import { Ico } from '../components/ui';
import { MoreSection } from '../screens/more/MoreSection';
import { ShoppingSection } from '../screens/shopping/ShoppingSection';
import { StockSection } from '../screens/stock/StockSection';
import { TemplatesSection } from '../screens/templates/TemplatesSection';
import { ErrorBoundary } from './ErrorBoundary';
import { OfflineSync } from './OfflineSync';
import { SideNav } from './SideNav';
import { useNeedsOnline } from './useOffline';
import { ToastProvider } from './toast';

export type SectionKey = 'stock' | 'shopping' | 'templates' | 'more';

/** Порядок задаёт направление перехода. */
const ORDER: SectionKey[] = ['stock', 'shopping', 'templates', 'more'];

/**
 * Адреса разделов:
 * - stock — «/», «/items/…» (позиции), «/?consume=1» (расход из меню «+»);
 * - shopping — «/shopping», «/purchases/…» (новая покупка, история);
 * - templates — «/templates/…»;
 * - more — «/more/…» (справочники), «/profile», «/weather».
 */
export function sectionOf(pathname: string): SectionKey {
  if (pathname.startsWith('/shopping') || pathname.startsWith('/purchases')) return 'shopping';
  if (pathname.startsWith('/templates')) return 'templates';
  if (pathname.startsWith('/more') || pathname.startsWith('/profile') || pathname.startsWith('/weather')) return 'more';
  return 'stock';
}

function screen(key: SectionKey, location: Location): ReactNode {
  switch (key) {
    case 'stock':
      return <StockSection location={location} />;
    case 'shopping':
      return <ShoppingSection location={location} />;
    case 'templates':
      return <TemplatesSection location={location} />;
    case 'more':
      return <MoreSection location={location} />;
  }
}

type View = { key: SectionKey; id: number; dir?: 'forward' | 'back'; leaving: boolean; location: Location };

const LEAVE_MS = 200; // --dur-fast (160 мс) + запас

/**
 * Корневые экраны разделов. На вложенных (карточка позиции, формы, применение
 * шаблона) нижнее меню на телефоне скрыто: у них своя горчичная кнопка, а «+»
 * в меню тоже горчичный — правило «одна горчичная на экран». Назад — кнопкой экрана.
 */
const ROOTS = new Set(['/', '/shopping', '/templates', '/more', '/purchases']);

/**
 * Меню «+»: покупка, расход, новая позиция. Общая шторка (components/Sheet):
 * фокус внутрь и обратно, Tab не уходит наружу, Esc, смахивание, анимация ухода.
 */
function PlusMenu({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const online = useNeedsOnline();
  const go = (to: string) => {
    onClose();
    void navigate(to);
  };
  return (
    <Sheet open={open} onClose={onClose} label="Добавить">
      <div className="app-stack">
        <button type="button" className="g-btn g-btn--block g-press" {...online.props} onClick={online.guard(() => go('/purchases/new'))}>
          <Ico icon={IconShoppingCart} /> Покупка
        </button>
        <button type="button" className="g-btn g-btn--block g-press" onClick={() => go('/?consume=1')}>
          <Ico icon={IconDroplet} /> Расход
        </button>
        <button type="button" className="g-btn g-btn--block g-press" {...online.props} onClick={online.guard(() => go('/items/new'))}>
          <Ico icon={IconCirclePlus} /> Новая позиция
        </button>
        {online.hint && <p className="g-field__hint">{online.hint}: покупки и новые позиции доступны только онлайн. Расход можно записать.</p>}
        <button type="button" className="g-btn g-btn--ghost g-btn--block" onClick={onClose}>
          <Ico icon={IconX} /> Закрыть
        </button>
      </div>
    </Sheet>
  );
}

export function Shell() {
  const location = useLocation();
  const section = sectionOf(location.pathname);
  const [views, setViews] = useState<View[]>(() => [{ key: section, id: 0, leaving: false, location }]);
  const [shown, setShown] = useState(section);
  const [plus, setPlus] = useState(false);

  // Смена раздела — во время рендера («подстройка состояния под пропсы»),
  // чтобы новый экран появился в том же кадре, что и новый адрес.
  if (section !== shown) {
    const dir = ORDER.indexOf(section) > ORDER.indexOf(shown) ? 'forward' : 'back';
    setShown(section);
    setViews((vs) => {
      const cur = vs.filter((v) => !v.leaving).map((v) => ({ ...v, leaving: true }));
      const id = Math.max(...vs.map((v) => v.id)) + 1;
      return [...cur, { key: section, id, dir, leaving: false, location }];
    });
  }

  const hasLeaving = views.some((v) => v.leaving);
  useEffect(() => {
    if (!hasLeaving) return;
    const t = setTimeout(() => setViews((vs) => vs.filter((v) => !v.leaving)), LEAVE_MS);
    return () => clearTimeout(t);
  }, [hasLeaving, views]);

  useEffect(() => {
    window.scrollTo(0, 0);
  }, [section]);

  const current = (key: SectionKey) => (section === key ? 'page' : undefined);

  return (
    <ToastProvider>
      <OfflineSync />
      <div className="app">
        <SideNav section={section} onAdd={() => setPlus(true)} />

        <main className="app-main">
          <div className="app-views">
            {views.map((v) => (
              <div key={v.id} className={`g-view${v.leaving ? ' is-leaving' : ''}`} data-dir={v.dir} inert={v.leaving || undefined}>
                <ErrorBoundary resetKey={(v.leaving ? v.location : location).pathname}>{screen(v.key, v.leaving ? v.location : location)}</ErrorBoundary>
              </div>
            ))}
          </div>
        </main>

        <nav className="g-tabbar app-tabbar" aria-label="Разделы" hidden={!ROOTS.has(location.pathname)}>
          <Link to="/" aria-current={current('stock')}>
            <span className="g-tabbar__ico">
              <Ico icon={IconPackage} />
            </span>
            Остатки
          </Link>
          <NavLink to="/shopping" aria-current={current('shopping')}>
            <span className="g-tabbar__ico">
              <Ico icon={IconShoppingCart} />
            </span>
            Купить
          </NavLink>
          <button type="button" className="app-tabbar__btn" aria-label="Добавить: покупка, расход, позиция" onClick={() => setPlus(true)}>
            <span className="g-tabbar__plus">
              <Ico icon={IconPlus} size={26} />
            </span>
          </button>
          <NavLink to="/templates" aria-current={current('templates')}>
            <span className="g-tabbar__ico">
              <Ico icon={IconStack2} />
            </span>
            Шаблоны
          </NavLink>
          <NavLink to="/more" aria-current={current('more')}>
            <span className="g-tabbar__ico">
              <Ico icon={IconDots} />
            </span>
            Ещё
          </NavLink>
        </nav>

        <PlusMenu open={plus} onClose={() => setPlus(false)} />
      </div>
    </ToastProvider>
  );
}
