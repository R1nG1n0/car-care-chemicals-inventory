// Смена пароля (ФТ-1.4). 403 field: current — ошибка под полем текущего пароля,
// а не «сессия кончилась».
import { useMutation } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { api, isApiError } from '../../api';
import { hasErrors, validatePasswordChange, type FieldErrors } from '../../lib/validate';
import { busyProps, Field } from '../../components/ui';

type Errors = FieldErrors<'current' | 'new' | 'repeat' | 'form'>;

export function PasswordForm({ onDone }: { onDone: () => void }) {
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [repeat, setRepeat] = useState('');
  const [errors, setErrors] = useState<Errors>({});

  const mut = useMutation({
    mutationFn: () => api.changePassword(current, next),
    onSuccess: onDone,
    onError: (err) => {
      if (!isApiError(err)) return;
      if (err.field === 'current' || err.field === 'new') setErrors({ [err.field]: err.message });
      else if (err.status === 429) setErrors({ current: err.message });
      else setErrors({ form: err.message });
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (mut.isPending) return;
    const v = validatePasswordChange({ current, next, repeat });
    setErrors(v);
    if (!hasErrors(v)) mut.mutate();
  };

  return (
    <form className="g-invite app-form" onSubmit={submit} noValidate aria-label="Смена пароля">
      <Field id="pw-current" label="Текущий пароль" type="password" autoComplete="current-password" autoFocus value={current} onChange={(e) => setCurrent(e.target.value)} error={errors.current} />
      <Field id="pw-new" label="Новый пароль" type="password" autoComplete="new-password" hint="Не короче 8 символов" value={next} onChange={(e) => setNext(e.target.value)} error={errors.new} />
      <Field id="pw-repeat" label="Повтор нового пароля" type="password" autoComplete="new-password" value={repeat} onChange={(e) => setRepeat(e.target.value)} error={errors.repeat} />
      {errors.form && (
        <p className="g-field__error" role="alert">
          {errors.form}
        </p>
      )}
      <p className="g-field__hint">Остальные устройства выйдут из учётной записи, на этом вход сохранится.</p>
      <button type="submit" className="g-btn g-btn--block g-press" {...busyProps(mut.isPending)}>
        {mut.isPending ? 'Сохраняю…' : 'Сохранить пароль'}
      </button>
    </form>
  );
}
