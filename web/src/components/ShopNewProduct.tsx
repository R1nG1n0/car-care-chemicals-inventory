// Новая позиция прямо из покупки, шаблона или списка покупок (ФТ-4.2):
// шторка с главными полями, остальное правится потом в карточке позиции.
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { fieldErrors, inventory, type ProductSummary, type Unit } from '../api';
import { invalidateRefs, invalidateStock } from '../app/queries';
import { validateProduct, type ProductErrors } from '../lib/product';
import { CategoryPick } from './ProductFields';
import { useStockRefs } from './refs';
import { Sheet } from './Sheet';
import { busyProps, Field } from './ui';

/** Поля, которые есть в шторке: ошибка сервера по ним — под поле. */
const SHOWN = ['name', 'category_id', 'package_size', 'min_stock'] as const;

type Props = {
  /** Набранное в поиске имя; null — шторка закрыта. */
  name: string | null;
  onClose: () => void;
  onCreated: (p: ProductSummary) => void;
};

function Form({ initialName, onCreated }: { initialName: string; onCreated: (p: ProductSummary) => void }) {
  const qc = useQueryClient();
  const refs = useStockRefs();
  const cats = refs.categoryList.filter((c) => !c.archived);
  const [name, setName] = useState(initialName);
  const [categoryId, setCategoryId] = useState<number | null>(null);
  const [unit, setUnit] = useState<Unit>('ml');
  const [size, setSize] = useState('');
  const [min, setMin] = useState('');
  const [errors, setErrors] = useState<ProductErrors>({});

  const create = useMutation({
    mutationFn: inventory.createProduct,
    onSuccess: async (p) => {
      // Позиция с минимумом и нулевым остатком сразу попадает в «Ниже минимума»
      await Promise.all([invalidateRefs(qc), invalidateStock(qc)]);
      onCreated(p);
    },
    // Поля, которых нет в шторке (бренд, штрихкод…), — в общий текст, а не в пустоту
    onError: (err) => setErrors(fieldErrors(err, SHOWN)),
  });

  const submit = () => {
    if (create.isPending) return;
    const r = validateProduct({ name, brand: '', category_id: categoryId, unit, package_size: size, min_stock: min, barcode: '', note: '' });
    if (!r.ok) {
      setErrors(r.errors);
      return;
    }
    setErrors({});
    create.mutate(r.value);
  };

  const u = unit === 'ml' ? 'мл' : 'шт';
  return (
    <form
      id="snp-form"
      className="app-stack"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <Field id="snp-name" label="Название" value={name} maxLength={80} error={errors.name} data-autofocus onChange={(e) => setName(e.target.value)} />
      <CategoryPick idPrefix="snp" categories={cats} value={categoryId} onChange={setCategoryId} error={errors.category_id} />
      <div className="g-field">
        <span className="g-label" id="snp-unit">
          Единица
        </span>
        <div className="g-seg" role="group" aria-labelledby="snp-unit">
          <button type="button" aria-pressed={unit === 'ml'} onClick={() => setUnit('ml')}>
            мл
          </button>
          <button type="button" aria-pressed={unit === 'pcs'} onClick={() => setUnit('pcs')}>
            шт
          </button>
        </div>
      </div>
      <div className="shop-two">
        <Field id="snp-size" label={`Упаковка, ${u}`} inputMode="numeric" value={size} error={errors.package_size} onChange={(e) => setSize(e.target.value)} />
        <Field id="snp-min" label={`Минимум, ${u}`} hint="Необязательно" inputMode="numeric" value={min} error={errors.min_stock} onChange={(e) => setMin(e.target.value)} />
      </div>
      {errors.form && (
        <p className="g-field__error" role="alert">
          {errors.form}
        </p>
      )}
      <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(create.isPending)}>
        {create.isPending ? 'Создаю…' : 'Создать позицию'}
      </button>
    </form>
  );
}

export function ShopNewProduct({ name, onClose, onCreated }: Props) {
  // Последнее имя держим, пока шторка уезжает: содержимое не пропадает раньше времени
  const [last, setLast] = useState(name);
  if (name !== null && name !== last) setLast(name);
  return (
    <Sheet open={name !== null} onClose={onClose} title="Новая позиция">
      {last !== null && <Form key={last} initialName={last} onCreated={onCreated} />}
    </Sheet>
  );
}
