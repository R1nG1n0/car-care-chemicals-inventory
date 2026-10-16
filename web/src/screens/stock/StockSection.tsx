// Остатки — список, карточка позиции, форма позиции, расход (этап 3, coder «stock»).
// Заготовка: адреса раздела разбирает вложенный <Routes>.
import { IconPackage } from '@tabler/icons-react';
import { Route, Routes, type Location } from 'react-router';
import { StubScreen } from '../StubScreen';

/** location — адрес, «замороженный» для уходящего экрана при смене раздела (см. Shell). */
export function StockSection({ location }: { location: Location }) {
  return (
    <Routes location={location}>
      <Route path="*" element={<StubScreen id="StockSection" title="Раздел" icon={IconPackage} empty="Скоро" text="Экран в работе." />} />
    </Routes>
  );
}
