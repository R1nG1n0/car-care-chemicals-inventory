// Ещё — меню, профиль, места, категории, магазины (этап 3, coder «shopping»).
// Заготовка: адреса раздела разбирает вложенный <Routes>.
import { IconPackage } from '@tabler/icons-react';
import { Route, Routes, type Location } from 'react-router';
import { ProfileScreen } from '../profile/ProfileScreen';
import { StubScreen } from '../StubScreen';

/** location — адрес, «замороженный» для уходящего экрана при смене раздела (см. Shell). */
export function MoreSection({ location }: { location: Location }) {
  return (
    <Routes location={location}>
      <Route path="/profile" element={<ProfileScreen />} />
      <Route path="*" element={<StubScreen id="MoreSection" title="Раздел" icon={IconPackage} empty="Скоро" text="Экран в работе." />} />
    </Routes>
  );
}
