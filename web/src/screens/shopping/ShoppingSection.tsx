// Купить — список покупок, новая покупка, история покупок.
// Адреса раздела разбирает вложенный <Routes>.
import { Route, Routes, type Location } from 'react-router';
import { PurchaseNew } from './PurchaseNew';
import { PurchasesHistory, PurchaseView } from './Purchases';
import { ShoppingScreen } from './ShoppingScreen';

/** location — адрес, «замороженный» для уходящего экрана при смене раздела (см. Shell). */
export function ShoppingSection({ location }: { location: Location }) {
  return (
    <Routes location={location}>
      <Route path="/purchases/new" element={<PurchaseNew key={location.key} />} />
      <Route path="/purchases/:id" element={<PurchaseView />} />
      <Route path="/purchases" element={<PurchasesHistory />} />
      <Route path="*" element={<ShoppingScreen />} />
    </Routes>
  );
}
