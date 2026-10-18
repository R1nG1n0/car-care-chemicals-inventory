// Сканер штрихкодов (ФТ-9.1, 9.6, 9.7): полноэкранный слой с видео задней камеры,
// рамка-видоискатель с бегущей линией, «поймал» — рамка сжимается, вспышка и
// галочка, короткая вибрация. Фонарик — если камера его умеет. «Ввести вручную»
// и объяснение, если камеры нет или доступа к ней нет. Стили — styles/scanner.css.
//
// Кадры распознаются ~8 раз в секунду (после каждого разбора — пауза TICK_MS),
// только область под рамкой с запасом. При закрытии камера выключается (все
// треки останавливаются). Фокус и Esc — как у шторки (useModalLayer).
import { IconBolt, IconBoltOff, IconCameraOff, IconCheck, IconX } from '@tabler/icons-react';
import { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { cameraFailure, parseScanned } from '../lib/barcode';
import { REDUCED_MOTION, usePresence } from './hooks';
import { buzz, CameraError, grabFrame, loadReader, openCamera, setTorch, stopStream, torchSupported, type Reader } from './scanEngine';
import { ScannerPanel, type ScanFail } from './ScannerPanel';
import { useModalLayer } from './Sheet';
import { Ico } from './ui';

export type ScannerProps = {
  open: boolean;
  onClose: () => void;
  /** Код из цифр (камера или ручной ввод). Слой не закрывается сам. */
  onCode: (code: string) => void;
  /** Заголовок слоя. */
  title?: string;
  /** Пока вызывающий ищет код — подпись под рамкой вместо «Код …». */
  busyText?: string | null;
};

const LEAVE_MS = 200;
const TICK_MS = 110;
/** Сколько показывать «поймал» перед передачей кода. */
const CAUGHT_MS = 420;
const NOTE_MS = 2200;
/** Сколько кадров подряд должен дать один и тот же код, чтобы его принять. */
const CONFIRM_FRAMES = 2;

export function Scanner(props: ScannerProps) {
  const { mounted, leaving } = usePresence(props.open, LEAVE_MS);
  if (!mounted) return null;
  return createPortal(<ScannerBody {...props} leaving={leaving} />, document.body);
}

type Status = 'starting' | 'loading' | 'scanning';

function ScannerBody({ onClose, onCode, title = 'Сканер штрихкодов', busyText, leaving }: ScannerProps & { leaving: boolean }) {
  const root = useRef<HTMLDivElement>(null);
  const video = useRef<HTMLVideoElement>(null);
  const frame = useRef<HTMLDivElement>(null);
  const track = useRef<MediaStreamTrack | null>(null);
  const closeBtn = useRef<HTMLButtonElement>(null);
  const caughtRef = useRef(false);
  const lastBad = useRef({ text: '', at: 0 });
  const onCodeRef = useRef(onCode);
  const titleId = useId();
  const close = useModalLayer(root, onClose);

  const [manual, setManual] = useState(false);
  const [hidden, setHidden] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [status, setStatus] = useState<Status>('starting');
  const [fail, setFail] = useState<ScanFail | null>(null);
  const [caught, setCaught] = useState<string | null>(null);
  const [note, setNote] = useState<{ id: number; text: string } | null>(null);
  const [torch, setTorchState] = useState<{ can: boolean; on: boolean }>({ can: false, on: false });

  useEffect(() => {
    onCodeRef.current = onCode;
  }, [onCode]);

  const camera = !manual && !fail && !leaving && !hidden;
  const typing = manual || fail !== null;

  // Вкладка или приложение ушли в фон — камеру выключаем (на компьютере гаснет
  // индикатор, цикл не тикает впустую), при возврате включаем заново. В iOS-PWA
  // доступ к камере при этом может спроситься ещё раз — так устроена платформа.
  useEffect(() => {
    const onVis = () => {
      if (caughtRef.current) return;
      setHidden(document.hidden);
      if (document.hidden) setStatus('starting');
    };
    document.addEventListener('visibilitychange', onVis);
    return () => document.removeEventListener('visibilitychange', onVis);
  }, []);

  // Клавиатура iOS не уменьшает layout viewport: нижнюю панель с полем ввода
  // поднимаем на высоту клавиатуры по visualViewport. На компьютере сдвиг нулевой.
  useEffect(() => {
    const vv = window.visualViewport;
    const el = root.current;
    if (!typing || !vv || !el) return;
    const sync = () => {
      const kb = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop));
      el.style.setProperty('--scan-kb', `${kb}px`);
      el.style.setProperty('--scan-vh', `${Math.round(vv.height)}px`);
    };
    sync();
    vv.addEventListener('resize', sync);
    vv.addEventListener('scroll', sync);
    return () => {
      vv.removeEventListener('resize', sync);
      vv.removeEventListener('scroll', sync);
      el.style.removeProperty('--scan-kb');
      el.style.removeProperty('--scan-vh');
    };
  }, [typing]);

  // Камера и цикл распознавания. Перезапуск — «Попробовать снова» (attempt)
  // или возврат из ручного ввода; выход из эффекта выключает камеру.
  useEffect(() => {
    const v = video.current;
    if (!camera || !v) return;
    let dead = false;
    let stream: MediaStream | null = null;
    let timer = 0;
    const canvas = document.createElement('canvas');
    const readerReady = loadReader();
    readerReady.catch(() => undefined); // ошибку покажет ожидание ниже

    // Код принимается, когда его увидели на CONFIRM_FRAMES кадрах подряд
    let cand = { code: '', n: 0 };

    const onText = (text: string) => {
      const r = parseScanned(text);
      if (r.ok) {
        cand = cand.code === r.code ? { code: r.code, n: cand.n + 1 } : { code: r.code, n: 1 };
        if (cand.n < CONFIRM_FRAMES) return;
        caughtRef.current = true;
        buzz();
        setCaught(r.code);
        return;
      }
      // Один и тот же QR в кадре — не мигаем сообщением на каждом кадре
      const now = Date.now();
      if (lastBad.current.text === text && now - lastBad.current.at < NOTE_MS + 800) return;
      lastBad.current = { text, at: now };
      setNote((n) => ({ id: (n?.id ?? 0) + 1, text: r.error }));
    };

    const run = async () => {
      try {
        stream = await openCamera();
      } catch (err) {
        if (!dead) setFail(err instanceof CameraError ? err.kind : cameraFailure(err));
        return;
      }
      if (dead) {
        stopStream(stream);
        return;
      }
      // iPhone: без playsinline видео уходит в полноэкранный плеер, без muted не стартует само
      v.muted = true;
      v.setAttribute('playsinline', '');
      v.srcObject = stream;
      v.play().catch(() => undefined);
      const t = stream.getVideoTracks()[0];
      track.current = t ?? null;
      t?.addEventListener('ended', onTrackEnded);
      setTorchState({ can: torchSupported(t), on: false });
      setStatus('loading');

      let read: Reader;
      try {
        read = await readerReady;
      } catch {
        if (!dead) setFail('reader');
        return;
      }
      if (dead) return;
      setStatus('scanning');

      const tick = async () => {
        if (dead || caughtRef.current) return;
        const box = frame.current;
        const img = !document.hidden && box ? grabFrame(v, box, canvas) : null;
        if (img) {
          try {
            const texts = await read(img);
            if (!dead && !caughtRef.current) {
              if (texts[0] !== undefined) onText(texts[0]);
              else cand = { code: '', n: 0 };
            }
          } catch {
            // Кадр не разобрался — следующий
          }
        }
        if (!dead && !caughtRef.current) timer = window.setTimeout(() => void tick(), TICK_MS);
      };
      void tick();
    };
    void run();

    // Камеру могли остановить снаружи (звонок, другое приложение): трек закончился,
    // замолчал или видео встало на паузу — включаем заново
    function onTrackEnded() {
      if (!dead && !caughtRef.current) setAttempt((a) => a + 1);
    }
    const onVisible = () => {
      if (document.hidden || caughtRef.current) return;
      const t = track.current;
      if (t && (t.readyState === 'ended' || t.muted || v.paused)) setAttempt((a) => a + 1);
    };
    document.addEventListener('visibilitychange', onVisible);

    return () => {
      dead = true;
      window.clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisible);
      track.current?.removeEventListener('ended', onTrackEnded);
      stopStream(stream);
      track.current = null;
      v.srcObject = null;
      canvas.width = canvas.height = 0;
    };
  }, [camera, attempt]);

  // «Поймал»: показать подтверждение, затем отдать код
  useEffect(() => {
    if (caught === null) return;
    const t = window.setTimeout(() => onCodeRef.current(caught), window.matchMedia(REDUCED_MOTION).matches ? 150 : CAUGHT_MS);
    return () => window.clearTimeout(t);
  }, [caught]);

  useEffect(() => {
    if (!note) return;
    const t = window.setTimeout(() => setNote(null), NOTE_MS);
    return () => window.clearTimeout(t);
  }, [note]);

  const restart = () => {
    closeBtn.current?.focus({ preventScroll: true });
    setFail(null);
    setStatus('starting');
    setAttempt((a) => a + 1);
  };
  const switchManual = (on: boolean) => {
    // Кнопка переключения исчезнет вместе с панелью — фокус на «Закрыть» (поле ввода берёт его само)
    if (!on) closeBtn.current?.focus({ preventScroll: true });
    if (!on) setStatus('starting');
    setTorchState({ can: false, on: false });
    setManual(on);
  };
  const toggleTorch = () => {
    const t = track.current;
    if (!t) return;
    const next = !torch.on;
    setTorch(t, next)
      .then(() => setTorchState({ can: true, on: next }))
      .catch(() => setTorchState({ can: false, on: false }));
  };

  const statusText =
    caught !== null
      ? (busyText ?? `Код ${caught}`)
      : status === 'starting'
        ? 'Включаю камеру…'
        : status === 'loading'
          ? 'Загружаю сканер…'
          : 'Наведите рамку на штрихкод';

  return (
    <div
      ref={root}
      className={`scan-layer${leaving ? ' is-leaving' : ''}`}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      tabIndex={-1}
      inert={leaving || undefined}
    >
      <div className="scan__stage">
        <video ref={video} className="scan__video" autoPlay muted playsInline aria-hidden="true" hidden={!camera} />
        {camera ? (
          <div ref={frame} className={`scan__frame${caught !== null ? ' is-caught' : ''}${status === 'scanning' ? ' is-live' : ''}`} aria-hidden="true">
            <i className="scan__corner scan__corner--tl" />
            <i className="scan__corner scan__corner--tr" />
            <i className="scan__corner scan__corner--bl" />
            <i className="scan__corner scan__corner--br" />
            <span className="scan__line" />
            {caught !== null && (
              <>
                <span className="scan__flash" />
                <span className="scan__ok">
                  <Ico icon={IconCheck} size={28} />
                </span>
              </>
            )}
          </div>
        ) : (
          <div className="scan__off" aria-hidden="true">
            <Ico icon={IconCameraOff} size={40} />
          </div>
        )}
        <div className="scan__bar">
          <h2 className="scan__title" id={titleId}>
            {title}
          </h2>
          {camera && torch.can && (
            <button type="button" className="scan__ctl" aria-pressed={torch.on} aria-label="Фонарик" onClick={toggleTorch}>
              <Ico icon={torch.on ? IconBoltOff : IconBolt} />
            </button>
          )}
          <button type="button" className="scan__ctl" aria-label="Закрыть сканер" data-autofocus ref={closeBtn} onClick={() => close.current()}>
            <Ico icon={IconX} />
          </button>
        </div>
      </div>
      <div className="scan__dock">
        {note && camera && (
          <p key={note.id} className="scan__note" role="status">
            {note.text}
          </p>
        )}
        <ScannerPanel status={statusText} busy={busyText ?? null} fail={fail} manual={manual} onManual={switchManual} onRetry={restart} onCode={(c) => onCodeRef.current(c)} />
      </div>
    </div>
  );
}
