// Новый шаблон и правка шаблона (ФТ-8.1): название, строки «позиция — количество»,
// удаление с подтверждением. Из «Из последнего расхода» строки приходят в state.items.
import { IconPlus, IconStack2, IconTrash, IconX } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router';
import { errorText, fieldErrors, inventory, isApiError, type ProductSummary, type Recipe } from '../../api';
import { keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { ProductPicker } from '../../components/ProductPicker';
import { ShopNewProduct } from '../../components/ShopNewProduct';
import { useProducts } from '../../components/refs';
import { busyProps, Empty, Field, HeadRow, Ico, LoadError, SkelList } from '../../components/ui';
import { validateRecipe, type ApplyRow, type RecipeErrors, type RecipeRowDraft } from '../../lib/recipeApply';
import { RecipeQty } from './RecipeQty';
import { BackLink } from '../../components/BackLink';

function Editor({ recipe, preset }: { recipe: Recipe | null; preset: ApplyRow[] | null }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const toast = useToast();
  const products = useProducts();
  const byId = new Map((products.data ?? []).map((p) => [p.id, p]));
  const seq = useRef(100);
  const form = useRef<HTMLFormElement>(null);
  const start = recipe ? recipe.items.map((i) => ({ product_id: i.product_id, qty: i.qty })) : (preset ?? []);
  const [name, setName] = useState(recipe?.name ?? '');
  const [rows, setRows] = useState<RecipeRowDraft[]>(() =>
    start.length ? start.map((r, i) => ({ key: i, product_id: r.product_id, qty: String(r.qty) })) : [{ key: 0, product_id: null, qty: '' }],
  );
  const [errors, setErrors] = useState<RecipeErrors>({ byRow: {} });
  const [formError, setFormError] = useState('');
  const [confirm, setConfirm] = useState(false);
  const [creating, setCreating] = useState<{ key: number; name: string } | null>(null);

  const setRow = (key: number, p: Partial<RecipeRowDraft>) => setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...p } : r)));
  const done = async (to: string, text: string) => {
    await qc.invalidateQueries({ predicate: (q) => q.queryKey[0] === 'recipes' || q.queryKey[0] === 'recipe' });
    toast.show({ text });
    navigate(to, { replace: true });
  };

  const save = useMutation({
    mutationFn: (b: Parameters<typeof inventory.createRecipe>[0]) => (recipe ? inventory.updateRecipe(recipe.id, b) : inventory.createRecipe(b)),
    onSuccess: (r) => done(`/templates/${r.id}`, recipe ? 'Шаблон сохранён' : `Шаблон «${r.name}» создан`),
    onError: (err) => {
      const f = fieldErrors(err, ['name'] as const);
      if (f.name) setErrors({ byRow: {}, name: f.name });
      else setFormError(f.form ?? errorText(err));
    },
  });
  const remove = useMutation({
    mutationFn: () => inventory.deleteRecipe(recipe!.id),
    onSuccess: () => done('/templates', `Шаблон «${recipe!.name}» удалён`),
    onError: (err) => setFormError(errorText(err)),
  });

  const submit = () => {
    if (save.isPending) return;
    const r = validateRecipe(name, rows);
    setFormError('');
    if (!r.ok) {
      setErrors(r.errors);
      // Фокус на первую ошибку — внутри этой формы, а не уходящего экрана
      requestAnimationFrame(() => form.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus());
      return;
    }
    setErrors({ byRow: {} });
    save.mutate(r.value);
  };

  const used = rows.map((r) => r.product_id).filter((x): x is number => x !== null);

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
          <BackLink to={recipe ? `/templates/${recipe.id}` : '/templates'} label="Отмена" />
          <h1 className="g-label">{recipe ? 'Правка шаблона' : 'Новый шаблон'}</h1>
        </HeadRow>
        {preset && !recipe && <p className="shop-hint">Строки взяты из последнего расхода — проверьте и дайте название.</p>}
        <Field id="te-name" label="Название" placeholder="Мойка кузова" maxLength={60} value={name} error={errors.name} onChange={(e) => setName(e.target.value)} />

        <div className="g-list">
          {rows.map((r, i) => {
            const p = r.product_id !== null ? byId.get(r.product_id) : undefined;
            const e = errors.byRow[r.key];
            const errId = `te-err-${r.key}`;
            return (
              <div className="g-row shop-tpl-row" key={r.key}>
                <div className="shop-tpl-edit">
                  <div className={e?.product ? 'g-field--error' : undefined}>
                    <ProductPicker
                      id={`te-p-${r.key}`}
                      label={`Строка ${i + 1}`}
                      value={r.product_id}
                      excludeIds={used.filter((id) => id !== r.product_id)}
                      onChange={(pr) => setRow(r.key, { product_id: pr.id, qty: r.qty || String(pr.last_consume_qty ?? '') })}
                      onCreate={(n) => setCreating({ key: r.key, name: n })}
                      limit={8}
                    />
                  </div>
                  <div className="shop-tpl-qty">
                    {p ? (
                      <RecipeQty
                        id={`te-q-${r.key}`}
                        name={p.name}
                        unit={p.unit}
                        value={r.qty}
                        invalid={Boolean(e?.qty)}
                        describedBy={e ? errId : undefined}
                        onChange={(qty) => setRow(r.key, { qty })}
                      />
                    ) : (
                      <span className="g-row__sub shop-tpl-later">Количество — после выбора позиции</span>
                    )}
                    {rows.length > 1 && (
                      <button type="button" className="g-btn g-btn--icon g-btn--ghost" aria-label={`Удалить строку ${i + 1}`} onClick={() => setRows((rs) => rs.filter((x) => x.key !== r.key))}>
                        <Ico icon={IconX} />
                      </button>
                    )}
                  </div>
                  {e && (
                    <span className="g-field__error" id={errId}>
                      {[e.product, e.qty && `Количество: ${e.qty}`].filter(Boolean).join('. ')}
                    </span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
        {rows.length < 30 && (
          <button type="button" className="g-btn g-btn--block g-press" onClick={() => setRows((rs) => [...rs, { key: seq.current++, product_id: null, qty: '' }])}>
            <Ico icon={IconPlus} /> Ещё позиция
          </button>
        )}
        {errors.rows && (
          <p className="g-field__error" role="alert">
            {errors.rows}
          </p>
        )}

        {recipe &&
          (confirm ? (
            <div className="g-invite" role="group" aria-labelledby="te-del">
              <b id="te-del">Удалить шаблон «{recipe.name}»?</b>
              <span className="g-invite__meta">Журнал расхода не изменится.</span>
              <div className="g-invite__actions">
                <button type="button" className="g-btn g-btn--sm g-btn--danger g-press" autoFocus onClick={() => !remove.isPending && remove.mutate()} {...busyProps(remove.isPending)}>
                  Удалить
                </button>
                <button type="button" className="g-btn g-btn--sm g-press" onClick={() => setConfirm(false)}>
                  Отмена
                </button>
              </div>
            </div>
          ) : (
            <button type="button" className="g-btn g-btn--ghost g-btn--block g-btn--danger-text" onClick={() => setConfirm(true)}>
              <Ico icon={IconTrash} /> Удалить шаблон
            </button>
          ))}

        <div className="shop-sticky shop-foot" data-toast-anchor>
          {formError && (
            <p className="g-field__error" role="alert">
              {formError}
            </p>
          )}
          <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(save.isPending)}>
            {save.isPending ? 'Сохраняю…' : 'Сохранить шаблон'}
          </button>
        </div>
      </form>
      <ShopNewProduct
        name={creating?.name ?? null}
        onClose={() => setCreating(null)}
        onCreated={(p: ProductSummary) => {
          if (creating) setRow(creating.key, { product_id: p.id });
          setCreating(null);
        }}
      />
    </>
  );
}

function presetItems(state: unknown): ApplyRow[] | null {
  const items = (state as { items?: unknown } | null)?.items;
  if (!Array.isArray(items)) return null;
  const ok = items.filter((i): i is ApplyRow => typeof i?.product_id === 'number' && typeof i?.qty === 'number');
  return ok.length ? ok : null;
}

export function TemplateNew() {
  const location = useLocation();
  return <Editor recipe={null} preset={presetItems(location.state)} />;
}

export function TemplateEditScreen() {
  const id = Number(useParams().id);
  const q = useQuery({ queryKey: keys.recipe(id), queryFn: ({ signal }) => inventory.recipe(id, signal), enabled: id > 0 });
  if (q.data) return <Editor key={q.data.id} recipe={q.data} preset={null} />;
  return (
    <div className="app-stack app-stack--fill">
      <BackLink to="/templates" label="Шаблоны" />
      {q.isPending && <SkelList label="Шаблон загружается" rows={3} />}
      {q.isError && isApiError(q.error, 404) && <Empty id="te-404" icon={IconStack2} title="Шаблон не найден" text="Возможно, его удалили." />}
      {q.isError && !isApiError(q.error, 404) && <LoadError id="te-err" error={q.error} what="Шаблон" onRetry={() => void q.refetch()} />}
    </div>
  );
}
