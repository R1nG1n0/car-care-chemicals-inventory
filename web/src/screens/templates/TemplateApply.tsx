// Применение шаблона (ФТ-8.2–8.6) — по макету «Применение шаблона»: количества можно
// поправить или убрать строку, нехватка подсвечивается, одно подтверждение «Списать всё»,
// затем уведомление с «Отменить» (отмена всех движений применения).
import { IconAlertTriangle, IconStack2, IconX } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { inventory, isApiError, type ApplyResult, type Recipe } from '../../api';
import { invalidateStock, keys } from '../../app/queries';
import { useSession } from '../../app/session';
import { useToast } from '../../app/toast';
import { errorText, ShopBack, ShopHead, ShopLoadError, ShopSkelList } from '../../components/ShopUi';
import { busyProps, Empty, Ico } from '../../components/ui';
import { formatMoney } from '../../lib/format';
import { applyItems, parseQty, qtyChanged, rowStep, shortageText } from '../../lib/recipeApply';
import { qtyText } from '../../lib/stock';
import { formatDay } from '../../lib/text';
import { RecipeQty } from './RecipeQty';

type Row = { product_id: number; qty: string };

const toRows = (r: Recipe): Row[] => r.items.map((i) => ({ product_id: i.product_id, qty: String(i.qty) }));

function shortageReport(res: ApplyResult, recipe: Recipe): string {
  return res.shortages
    .map((s) => {
      const it = recipe.items.find((i) => i.product_id === s.product_id);
      const name = it?.product.name ?? 'Позиция';
      return it ? `${name} — списано ${qtyText(s.consumed, it.product.unit)} из ${qtyText(s.requested, it.product.unit)}` : name;
    })
    .join('; ');
}

function Apply({ recipe, report, setReport }: { recipe: Recipe; report: string; setReport: (s: string) => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const step = useSession().data?.settings.step_small ?? 10;
  const [rows, setRows] = useState<Row[]>(() => toRows(recipe));
  const [error, setError] = useState('');
  // После списания строки начинаются заново — когда придёт свежий шаблон с новыми
  // остатками. Обычное обновление в фоне (возврат во вкладку) правки не сбрасывает.
  const [resetOnNext, setResetOnNext] = useState(false);
  const [seen, setSeen] = useState(recipe);
  if (recipe !== seen) {
    setSeen(recipe);
    if (resetOnNext) {
      setResetOnNext(false);
      setRows(toRows(recipe));
    }
  }

  const parsed = rows.map((r) => ({ product_id: r.product_id, qty: parseQty(r.qty) ?? 0 }));
  const items = applyItems(parsed);
  const changed = qtyChanged(parsed, recipe.items);

  const revert = useMutation({
    mutationFn: (ids: number[]) => inventory.revert(ids),
    onSuccess: async () => {
      await invalidateStock(qc);
      setReport('');
      toast.show({ text: 'Расход по шаблону отменён' });
    },
    onError: (err) => toast.show({ text: `Не отменилось: ${errorText(err)}` }),
  });

  const apply = useMutation({
    mutationFn: () => inventory.applyRecipe(recipe.id, items),
    onSuccess: async (res) => {
      setError('');
      setResetOnNext(true);
      setReport(res.shortages.length ? `Не хватило: ${shortageReport(res, recipe)}` : '');
      const cost = res.cost !== null ? ` · ${formatMoney(res.cost)}` : '';
      const ids = res.movements.map((m) => m.id);
      toast.show({
        text: `Списано по шаблону «${recipe.name}»${cost}`,
        action: ids.length ? { label: 'Отменить', onClick: () => revert.mutate(ids) } : undefined,
      });
      await invalidateStock(qc);
    },
    onError: (err) => setError(isApiError(err) ? err.message : errorText(err)),
  });

  const submit = () => {
    if (apply.isPending) return;
    if (items.length === 0) return setError('Нечего списывать: все количества равны нулю');
    apply.mutate();
  };

  return (
    <div className="app-stack">
      <ShopHead>
        <ShopBack to="/templates" label="Шаблоны" />
        <Link to={`/templates/${recipe.id}/edit`} className="g-btn g-btn--sm g-btn--ghost">
          Изменить
        </Link>
      </ShopHead>
      <h1 className="g-title">{recipe.name}</h1>
      <p className="shop-hint">{recipe.last_used_at ? `Последний раз ${formatDay(recipe.last_used_at)} · число можно вписать` : 'Ещё не применялся · число можно вписать'}</p>

      {rows.length === 0 ? (
        <p className="shop-hint">Все строки убраны. Вернитесь к шаблону, чтобы начать заново.</p>
      ) : (
        <div className="g-list">
          {rows.map((r, i) => {
            const it = recipe.items.find((x) => x.product_id === r.product_id)!;
            const qty = parseQty(r.qty) ?? 0;
            const warn = qty > 0 ? shortageText(qty, it.available, it.product.unit) : null;
            const warnId = `ta-warn-${r.product_id}`;
            return (
              <div className={`g-row shop-tpl-row${warn ? ' g-row--warn' : ''}`} key={r.product_id}>
                <div className="g-row__main shop-between">
                  <div>
                    <div className="g-row__title">{it.product.name}</div>
                    {warn ? (
                      <div className="shop-short" id={warnId}>
                        <Ico icon={IconAlertTriangle} size={14} /> {warn}
                      </div>
                    ) : (
                      <div className="g-row__sub">есть {qtyText(it.available, it.product.unit)}</div>
                    )}
                  </div>
                  <button
                    type="button"
                    className="g-btn g-btn--icon g-btn--ghost"
                    aria-label={`Убрать строку: ${it.product.name}`}
                    onClick={() => setRows((rs) => rs.filter((x) => x.product_id !== r.product_id))}
                  >
                    <Ico icon={IconX} />
                  </button>
                </div>
                <RecipeQty
                  id={`ta-q-${r.product_id}`}
                  name={it.product.name}
                  unit={it.product.unit}
                  step={rowStep(it.product.unit, step)}
                  value={r.qty}
                  describedBy={warn ? warnId : undefined}
                  onChange={(qty) => setRows((rs) => rs.map((x, j) => (j === i ? { ...x, qty } : x)))}
                />
              </div>
            );
          })}
        </div>
      )}

      <div className="shop-between">
        <span className="g-muted">Себестоимость</span>
        <span className="shop-sum">{recipe.cost !== null ? `≈ ${formatMoney(recipe.cost)}` : '—'}</span>
      </div>
      {(changed || recipe.cost === null) && (
        <p className="shop-hint">{recipe.cost === null ? 'Цен у экземпляров нет — себестоимость не считается.' : 'Оценка — по количествам шаблона; точная сумма будет в уведомлении после списания.'}</p>
      )}
      {report && (
        <p className="g-banner" role="status">
          <Ico icon={IconAlertTriangle} />
          <span className="g-banner__text">{report}</span>
        </p>
      )}
      {error && (
        <p className="g-field__error" role="alert">
          {error}
        </p>
      )}
      <div className="shop-sticky">
        <button type="button" className="g-btn g-btn--cta g-btn--block g-press" onClick={submit} {...busyProps(apply.isPending)}>
          {apply.isPending ? 'Списываю…' : 'Списать всё'}
        </button>
      </div>
    </div>
  );
}

export function TemplateApply() {
  const id = Number(useParams().id);
  const q = useQuery({ queryKey: keys.recipe(id), queryFn: ({ signal }) => inventory.recipe(id, signal), enabled: id > 0 });
  // Отчёт о нехватке — здесь, чтобы пережить перезагрузку шаблона после списания
  const [report, setReport] = useState('');
  if (q.data) return <Apply key={q.data.id} recipe={q.data} report={report} setReport={setReport} />;
  return (
    <div className="app-stack app-stack--fill">
      <ShopBack to="/templates" label="Шаблоны" />
      {q.isPending && <ShopSkelList label="Шаблон загружается" rows={3} />}
      {q.isError && isApiError(q.error, 404) && <Empty id="ta-404" icon={IconStack2} title="Шаблон не найден" text="Возможно, его удалили." />}
      {q.isError && !isApiError(q.error, 404) && <ShopLoadError id="ta-err" error={q.error} what="Шаблон" onRetry={() => void q.refetch()} />}
    </div>
  );
}
