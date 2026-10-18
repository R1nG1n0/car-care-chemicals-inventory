// Форма позиции /items/new и /items/:id/edit (макет «Новая позиция», ФТ-3):
// название (?name= подставляется), бренд с подсказкой, категория чипами,
// мл/шт, размер упаковки, минимум, штрихкод (сканер заполняет поле, ФТ-9.3;
// ?barcode= подставляется — «Создать позицию» после сканирования), заметка.
// Ошибки сервера — под полями по field. После создания — на карточку позиции.
import { IconBarcode } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router';
import { fieldErrors, inventory, type ProductInput, type ProductSummary } from '../../api';
import { invalidateRefs, invalidateStock, keys } from '../../app/queries';
import { BackLink } from '../../components/BackLink';
import { BrandField, CategoryPick } from '../../components/ProductFields';
import { Scanner } from '../../components/Scanner';
import { useStockRefs } from '../../components/refs';
import { busyProps, Field, Ico, LoadError } from '../../components/ui';
import { parseManual } from '../../lib/barcode';
import { PRODUCT_FIELDS, validateProduct, type ProductErrors, type ProductFields } from '../../lib/product';
import { unitLabel } from '../../lib/stock';

/** ?barcode= из ссылки «Создать позицию»: как при ручном вводе, мусор отбрасывается. */
function barcodeParam(raw: string | null): string {
  const r = raw ? parseManual(raw) : null;
  return r?.ok ? r.code : '';
}

export function ProductForm() {
  const params = useParams();
  const id = params.id ? Number(params.id) : null;
  const detail = useQuery({
    queryKey: keys.product(id ?? 0),
    queryFn: ({ signal }) => inventory.product(id!, signal),
    enabled: id !== null,
  });
  if (id !== null && !detail.data) {
    return (
      <div className="stock-item">
        <div className="stock-item__bar">
          <BackLink to={`/items/${id}`} label="Отмена" />
        </div>
        {detail.isError ? (
          <LoadError id="pf-err" error={detail.error} what="Позиция" onRetry={() => void detail.refetch()} />
        ) : (
          <p className="g-muted" aria-busy="true">
            Загружаю позицию…
          </p>
        )}
      </div>
    );
  }
  return <FormBody key={id ?? 'new'} product={detail.data?.product} hasUnits={(detail.data?.units.length ?? 0) > 0} />;
}

function FormBody({ product, hasUnits }: { product: ProductSummary | undefined; hasUnits: boolean }) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [search] = useSearchParams();
  const refs = useStockRefs();
  const [f, setF] = useState<ProductFields>(() => ({
    name: product?.name ?? search.get('name') ?? '',
    brand: product?.brand ?? '',
    category_id: product?.category_id ?? null,
    unit: product?.unit ?? 'ml',
    package_size: product ? String(product.package_size) : '',
    min_stock: product?.min_stock != null ? String(product.min_stock) : '',
    barcode: product?.barcode ?? barcodeParam(search.get('barcode')),
    note: product?.note ?? '',
  }));
  const [errors, setErrors] = useState<ProductErrors>({});
  const [scanning, setScanning] = useState(false);
  const cats = refs.categoryList.filter((c) => !c.archived || c.id === f.category_id);
  const u = unitLabel(f.unit);
  const backTo = product ? `/items/${product.id}` : '/';

  const set = <K extends keyof ProductFields>(k: K, v: ProductFields[K]) => {
    setF((x) => ({ ...x, [k]: v }));
    setErrors((e) => ({ ...e, [k]: undefined, form: undefined }));
  };

  const save = useMutation({
    mutationFn: (v: ProductInput) => (product ? inventory.updateProduct(product.id, v) : inventory.createProduct(v)),
    onSuccess: async (r) => {
      await Promise.all([invalidateRefs(qc), invalidateStock(qc), qc.invalidateQueries({ queryKey: keys.brands })]);
      navigate(`/items/${r.id}`, { replace: true });
    },
    onError: (err) => setErrors(fieldErrors(err, PRODUCT_FIELDS)),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (save.isPending) return;
    const r = validateProduct(f);
    if (!r.ok) {
      setErrors(r.errors);
      const first = Object.keys(r.errors)[0];
      document.getElementById(`np-${first}`)?.focus();
      return;
    }
    setErrors({});
    save.mutate(r.value);
  };

  return (
    <div className="stock-item">
      <div className="stock-item__bar">
        <BackLink to={backTo} label="Отмена" />
        <h1 className="g-label">{product ? 'Изменить позицию' : 'Новая позиция'}</h1>
      </div>
      <form className="g-form stock-form" onSubmit={submit} noValidate>
        <div className="stock-form">
          <Field id="np-name" label="Название" value={f.name} maxLength={80} autoComplete="off" onChange={(e) => set('name', e.target.value)} error={errors.name} />
          <BrandField value={f.brand} onChange={(v) => set('brand', v)} error={errors.brand} />
          <CategoryPick categories={cats} value={f.category_id} onChange={(v) => set('category_id', v)} error={errors.category_id} />
          <div className="g-field">
            <span className="g-label" id="np-unit-l">
              Единица
            </span>
            <div className="g-seg" role="group" aria-labelledby="np-unit-l" aria-describedby={hasUnits ? 'np-unit-hint' : undefined}>
              {(['ml', 'pcs'] as const).map((x) => (
                <button key={x} id={x === 'ml' ? 'np-unit' : undefined} type="button" aria-pressed={f.unit === x} aria-disabled={(hasUnits && f.unit !== x) || undefined} onClick={() => !hasUnits && set('unit', x)}>
                  {unitLabel(x)}
                </button>
              ))}
            </div>
            {hasUnits && (
              <span className="g-field__hint" id="np-unit-hint">
                Единицу нельзя сменить: у позиции есть экземпляры
              </span>
            )}
            {errors.unit && <span className="g-field__error">{errors.unit}</span>}
          </div>
          <div className="app-two">
            <Field id="np-package_size" label={`Размер упаковки, ${u}`} inputMode="numeric" value={f.package_size} onChange={(e) => set('package_size', e.target.value)} error={errors.package_size} />
            <Field id="np-min_stock" label={`Мин. остаток, ${u}`} inputMode="numeric" value={f.min_stock} placeholder="не задан" onChange={(e) => set('min_stock', e.target.value)} error={errors.min_stock} />
          </div>
          <div className={`g-field${errors.barcode ? ' g-field--error' : ''}`}>
            <label className="g-label" htmlFor="np-barcode">
              Штрихкод <span className="g-field__opt">· необязательно</span>
            </label>
            <div className="g-field__row">
              <input
                className="g-input"
                id="np-barcode"
                inputMode="numeric"
                autoComplete="off"
                placeholder="4007218000123"
                value={f.barcode}
                aria-invalid={errors.barcode ? true : undefined}
                aria-describedby={errors.barcode ? 'np-barcode-err' : undefined}
                onChange={(e) => set('barcode', e.target.value)}
              />
              <button type="button" className="g-btn g-btn--sm g-press" onClick={() => setScanning(true)}>
                <Ico icon={IconBarcode} /> Сканировать
              </button>
            </div>
            {errors.barcode && (
              <span className="g-field__error" id="np-barcode-err">
                {errors.barcode}
              </span>
            )}
          </div>
          <div className={`g-field${errors.note ? ' g-field--error' : ''}`}>
            <label className="g-label" htmlFor="np-note">
              Заметка <span className="g-field__opt">· необязательно</span>
            </label>
            <textarea
              className="g-input g-input--area"
              id="np-note"
              placeholder="Разведение, особенности"
              maxLength={500}
              value={f.note}
              aria-invalid={errors.note ? true : undefined}
              onChange={(e) => set('note', e.target.value)}
            />
            {errors.note && <span className="g-field__error">{errors.note}</span>}
          </div>
        </div>
        {errors.form && (
          <p className="g-field__error" role="alert">
            {errors.form}
          </p>
        )}
        <div className="g-form__foot">
          <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(save.isPending)}>
            {save.isPending ? 'Сохраняю…' : 'Сохранить позицию'}
          </button>
        </div>
      </form>
      <Scanner
        open={scanning}
        title="Штрихкод позиции"
        onClose={() => setScanning(false)}
        onCode={(code) => {
          set('barcode', code);
          setScanning(false);
        }}
      />
    </div>
  );
}
