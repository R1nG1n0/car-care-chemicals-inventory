// Заметки (ФТ-18.2): список — новые и изменённые сверху, поиск по заголовку и тексту.
// Поиск уходит на сервер после паузы в наборе; без связи (или пока ответа нет) —
// по сохранённому списку на телефоне.
import { IconNote, IconPlus, IconSearch } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { Link } from 'react-router';
import { notes, type Note } from '../../api';
import { keys } from '../../app/queries';
import { useOnline } from '../../app/session';
import { useNeedsOnline } from '../../app/useOffline';
import { BackLink } from '../../components/BackLink';
import { useDebounced } from '../../components/hooks';
import { Empty, HeadRow, Ico, LoadError, SkelList } from '../../components/ui';
import { excerpt, filterNotes, sortNotes } from '../../lib/notes';
import { formatDay } from '../../lib/text';

export function NoteRow({ note }: { note: Note }) {
  const text = excerpt(note.body);
  return (
    <Link to={`/more/notes/${note.id}`} className="g-row g-row--link note-row">
      <span className="g-row__main">
        <span className="g-row__title shop-block">{note.title}</span>
        {text && <span className="note-row__text">{text}</span>}
        <span className="g-row__sub shop-block">{formatDay(note.updated_at)}</span>
      </span>
    </Link>
  );
}

export function NotesList() {
  const online = useOnline();
  const needs = useNeedsOnline();
  const [q, setQ] = useState('');
  const settled = useDebounced(q.trim());
  const all = useQuery({ queryKey: keys.notes(), queryFn: ({ signal }) => notes.list({}, signal) });
  const found = useQuery({
    queryKey: keys.notes({ q: settled }),
    queryFn: ({ signal }) => notes.list({ q: settled }, signal),
    enabled: online && settled !== '',
    retry: false,
  });

  // Ответ сервера — только на уже набранный запрос; иначе (пауза, нет связи, ошибка) — сохранённый список
  const fromServer = online && settled !== '' && settled === q.trim() && found.data ? found.data : null;
  const list = fromServer ?? sortNotes(filterNotes(all.data ?? [], { q }));
  const searching = q.trim() !== '';

  return (
    <div className="app-stack">
      <HeadRow>
        <BackLink to="/more" label="Ещё" />
      </HeadRow>
      <h1 className="g-title">Заметки</h1>
      <Link to="/more/notes/new" className="g-btn g-btn--cta g-btn--block g-press" {...needs.props} onClick={(e) => needs.offline && e.preventDefault()}>
        <Ico icon={IconPlus} /> Новая заметка
      </Link>
      <div className="note-find">
        <Ico icon={IconSearch} size={18} />
        <input className="g-input" type="search" aria-label="Поиск по заметкам" autoComplete="off" placeholder="Поиск по заголовку и тексту" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      {!online && <p className="g-field__hint">Нужна связь: заметки можно создавать и править только онлайн. Список из сохранённых данных.</p>}

      {all.isPending && all.fetchStatus !== 'idle' && <SkelList label="Заметки загружаются" rows={3} />}
      {all.isError && !all.data && <LoadError id="notes-err" error={all.error} what="Заметки" onRetry={() => void all.refetch()} />}
      {all.data && list.length === 0 && (
        <Empty
          id="notes-empty"
          icon={IconNote}
          title={searching ? 'Ничего не найдено' : 'Заметок пока нет'}
          text={searching ? 'Попробуйте другое слово.' : 'Рецепты разведения, порядок обработки салона, что с чем сочетать.'}
        />
      )}
      {list.length > 0 && (
        <div className="g-list" aria-live="polite">
          {list.map((n) => (
            <NoteRow key={n.id} note={n} />
          ))}
        </div>
      )}
    </div>
  );
}
