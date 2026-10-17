// Шаблоны (ФТ-8) — по макету «Шаблоны»: строка целиком нажимается и открывает
// применение; «+ Новый», «Из последнего расхода» (ФТ-8.5), порядок.
import { IconArrowsSort, IconCheck, IconChevronRight, IconHistory, IconPlus, IconStack2 } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { inventory, type Recipe } from '../../api';
import { keys } from '../../app/queries';
import { errorText, ShopHead, ShopLoadError, ShopSkelList } from '../../components/ShopUi';
import { busyProps, Empty, Ico } from '../../components/ui';
import { formatMoney } from '../../lib/format';
import { lastManualConsume, type ApplyRow } from '../../lib/recipeApply';
import { moveItem } from '../../lib/refsOrder';
import { plural } from '../../lib/text';
import { SortableList } from '../more/SortableList';

const POSITIONS = ['позиция', 'позиции', 'позиций'] as const;
const LOG_PAGES = 4; // до 200 последних движений — дальше «последний расход» уже не последний

export function recipeSub(r: Recipe): string {
  const n = r.items.length;
  return [`${n} ${plural(n, POSITIONS)}`, r.cost !== null ? `≈ ${formatMoney(r.cost)}` : ''].filter(Boolean).join(' · ');
}

/** Ищет последний ручной расход по журналу, листая назад. */
async function findLastConsume(): Promise<ApplyRow[] | null> {
  let before: number | undefined;
  for (let i = 0; i < LOG_PAGES; i++) {
    const page = await inventory.movements({ before });
    const found = lastManualConsume(page);
    if (found) return found;
    if (page.length < 50) return null;
    before = page[page.length - 1]!.id;
  }
  return null;
}

export function TemplatesList() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const list = useQuery({ queryKey: keys.recipes, queryFn: ({ signal }) => inventory.recipes(signal) });
  const [sorting, setSorting] = useState(false);
  const [order, setOrder] = useState<number[] | null>(null);
  const [error, setError] = useState('');

  const recipes = list.data ?? [];
  const shown = order ? order.map((id) => recipes.find((r) => r.id === id)).filter((r): r is Recipe => Boolean(r)) : recipes;

  const fromLast = useMutation({
    mutationFn: findLastConsume,
    onSuccess: (items) => {
      if (!items) setError('В журнале нет ручного расхода. Спишите что-нибудь — и сохраните это шаблоном.');
      else navigate('/templates/new', { state: { items } });
    },
    onError: (err) => setError(errorText(err)),
  });

  const reorder = useMutation({
    mutationFn: inventory.reorderRecipes,
    onError: (err) => setError(`Порядок не сохранился: ${errorText(err)}`),
    onSettled: async () => {
      await qc.invalidateQueries({ queryKey: keys.recipes });
      setOrder(null);
    },
  });

  const move = (from: number, to: number) => {
    const ids = moveItem(
      shown.map((r) => r.id),
      from,
      to,
    );
    setOrder(ids);
    reorder.mutate(ids);
  };

  const fromLastBtn = (
    <button type="button" className="g-btn g-btn--block g-press" onClick={() => !fromLast.isPending && fromLast.mutate()} {...busyProps(fromLast.isPending)}>
      <Ico icon={IconHistory} /> {fromLast.isPending ? 'Ищу в журнале…' : 'Из последнего расхода'}
    </button>
  );
  const errorLine = error && (
    <p className="g-field__error" role="alert">
      {error}
    </p>
  );
  const head = (
    <ShopHead>
      <h1 className="g-title">Шаблоны</h1>
      <Link to="/templates/new" className="g-btn g-btn--sm g-press">
        <Ico icon={IconPlus} /> Новый
      </Link>
    </ShopHead>
  );

  if (list.isPending)
    return (
      <div className="app-stack">
        {head}
        <ShopSkelList label="Шаблоны загружаются" />
      </div>
    );
  if (list.isError && !list.data)
    return (
      <div className="app-stack app-stack--fill">
        {head}
        <ShopLoadError id="tpl-err" error={list.error} what="Шаблоны" onRetry={() => void list.refetch()} />
      </div>
    );
  if (recipes.length === 0)
    return (
      <div className="app-stack app-stack--fill">
        {head}
        <Empty
          id="tpl-empty"
          icon={IconStack2}
          title="Шаблонов пока нет"
          text="Шаблон списывает несколько позиций одним нажатием — например, всё для мойки кузова."
          actions={
            <>
              {fromLastBtn}
              {errorLine}
            </>
          }
        />
      </div>
    );

  return (
    <div className="app-stack">
      {head}
      {sorting ? (
        <SortableList items={shown} label="Порядок шаблонов" nameOf={(r) => r.name} onMove={move}>
          {(r) => (
            <div className="g-row__main">
              <div className="g-row__title">{r.name}</div>
              <div className="g-row__sub">{recipeSub(r)}</div>
            </div>
          )}
        </SortableList>
      ) : (
        <nav className="g-list" aria-label="Шаблоны">
          {shown.map((r) => {
            const short = r.items.some((i) => i.short);
            return (
              <Link key={r.id} to={`/templates/${r.id}`} className="g-row g-row--link">
                <span className="g-row__main">
                  <span className="g-row__title shop-block">{r.name}</span>
                  <span className="g-row__sub shop-block">{recipeSub(r)}</span>
                </span>
                {short && <span className="g-badge g-badge--danger">не хватает</span>}
                <span className="g-row__go">
                  <Ico icon={IconChevronRight} />
                </span>
              </Link>
            );
          })}
        </nav>
      )}
      {recipes.length > 1 && (
        <button type="button" className="g-btn g-btn--ghost g-btn--block" aria-pressed={sorting} onClick={() => setSorting((v) => !v)}>
          <Ico icon={sorting ? IconCheck : IconArrowsSort} /> {sorting ? 'Готово' : 'Изменить порядок'}
        </button>
      )}
      {fromLastBtn}
      {errorLine}
      <p className="shop-hint">Нажмите шаблон, чтобы проверить количества и списать. Шаблон можно сохранить из последнего расхода.</p>
    </div>
  );
}
