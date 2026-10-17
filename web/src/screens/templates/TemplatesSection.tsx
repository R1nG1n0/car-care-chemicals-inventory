// Шаблоны — список, применение, правка (ФТ-8).
// Адреса раздела разбирает вложенный <Routes>.
import { Route, Routes, type Location } from 'react-router';
import { TemplateApply } from './TemplateApply';
import { TemplateEditScreen, TemplateNew } from './TemplateEdit';
import { TemplatesList } from './TemplatesList';

/** location — адрес, «замороженный» для уходящего экрана при смене раздела (см. Shell). */
export function TemplatesSection({ location }: { location: Location }) {
  return (
    <Routes location={location}>
      <Route path="/templates/new" element={<TemplateNew key={location.key} />} />
      <Route path="/templates/:id/edit" element={<TemplateEditScreen />} />
      <Route path="/templates/:id" element={<TemplateApply />} />
      <Route path="*" element={<TemplatesList />} />
    </Routes>
  );
}
