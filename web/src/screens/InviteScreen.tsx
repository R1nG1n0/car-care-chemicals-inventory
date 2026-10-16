// Приглашение (ФТ-1.3): проверка ссылки → логин, пароль, повтор → сразу в приложение.
import { IconLinkOff, IconUserPlus } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { api, isApiError, type ApiError } from '../api';
import { setSession, useSession } from '../app/session';
import { formatDay } from '../lib/text';
import { hasErrors, validateSignup, type FieldErrors } from '../lib/validate';
import { BootScreen, OfflineScreen } from './SystemScreens';
import { busyProps, Empty, Field, Ico, Logo } from '../components/ui';

type Errors = FieldErrors<'login' | 'password' | 'repeat' | 'form'>;

function serverErrors(err: ApiError): Errors {
  if (err.field === 'login' || err.status === 409) return { login: err.message };
  if (err.field === 'password') return { password: err.message };
  return { form: err.message };
}

function Invalid({ text }: { text: string }) {
  return (
    <main className="app-plain">
      <Logo />
      <Empty
        id="invite-bad"
        icon={IconLinkOff}
        title="Приглашение недействительно"
        text={`${text}. Попросите у владельца новую ссылку.`}
        actions={
          <Link to="/login" className="g-btn g-btn--block g-press">
            Ко входу
          </Link>
        }
      />
    </main>
  );
}

export function InviteScreen() {
  const { token = '' } = useParams();
  const check = useQuery({
    queryKey: ['invite', token],
    queryFn: ({ signal }) => api.checkInvite(token, signal),
    retry: false,
    staleTime: Infinity,
  });

  if (check.isPending) return <BootScreen />;
  if (check.isError) {
    if (isApiError(check.error, 404)) return <Invalid text={check.error.message.replace(/\.$/, '')} />;
    return <OfflineScreen error={check.error} lastAttempt={check.errorUpdatedAt} retrying={check.isFetching} onRetry={() => void check.refetch()} />;
  }
  return <SignupForm token={token} expiresAt={check.data.expires_at} />;
}

function SignupForm({ token, expiresAt }: { token: string; expiresAt: number }) {
  const client = useQueryClient();
  const navigate = useNavigate();
  const session = useSession();
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [repeat, setRepeat] = useState('');
  const [errors, setErrors] = useState<Errors>({});
  const [gone, setGone] = useState<string | null>(null);

  const mut = useMutation({
    mutationFn: () => api.acceptInvite(token, login.trim(), password),
    onSuccess: (s) => {
      client.removeQueries({ queryKey: ['invite', token] });
      setSession(client, s);
      navigate('/', { replace: true });
    },
    onError: (err) => {
      if (isApiError(err, 404)) setGone(err.message.replace(/\.$/, ''));
      else if (isApiError(err)) setErrors(serverErrors(err));
    },
  });

  if (gone) return <Invalid text={gone} />;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (mut.isPending) return;
    const v = validateSignup({ login: login.trim(), password, repeat });
    setErrors(v);
    if (!hasErrors(v)) mut.mutate();
  };

  const current = session.data?.user.login;
  return (
    <main className="app-plain">
      <form className="app-auth" onSubmit={submit} noValidate>
        <div>
          <Logo size="lg" />
          <p className="g-muted app-auth__sub">
            Приглашение в garage. Придумайте логин и пароль — склад будет только ваш. Ссылка действует до {formatDay(expiresAt)}.
          </p>
        </div>
        {current && <p className="g-banner">Вы вошли как {current}. После регистрации вход будет под новой учётной записью.</p>}
        <Field
          id="inv-login"
          label="Логин"
          autoFocus
          autoComplete="username"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          hint="3–32 символа: латиница, цифры, _ . -"
          value={login}
          onChange={(e) => setLogin(e.target.value)}
          error={errors.login}
        />
        <Field
          id="inv-pass"
          label="Пароль"
          type="password"
          autoComplete="new-password"
          hint="Не короче 8 символов"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          error={errors.password}
        />
        <Field
          id="inv-repeat"
          label="Повтор пароля"
          type="password"
          autoComplete="new-password"
          value={repeat}
          onChange={(e) => setRepeat(e.target.value)}
          error={errors.repeat}
        />
        <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(mut.isPending)}>
          <Ico icon={IconUserPlus} /> {mut.isPending ? 'Создаю…' : 'Создать учётную запись'}
        </button>
        {errors.form && (
          <p className="g-field__error" role="alert">
            {errors.form}
          </p>
        )}
      </form>
    </main>
  );
}
