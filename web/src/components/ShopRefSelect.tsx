// Выбор магазина или места с пунктом «Новый…» прямо в списке (ФТ-2.2: место
// добавляется и из формы покупки). Новое имя вводится на месте, без ухода с формы.
import { IconCheck, IconX } from '@tabler/icons-react';
import { useState } from 'react';
import { refNameError } from '../lib/refsValidate';
import { busyProps, Ico } from './ui';
import { errorText } from '../api';

const NEW = '__new';

type Props = {
  id: string;
  label: string;
  value: number | null;
  onChange: (id: number | null) => void;
  options: readonly { id: number; name: string }[];
  /** Подпись пустого выбора: «Без магазина», «Как в прошлый раз». */
  noneLabel: string;
  newLabel: string;
  newPlaceholder: string;
  /** Создаёт запись и возвращает её id. */
  onCreate: (name: string) => Promise<number>;
};

export function ShopRefSelect({ id, label, value, onChange, options, noneLabel, newLabel, newPlaceholder, onCreate }: Props) {
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const errId = `${id}-err`;

  const create = async () => {
    const e = refNameError(name);
    if (e) return setError(e);
    setBusy(true);
    try {
      const newId = await onCreate(name.trim());
      onChange(newId);
      setAdding(false);
      setName('');
      setError('');
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    // При вводе нового имени поле занимает всю ширину формы (.shop-ref--adding)
    <div className={`g-field${adding ? ' shop-ref--adding' : ''}${error ? ' g-field--error' : ''}`}>
      <label className="g-label" htmlFor={id}>
        {label}
      </label>
      {adding ? (
        <div className="shop-new-ref">
          <input
            id={id}
            className="g-input"
            value={name}
            maxLength={40}
            autoFocus
            placeholder={newPlaceholder}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errId : undefined}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                if (!busy) void create();
              } else if (e.key === 'Escape') setAdding(false);
            }}
          />
          <button type="button" className="g-btn g-btn--icon g-btn--accent g-press" aria-label={name.trim() ? `Добавить «${name.trim()}»` : `${label}: добавить новое`} onClick={() => !busy && void create()} {...busyProps(busy)}>
            <Ico icon={IconCheck} />
          </button>
          <button
            type="button"
            className="g-btn g-btn--icon g-btn--ghost"
            aria-label="Отменить"
            onClick={() => {
              setAdding(false);
              setError('');
            }}
          >
            <Ico icon={IconX} />
          </button>
        </div>
      ) : (
        <select
          id={id}
          className="g-input"
          value={value ?? ''}
          onChange={(e) => {
            const v = e.target.value;
            if (v === NEW) {
              setAdding(true);
              return;
            }
            onChange(v ? Number(v) : null);
          }}
        >
          <option value="">{noneLabel}</option>
          {options.map((o) => (
            <option key={o.id} value={o.id}>
              {o.name}
            </option>
          ))}
          <option value={NEW}>{newLabel}</option>
        </select>
      )}
      {error && (
        <span className="g-field__error" id={errId}>
          {error}
        </span>
      )}
    </div>
  );
}
