// Места хранения (ФТ-2.2) и категории (ФТ-2.1) — редактируемые списки по макетам
// «Места хранения» и «Справочники — правка категории»: ручка перетаскивания,
// переименование прямо в строке, число экземпляров / позиций, архив.
import { IconArrowDown, IconArrowUp, IconArchive, IconCheck, IconChevronDown, IconChevronRight, IconDotsVertical, IconPencil, IconPlus, IconRestore, IconX } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Link } from 'react-router';
import { inventory, type CatColor, type Category, type Location } from '../../api';
import { invalidateRefs, keys } from '../../app/queries';
import { ShopSwatches } from '../../components/ShopSwatches';
import { catClass, errorText, ShopBack, ShopHead, ShopLoadError, ShopSkelList } from '../../components/ShopUi';
import { busyProps, Ico } from '../../components/ui';
import { moveItem } from '../../lib/refsOrder';
import { refNameError } from '../../lib/refsValidate';
import { plural } from '../../lib/text';
import { SortableList } from './SortableList';

export type RefKind = 'places' | 'categories';

type Item = { id: number; name: string; archived: boolean; count: number; color?: CatColor };
type Edit = { id: number | 'new'; name: string; color: CatColor };

const TEXT = {
  places: {
    title: 'Места',
    add: 'Место',
    hint: 'Перетащите за ручку, чтобы поменять порядок. Нажмите на название, чтобы переименовать.',
    one: 'места',
    archiveWord: ['место', 'места', 'мест'] as const,
    count: (n: number) => (n ? `${n} ${plural(n, ['экземпляр', 'экземпляра', 'экземпляров'])}` : 'пусто'),
    placeholder: 'Коробка 3, Полка 2',
  },
  categories: {
    title: 'Категории',
    add: 'Категория',
    hint: 'Перетащите за ручку, чтобы поменять порядок. Нажмите на название, чтобы переименовать и сменить цвет.',
    one: 'категории',
    archiveWord: ['категория', 'категории', 'категорий'] as const,
    count: (n: number) => (n ? `${n} ${plural(n, ['позиция', 'позиции', 'позиций'])}` : 'нет позиций'),
    placeholder: 'Глины и полироли',
  },
};

const fromLocation = (l: Location): Item => ({ id: l.id, name: l.name, archived: l.archived, count: l.units_count });
const fromCategory = (c: Category): Item => ({ id: c.id, name: c.name, archived: c.archived, count: c.products_count, color: c.color });

type RefApi = {
  key: readonly unknown[];
  list: (signal?: AbortSignal) => Promise<Item[]>;
  create: (e: Edit) => Promise<unknown>;
  update: (id: number, b: { name?: string; color?: CatColor; archived?: boolean }) => Promise<unknown>;
  reorder: (ids: number[]) => Promise<unknown>;
};

function api(kind: RefKind): RefApi {
  return kind === 'places'
    ? {
        key: keys.locations(true),
        list: async (signal?: AbortSignal) => (await inventory.locations(true, signal)).map(fromLocation),
        create: (e: Edit) => inventory.createLocation({ name: e.name }),
        update: (id: number, b: { name?: string; color?: CatColor; archived?: boolean }) =>
          inventory.updateLocation(id, { name: b.name, archived: b.archived }),
        reorder: inventory.reorderLocations,
      }
    : {
        key: keys.categories(true),
        list: async (signal?: AbortSignal) => (await inventory.categories(true, signal)).map(fromCategory),
        create: (e: Edit) => inventory.createCategory({ name: e.name, color: e.color }),
        update: inventory.updateCategory,
        reorder: inventory.reorderCategories,
      };
}

/** Строка правки: название (+ цвет у категории), сохранить, отменить. */
function EditRow({ kind, edit, setEdit, onSave, saving, error }: {
  kind: RefKind;
  edit: Edit;
  setEdit: (e: Edit | null) => void;
  onSave: () => void;
  saving: boolean;
  error: string;
}) {
  const t = TEXT[kind];
  const errId = `ref-err-${edit.id}`;
  return (
    <>
      <form
        className="g-row__main shop-edit"
        onSubmit={(e) => {
          e.preventDefault();
          if (!saving) onSave();
        }}
      >
        <input
          className="g-input"
          value={edit.name}
          maxLength={40}
          autoFocus
          onFocus={(e) => e.currentTarget.select()}
          placeholder={t.placeholder}
          aria-label={kind === 'places' ? 'Название места' : 'Название категории'}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? errId : undefined}
          onChange={(e) => setEdit({ ...edit, name: e.target.value })}
          onKeyDown={(e) => e.key === 'Escape' && setEdit(null)}
        />
        <button type="submit" className="g-btn g-btn--icon g-btn--accent g-press" aria-label="Сохранить" {...busyProps(saving)}>
          <Ico icon={IconCheck} />
        </button>
        <button type="button" className="g-btn g-btn--icon g-btn--ghost" aria-label="Отменить" onClick={() => setEdit(null)}>
          <Ico icon={IconX} />
        </button>
      </form>
      {(kind === 'categories' || error) && (
        <div className="g-row__extra">
          {kind === 'categories' && (
            <>
              <span className="g-label">Цвет</span>
              <ShopSwatches value={edit.color} onChange={(color) => setEdit({ ...edit, color })} label={`Цвет категории ${edit.name}`} />
            </>
          )}
          {error && (
            <span className="g-field__error" id={errId} role="alert">
              {error}
            </span>
          )}
        </div>
      )}
    </>
  );
}

export function RefsScreen({ kind }: { kind: RefKind }) {
  const t = TEXT[kind];
  const a = api(kind);
  const qc = useQueryClient();
  const list = useQuery({ queryKey: a.key, queryFn: ({ signal }) => a.list(signal) });
  const [order, setOrder] = useState<number[] | null>(null);
  const [edit, setEdit] = useState<Edit | null>(null);
  const [menu, setMenu] = useState<number | null>(null);
  const [rowError, setRowError] = useState<{ id: number | 'new' | 'order'; text: string } | null>(null);
  const [showArchive, setShowArchive] = useState(false);

  const all = list.data ?? [];
  const activeServer = all.filter((i) => !i.archived);
  const active = order ? order.map((id) => activeServer.find((i) => i.id === id)).filter((i): i is Item => Boolean(i)) : activeServer;
  const archived = all.filter((i) => i.archived);

  const done = async () => {
    setRowError(null);
    await invalidateRefs(qc);
  };

  const save = useMutation({
    mutationFn: (e: Edit) => (e.id === 'new' ? a.create(e) : a.update(e.id, { name: e.name, color: kind === 'categories' ? e.color : undefined })),
    onSuccess: async () => {
      setEdit(null);
      await done();
    },
    onError: (err, e) => setRowError({ id: e.id, text: errorText(err) }),
  });

  const archive = useMutation({
    mutationFn: ({ id, archived }: { id: number; archived: boolean }) => a.update(id, { archived }),
    onSuccess: async () => {
      setMenu(null);
      await done();
    },
    onError: (err, v) => setRowError({ id: v.id, text: errorText(err) }),
  });

  const reorder = useMutation({
    mutationFn: (ids: number[]) => a.reorder(ids),
    onError: (err) => setRowError({ id: 'order', text: `Порядок не сохранился: ${errorText(err)}` }),
    onSettled: async () => {
      await invalidateRefs(qc);
      setOrder(null);
    },
  });

  const move = (from: number, to: number) => {
    const ids = moveItem(
      active.map((i) => i.id),
      from,
      to,
    );
    setOrder(ids);
    reorder.mutate(ids);
  };

  const startEdit = (i: Item) => {
    setMenu(null);
    setRowError(null);
    setEdit({ id: i.id, name: i.name, color: i.color ?? 'slate' });
  };
  const doSave = () => {
    if (!edit) return;
    const nameErr = refNameError(edit.name);
    if (nameErr) {
      setRowError({ id: edit.id, text: nameErr });
      return;
    }
    save.mutate({ ...edit, name: edit.name.trim() });
  };
  const errFor = (id: number | 'new') => (rowError?.id === id ? rowError.text : '');

  const title = (i: Item) => (
    <>
      {kind === 'categories' && <span className={`g-cat-sq ${catClass(i.color)}`} aria-hidden="true" />}
      <button type="button" className="g-row__main shop-rowbtn" onClick={() => startEdit(i)} aria-label={`Переименовать ${i.name}`}>
        <span className="g-row__title">{i.name}</span>
        <span className="g-row__sub">{t.count(i.count)}</span>
      </button>
    </>
  );

  const rowMenu = (i: Item, index: number) => (
    <div className="g-row__extra shop-rowmenu" id={`ref-menu-${i.id}`}>
      <div className="shop-rowmenu__btns">
        {index > 0 && (
          <button type="button" className="g-btn g-btn--sm g-press" onClick={() => move(index, index - 1)}>
            <Ico icon={IconArrowUp} /> Выше
          </button>
        )}
        {index < active.length - 1 && (
          <button type="button" className="g-btn g-btn--sm g-press" onClick={() => move(index, index + 1)}>
            <Ico icon={IconArrowDown} /> Ниже
          </button>
        )}
        <button type="button" className="g-btn g-btn--sm g-press" onClick={() => startEdit(i)}>
          <Ico icon={IconPencil} /> Переименовать
        </button>
        {kind === 'places' && i.count > 0 && (
          <Link className="g-btn g-btn--sm g-press" to={`/?location=${i.id}`}>
            Экземпляры
          </Link>
        )}
        <button
          type="button"
          className="g-btn g-btn--sm g-btn--ghost g-btn--danger-text"
          onClick={() => !archive.isPending && archive.mutate({ id: i.id, archived: true })}
          {...busyProps(archive.isPending && archive.variables?.id === i.id)}
        >
          <Ico icon={IconArchive} /> В архив
        </button>
      </div>
      {errFor(i.id) && (
        <span className="g-field__error" role="alert">
          {errFor(i.id)}
        </span>
      )}
    </div>
  );

  return (
    <div className="app-stack">
      <ShopBack to="/more" label="Ещё" />
      <ShopHead>
        <h1 className="g-title">{t.title}</h1>
        <button
          type="button"
          className="g-btn g-btn--sm g-press"
          onClick={() => {
            setMenu(null);
            setRowError(null);
            setEdit({ id: 'new', name: '', color: 'slate' });
          }}
        >
          <Ico icon={IconPlus} /> {t.add}
        </button>
      </ShopHead>
      <p className="shop-hint">{t.hint}</p>

      {edit?.id === 'new' && (
        <div className="g-list">
          <div className="g-row g-row--edit">
            <EditRow kind={kind} edit={edit} setEdit={setEdit} onSave={doSave} saving={save.isPending} error={errFor('new')} />
          </div>
        </div>
      )}

      {list.isPending && <ShopSkelList label={`${t.title} загружаются`} />}
      {list.isError && !list.data && <ShopLoadError id="refs-err" error={list.error} what={t.title} onRetry={() => void list.refetch()} />}
      {list.data && active.length === 0 && edit?.id !== 'new' && (
        <p className="shop-hint">Пока пусто. Нажмите «+ {t.add}», чтобы добавить первое.</p>
      )}
      {active.length > 0 && (
        <SortableList
          items={active}
          label={t.title}
          nameOf={(i) => i.name}
          onMove={move}
          rowClass={(i) => (edit?.id === i.id ? 'g-row--edit' : menu === i.id ? 'shop-row--open' : '')}
        >
          {(i, index) =>
            edit?.id === i.id ? (
              <EditRow kind={kind} edit={edit} setEdit={setEdit} onSave={doSave} saving={save.isPending} error={errFor(i.id)} />
            ) : (
              <>
                {title(i)}
                <button
                  type="button"
                  className="g-btn g-btn--icon g-btn--ghost"
                  aria-label={`Меню ${t.one} ${i.name}`}
                  aria-expanded={menu === i.id}
                  aria-controls={`ref-menu-${i.id}`}
                  onClick={() => {
                    setRowError(null);
                    setMenu(menu === i.id ? null : i.id);
                  }}
                >
                  <Ico icon={IconDotsVertical} />
                </button>
                {menu === i.id && rowMenu(i, index)}
              </>
            )
          }
        </SortableList>
      )}
      {rowError?.id === 'order' && (
        <p className="g-field__error" role="alert">
          {rowError.text}
        </p>
      )}

      {archived.length > 0 && (
        <section className="g-section">
          <button type="button" className="shop-archive-link" aria-expanded={showArchive} aria-controls="ref-archive" onClick={() => setShowArchive((v) => !v)}>
            <span>
              Архив · {archived.length} {plural(archived.length, t.archiveWord)}
            </span>
            <Ico icon={showArchive ? IconChevronDown : IconChevronRight} />
          </button>
          {showArchive && (
            <div className="g-list" id="ref-archive">
              {archived.map((i) => (
                <div className="g-row shop-row--wrap" key={i.id}>
                  {kind === 'categories' && <span className={`g-cat-sq ${catClass(i.color)}`} aria-hidden="true" />}
                  <div className="g-row__main">
                    <div className="g-row__title">{i.name}</div>
                    <div className="g-row__sub">{t.count(i.count)}</div>
                  </div>
                  <button
                    type="button"
                    className="g-btn g-btn--sm g-press"
                    onClick={() => !archive.isPending && archive.mutate({ id: i.id, archived: false })}
                    {...busyProps(archive.isPending && archive.variables?.id === i.id)}
                  >
                    <Ico icon={IconRestore} /> Вернуть
                  </button>
                  {errFor(i.id) && (
                    <span className="g-row__extra g-field__error" role="alert">
                      {errFor(i.id)}
                    </span>
                  )}
                </div>
              ))}
            </div>
          )}
        </section>
      )}
    </div>
  );
}
