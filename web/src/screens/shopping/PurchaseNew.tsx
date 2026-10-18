// Новая покупка (ФТ-4) — по макету «Новая покупка». ?product=id подставляет строку
// (кнопка «Купить» из списка покупок и карточки позиции).
import { IconPlus } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useRef, useState } from 'react';
import { useSearchParams } from 'react-router';
import { errorText, fieldErrors, inventory, type ProductSummary } from '../../api';
import { invalidateRefs, invalidateStock, keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { ShopNewProduct } from '../../components/ShopNewProduct';
import { ShopRefSelect } from '../../components/ShopRefSelect';
import { useProducts, useStockRefs } from '../../components/refs';
import { busyProps, HeadRow, Ico } from '../../components/ui';
import { formatMoney } from '../../lib/format';
import {
  emptyLine,
  MAX_LINES,
  packagesTotal,
  purchaseHasErrors,
  purchaseTotal,
  todayISO,
  toPurchaseInput,
  validatePurchase,
  type LineDraft,
  type PurchaseErrors,
} from '../../lib/shopPurchase';
import { PACKAGES, plural } from '../../lib/text';
import { PurchaseLineCard } from './PurchaseLineCard';
import { BackLink, useGoBack } from '../../components/BackLink';

export function PurchaseNew() {
  const qc = useQueryClient();
  const toast = useToast();
  // Назад, если пришли из приложения, иначе — в «Купить». «Отмена» — так же.
  const goBack = useGoBack('/shopping');
  const form = useRef<HTMLFormElement>(null);
  const [params] = useSearchParams();
  const preset = Number(params.get('product')) || null;
  const seq = useRef(1);
  const [date, setDate] = useState(todayISO);
  const [storeId, setStoreId] = useState<number | null>(null);
  const [lines, setLines] = useState<LineDraft[]>(() => [emptyLine(0, preset)]);
  const [errors, setErrors] = useState<PurchaseErrors>({ byLine: {} });
  const [formError, setFormError] = useState('');
  const [creating, setCreating] = useState<{ key: number; name: string } | null>(null);

  const products = useProducts();
  const refs = useStockRefs();
  const stores = useQuery({ queryKey: keys.stores(false), queryFn: ({ signal }) => inventory.stores(false, signal) });
  const byId = new Map((products.data ?? []).map((p) => [p.id, p]));
  const locations = refs.locationList.filter((l) => !l.archived);

  const setLine = (l: LineDraft) => setLines((ls) => ls.map((x) => (x.key === l.key ? l : x)));
  const addLine = () => setLines((ls) => [...ls, emptyLine(seq.current++)]);
  const removeLine = (key: number) => setLines((ls) => ls.filter((x) => x.key !== key));

  const createStore = async (name: string) => {
    const s = await inventory.createStore({ name, url: '' });
    await invalidateRefs(qc);
    return s.id;
  };
  const createLocation = async (name: string) => {
    const l = await inventory.createLocation({ name });
    await invalidateRefs(qc);
    return l.id;
  };

  const save = useMutation({
    mutationFn: inventory.createPurchase,
    onSuccess: async (_p, body) => {
      await invalidateStock(qc);
      const n = body.lines.reduce((s, l) => s + l.packages, 0);
      toast.show({ text: `Покупка сохранена · ${n} ${plural(n, PACKAGES)}` });
      goBack();
    },
    onError: (err) => {
      const f = fieldErrors(err, ['date', 'location_id'] as const);
      if (f.date) setErrors({ byLine: {}, date: f.date });
      // Место из строки ушло в архив на другом устройстве — подсказываем, где искать
      else if (f.location_id) setFormError(`${f.location_id} — проверьте поле «Место» в строках`);
      else setFormError(f.form ?? errorText(err));
    },
  });

  const submit = () => {
    if (save.isPending) return;
    const draft = { date, store_id: storeId, lines };
    const e = validatePurchase(draft);
    setErrors(e);
    setFormError('');
    if (purchaseHasErrors(e)) {
      // Фокус на первое поле с ошибкой — экранный диктор прочитает её
      requestAnimationFrame(() => form.current?.querySelector<HTMLElement>('.g-field--error input, .g-field--error select')?.focus());
      return;
    }
    save.mutate(toPurchaseInput(draft));
  };

  const count = packagesTotal(lines);
  const used = lines.map((l) => l.product_id).filter((x): x is number => x !== null);

  return (
    <>
      <form
        ref={form}
        className="app-stack"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <HeadRow>
          <BackLink to="/shopping" label="Отмена" history />
          <h1 className="g-label">Новая покупка</h1>
        </HeadRow>

        <div className="shop-two">
          <div className={`g-field${errors.date ? ' g-field--error' : ''}`}>
            <label className="g-label" htmlFor="pn-date">
              Дата
            </label>
            <input
              id="pn-date"
              className="g-input"
              type="date"
              value={date}
              max={todayISO()}
              aria-invalid={errors.date ? true : undefined}
              aria-describedby={errors.date ? 'pn-date-err' : undefined}
              onChange={(e) => setDate(e.target.value)}
            />
            {errors.date && (
              <span className="g-field__error" id="pn-date-err">
                {errors.date}
              </span>
            )}
          </div>
          <ShopRefSelect
            id="pn-store"
            label="Магазин"
            value={storeId}
            onChange={setStoreId}
            options={stores.data ?? []}
            noneLabel="Не указан"
            newLabel="Новый магазин…"
            newPlaceholder="Автохим"
            onCreate={createStore}
          />
        </div>

        {lines.map((l, i) => (
          <PurchaseLineCard
            key={l.key}
            n={i + 1}
            line={l}
            product={l.product_id !== null ? byId.get(l.product_id) : undefined}
            category={l.product_id !== null ? refs.categories.get(byId.get(l.product_id)?.category_id ?? -1) : undefined}
            locations={locations}
            excludeIds={used.filter((id) => id !== l.product_id)}
            errors={errors.byLine[l.key]}
            canRemove={lines.length > 1}
            onChange={setLine}
            onRemove={() => removeLine(l.key)}
            onCreateProduct={(name) => setCreating({ key: l.key, name })}
            onCreateLocation={createLocation}
          />
        ))}

        {lines.length < MAX_LINES && (
          <button type="button" className="g-btn g-btn--block g-press" onClick={addLine}>
            <Ico icon={IconPlus} /> Ещё позиция
          </button>
        )}
        {errors.lines && (
          <p className="g-field__error" role="alert">
            {errors.lines}
          </p>
        )}

        <div className="shop-sticky shop-foot" data-toast-anchor>
          <div className="shop-between">
            <span className="g-muted">
              Итого · {count} {plural(count, PACKAGES)}
            </span>
            <span className="shop-sum">{formatMoney(purchaseTotal(lines))}</span>
          </div>
          {formError && (
            <p className="g-field__error" role="alert">
              {formError}
            </p>
          )}
          <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(save.isPending)}>
            {save.isPending ? 'Сохраняю…' : 'Сохранить покупку'}
          </button>
        </div>
      </form>
      <ShopNewProduct
        name={creating?.name ?? null}
        onClose={() => setCreating(null)}
        onCreated={(p: ProductSummary) => {
          if (creating) setLines((ls) => ls.map((x) => (x.key === creating.key ? { ...x, product_id: p.id } : x)));
          setCreating(null);
        }}
      />
    </>
  );
}
