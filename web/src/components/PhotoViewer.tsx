// Фото позиции крупно (ФТ-10.4.1): тёмная подложка на весь экран, картинка целиком.
// Закрывается нажатием в любое место, кнопкой и Esc; фокус возвращается на кнопку,
// которая открыла просмотр (useModalLayer). Появление — прозрачность и лёгкий масштаб.
import { IconX } from '@tabler/icons-react';
import { useRef } from 'react';
import { createPortal } from 'react-dom';
import { Ico } from './ui';
import { usePresence } from './hooks';
import { useModalLayer } from './Sheet';

const LEAVE_MS = 200;

export function PhotoViewer({ open, onClose, src, alt }: { open: boolean; onClose: () => void; src: string; alt: string }) {
  const { mounted, leaving } = usePresence(open, LEAVE_MS);
  if (!mounted) return null;
  return createPortal(<ViewerBody leaving={leaving} onClose={onClose} src={src} alt={alt} />, document.body);
}

function ViewerBody({ leaving, onClose, src, alt }: { leaving: boolean; onClose: () => void; src: string; alt: string }) {
  const layer = useRef<HTMLDivElement>(null);
  const close = useModalLayer(layer, onClose);
  return (
    <div
      ref={layer}
      className={`photo-viewer${leaving ? ' is-leaving' : ''}`}
      role="dialog"
      aria-modal="true"
      aria-label={alt}
      tabIndex={-1}
      inert={leaving || undefined}
      onClick={() => close.current()}
    >
      <img className="photo-viewer__img" src={src} alt={alt} decoding="async" draggable={false} />
      <button type="button" className="g-btn g-btn--icon photo-viewer__close" aria-label="Закрыть" data-autofocus onClick={() => close.current()}>
        <Ico icon={IconX} />
      </button>
    </div>
  );
}
