// Редактор пропорций позиции в мл (ФТ-17.1): строки «название + 1:N», добавить,
// удалить, переставить стрелками (порядок задаёт владелец), до 10 штук.
import { IconArrowDown, IconArrowUp, IconPlus, IconX } from '@tabler/icons-react';
import { useLayoutEffect, useRef, useState } from 'react';
import { MAX_DILUTIONS, MAX_DILUTION_NAME, type DilutionDraft, type DilutionRowErrors } from '../lib/dilution';
import { moveItem } from '../lib/refsOrder';
import { Ico } from './ui';

type Props = {
  value: DilutionDraft[];
  onChange: (v: DilutionDraft[]) => void;
  errors: Record<number, DilutionRowErrors>;
  formError?: string | undefined;
  idPrefix?: string;
};

export function DilutionsEditor({ value, onChange, errors, formError, idPrefix = 'np' }: Props) {
  const seq = useRef(1000);
  const refocus = useRef<string | null>(null);
  const [fresh, setFresh] = useState<number | null>(null);
  const id = (key: number, part: string) => `${idPrefix}-dil-${key}-${part}`;

  // Стрелка переставила строку — фокус остаётся на той же кнопке на новом месте
  useLayoutEffect(() => {
    if (refocus.current) document.getElementById(refocus.current)?.focus();
    refocus.current = null;
  });

  const set = (key: number, p: Partial<DilutionDraft>) => onChange(value.map((d) => (d.key === key ? { ...d, ...p } : d)));
  const move = (i: number, to: number, part: 'up' | 'down') => {
    if (to < 0 || to >= value.length) return;
    refocus.current = id(value[i]!.key, part);
    onChange(moveItem(value, i, to));
  };
  const add = () => {
    const key = seq.current++;
    setFresh(key);
    onChange([...value, { key, name: '', ratio: '' }]);
    // Фокус — в название новой строки (после отрисовки)
    requestAnimationFrame(() => document.getElementById(id(key, 'name'))?.focus());
  };

  return (
    <div className={`g-field${formError ? ' g-field--error' : ''}`} id={`${idPrefix}-dilutions`} tabIndex={-1}>
      <span className="g-label">
        Пропорции разбавления <span className="g-field__opt">· необязательно</span>
      </span>
      {value.length === 0 && <p className="g-muted">Нет пропорций. Добавьте, например «Предмойка 1:10» — расход можно будет вводить объёмом раствора.</p>}
      <div className="dil-rows">
        {value.map((d, i) => {
          const e = errors[d.key];
          return (
            <div key={d.key} className={`dil-row${fresh === d.key ? ' g-enter' : ''}`} role="group" aria-label={`Пропорция ${i + 1}`}>
              <div className={e?.name ? 'g-field--error' : undefined}>
                <input
                  className="g-input"
                  id={id(d.key, 'name')}
                  aria-label={`Название пропорции ${i + 1}`}
                  placeholder="Предмойка"
                  maxLength={MAX_DILUTION_NAME}
                  autoComplete="off"
                  value={d.name}
                  aria-invalid={e?.name ? true : undefined}
                  aria-describedby={e?.name ? id(d.key, 'err') : undefined}
                  onChange={(ev) => set(d.key, { name: ev.target.value })}
                />
              </div>
              <div className="dil-row__line">
                <div className={`g-qty__value dil-row__ratio${e?.ratio ? ' g-field--error' : ''}`}>
                  <b aria-hidden="true">1:</b>
                  <input
                    id={id(d.key, 'ratio')}
                    inputMode="numeric"
                    autoComplete="off"
                    maxLength={4}
                    placeholder="10"
                    aria-label={`Пропорция ${i + 1}: частей воды на часть концентрата`}
                    value={d.ratio}
                    aria-invalid={e?.ratio ? true : undefined}
                    aria-describedby={e?.ratio ? id(d.key, 'err') : undefined}
                    onChange={(ev) => set(d.key, { ratio: ev.target.value.replace(/[^\d\s]/gu, '') })}
                  />
                </div>
                <span className="dil-row__tools">
                  <button type="button" className="g-btn g-btn--icon g-btn--ghost" id={id(d.key, 'up')} aria-label={`Выше: ${d.name || `пропорция ${i + 1}`}`} aria-disabled={i === 0 || undefined} onClick={() => move(i, i - 1, 'up')}>
                    <Ico icon={IconArrowUp} />
                  </button>
                  <button type="button" className="g-btn g-btn--icon g-btn--ghost" id={id(d.key, 'down')} aria-label={`Ниже: ${d.name || `пропорция ${i + 1}`}`} aria-disabled={i === value.length - 1 || undefined} onClick={() => move(i, i + 1, 'down')}>
                    <Ico icon={IconArrowDown} />
                  </button>
                  <button type="button" className="g-btn g-btn--icon g-btn--ghost" aria-label={`Удалить: ${d.name || `пропорция ${i + 1}`}`} onClick={() => onChange(value.filter((x) => x.key !== d.key))}>
                    <Ico icon={IconX} />
                  </button>
                </span>
              </div>
              {e && (
                <span className="g-field__error" id={id(d.key, 'err')}>
                  {[e.name, e.ratio && `Пропорция: ${e.ratio}`].filter(Boolean).join('. ')}
                </span>
              )}
            </div>
          );
        })}
      </div>
      {value.length < MAX_DILUTIONS ? (
        <button type="button" className="g-btn g-btn--block g-press" onClick={add}>
          <Ico icon={IconPlus} /> Добавить пропорцию
        </button>
      ) : (
        <span className="g-field__hint">Предел — {MAX_DILUTIONS} пропорций</span>
      )}
      {formError && (
        <span className="g-field__error" role="alert">
          {formError}
        </span>
      )}
    </div>
  );
}
