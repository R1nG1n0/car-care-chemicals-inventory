// Telegram-бот (ФТ-11.2): привязка одноразовой ссылкой. После перехода в бота
// статус обновляется сам — опросом, пока ссылка жива, и при возврате на вкладку.
import { IconBrandTelegram, IconExternalLink, IconPlugConnectedX } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { errorText, isApiError, telegram, type TelegramLink } from '../../api';
import { busyProps, Ico } from '../../components/ui';
import { linkAlive, linkedText, pollInterval, TELEGRAM_LINK_HINT, TELEGRAM_OFF_TEXT, TELEGRAM_PENDING_TEXT, TELEGRAM_PITCH, TELEGRAM_UNLINKED_TEXT, telegramView } from '../../lib/telegram';

const telegramKey = ['telegram'] as const;
const nowSec = () => Math.floor(Date.now() / 1000);

export function Telegram() {
  const client = useQueryClient();
  const [link, setLink] = useState<TelegramLink | null>(null);
  const [confirm, setConfirm] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const connectRef = useRef<HTMLButtonElement>(null);
  const refocus = useRef(false);
  const expiresAt = link?.expires_at ?? null;

  const status = useQuery({
    queryKey: telegramKey,
    queryFn: ({ signal }) => telegram.status(signal),
    refetchOnWindowFocus: true,
    refetchInterval: (query) => pollInterval(expiresAt, query.state.data, nowSec()),
  });
  const data = status.data;

  // Ссылка просрочилась — убираем её с экрана (после привязки её не показывает само условие ниже).
  useEffect(() => {
    if (!link) return;
    const left = link.expires_at * 1000 - Date.now();
    const timer = window.setTimeout(() => setLink(null), Math.max(left, 0));
    return () => window.clearTimeout(timer);
  }, [link]);

  const connect = useMutation({
    mutationFn: telegram.link,
    onSuccess: (l) => {
      setLink(l);
      setError('');
      setNotice('');
      // После await браузер может заблокировать открытие — тогда остаётся кнопка-ссылка.
      try {
        window.open(l.url, '_blank', 'noopener');
      } catch {
        /* ссылку покажем кнопкой */
      }
    },
    onError: (err) => {
      if (isApiError(err, 503)) {
        setError(TELEGRAM_OFF_TEXT);
        void client.invalidateQueries({ queryKey: telegramKey });
      } else {
        setError(errorText(err));
      }
    },
  });

  const unlink = useMutation({
    mutationFn: telegram.unlink,
    onSuccess: () => {
      setLink(null);
      setConfirm(false);
      setError('');
      setNotice(TELEGRAM_UNLINKED_TEXT);
      refocus.current = true;
      void client.invalidateQueries({ queryKey: telegramKey });
    },
    onError: (err) => setError(errorText(err)),
  });

  const view = data ? telegramView(data) : null;

  // Кнопка «Отключить» исчезла вместе с карточкой — фокус не теряем, а переводим на «Подключить».
  useEffect(() => {
    if (view === 'unlinked' && refocus.current) {
      refocus.current = false;
      connectRef.current?.focus();
    }
  }, [view]);

  // Один живой регион, всегда в DOM: меняется только текст, и его озвучивают.
  const liveText = view === 'disabled' ? TELEGRAM_OFF_TEXT : view === 'pending' ? TELEGRAM_PENDING_TEXT : notice;

  return (
    <section className="g-section" aria-labelledby="pf-tg">
      <span className="g-label" id="pf-tg">
        Telegram
      </span>
      {status.isPending && (
        <div className="g-skel" aria-busy="true" aria-label="Telegram загружается">
          <div className="g-skel__body">
            <div className="g-skel__line" />
          </div>
        </div>
      )}
      {status.isError && !data && (
        <div className="g-section">
          <p className="g-field__hint">Статус Telegram не загрузился.</p>
          <button type="button" className="g-btn g-btn--sm g-press" onClick={() => !status.isFetching && void status.refetch()} {...busyProps(status.isFetching)}>
            Повторить
          </button>
        </div>
      )}
      <p className="g-field__hint" role="status">
        {liveText}
      </p>
      {view === 'unlinked' && (
        <div className="g-section g-enter" key="unlinked">
          <p className="g-field__hint">{TELEGRAM_PITCH}</p>
          {link && linkAlive(link.expires_at, nowSec()) ? (
            <div className="g-invite g-enter" role="group" aria-label="Ссылка на бота">
              <a className="g-btn g-btn--block g-press" href={link.url} target="_blank" rel="noopener">
                <Ico icon={IconExternalLink} /> Открыть бота
              </a>
              <span className="g-invite__meta" role="status">
                {TELEGRAM_LINK_HINT}. Нажмите «Start» в боте — здесь всё обновится само.
              </span>
            </div>
          ) : (
            <button type="button" ref={connectRef} className="g-btn g-btn--block g-press" onClick={() => !connect.isPending && connect.mutate()} {...busyProps(connect.isPending)}>
              <Ico icon={IconBrandTelegram} /> {connect.isPending ? 'Готовлю ссылку…' : 'Подключить Telegram'}
            </button>
          )}
        </div>
      )}
      {view === 'linked' && data && (
        <div className="g-section g-enter" key="linked">
          <div className="g-row">
            <div className="g-row__main">
              <div className="g-row__sub">Подключён</div>
              <div className="g-row__title">{linkedText(data)}</div>
            </div>
            <Ico icon={IconBrandTelegram} />
          </div>
          {!confirm ? (
            <button type="button" className="g-btn g-btn--ghost g-btn--block g-btn--danger-text" onClick={() => setConfirm(true)}>
              <Ico icon={IconPlugConnectedX} /> Отключить
            </button>
          ) : (
            <div className="g-invite g-enter" role="group" aria-labelledby="pf-tg-off">
              <b id="pf-tg-off">Отключить Telegram?</b>
              <span className="g-invite__meta">Бот перестанет отвечать. Подключить заново можно в любой момент.</span>
              <div className="g-invite__actions">
                <button type="button" className="g-btn g-btn--sm g-btn--danger g-press" autoFocus onClick={() => !unlink.isPending && unlink.mutate()} {...busyProps(unlink.isPending)}>
                  Отключить
                </button>
                <button type="button" className="g-btn g-btn--sm g-press" onClick={() => setConfirm(false)}>
                  Отмена
                </button>
              </div>
            </div>
          )}
        </div>
      )}
      {error && (
        <p className="g-field__error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
