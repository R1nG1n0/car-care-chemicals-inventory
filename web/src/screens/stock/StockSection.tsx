// Остатки — список, карточка позиции, форма позиции, расход (этап 3, coder «stock»).
// Адреса раздела разбирает вложенный <Routes>.
import { Route, Routes, type Location } from 'react-router';
import { ProductForm } from './ProductForm';
import { ProductScreen } from './ProductScreen';
import { StockHome } from './StockHome';

/** location — адрес, «замороженный» для уходящего экрана при смене раздела (см. Shell). */
export function StockSection({ location }: { location: Location }) {
  return (
    <Routes location={location}>
      <Route path="/items/new" element={<ProductForm />} />
      <Route path="/items/:id/edit" element={<ProductForm />} />
      <Route path="/items/:id" element={<ProductScreen />} />
      <Route path="*" element={<StockHome />} />
    </Routes>
  );
}
