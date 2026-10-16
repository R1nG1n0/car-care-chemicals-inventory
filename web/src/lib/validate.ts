// Проверка форм до отправки — те же правила, что на сервере (docs/API.md, ТБ-7).
// Сервер проверяет всё повторно; здесь — чтобы не гонять заведомо плохой ввод.

export type FieldErrors<K extends string> = Partial<Record<K, string>>;

const LOGIN_RE = /^[A-Za-z0-9_.-]{3,32}$/;

/** Длина в символах, а не в единицах UTF-16: эмодзи считается за один. */
export function charCount(s: string): number {
  return [...s].length;
}

export function loginError(login: string): string | undefined {
  if (login.trim() === '') return 'Введите логин';
  if (!LOGIN_RE.test(login)) return 'От 3 до 32 символов: латиница, цифры, _ . -';
  return undefined;
}

export function passwordError(password: string): string | undefined {
  const n = charCount(password);
  if (n === 0) return 'Введите пароль';
  if (n < 8) return 'Не короче 8 символов';
  if (n > 128) return 'Не длиннее 128 символов';
  return undefined;
}

/** Убирает поля без ошибки: пустой объект — форма в порядке. */
function compact<K extends string>(errors: FieldErrors<K>): FieldErrors<K> {
  return Object.fromEntries(Object.entries(errors).filter(([, v]) => v !== undefined)) as FieldErrors<K>;
}

export function hasErrors(errors: object): boolean {
  return Object.keys(errors).length > 0;
}

/** Вход: только непустые поля — правила логина сервер проверит сам, ответ одинаковый. */
export function validateLogin(v: { login: string; password: string }): FieldErrors<'login' | 'password'> {
  return compact({
    login: v.login.trim() === '' ? 'Введите логин' : undefined,
    password: v.password === '' ? 'Введите пароль' : undefined,
  });
}

/** Принятие приглашения: логин, пароль, повтор. */
export function validateSignup(v: { login: string; password: string; repeat: string }): FieldErrors<'login' | 'password' | 'repeat'> {
  const password = passwordError(v.password);
  return compact({
    login: loginError(v.login),
    password,
    repeat: !password && v.repeat !== v.password ? 'Пароли не совпадают' : undefined,
  });
}

/** Смена пароля: текущий, новый, повтор. */
export function validatePasswordChange(v: { current: string; next: string; repeat: string }): FieldErrors<'current' | 'new' | 'repeat'> {
  const next = passwordError(v.next);
  return compact({
    current: v.current === '' ? 'Введите текущий пароль' : undefined,
    new: next,
    repeat: !next && v.repeat !== v.next ? 'Пароли не совпадают' : undefined,
  });
}

function parseInt10(s: string): number | undefined {
  const t = s.trim();
  return /^\d+$/.test(t) ? Number(t) : undefined;
}

export type StepsResult =
  | { ok: true; value: { step_small: number; step_large: number } }
  | { ok: false; errors: FieldErrors<'step_small' | 'step_large'> };

/** Шаги кнопок количества: целые, 1–1000 и 1–5000, малый меньше большого. */
export function validateSteps(small: string, large: string): StepsResult {
  const s = parseInt10(small);
  const l = parseInt10(large);
  const errors = compact<'step_small' | 'step_large'>({
    step_small: s === undefined ? 'Целое число' : s < 1 || s > 1000 ? 'От 1 до 1000' : undefined,
    step_large: l === undefined ? 'Целое число' : l < 1 || l > 5000 ? 'От 1 до 5000' : undefined,
  });
  if (!hasErrors(errors) && s! >= l!) errors.step_large = 'Больше малого шага';
  if (hasErrors(errors)) return { ok: false, errors };
  return { ok: true, value: { step_small: s!, step_large: l! } };
}
