// Магазины (ФТ-2.3): название и необязательная ссылка, архив вместо удаления (ФТ-2.4).
// Порядок — по имени, перетаскивания нет.
import { IconArchive, IconCheck, IconChevronDown, IconChevronRight, IconExternalLink, IconPencil, IconPlus, IconRestore, IconX } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { isApiError, inventory, type Store } from '../../api';
import { invalidateRefs, keys } from '../../app/queries';
import { errorText, ShopBack, ShopHead, ShopLoadError, ShopSkelList } from '../../components/ShopUi';
import { busyProps, Field, Ico } from '../../components/ui';
import { refNameError, storeUrlError } from '../../lib/refsValidate';
import { plural } from '../../lib/text';

type Edit = { id: number | 'new'; name: string; url: string };
type Errors = { name?: string; url?: string; form?: string };

function StoreForm({ edit, setEdit, onSave, saving, errors }: {
  edit: Edit;
  setEdit: (e: Edit | null) => void;
  onSave: () => void;
  saving: boolean;
  errors: Errors;
}) {
  const id = `st-${edit.id}`;
  return (
    <form
      className="g-row g-row--edit shop-store-form"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        if (!saving) onSave();
      }}
      onKeyDown={(e) => e.key === 'Escape' && setEdit(null)}
    >
      <Field id={`${id}-name`} label="Название" value={edit.name} maxLength={40} autoFocus error={errors.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
      <Field
        id={`${id}-url`}
        label="Ссылка · необязательно"
        type="url"
        inputMode="url"
        placeholder="https://"
        value={edit.url}
        error={errors.url}
        onChange={(e) => setEdit({ ...edit, url: e.target.value })}
      />
      {errors.form && (
        <p className="g-field__error" role="alert">
          {errors.form}
        </p>
      )}
      <div className="shop-rowmenu__btns">
        <button type="submit" className="g-btn g-btn--sm g-btn--accent g-press" {...busyProps(saving)}>
          <Ico icon={IconCheck} /> Сохранить
        </button>
        <button type="button" className="g-btn g-btn--sm g-btn--ghost" onClick={() => setEdit(null)}>
          <Ico icon={IconX} /> Отменить
        </button>
      </div>
    </form>
  );
}

export function StoresScreen() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: keys.stores(true), queryFn: ({ signal }) => inventory.stores(true, signal) });
  const [edit, setEdit] = useState<Edit | null>(null);
  const [errors, setErrors] = useState<Errors>({});
  const [rowError, setRowError] = useState<{ id: number; text: string } | null>(null);
  const [showArchive, setShowArchive] = useState(false);

  const all = list.data ?? [];
  const active = all.filter((s) => !s.archived);
  const archived = all.filter((s) => s.archived);

  const save = useMutation({
    mutationFn: (e: Edit) => {
      const b = { name: e.name.trim(), url: e.url.trim() };
      return e.id === 'new' ? inventory.createStore(b) : inventory.updateStore(e.id, b);
    },
    onSuccess: async () => {
      setEdit(null);
      await invalidateRefs(qc);
    },
    onError: (err) => {
      if (isApiError(err) && (err.field === 'name' || err.field === 'url')) setErrors({ [err.field]: err.message });
      else setErrors({ form: errorText(err) });
    },
  });

  const archive = useMutation({
    mutationFn: ({ id, archived }: { id: number; archived: boolean }) => inventory.updateStore(id, { archived }),
    onSuccess: async () => {
      setRowError(null);
      await invalidateRefs(qc);
    },
    onError: (err, v) => setRowError({ id: v.id, text: errorText(err) }),
  });

  const open = (e: Edit) => {
    setErrors({});
    setEdit(e);
  };
  const doSave = () => {
    if (!edit) return;
    const e: Errors = {};
    const nameErr = refNameError(edit.name);
    if (nameErr) e.name = nameErr;
    const urlErr = storeUrlError(edit.url);
    if (urlErr) e.url = urlErr;
    setErrors(e);
    if (!e.name && !e.url) save.mutate(edit);
  };

  const row = (s: Store) =>
    edit?.id === s.id ? (
      <StoreForm key={s.id} edit={edit} setEdit={setEdit} onSave={doSave} saving={save.isPending} errors={errors} />
    ) : (
      <div className="g-row shop-row--wrap" key={s.id}>
        <div className="g-row__main">
          <div className="g-row__title">{s.name}</div>
          {s.url && (
            <a className="g-row__sub shop-link" href={s.url} target="_blank" rel="noopener noreferrer">
              {s.url.replace(/^https?:\/\//i, '')} <Ico icon={IconExternalLink} size={14} />
            </a>
          )}
        </div>
        <button type="button" className="g-btn g-btn--icon g-btn--ghost" aria-label={`Изменить ${s.name}`} onClick={() => open({ id: s.id, name: s.name, url: s.url })}>
          <Ico icon={IconPencil} />
        </button>
        <button
          type="button"
          className="g-btn g-btn--icon g-btn--ghost"
          aria-label={`В архив: ${s.name}`}
          onClick={() => !archive.isPending && archive.mutate({ id: s.id, archived: true })}
          {...busyProps(archive.isPending && archive.variables?.id === s.id)}
        >
          <Ico icon={IconArchive} />
        </button>
        {rowError?.id === s.id && (
          <span className="g-row__extra g-field__error" role="alert">
            {rowError.text}
          </span>
        )}
      </div>
    );

  return (
    <div className="app-stack">
      <ShopBack to="/more" label="Ещё" />
      <ShopHead>
        <h1 className="g-title">Магазины</h1>
        <button type="button" className="g-btn g-btn--sm g-press" onClick={() => open({ id: 'new', name: '', url: '' })}>
          <Ico icon={IconPlus} /> Магазин
        </button>
      </ShopHead>
      <p className="shop-hint">Магазин выбирается в покупке; по нему видно, где дешевле. Ссылка — для интернет-магазина.</p>
      {edit?.id === 'new' && (
        <div className="g-list">
          <StoreForm edit={edit} setEdit={setEdit} onSave={doSave} saving={save.isPending} errors={errors} />
        </div>
      )}
      {list.isPending && <ShopSkelList label="Магазины загружаются" />}
      {list.isError && !list.data && <ShopLoadError id="stores-err" error={list.error} what="Магазины" onRetry={() => void list.refetch()} />}
      {list.data && active.length === 0 && edit?.id !== 'new' && <p className="shop-hint">Магазинов пока нет. Их можно добавить и прямо в покупке.</p>}
      {active.length > 0 && <div className="g-list">{active.map(row)}</div>}
      {archived.length > 0 && (
        <section className="g-section">
          <button type="button" className="shop-archive-link" aria-expanded={showArchive} aria-controls="st-archive" onClick={() => setShowArchive((v) => !v)}>
            <span>
              Архив · {archived.length} {plural(archived.length, ['магазин', 'магазина', 'магазинов'])}
            </span>
            <Ico icon={showArchive ? IconChevronDown : IconChevronRight} />
          </button>
          {showArchive && (
            <div className="g-list" id="st-archive">
              {archived.map((s) => (
                <div className="g-row" key={s.id}>
                  <div className="g-row__main g-row__title">{s.name}</div>
                  <button
                    type="button"
                    className="g-btn g-btn--sm g-press"
                    onClick={() => !archive.isPending && archive.mutate({ id: s.id, archived: false })}
                    {...busyProps(archive.isPending && archive.variables?.id === s.id)}
                  >
                    <Ico icon={IconRestore} /> Вернуть
                  </button>
                </div>
              ))}
            </div>
          )}
        </section>
      )}
    </div>
  );
}
