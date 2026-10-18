// Применение шаблона (ФТ-8.2–8.6) — по макету «Применение шаблона»: количества можно
// поправить (как при расходе: число или кнопки шагов) или убрать строку, нехватка
// подсвечивается, одно подтверждение «Списать всё», затем уведомление с «Отменить»
// (отмена всех движений применения).
import { IconAlertTriangle, IconStack2, IconX } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { errorText, inventory, isApiError, type Recipe } from '../../api';
import { invalidateStock, keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { useRevert } from '../../app/useRevert';
import { BackLink } from '../../components/BackLink';
import { busyProps, Empty, HeadRow, Ico, LoadError, SkelList } from '../../components/ui';
import { formatMoney } from '../../lib/format';
import { applyItems, parseRowQty, qtyChanged, shortageReport, shortageText } from '../../lib/recipeApply';
import { qtyText } from '../../lib/stock';
import { formatDay } from '../../lib/text';
import { RecipeQty } from './RecipeQty';

type Row = { product_id: number; qty: string };

const toRows = (r: Recipe): Row[] => r.items.map((i) => ({ product_id: i.product_id, qty: String(i.qty) }));

function Apply({ recipe, report, setReport }: { recipe: Recipe; report: string; setReport: (s: string) => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const revert = useRevert();
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

  // Строки только с позициями, которые есть в шаблоне: после фонового перезапроса
  // (правка на другом устройстве) позиция могла пропасть — такая строка не рисуется
  // и не списывается.
  const shown = rows.flatMap((r) => {
    const it = recipe.items.find((x) => x.product_id === r.product_id);
    return it ? [{ row: r, it }] : [];
  });
  const parsed = shown.map(({ row }) => ({ product_id: row.product_id, qty: parseRowQty(row.qty) ?? 0 }));
  const items = applyItems(parsed);
  const changed = qtyChanged(parsed, recipe.items);
  const setQty = (id: number, qty: string) => setRows((rs) => rs.map((x) => (x.product_id === id ? { ...x, qty } : x)));

  const apply = useMutation({
    mutationFn: () => inventory.applyRecipe(recipe.id, items),
    onSuccess: async (res) => {
      setError('');
      setResetOnNext(true);
      setReport(res.shortages.length ? `Не хватило: ${shortageReport(res.shortages, recipe.items)}` : '');
      const cost = res.cost !== null ? ` · ${formatMoney(res.cost)}` : '';
      const ids = res.movements.map((m) => m.id);
      toast.show({
        text: `Списано по шаблону «${recipe.name}»${cost}`,
        action: ids.length
          ? {
              label: 'Отменить',
              onClick: () =>
                void revert(ids, 'Расход по шаблону отменён')
                  .then(() => setReport(''))
                  .catch(() => undefined),
            }
          : undefined,
      });
      await invalidateStock(qc);
    },
    onError: (err) => setError(errorText(err)),
  });

  const submit = () => {
    if (apply.isPending) return;
    if (parsed.some((r) => r.qty > 100_000)) return setError('В строке — не больше 100 000');
    if (items.length === 0) return setError('Нечего списывать: все количества равны нулю');
    apply.mutate();
  };

  return (
    <div className="app-stack">
      <HeadRow>
        <BackLink to="/templates" label="Шаблоны" />
        <Link to={`/templates/${recipe.id}/edit`} className="g-btn g-btn--sm g-btn--ghost">
          Изменить
        </Link>
      </HeadRow>
      <h1 className="g-title">{recipe.name}</h1>
      <p className="shop-hint">{recipe.last_used_at ? `Последний раз ${formatDay(recipe.last_used_at)} · число можно вписать` : 'Ещё не применялся · число можно вписать'}</p>

      {shown.length === 0 ? (
        <p className="shop-hint">Все строки убраны. Вернитесь к шаблону, чтобы начать заново.</p>
      ) : (
        <div className="g-list">
          {shown.map(({ row: r, it }) => {
            const qty = parseRowQty(r.qty) ?? 0;
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
                  value={r.qty}
                  describedBy={warn ? warnId : undefined}
                  onChange={(v) => setQty(r.product_id, v)}
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
      {/* Отчёт и ошибка — в закреплённой панели над кнопкой: уведомление встаёт над
          панелью (data-toast-anchor) и их не закрывает */}
      <div className="shop-sticky shop-foot" data-toast-anchor>
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
      <BackLink to="/templates" label="Шаблоны" />
      {q.isPending && <SkelList label="Шаблон загружается" rows={3} />}
      {q.isError && isApiError(q.error, 404) && <Empty id="ta-404" icon={IconStack2} title="Шаблон не найден" text="Возможно, его удалили." />}
      {q.isError && !isApiError(q.error, 404) && <LoadError id="ta-err" error={q.error} what="Шаблон" onRetry={() => void q.refetch()} />}
    </div>
  );
}
