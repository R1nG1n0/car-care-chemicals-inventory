// Каркас приложения: нижнее меню на телефоне (.g-tabbar), боковое на ширине
// от 1024 px (.g-side), смена раздела с анимацией .g-view (motion.css):
// новый экран въезжает по порядку разделов, старый гаснет поверх.
import { IconDots, IconPackage, IconPlus, IconShoppingCart, IconStack2, IconUser } from '@tabler/icons-react';
import { useEffect, useState, type ReactNode } from 'react';
import { NavLink, useLocation } from 'react-router';
import { Ico, Logo } from '../components/ui';
import { ProfileScreen } from '../screens/profile/ProfileScreen';
import { StubScreen } from '../screens/StubScreen';

export type SectionKey = 'stock' | 'shopping' | 'templates' | 'profile';

/** Порядок задаёт направление перехода. */
const ORDER: SectionKey[] = ['stock', 'shopping', 'templates', 'profile'];

export function sectionOf(pathname: string): SectionKey {
  if (pathname.startsWith('/shopping')) return 'shopping';
  if (pathname.startsWith('/templates')) return 'templates';
  if (pathname.startsWith('/profile') || pathname.startsWith('/more')) return 'profile';
  return 'stock';
}

function screen(key: SectionKey): ReactNode {
  switch (key) {
    case 'stock':
      return <StubScreen id="stock" title="Остатки" icon={IconPackage} empty="Здесь будут остатки" text="Экраны остатков появятся на этапе 3. Тогда же — покупки и расход." />;
    case 'shopping':
      return <StubScreen id="shopping" title="Купить" icon={IconShoppingCart} empty="Список покупок" text="Позиции ниже минимального остатка будут появляться здесь сами." />;
    case 'templates':
      return <StubScreen id="templates" title="Шаблоны" icon={IconStack2} empty="Шаблоны расхода" text="Наборы «позиция — количество» для частых работ: списать всё одним нажатием." />;
    case 'profile':
      return <ProfileScreen />;
  }
}

type View = { key: SectionKey; id: number; dir?: 'forward' | 'back'; leaving: boolean };

const LEAVE_MS = 200; // --dur-fast (160 мс) + запас

/** Нажатие на «+»: до этапа 3 — только подсказка. */
function PlusHint({ shown }: { shown: boolean }) {
  return (
    <div className="app-toast-slot" role="status">
      {shown && <div className="g-toast app-toast--plain">Покупка и расход появятся на этапе 3</div>}
    </div>
  );
}

export function Shell() {
  const { pathname } = useLocation();
  const section = sectionOf(pathname);
  const [views, setViews] = useState<View[]>(() => [{ key: section, id: 0, leaving: false }]);
  const [shown, setShown] = useState(section);
  const [hint, setHint] = useState(0);

  // Смена раздела — во время рендера («подстройка состояния под пропсы»),
  // чтобы новый экран появился в том же кадре, что и новый адрес.
  if (section !== shown) {
    const dir = ORDER.indexOf(section) > ORDER.indexOf(shown) ? 'forward' : 'back';
    setShown(section);
    setViews((vs) => {
      const cur = vs.filter((v) => !v.leaving).map((v) => ({ ...v, leaving: true }));
      const id = Math.max(...vs.map((v) => v.id)) + 1;
      return [...cur, { key: section, id, dir, leaving: false }];
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

  useEffect(() => {
    if (!hint) return;
    const t = setTimeout(() => setHint(0), 3000);
    return () => clearTimeout(t);
  }, [hint]);

  const plus = () => setHint((n) => n + 1);
  const current = (key: SectionKey) => (section === key ? 'page' : undefined);

  return (
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
        <NavLink to="/profile" aria-current={current('profile')}>
          <Ico icon={IconUser} />
          Профиль
        </NavLink>
        <button type="button" className="g-btn g-press app-side__plus" aria-disabled="true" aria-describedby="plus-hint" onClick={plus}>
          <Ico icon={IconPlus} /> Покупка
        </button>
        <span className="g-field__hint" id="plus-hint">
          Покупка и расход — на этапе 3
        </span>
      </nav>

      <main className="app-main">
        <div className="app-views">
          {views.map((v) => (
            <div key={v.id} className={`g-view${v.leaving ? ' is-leaving' : ''}`} data-dir={v.dir} inert={v.leaving || undefined}>
              {screen(v.key)}
            </div>
          ))}
        </div>
      </main>

      <PlusHint shown={hint > 0} />

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
        <button type="button" className="app-tabbar__btn" aria-label="Добавить покупку или расход — появится на этапе 3" aria-disabled="true" onClick={plus}>
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
        <NavLink to="/profile" aria-current={current('profile')}>
          <span className="g-tabbar__ico">
            <Ico icon={IconDots} />
          </span>
          Ещё
        </NavLink>
      </nav>
    </div>
  );
}
