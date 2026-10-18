// Общие куски экранов на классах дизайн-системы «Сумерки».
import { IconWifiOff, type TablerIcon } from '@tabler/icons-react';
import type { InputHTMLAttributes, ReactNode, Ref } from 'react';
import { isNetworkError, OFFLINE_TEXT, OFFLINE_TITLE } from '../api';

/** Иконка Tabler: контурная, обводка 2, 20 px, декоративная. */
export function Ico({ icon: I, size = 20 }: { icon: TablerIcon; size?: number }) {
  return <I className="ti" size={size} stroke={2} aria-hidden="true" focusable="false" />;
}

export function Logo({ size = 'md' }: { size?: 'md' | 'lg' }) {
  return (
    <div className={`app-logo app-logo--${size}`}>
      garage<b>.</b>
    </div>
  );
}

type FieldProps = InputHTMLAttributes<HTMLInputElement> & {
  id: string;
  label: string;
  error?: string | undefined;
  hint?: string | undefined;
  ref?: Ref<HTMLInputElement>;
};

/** Поле с подписью и ошибкой, связанной через aria-describedby. */
export function Field({ id, label, error, hint, ref, ...input }: FieldProps) {
  const errId = `${id}-err`;
  const hintId = `${id}-hint`;
  const described = [hint ? hintId : '', error ? errId : ''].filter(Boolean).join(' ') || undefined;
  return (
    <div className={`g-field${error ? ' g-field--error' : ''}`}>
      <label className="g-label" htmlFor={id}>
        {label}
      </label>
      <input ref={ref} className="g-input" id={id} aria-invalid={error ? true : undefined} aria-describedby={described} {...input} />
      {hint && (
        <span className="g-field__hint" id={hintId}>
          {hint}
        </span>
      )}
      {error && (
        <span className="g-field__error" id={errId}>
          {error}
        </span>
      )}
    </div>
  );
}

type EmptyProps = {
  id: string;
  icon: TablerIcon;
  title: string;
  text: ReactNode;
  error?: boolean;
  actions?: ReactNode;
  meta?: ReactNode;
};

/** Пустое состояние или «нет связи» (.g-empty--error). */
export function Empty({ id, icon, title, text, error, actions, meta }: EmptyProps) {
  return (
    <section className={`g-empty${error ? ' g-empty--error' : ''}`} aria-labelledby={id}>
      <span className="g-empty__icon">
        <Ico icon={icon} size={36} />
      </span>
      <h2 className="g-empty__title" id={id}>
        {title}
      </h2>
      <p className="g-empty__text">{text}</p>
      {actions && <div className="g-empty__actions">{actions}</div>}
      {meta && <div className="g-empty__meta">{meta}</div>}
    </section>
  );
}

/** Свойства кнопки в состоянии отправки: неактивна, но остаётся в фокусе. */
export function busyProps(busy: boolean) {
  return busy ? ({ 'aria-busy': true, 'aria-disabled': true } as const) : {};
}

/** Строка заголовка: слева «Назад» или заголовок, справа действие. */
export function HeadRow({ children }: { children: ReactNode }) {
  return <div className="app-between">{children}</div>;
}

/** Подложки загрузки в форме списка строк (И-3а). */
export function SkelList({ rows = 4, label }: { rows?: number; label: string }) {
  return (
    <div className="g-skel app-skel" aria-busy="true" aria-label={label}>
      {Array.from({ length: rows }, (_, i) => (
        <div className="g-skel__body" key={i}>
          <div className="g-skel__line" />
          <div className="g-skel__line g-skel__line--short" />
        </div>
      ))}
    </div>
  );
}

/** Данные не загрузились: «нет связи» или ошибка сервера, с «Повторить». Один текст на весь сайт. */
export function LoadError({ id, error, onRetry, what }: { id: string; error: unknown; onRetry: () => void; what: string }) {
  const offline = isNetworkError(error);
  return (
    <Empty
      id={id}
      icon={IconWifiOff}
      error
      title={offline ? OFFLINE_TITLE : `${what}: не загрузилось`}
      text={offline ? OFFLINE_TEXT : error instanceof Error ? error.message : 'Попробуйте ещё раз.'}
      actions={
        <button type="button" className="g-btn g-btn--block g-press" onClick={onRetry}>
          Повторить
        </button>
      }
    />
  );
}
