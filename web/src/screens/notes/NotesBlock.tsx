// Блок «Заметки» в карточке позиции и в шаблоне (ФТ-18.3): связанные заметки
// (заголовок и первые строки) и «Новая заметка», сразу привязанная. Заметки берутся
// из общего списка — он же сохраняется на телефоне, поэтому без связи блок виден;
// «Новая заметка» без связи неактивна.
import { IconPlus } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useId } from 'react';
import { Link } from 'react-router';
import { notes } from '../../api';
import { keys } from '../../app/queries';
import { useNeedsOnline } from '../../app/useOffline';
import { Ico } from '../../components/ui';
import { filterNotes, newNoteUrl, sortNotes } from '../../lib/notes';
import { NoteRow } from './NotesList';

export function NotesBlock({ productId, recipeId }: { productId?: number; recipeId?: number }) {
  const online = useNeedsOnline();
  const h = useId();
  const all = useQuery({ queryKey: keys.notes(), queryFn: ({ signal }) => notes.list({}, signal) });
  const mine = sortNotes(filterNotes(all.data ?? [], { ...(productId ? { product_id: productId } : {}), ...(recipeId ? { recipe_id: recipeId } : {}) }));
  // Нет заметок и нет связи — показывать нечего и создать нельзя
  if (mine.length === 0 && online.offline) return null;
  return (
    <section className="app-stack" aria-labelledby={h}>
      <h2 className="g-label" id={h}>
        Заметки{mine.length > 0 ? ` · ${mine.length}` : ''}
      </h2>
      {mine.length > 0 && (
        <div className="g-list">
          {mine.map((n) => (
            <NoteRow key={n.id} note={n} />
          ))}
        </div>
      )}
      <Link
        to={newNoteUrl({ ...(productId ? { product_id: productId } : {}), ...(recipeId ? { recipe_id: recipeId } : {}) })}
        className="g-btn g-btn--block g-press"
        {...online.props}
        onClick={(e) => online.offline && e.preventDefault()}
      >
        <Ico icon={IconPlus} /> Новая заметка
      </Link>
    </section>
  );
}
