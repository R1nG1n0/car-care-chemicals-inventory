// Нижняя панель сканера: подсказка и «Ввести вручную», поле ручного ввода,
// объяснение, если камеры нет или к ней нет доступа (ФТ-9.7).
import { IconCamera, IconKeyboard } from '@tabler/icons-react';
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { parseManual, type CameraFailure } from '../lib/barcode';
import { busyProps, Ico } from './ui';

export type ScanFail = CameraFailure | 'reader';

type FailInfo = { title: string; text: string; steps?: string[]; retry: boolean };

/** Почему камера не работает и что с этим сделать — без технических слов. */
export function failInfo(kind: ScanFail): FailInfo {
  switch (kind) {
    case 'insecure':
      return {
        title: 'Камера работает только по HTTPS',
        text: 'Браузер даёт доступ к камере только защищённым сайтам. Откройте garage по адресу, который начинается с https://, или введите код вручную.',
        retry: false,
      };
    case 'unsupported':
      return { title: 'Браузер не даёт доступ к камере', text: 'Обновите браузер или откройте garage в Safari или Chrome. Пока можно ввести код вручную.', retry: false };
    case 'denied':
      return {
        title: 'Нет доступа к камере',
        text: 'Разрешите камеру для этого сайта и нажмите «Попробовать снова»:',
        steps: [
          'Safari на iPhone: «аА» слева в адресной строке → «Настройки веб-сайта» → «Камера» → «Разрешить».',
          'garage на экране «Домой»: Настройки iPhone → Safari → «Камера» → «Разрешить».',
          'Chrome: значок слева от адреса → «Разрешения» → «Камера».',
        ],
        retry: true,
      };
    case 'nocamera':
      return { title: 'Камера не найдена', text: 'На этом устройстве нет камеры. Введите цифры под штрихкодом.', retry: true };
    case 'busy':
      return { title: 'Камера занята', text: 'Её использует другое приложение. Закройте его и попробуйте снова.', retry: true };
    case 'reader':
      return { title: 'Сканер не загрузился', text: 'Проверьте связь и попробуйте снова. Код можно ввести вручную.', retry: true };
    default:
      return { title: 'Камера не включилась', text: 'Попробуйте снова или введите код вручную.', retry: true };
  }
}

type PanelProps = {
  /** Подпись под рамкой: «Наведите рамку…», «Ищу позицию…». */
  status: string;
  /** Пока идёт поиск кода: подпись («Ищу позицию…») и «Готово» не нажимается. */
  busy: string | null;
  fail: ScanFail | null;
  manual: boolean;
  onManual: (on: boolean) => void;
  onRetry: () => void;
  onCode: (code: string) => void;
};

export function ScannerPanel({ status, busy, fail, manual, onManual, onRetry, onCode }: PanelProps) {
  if (fail) {
    const info = failInfo(fail);
    return (
      <div className="scan__panel">
        <div className="scan__fail" role="alert">
          <h3 className="scan__fail-title">{info.title}</h3>
          <p className="scan__fail-text">{info.text}</p>
          {info.steps && (
            <ul className="scan__steps">
              {info.steps.map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ul>
          )}
        </div>
        <BusyLine text={busy} />
        <ManualForm onCode={onCode} autoFocus={false} busy={busy !== null} />
        {info.retry && (
          <button type="button" className="g-btn g-btn--block g-press" onClick={onRetry}>
            <Ico icon={IconCamera} /> Попробовать снова
          </button>
        )}
      </div>
    );
  }
  if (manual) {
    return (
      <div className="scan__panel">
        <BusyLine text={busy} />
        <ManualForm onCode={onCode} autoFocus busy={busy !== null} />
        <button type="button" className="g-btn g-btn--block g-btn--ghost" onClick={() => onManual(false)}>
          <Ico icon={IconCamera} /> Сканировать камерой
        </button>
      </div>
    );
  }
  return (
    <div className="scan__panel">
      <p className="scan__status" aria-live="polite">
        {status}
      </p>
      <button type="button" className="g-btn g-btn--block g-press" onClick={() => onManual(true)}>
        <Ico icon={IconKeyboard} /> Ввести вручную
      </button>
    </div>
  );
}

function BusyLine({ text }: { text: string | null }) {
  return text === null ? null : (
    <p className="scan__status" role="status">
      {text}
    </p>
  );
}

function ManualForm({ onCode, autoFocus, busy }: { onCode: (code: string) => void; autoFocus: boolean; busy: boolean }) {
  const [v, setV] = useState('');
  const [error, setError] = useState<string | null>(null);
  const input = useRef<HTMLInputElement>(null);
  // Фокус ставим сами: слой сканера уже открыт, [data-autofocus] срабатывает только при открытии
  useEffect(() => {
    if (autoFocus) input.current?.focus({ preventScroll: true });
  }, [autoFocus]);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    const r = parseManual(v);
    if (!r.ok) {
      setError(r.error);
      input.current?.focus();
      return;
    }
    onCode(r.code);
  };
  return (
    <form className="scan__manual" onSubmit={submit} noValidate>
      <div className={`g-field${error ? ' g-field--error' : ''}`}>
        <label className="g-label" htmlFor="scan-manual">
          Цифры под штрихкодом
        </label>
        <input
          className="g-input"
          id="scan-manual"
          inputMode="numeric"
          autoComplete="off"
          enterKeyHint="done"
          placeholder="4600000000000"
          maxLength={48}
          value={v}
          ref={input}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? 'scan-manual-err' : undefined}
          onChange={(e) => {
            setV(e.target.value);
            setError(null);
          }}
        />
        {error && (
          <span className="g-field__error" id="scan-manual-err">
            {error}
          </span>
        )}
      </div>
      <button type="submit" className="g-btn g-btn--cta g-btn--block g-press" {...busyProps(busy)}>
        Готово
      </button>
    </form>
  );
}
