// Приглашения (ФТ-1.3), только владелец. Ссылка видна один раз — сразу после
// создания (в базе лежит хеш); в списке — только статус и срок.
import { IconCopy, IconLinkOff, IconUserPlus } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useRef, useState } from 'react';
import { api, isApiError, type CreatedInvite, type Invite } from '../../api';
import { formatDay, INVITE_STATUS, inviteMeta } from '../../lib/text';
import { busyProps, Ico } from '../../components/ui';

const invitesKey = ['invites'] as const;
const nowSec = () => Math.floor(Date.now() / 1000);

/** Копирование: Clipboard API, иначе — выделить текст и попробовать copy. */
async function copyText(text: string, node: HTMLElement | null): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    if (!node) return false;
    const range = document.createRange();
    range.selectNodeContents(node);
    const sel = window.getSelection();
    sel?.removeAllRanges();
    sel?.addRange(range);
    try {
      return document.execCommand('copy');
    } catch {
      return false;
    }
  }
}

function FreshInvite({ created, onRevoke, revoking }: { created: CreatedInvite; onRevoke: () => void; revoking: boolean }) {
  const linkRef = useRef<HTMLParagraphElement>(null);
  const [copy, setCopy] = useState<'idle' | 'ok' | 'manual'>('idle');
  const doCopy = async () => setCopy((await copyText(created.url, linkRef.current)) ? 'ok' : 'manual');

  return (
    <article className="g-invite" aria-label="Новое приглашение">
      <div className="g-invite__head">
        <b>Одноразовая ссылка</b>
        <span className="g-badge g-badge--accent">активно</span>
      </div>
      <p className="g-invite__link" ref={linkRef}>
        {created.url}
      </p>
      <div className="g-invite__meta">
        {inviteMeta(created.invite, nowSec())}. Ссылка показывается один раз — скопируйте и отправьте её.
      </div>
      <div className="g-invite__actions">
        <button type="button" className="g-btn g-btn--sm g-press" onClick={() => void doCopy()}>
          <Ico icon={IconCopy} /> {copy === 'ok' ? 'Скопировано' : 'Копировать'}
        </button>
        <button type="button" className="g-btn g-btn--sm g-btn--ghost g-btn--danger-text" onClick={() => !revoking && onRevoke()} {...busyProps(revoking)}>
          <Ico icon={IconLinkOff} /> Отозвать
        </button>
      </div>
      <span className="g-invite__meta" role="status">
        {copy === 'manual' ? 'Не получилось скопировать — ссылка выделена, скопируйте её вручную.' : ''}
      </span>
    </article>
  );
}

function InviteRow({ inv, onRevoke, revoking }: { inv: Invite; onRevoke: () => void; revoking: boolean }) {
  const active = inv.status === 'active' && inv.expires_at > nowSec();
  return (
    <div className="g-row">
      <div className="g-row__main">
        <div className="g-row__title">
          Приглашение <span className={`g-badge${active ? ' g-badge--accent' : ''}`}>{active ? INVITE_STATUS.active : INVITE_STATUS[inv.status === 'active' ? 'expired' : inv.status]}</span>
        </div>
        <div className="g-row__sub">{active ? inviteMeta(inv, nowSec()) : `Создано ${formatDay(inv.created_at)}`}</div>
      </div>
      {active && (
        <button type="button" className="g-btn g-btn--sm g-btn--ghost g-btn--danger-text" onClick={() => !revoking && onRevoke()} {...busyProps(revoking)}>
          Отозвать
        </button>
      )}
    </div>
  );
}

export function Invites() {
  const client = useQueryClient();
  const list = useQuery({ queryKey: invitesKey, queryFn: ({ signal }) => api.invites(signal) });
  const [created, setCreated] = useState<CreatedInvite | null>(null);
  const [error, setError] = useState('');

  const create = useMutation({
    mutationFn: api.createInvite,
    onSuccess: (c) => {
      setCreated(c);
      setError('');
      void client.invalidateQueries({ queryKey: invitesKey });
    },
    onError: (err) => isApiError(err) && setError(err.message),
  });

  const revoke = useMutation({
    mutationFn: (id: number) => api.revokeInvite(id),
    onSuccess: (_, id) => {
      if (created?.invite.id === id) setCreated(null);
      setError('');
      void client.invalidateQueries({ queryKey: invitesKey });
    },
    onError: (err) => {
      if (isApiError(err)) setError(err.status === 404 ? 'Приглашение уже использовано или отозвано' : err.message);
      void client.invalidateQueries({ queryKey: invitesKey });
    },
  });

  const revoking = (id: number) => revoke.isPending && revoke.variables === id;
  // Только что созданное показано карточкой со ссылкой — в списке его не дублируем.
  const rest = (list.data ?? []).filter((i) => i.id !== created?.invite.id);

  return (
    <section className="g-section" aria-labelledby="pf-inv">
      <span className="g-label" id="pf-inv">
        Приглашения
      </span>
      {created && <FreshInvite created={created} revoking={revoking(created.invite.id)} onRevoke={() => revoke.mutate(created.invite.id)} />}
      <button type="button" className="g-btn g-btn--cta g-btn--block g-press" onClick={() => !create.isPending && create.mutate()} {...busyProps(create.isPending)}>
        <Ico icon={IconUserPlus} /> {create.isPending ? 'Создаю…' : 'Создать приглашение'}
      </button>
      {error && (
        <p className="g-field__error" role="alert">
          {error}
        </p>
      )}
      {list.isPending && (
        <div className="g-skel" aria-busy="true" aria-label="Приглашения загружаются">
          <div className="g-skel__body">
            <div className="g-skel__line" />
            <div className="g-skel__line g-skel__line--short" />
          </div>
        </div>
      )}
      {list.isError && !list.data && <p className="g-field__hint">Список приглашений не загрузился.</p>}
      {list.data && rest.length === 0 && !created && <p className="g-field__hint">Приглашений пока нет. Ссылка действует 7 дней и подходит одному человеку.</p>}
      {rest.length > 0 && (
        <div className="g-list">
          {rest.map((inv) => (
            <InviteRow key={inv.id} inv={inv} revoking={revoking(inv.id)} onRevoke={() => revoke.mutate(inv.id)} />
          ))}
        </div>
      )}
    </section>
  );
}
