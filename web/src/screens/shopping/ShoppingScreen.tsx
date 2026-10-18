// Купить (ФТ-7) — по макетам «Список покупок» и «Пустой список покупок»:
// позиции ниже минимума приходят сами, остальное добавляется вручную и вычёркивается.
// Удаление пункта — сразу, с «Отменить» в уведомлении (как расход и архив позиции).
import { IconCheck, IconPlus, IconReceipt, IconShoppingCartCheck, IconTrash } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useRef, useState } from 'react';
import { Link } from 'react-router';
import { errorText, inventory, type ShoppingItem } from '../../api';
import { keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { useFlip } from '../../components/useFlip';
import { Empty, HeadRow, Ico, LoadError, SkelList } from '../../components/ui';
import { lowSub, manualSub, manualTitle, sortManual, toggleDone } from '../../lib/shopList';
import { qtyText } from '../../lib/stock';
import { ShopAddSheet } from './ShopAddSheet';

type ShoppingData = { low: Awaited<ReturnType<typeof inventory.shopping>>['low']; manual: ShoppingItem[] };

const nowSec = () => Math.floor(Date.now() / 1000);

export function ShoppingScreen() {
  const qc = useQueryClient();
  const toast = useToast();
  const list = useQuery({ queryKey: keys.shopping, queryFn: ({ signal }) => inventory.shopping(signal) });
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState('');

  const patch = (fn: (items: ShoppingItem[]) => ShoppingItem[]) =>
    qc.setQueryData<ShoppingData>(keys.shopping, (d) => (d ? { ...d, manual: fn(d.manual) } : d));

  // Отметка и удаление — сразу на экране, откат при ошибке. Изменений может быть
  // несколько подряд: снимок «до» верен, только если других в работе нет, иначе
  // откат затёр бы соседнее — тогда правду вернёт перезапрос после последнего.
  const pending = useRef(0);
  const begin = async (fn: (items: ShoppingItem[]) => ShoppingItem[]) => {
    setError('');
    pending.current += 1;
    await qc.cancelQueries({ queryKey: keys.shopping });
    const before = pending.current === 1 ? qc.getQueryData<ShoppingData>(keys.shopping) : undefined;
    patch(fn);
    return { before };
  };
  const fail = (err: unknown, ctx: { before: ShoppingData | undefined } | undefined) => {
    if (ctx?.before && pending.current === 1) qc.setQueryData(keys.shopping, ctx.before);
    setError(errorText(err));
  };
  const settle = async () => {
    pending.current -= 1;
    if (pending.current === 0) await qc.invalidateQueries({ queryKey: keys.shopping });
  };

  const done = useMutation({
    mutationFn: ({ id, done }: { id: number; done: boolean }) => inventory.setShoppingDone(id, done),
    onMutate: ({ id, done }) => begin((items) => toggleDone(items, id, done, nowSec())),
    onError: (err, _v, ctx) => fail(err, ctx),
    onSettled: settle,
  });

  // «Отменить» удаление: пункт добавляется заново (тот же product_id или текст),
  // отметка «куплено» возвращается
  const restore = async (item: ShoppingItem) => {
    try {
      const again = await inventory.addShopping(item.product_id !== null ? { product_id: item.product_id } : { title: item.title });
      if (item.done_at !== null) await inventory.setShoppingDone(again.id, true);
      toast.show({ text: 'Пункт возвращён' });
    } catch (err) {
      toast.show({ text: `Не вернулось: ${errorText(err)}` });
    } finally {
      await qc.invalidateQueries({ queryKey: keys.shopping });
    }
  };

  const remove = useMutation({
    mutationFn: (item: ShoppingItem) => inventory.deleteShopping(item.id),
    onMutate: (item) => begin((items) => items.filter((i) => i.id !== item.id)),
    onSuccess: (_r, item) =>
      toast.show({ text: `Удалено: ${manualTitle(item)}`, action: { label: 'Отменить', onClick: () => void restore(item) } }),
    onError: (err, _v, ctx) => fail(err, ctx),
    onSettled: settle,
  });

  const manual = sortManual(list.data?.manual ?? []);
  const low = list.data?.low ?? [];
  const flip = useFlip(manual.map((i) => i.id));
  const listed = manual.filter((i) => i.done_at === null && i.product_id !== null).map((i) => i.product_id!);

  const head = (
    <HeadRow>
      <h1 className="g-title">Купить</h1>
      <Link to="/purchases" className="g-btn g-btn--sm g-btn--ghost">
        <Ico icon={IconReceipt} /> История
      </Link>
    </HeadRow>
  );
  const addBtn = (
    <button type="button" className="g-btn g-btn--block g-press" onClick={() => setAdding(true)}>
      <Ico icon={IconPlus} /> Добавить в список
    </button>
  );
  const sheet = <ShopAddSheet open={adding} onClose={() => setAdding(false)} excludeIds={[...listed, ...low.map((l) => l.product.id)]} />;

  if (list.isPending) {
    return (
      <div className="app-stack">
        {head}
        <SkelList label="Список покупок загружается" />
      </div>
    );
  }
  if (list.isError && !list.data) {
    return (
      <div className="app-stack app-stack--fill">
        {head}
        <LoadError id="shop-err" error={list.error} what="Список покупок" onRetry={() => void list.refetch()} />
      </div>
    );
  }
  if (low.length === 0 && manual.length === 0) {
    return (
      <div className="app-stack app-stack--fill">
        {head}
        <Empty
          id="shop-empty"
          icon={IconShoppingCartCheck}
          title="Всё куплено"
          text="Позиции ниже минимального остатка появятся здесь сами. Остальное добавьте вручную."
          actions={addBtn}
        />
        {sheet}
      </div>
    );
  }

  return (
    <div className="app-stack">
      {head}
      {low.length > 0 && (
        <section className="g-section" aria-labelledby="shop-low">
          <h2 className="g-label shop-h" id="shop-low">
            Ниже минимума
          </h2>
          <div className="g-list">
            {low.map((l) => (
              <div className="g-row" key={l.product.id}>
                <Link to={`/items/${l.product.id}`} className="g-row__main shop-rowbtn">
                  <span className="g-row__title">{l.product.name}</span>
                  <span className="g-row__sub">{lowSub(l)}</span>
                </Link>
                <span className="g-badge g-badge--danger">{qtyText(l.product.stock.total, l.product.unit)}</span>
                <Link to={`/purchases/new?product=${l.product.id}`} className="g-btn g-btn--sm g-press" aria-label={`Купить ${l.product.name}`}>
                  Купить
                </Link>
              </div>
            ))}
          </div>
        </section>
      )}

      {manual.length > 0 && (
        <section className="g-section" aria-labelledby="shop-manual">
          <h2 className="g-label shop-h" id="shop-manual">
            Добавлено вручную
          </h2>
          <div className="g-list">
            {manual.map((i) => {
              const on = i.done_at !== null;
              const title = manualTitle(i);
              return (
                <div className={`g-row shop-item${on ? ' shop-item--done' : ''}`} key={i.id} ref={flip(i.id)}>
                  <button
                    type="button"
                    className={`g-check${on ? ' g-check--on' : ''}`}
                    aria-pressed={on}
                    aria-label={`Куплено: ${title}`}
                    onClick={() => done.mutate({ id: i.id, done: !on })}
                  >
                    <span className="g-check__box">
                      <Ico icon={IconCheck} size={16} />
                    </span>
                  </button>
                  {i.product ? (
                    <Link to={`/items/${i.product.id}`} className="g-row__main shop-rowbtn">
                      <span className="g-row__title">{title}</span>
                      <span className="g-row__sub">{manualSub(i)}</span>
                    </Link>
                  ) : (
                    <div className="g-row__main">
                      <div className="g-row__title">{title}</div>
                      <div className="g-row__sub">{manualSub(i)}</div>
                    </div>
                  )}
                  <button type="button" className="g-btn g-btn--icon g-btn--ghost shop-del" aria-label={`Удалить из списка: ${title}`} onClick={() => remove.mutate(i)}>
                    <Ico icon={IconTrash} />
                  </button>
                </div>
              );
            })}
          </div>
        </section>
      )}
      {error && (
        <p className="g-field__error" role="alert">
          {error}
        </p>
      )}
      {addBtn}
      {sheet}
    </div>
  );
}
