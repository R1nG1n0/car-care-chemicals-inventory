// Общие куски экранов «Купить», «Шаблоны», «Ещё»: шапка с «Назад»,
// загрузка, «не загрузилось», класс цвета категории.
import { IconChevronLeft, IconWifiOff } from '@tabler/icons-react';
import type { ReactNode } from 'react';
import { Link } from 'react-router';
import type { CatColor } from '../api';
import { isNetworkError } from '../api';
import { Empty, Ico } from './ui';

/** Цвет категории классом (.shop-cat--blue задаёт --cat и --sw): атрибут style запрещён CSP. */
export const catClass = (c: CatColor | undefined) => `shop-cat--${c ?? 'slate'}`;

export function ShopBack({ to, label }: { to: string; label: string }) {
  return (
    <Link to={to} className="g-btn g-btn--ghost g-btn--sm shop-back">
      <Ico icon={IconChevronLeft} />
      {label}
    </Link>
  );
}

/** Строка заголовка: слева «Назад» или заголовок, справа действие. */
export function ShopHead({ children }: { children: ReactNode }) {
  return <div className="shop-between">{children}</div>;
}

/** Подложки загрузки в форме списка строк (И-3а). */
export function ShopSkelList({ rows = 4, label }: { rows?: number; label: string }) {
  return (
    <div className="g-skel shop-skel" aria-busy="true" aria-label={label}>
      {Array.from({ length: rows }, (_, i) => (
        <div className="g-skel__body" key={i}>
          <div className="g-skel__line" />
          <div className="g-skel__line g-skel__line--short" />
        </div>
      ))}
    </div>
  );
}

/** Данные не загрузились: «нет связи» или ошибка сервера, с «Повторить». */
export function ShopLoadError({ id, error, onRetry, what }: { id: string; error: unknown; onRetry: () => void; what: string }) {
  const offline = isNetworkError(error);
  return (
    <Empty
      id={id}
      icon={IconWifiOff}
      error
      title={offline ? 'Сервер недоступен' : `${what} не загрузились`}
      text={offline ? 'Проверьте, что VPN включён.' : error instanceof Error ? error.message : 'Попробуйте ещё раз.'}
      actions={
        <button type="button" className="g-btn g-btn--block g-press" onClick={onRetry}>
          Повторить
        </button>
      }
    />
  );
}

/** Текст ошибки мутации для показа под полем или кнопкой. */
export function errorText(err: unknown): string {
  if (isNetworkError(err)) return 'Нет связи — не сохранилось. Проверьте VPN и повторите';
  return err instanceof Error ? err.message : 'Не получилось';
}
