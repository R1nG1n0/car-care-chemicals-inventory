// Заметка (ФТ-18): просмотр (переносы строк сохраняются, ссылки — просто текст) и правка:
// заголовок, текст с автоподстройкой высоты, привязки к позициям и шаблонам, удаление с подтверждением.
// /more/notes/new — сразу правка; ?product=ID и ?recipe=ID привязывают заметку при создании.
// Без связи заметку можно только читать (ФТ-18.4).
import { IconNote, IconPencil, IconTrash } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useRef, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router';
import { errorText, fieldErrors, isApiError, notes, type Note, type NoteInput } from '../../api';
import { invalidateNotes, keys } from '../../app/queries';
import { useToast } from '../../app/toast';
import { useNeedsOnline } from '../../app/useOffline';
import { BackLink } from '../../components/BackLink';
import { useAutoHeight } from '../../components/hooks';
import { busyProps, Empty, Field, HeadRow, Ico, LoadError, SkelList } from '../../components/ui';
import { linkParam, MAX_NOTE_BODY, MAX_NOTE_TITLE, noteChanged, validateNote, type NoteDraft, type NoteErrors } from '../../lib/notes';
import { formatDay } from '../../lib/text';
import { LinkChips, NoteLinks } from './NoteLinks';

function Editor({ note, preset, onDone }: { note: Note | null; preset: Pick<NoteDraft, 'product_ids' | 'recipe_ids'>; onDone: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const navigate = useNavigate();
  const [d, setD] = useState<NoteDraft>({ title: note?.title ?? '', body: note?.body ?? '', product_ids: note?.product_ids ?? preset.product_ids, recipe_ids: note?.recipe_ids ?? preset.recipe_ids });
  const [errors, setErrors] = useState<NoteErrors & { form?: string }>({});
  const [confirm, setConfirm] = useState(false);
  const area = useRef<HTMLTextAreaElement>(null);
  useAutoHeight(area, d.body);
  const set = (p: Partial<NoteDraft>) => {
    setD((x) => ({ ...x, ...p }));
    setErrors({});
  };

  const save = useMutation({
    mutationFn: (v: Required<NoteInput>) => (note ? notes.update(note.id, v) : notes.create(v)),
    onSuccess: async (r) => {
      await invalidateNotes(qc);
      toast.show({ text: note ? 'Заметка сохранена' : 'Заметка создана' });
      if (note) onDone();
      else navigate(`/more/notes/${r.id}`, { replace: true });
    },
    onError: (err) => {
      const f = fieldErrors(err, ['title', 'body'] as const);
      setErrors(f.title || f.body ? f : { form: errorText(err) });
    },
  });
  const remove = useMutation({
    mutationFn: () => notes.remove(note!.id),
    onSuccess: async () => {
      await invalidateNotes(qc);
      toast.show({ text: `Заметка «${note!.title}» удалена` });
      navigate('/more/notes', { replace: true });
    },
    onError: (err) => setErrors({ form: errorText(err) }),
  });

  const submit = () => {
    if (save.isPending) return;
    const r = validateNote(d);
    if (!r.ok) {
      setErrors(r.errors);
      document.getElementById(r.errors.title ? 'note-title' : 'note-body')?.focus();
      return;
    }
    save.mutate(r.value);
  };

  return (
    <form
      className="app-stack"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <HeadRow>
        <BackLink to={note ? `/more/notes/${note.id}` : '/more/notes'} label="Отмена" />
        <h1 className="g-label">{note ? 'Правка заметки' : 'Новая заметка'}</h1>
      </HeadRow>
      <Field id="note-title" label="Заголовок" maxLength={MAX_NOTE_TITLE} autoComplete="off" placeholder="Рецепт для салона" value={d.title} error={errors.title} onChange={(e) => set({ title: e.target.value })} />
      <div className={`g-field${errors.body ? ' g-field--error' : ''}`}>
        <label className="g-label" htmlFor="note-body">
          Текст <span className="g-field__opt">· необязательно</span>
        </label>
        <textarea
          ref={area}
          id="note-body"
          className="g-input g-input--area note-area"
          rows={6}
          maxLength={MAX_NOTE_BODY}
          placeholder="Пропорции, порядок действий, что с чем нельзя смешивать"
          value={d.body}
          aria-invalid={errors.body ? true : undefined}
          aria-describedby={errors.body ? 'note-body-err' : undefined}
          onChange={(e) => set({ body: e.target.value })}
        />
        {errors.body && (
          <span className="g-field__error" id="note-body-err">
            {errors.body}
          </span>
        )}
      </div>
      <NoteLinks products={d.product_ids} recipes={d.recipe_ids} onProducts={(v) => set({ product_ids: v })} onRecipes={(v) => set({ recipe_ids: v })} />

      {note &&
        (confirm ? (
          <div className="g-invite" role="group" aria-labelledby="note-del">
            <b id="note-del">Удалить заметку «{note.title}»?</b>
            <span className="g-invite__meta">Вернуть её будет нельзя.</span>
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
            <Ico icon={IconTrash} /> Удалить заметку
          </button>
        ))}

      <div className="shop-sticky shop-foot" data-toast-anchor>
        {errors.form && (
          <p className="g-field__error" role="alert">
            {errors.form}
          </p>
        )}
        <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" aria-disabled={!noteChanged(note, d) || undefined} {...busyProps(save.isPending)}>
          {save.isPending ? 'Сохраняю…' : 'Сохранить заметку'}
        </button>
      </div>
    </form>
  );
}

function NoteView({ note, onEdit }: { note: Note; onEdit: () => void }) {
  const online = useNeedsOnline();
  return (
    <div className="app-stack">
      <HeadRow>
        <BackLink to="/more/notes" label="Заметки" history />
        <button type="button" className="g-btn g-btn--sm g-btn--ghost" {...online.props} onClick={online.guard(onEdit)}>
          <Ico icon={IconPencil} /> Изменить
        </button>
      </HeadRow>
      <h1 className="g-title note-title">{note.title}</h1>
      <p className="g-field__hint">Изменена {formatDay(note.updated_at)}</p>
      {note.body ? <p className="note-body">{note.body}</p> : <p className="g-muted">Текста нет.</p>}
      <LinkChips products={note.product_ids} recipes={note.recipe_ids} />
      {online.hint && <p className="g-field__hint">{online.hint}: заметку можно править только онлайн.</p>}
    </div>
  );
}

function Existing({ id }: { id: number }) {
  const [editing, setEditing] = useState(false);
  const q = useQuery({ queryKey: keys.note(id), queryFn: ({ signal }) => notes.get(id, signal), enabled: id > 0 });
  // Одной заметки в кэше нет (без связи, открыли из блока) — берём из сохранённого списка
  const list = useQuery({ queryKey: keys.notes(), queryFn: ({ signal }) => notes.list({}, signal), enabled: !q.data });
  const note = q.data ?? list.data?.find((n) => n.id === id);
  if (note) {
    return editing ? <Editor key={note.updated_at} note={note} preset={{ product_ids: [], recipe_ids: [] }} onDone={() => setEditing(false)} /> : <NoteView note={note} onEdit={() => setEditing(true)} />;
  }
  return (
    <div className="app-stack">
      <BackLink to="/more/notes" label="Заметки" />
      {q.isPending && q.fetchStatus !== 'idle' && <SkelList label="Заметка загружается" rows={3} />}
      {q.isError && isApiError(q.error, 404) && <Empty id="note-404" icon={IconNote} title="Заметка не найдена" text="Возможно, её удалили." />}
      {q.isError && !isApiError(q.error, 404) && <LoadError id="note-err" error={q.error} what="Заметка" onRetry={() => void q.refetch()} />}
    </div>
  );
}

export function NoteScreen() {
  const id = Number(useParams().id);
  return <Existing key={id} id={id} />;
}

export function NoteNew() {
  const [sp] = useSearchParams();
  const online = useNeedsOnline();
  if (online.offline) {
    return (
      <div className="app-stack">
        <BackLink to="/more/notes" label="Заметки" />
        <Empty id="note-new-off" icon={IconNote} error title="Нужна связь" text="Заметки создаются только онлайн." />
      </div>
    );
  }
  return <Editor note={null} preset={{ product_ids: linkParam(sp.get('product')), recipe_ids: linkParam(sp.get('recipe')) }} onDone={() => undefined} />;
}
