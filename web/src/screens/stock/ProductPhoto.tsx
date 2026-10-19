// Фото на карточке позиции (ФТ-10.4): нет фото — «Добавить фото»; есть — снимок,
// по нажатию открывается крупно, внизу «Заменить» и «Удалить» (с подтверждением).
// Выбор файла — <input type="file" accept="image/*" без capture>: на iPhone браузер
// сам предлагает «Снять фото / Медиатека / Выбрать файл». Снимок сжимается в браузере
// (lib/photoEncode), на сервер уходит JPEG ≤ 300 КБ. Предпросмотр — после загрузки.
import { IconCamera, IconPhotoEdit, IconTrash } from '@tabler/icons-react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRef, useState, type ChangeEvent } from 'react';
import { errorText, inventory, type ProductSummary } from '../../api';
import { invalidateStock } from '../../app/queries';
import { useToast } from '../../app/toast';
import { PhotoViewer } from '../../components/PhotoViewer';
import { Sheet } from '../../components/Sheet';
import { Ico } from '../../components/ui';
import { PhotoError, photoUrl } from '../../lib/photo';
import { compressPhoto } from '../../lib/photoEncode';

export function ProductPhoto({ product: p }: { product: ProductSummary }) {
  const qc = useQueryClient();
  const toast = useToast();
  const input = useRef<HTMLInputElement>(null);
  const [viewer, setViewer] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const url = photoUrl(p.id, p.photo_updated_at);

  const failed = (err: unknown) => toast.show({ text: err instanceof PhotoError ? err.message : errorText(err) });
  const upload = useMutation({
    mutationFn: async (file: File) => inventory.setPhoto(p.id, await compressPhoto(file)),
    onSuccess: async () => {
      await invalidateStock(qc);
      toast.show({ text: 'Фото сохранено' });
    },
    onError: failed,
  });
  const remove = useMutation({
    mutationFn: () => inventory.deletePhoto(p.id),
    onSuccess: async () => {
      setConfirm(false);
      await invalidateStock(qc);
      toast.show({ text: 'Фото удалено' });
    },
    onError: (err) => {
      setConfirm(false);
      failed(err);
    },
  });
  const busy = upload.isPending || remove.isPending;

  const picked = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    // Сброс: тот же снимок можно выбрать повторно после ошибки
    e.target.value = '';
    if (file) upload.mutate(file);
  };
  const pick = () => input.current?.click();

  return (
    <section className="stock-photo" aria-labelledby="item-photo-h">
      <h2 className="g-label" id="item-photo-h">
        Фото
      </h2>
      {url ? (
        <>
          <button type="button" className="stock-photo__view g-press" aria-label="Открыть фото крупно" onClick={() => setViewer(true)}>
            <img src={url} alt={`Фото: ${p.name}`} decoding="async" draggable={false} />
          </button>
          <div className="app-two">
            <button type="button" className="g-btn g-press" onClick={pick} disabled={busy} aria-busy={upload.isPending || undefined}>
              <Ico icon={IconPhotoEdit} /> {upload.isPending ? 'Загружаю…' : 'Заменить'}
            </button>
            <button type="button" className="g-btn g-press" onClick={() => setConfirm(true)} disabled={busy}>
              <Ico icon={IconTrash} /> Удалить
            </button>
          </div>
        </>
      ) : (
        <>
          <button
            type="button"
            className="g-btn g-btn--block g-press"
            onClick={pick}
            disabled={busy}
            aria-busy={upload.isPending || undefined}
            aria-describedby="item-photo-hint"
          >
            <Ico icon={IconCamera} /> {upload.isPending ? 'Загружаю…' : 'Добавить фото'}
          </button>
          <p className="g-field__hint" id="item-photo-hint">
            Снять или выбрать из галереи. Фото сожмётся само.
          </p>
        </>
      )}
      <input ref={input} type="file" accept="image/*" hidden onChange={picked} data-testid="photo-input" />

      {url && <PhotoViewer open={viewer} onClose={() => setViewer(false)} src={url} alt={`Фото: ${p.name}`} />}
      <Sheet open={confirm} onClose={() => setConfirm(false)} title="Удалить фото?">
        <p className="g-muted">Фото позиции «{p.name}» будет удалено. Остатки и история не изменятся.</p>
        <div className="app-two">
          <button type="button" className="g-btn g-press" onClick={() => setConfirm(false)} data-autofocus>
            Отмена
          </button>
          <button type="button" className="g-btn g-btn--danger g-press" onClick={() => remove.mutate()} aria-busy={remove.isPending || undefined}>
            Удалить
          </button>
        </div>
      </Sheet>
    </section>
  );
}
