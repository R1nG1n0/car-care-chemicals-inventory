// Ещё — меню, профиль, места, категории, магазины, история покупок.
// Адреса раздела разбирает вложенный <Routes>.
import { IconBox, IconBuildingStore, IconChevronRight, IconReceipt, IconTags, IconUser, type TablerIcon } from '@tabler/icons-react';
import { Link, Route, Routes, type Location } from 'react-router';
import { ShopBack } from '../../components/ShopUi';
import { Ico } from '../../components/ui';
import { ProfileScreen } from '../profile/ProfileScreen';
import { RefsScreen } from './RefsScreen';
import { StoresScreen } from './StoresScreen';

const MENU: { to: string; icon: TablerIcon; title: string; sub: string }[] = [
  { to: '/profile', icon: IconUser, title: 'Профиль', sub: 'Пароль, шаги кнопок, приглашения, выход' },
  { to: '/more/places', icon: IconBox, title: 'Места', sub: 'Где лежит: полки, коробки, багажник' },
  { to: '/more/categories', icon: IconTags, title: 'Категории', sub: 'Названия и цвета' },
  { to: '/more/stores', icon: IconBuildingStore, title: 'Магазины', sub: 'Где покупаете' },
  { to: '/purchases', icon: IconReceipt, title: 'История покупок', sub: 'Все покупки, новые сверху' },
];

function MoreMenu() {
  return (
    <div className="app-stack">
      <h1 className="g-title">Ещё</h1>
      <nav className="g-list" aria-label="Ещё">
        {MENU.map((m) => (
          <Link key={m.to} to={m.to} className="g-row g-row--link">
            <Ico icon={m.icon} />
            <span className="g-row__main">
              <span className="g-row__title shop-block">{m.title}</span>
              <span className="g-row__sub shop-block">{m.sub}</span>
            </span>
            <span className="g-row__go">
              <Ico icon={IconChevronRight} />
            </span>
          </Link>
        ))}
      </nav>
    </div>
  );
}

/** location — адрес, «замороженный» для уходящего экрана при смене раздела (см. Shell). */
export function MoreSection({ location }: { location: Location }) {
  return (
    <Routes location={location}>
      <Route
        path="/profile"
        element={
          <div className="app-stack">
            <ShopBack to="/more" label="Ещё" />
            <ProfileScreen />
          </div>
        }
      />
      <Route path="/more/places" element={<RefsScreen key="places" kind="places" />} />
      <Route path="/more/categories" element={<RefsScreen key="categories" kind="categories" />} />
      <Route path="/more/stores" element={<StoresScreen />} />
      <Route path="*" element={<MoreMenu />} />
    </Routes>
  );
}
