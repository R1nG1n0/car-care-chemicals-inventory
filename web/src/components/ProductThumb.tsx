// Миниатюра фото позиции в списке (ФТ-10.4.1): 40 × 40 px, только если фото есть.
// Размеры заданы, поэтому вёрстка не прыгает; loading="lazy" — картинки далеко
// за экраном не грузятся; фон до загрузки — цвет подложки.
import { useState } from 'react';
import { photoUrl } from '../lib/photo';

export function ProductThumb({ id, updatedAt }: { id: number; updatedAt: number | null }) {
  const src = photoUrl(id, updatedAt);
  const [failed, setFailed] = useState<string | null>(null);
  // Не загрузилась (удалена с другого устройства, нет связи) — место остаётся, картинки нет
  if (!src) return null;
  return (
    <img
      className={`stock-thumb${failed === src ? ' is-broken' : ''}`}
      src={src}
      alt=""
      width={40}
      height={40}
      loading="lazy"
      decoding="async"
      draggable={false}
      onError={() => setFailed(src)}
    />
  );
}
