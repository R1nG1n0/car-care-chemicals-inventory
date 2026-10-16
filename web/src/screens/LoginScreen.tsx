// Вход (ФТ-1.1) — по макету «Вход · ошибка пароля».
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { api, isApiError, type ApiError } from '../api';
import { setSession } from '../app/session';
import { returnPath } from '../lib/nav';
import { tooManyAttemptsText } from '../lib/text';
import { hasErrors, validateLogin, type FieldErrors } from '../lib/validate';
import { busyProps, Field, Logo } from '../components/ui';

type Errors = FieldErrors<'login' | 'password'>;

function serverErrors(err: ApiError): Errors {
  if (err.status === 429) return { password: tooManyAttemptsText(err.retryAfter) };
  // Неверный логин или пароль — без field: под полем пароля, как в макете.
  return err.field === 'login' ? { login: err.message } : { password: err.message };
}

export function LoginScreen() {
  const client = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [errors, setErrors] = useState<Errors>({});

  const mut = useMutation({
    mutationFn: () => api.login(login.trim(), password),
    onSuccess: (s) => {
      setSession(client, s);
      navigate(returnPath(location.state), { replace: true });
    },
    onError: (err) => {
      // «Нет связи» показывает полоса вверху; введённое остаётся в полях.
      if (isApiError(err)) setErrors(serverErrors(err));
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (mut.isPending) return;
    const v = validateLogin({ login, password });
    setErrors(v);
    if (!hasErrors(v)) mut.mutate();
  };

  return (
    <main className="app-plain">
      <form className="app-auth" onSubmit={submit} noValidate>
        <div>
          <Logo size="lg" />
          <p className="g-muted app-auth__sub">Учёт автохимии и инструментов</p>
        </div>
        <Field
          id="login-name"
          label="Логин"
          autoFocus
          autoComplete="username"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          value={login}
          onChange={(e) => setLogin(e.target.value)}
          error={errors.login}
        />
        <Field
          id="login-pass"
          label="Пароль"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          error={errors.password}
        />
        <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(mut.isPending)}>
          {mut.isPending ? 'Вход…' : 'Войти'}
        </button>
        <p className="g-muted app-auth__note">Нет учётной записи? Попросите приглашение у владельца.</p>
      </form>
    </main>
  );
}
