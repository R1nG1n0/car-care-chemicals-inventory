// Шаги кнопок количества (ФТ-5.8): малый и большой, PUT /api/auth/settings.
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { api, isApiError, type Session, type Settings } from '../../api';
import { meKey } from '../../app/session';
import { validateSteps, type FieldErrors } from '../../lib/validate';
import { busyProps, Field } from '../../components/ui';

type Errors = FieldErrors<'step_small' | 'step_large' | 'form'>;

export function StepsForm({ settings }: { settings: Settings }) {
  const client = useQueryClient();
  const [small, setSmall] = useState(String(settings.step_small));
  const [large, setLarge] = useState(String(settings.step_large));
  const [errors, setErrors] = useState<Errors>({});
  const [saved, setSaved] = useState(false);

  const mut = useMutation({
    mutationFn: (s: Settings) => api.saveSettings(s),
    onSuccess: (s) => {
      client.setQueryData<Session | null>(meKey, (old) => (old ? { ...old, settings: s } : old));
      setSmall(String(s.step_small));
      setLarge(String(s.step_large));
      setSaved(true);
    },
    onError: (err) => {
      if (!isApiError(err)) return;
      if (err.field === 'step_small' || err.field === 'step_large') setErrors({ [err.field]: err.message });
      else setErrors({ form: err.message });
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (mut.isPending) return;
    const r = validateSteps(small, large);
    setErrors(r.ok ? {} : r.errors);
    if (r.ok) mut.mutate(r.value);
  };

  const edit = (set: (v: string) => void) => (e: { target: { value: string } }) => {
    set(e.target.value);
    setSaved(false);
  };

  return (
    <form className="g-section" onSubmit={submit} noValidate aria-labelledby="pf-steps">
      <span className="g-label" id="pf-steps">
        Шаги кнопок количества, мл
      </span>
      <div className="app-two">
        <Field id="pf-s1" label="Малый" inputMode="numeric" enterKeyHint="done" value={small} onChange={edit(setSmall)} error={errors.step_small} />
        <Field id="pf-s2" label="Большой" inputMode="numeric" enterKeyHint="done" value={large} onChange={edit(setLarge)} error={errors.step_large} />
      </div>
      {errors.form && (
        <p className="g-field__error" role="alert">
          {errors.form}
        </p>
      )}
      <div className="app-inline">
        <button type="submit" className="g-btn g-btn--sm g-press" {...busyProps(mut.isPending)}>
          {mut.isPending ? 'Сохраняю…' : 'Сохранить шаги'}
        </button>
        <span className="g-field__hint" role="status">
          {saved ? 'Сохранено' : ''}
        </span>
      </div>
    </form>
  );
}
