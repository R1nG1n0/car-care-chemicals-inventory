// Каркас приложения: нижнее меню на телефоне (.g-tabbar), боковое на ширине
// от 1024 px (.g-side), смена раздела с анимацией .g-view (motion.css):
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
import { NavLink, useLocation, useNavigate, type Location } from 'react-router';
import { Ico, Logo } from '../components/ui';
import { MoreSection } from '../screens/more/MoreSection';
import { ShoppingSection } from '../screens/shopping/ShoppingSection';
import { StockSection } from '../screens/stock/StockSection';
import { TemplatesSection } from '../screens/templates/TemplatesSection';
import { ToastProvider } from './toast';

export type SectionKey = 'stock' | 'shopping' | 'templates' | 'more';

/** Порядок задаёт направление перехода. */
const ORDER: SectionKey[] = ['stock', 'shopping', 'templates', 'more'];

/**
 * Адреса разделов:
 * - stock — «/», «/items/…» (позиции), «/?consume=1» (расход из меню «+»);
 * - shopping — «/shopping», «/purchases/…» (новая покупка, история);
 * - templates — «/templates/…»;
 * - more — «/more/…» (справочники), «/profile».
 */
export function sectionOf(pathname: string): SectionKey {
  if (pathname.startsWith('/shopping') || pathname.startsWith('/purchases')) return 'shopping';
  if (pathname.startsWith('/templates')) return 'templates';
  if (pathname.startsWith('/more') || pathname.startsWith('/profile')) return 'more';
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

/** Меню «+»: покупка, расход, новая позиция. Шторка снизу (.g-sheet). */
function PlusMenu({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);
  if (!open) return null;
  const go = (to: string) => {
    onClose();
    navigate(to);
  };
  return (
    <div className="app-overlay" onClick={onClose}>
      <section className="g-sheet app-plus" role="dialog" aria-modal="true" aria-label="Добавить" onClick={(e) => e.stopPropagation()}>
        <div className="g-sheet__grab" />
        <div className="app-stack">
          <button type="button" className="g-btn g-btn--block g-press" onClick={() => go('/purchases/new')}>
            <Ico icon={IconShoppingCart} /> Покупка
          </button>
          <button type="button" className="g-btn g-btn--block g-press" onClick={() => go('/?consume=1')}>
            <Ico icon={IconDroplet} /> Расход
          </button>
          <button type="button" className="g-btn g-btn--block g-press" onClick={() => go('/items/new')}>
            <Ico icon={IconCirclePlus} /> Новая позиция
          </button>
          <button type="button" className="g-btn g-btn--ghost g-btn--block" onClick={onClose}>
            <Ico icon={IconX} /> Закрыть
          </button>
        </div>
      </section>
    </div>
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
      <div className="app">
        <nav className="g-side app-side" aria-label="Разделы">
          <div className="app-side__logo">
            <Logo />
          </div>
          <NavLink to="/" end aria-current={current('stock')}>
            <Ico icon={IconPackage} />
            Остатки
          </NavLink>
          <NavLink to="/shopping" aria-current={current('shopping')}>
            <Ico icon={IconShoppingCart} />
            Купить
          </NavLink>
          <NavLink to="/templates" aria-current={current('templates')}>
            <Ico icon={IconStack2} />
            Шаблоны
          </NavLink>
          <NavLink to="/more" aria-current={current('more')}>
            <Ico icon={IconDots} />
            Ещё
          </NavLink>
          <button type="button" className="g-btn g-press app-side__plus" onClick={() => setPlus(true)}>
            <Ico icon={IconPlus} /> Добавить
          </button>
        </nav>

        <main className="app-main">
          <div className="app-views">
            {views.map((v) => (
              <div key={v.id} className={`g-view${v.leaving ? ' is-leaving' : ''}`} data-dir={v.dir} inert={v.leaving || undefined}>
                {screen(v.key, v.leaving ? v.location : location)}
              </div>
            ))}
          </div>
        </main>

        <nav className="g-tabbar app-tabbar" aria-label="Разделы">
          <NavLink to="/" end aria-current={current('stock')}>
            <span className="g-tabbar__ico">
              <Ico icon={IconPackage} />
            </span>
            Остатки
          </NavLink>
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
