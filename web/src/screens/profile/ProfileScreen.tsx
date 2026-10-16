// Профиль (ФТ-1.4) — по макету «Профиль и приглашения».
import { IconChevronDown, IconChevronRight, IconDownload, IconLogout, IconShare2, IconUser } from '@tabler/icons-react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { api } from '../../api';
import { promptInstall, useInstallMode } from '../../app/install';
import { clearSession, useSession } from '../../app/session';
import { busyProps, Ico } from '../../components/ui';
import { Invites } from './Invites';
import { PasswordForm } from './PasswordForm';
import { StepsForm } from './StepsForm';

function InstallSection() {
  const mode = useInstallMode();
  if (mode === 'none') return null;
  return (
    <section className="g-section" aria-labelledby="pf-install">
      <span className="g-label" id="pf-install">
        Приложение
      </span>
      {mode === 'prompt' ? (
        <button type="button" className="g-btn g-btn--block g-press" onClick={() => void promptInstall()}>
          <Ico icon={IconDownload} /> Установить приложение
        </button>
      ) : (
        <p className="g-banner">
          <Ico icon={IconShare2} />
          <span className="g-banner__text">Чтобы открыть garage как приложение: «Поделиться» → «На экран „Домой“».</span>
        </p>
      )}
    </section>
  );
}

function LogoutButtons() {
  const client = useQueryClient();
  const navigate = useNavigate();
  const [confirm, setConfirm] = useState(false);
  const done = () => {
    clearSession(client);
    navigate('/login', { replace: true });
  };
  const logout = useMutation({ mutationFn: api.logout, onSuccess: done });
  const logoutAll = useMutation({ mutationFn: api.logoutAll, onSuccess: done });

  return (
    <div className="g-section">
      <button type="button" className="g-btn g-btn--block g-press" onClick={() => !logout.isPending && logout.mutate()} {...busyProps(logout.isPending)}>
        <Ico icon={IconLogout} /> Выйти
      </button>
      {!confirm ? (
        <button type="button" className="g-btn g-btn--ghost g-btn--block g-btn--danger-text" onClick={() => setConfirm(true)}>
          <Ico icon={IconLogout} /> Выйти на всех устройствах
        </button>
      ) : (
        <div className="g-invite" role="group" aria-labelledby="pf-all">
          <b id="pf-all">Выйти на всех устройствах?</b>
          <span className="g-invite__meta">Сессии на всех телефонах и компьютерах закончатся, включая эту.</span>
          <div className="g-invite__actions">
            <button type="button" className="g-btn g-btn--sm g-btn--danger g-press" autoFocus onClick={() => !logoutAll.isPending && logoutAll.mutate()} {...busyProps(logoutAll.isPending)}>
              Выйти везде
            </button>
            <button type="button" className="g-btn g-btn--sm g-press" onClick={() => setConfirm(false)}>
              Отмена
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export function ProfileScreen() {
  const { data: session } = useSession();
  const [pwOpen, setPwOpen] = useState(false);
  const [pwDone, setPwDone] = useState(false);
  if (!session) return null;

  return (
    <div className="app-stack">
      <h1 className="g-title">Профиль</h1>
      <div className="g-list">
        <div className="g-row">
          <div className="g-row__main">
            <div className="g-row__sub">Логин</div>
            <div className="g-row__title">{session.user.login}</div>
          </div>
          {session.user.is_owner && <span className="g-badge">владелец</span>}
          <Ico icon={IconUser} />
        </div>
        <button
          type="button"
          className="g-row g-row--link app-row-btn"
          aria-expanded={pwOpen}
          aria-controls="pf-pw"
          onClick={() => {
            setPwOpen((v) => !v);
            setPwDone(false);
          }}
        >
          <span className="g-row__main g-row__title">Сменить пароль</span>
          <span className="g-row__go">
            <Ico icon={pwOpen ? IconChevronDown : IconChevronRight} />
          </span>
        </button>
      </div>
      <div id="pf-pw">
        {pwOpen && (
          <PasswordForm
            onDone={() => {
              setPwOpen(false);
              setPwDone(true);
            }}
          />
        )}
        <p className="g-field__hint" role="status">
          {pwDone ? 'Пароль изменён. На других устройствах нужно войти заново.' : ''}
        </p>
      </div>
      <StepsForm settings={session.settings} />
      <InstallSection />
      {session.user.is_owner && <Invites />}
      <LogoutButtons />
    </div>
  );
}
