import { describe, expect, it } from 'vitest';
import {
  encodeWithin,
  fitSize,
  PHOTO_MAX_BYTES,
  PHOTO_MAX_SIDE,
  PHOTO_SOURCE_MAX_BYTES,
  PhotoError,
  photoUrl,
  pickedFileError,
  qualitySteps,
  sniffImageType,
} from './photo';

describe('fitSize', () => {
  it('большое уменьшает до 1024 по длинной стороне, пропорции сохраняет', () => {
    expect(fitSize(4032, 3024)).toEqual({ width: 1024, height: 768 });
    expect(fitSize(3024, 4032)).toEqual({ width: 768, height: 1024 });
    expect(fitSize(2000, 2000)).toEqual({ width: 1024, height: 1024 });
  });

  it('маленькое не увеличивает', () => {
    expect(fitSize(640, 480)).toEqual({ width: 640, height: 480 });
    expect(fitSize(1024, 1024)).toEqual({ width: 1024, height: 1024 });
  });

  it('другой предел', () => {
    expect(fitSize(4000, 1000, 800)).toEqual({ width: 800, height: 200 });
  });

  it('очень вытянутое: короткая сторона не меньше 1', () => {
    expect(fitSize(40000, 10)).toEqual({ width: 1024, height: 1 });
    expect(fitSize(0, 0)).toEqual({ width: 1, height: 1 });
  });

  it('дробные размеры округляются вниз до целых', () => {
    expect(fitSize(500.9, 300.2)).toEqual({ width: 500, height: 300 });
  });

  it('длинная сторона после масштаба ровно max', () => {
    for (const [w, h] of [[4001, 3001], [5000, 3333], [1025, 1], [9999, 7777]] as const) {
      const r = fitSize(w, h);
      expect(Math.max(r.width, r.height)).toBe(PHOTO_MAX_SIDE);
    }
  });
});

describe('qualitySteps', () => {
  it('от 0,85 до 0,5 с шагом 0,05', () => {
    expect(qualitySteps()).toEqual([0.85, 0.8, 0.75, 0.7, 0.65, 0.6, 0.55, 0.5]);
  });

  it('границы', () => {
    expect(qualitySteps(0.5, 0.5)).toEqual([0.5]);
    expect(qualitySteps(0.4, 0.5)).toEqual([]);
  });
});

describe('encodeWithin', () => {
  const mk = (sizes: (side: number, q: number) => number) => {
    const calls: [number, number][] = [];
    const render = async (side: number, q: number) => {
      calls.push([side, q]);
      return { size: sizes(side, q) };
    };
    return { render, calls };
  };

  it('сразу укладывается — одно кодирование на 0,85', async () => {
    const { render, calls } = mk(() => 100_000);
    expect((await encodeWithin(render)).size).toBe(100_000);
    expect(calls).toEqual([[1024, 0.85]]);
  });

  it('снижает качество, пока не уложится, и останавливается на первом подходящем', async () => {
    const { render, calls } = mk((_, q) => Math.round(q * 500_000));
    const r = await encodeWithin(render);
    // 0,6 × 500 000 = 300 000 ≤ 307 200 — первое подходящее
    expect(r.size).toBe(300_000);
    expect(calls.map((c) => c[1])).toEqual([0.85, 0.8, 0.75, 0.7, 0.65, 0.6]);
    expect(calls.every((c) => c[0] === 1024)).toBe(true);
  });

  it('ровно предел подходит, на байт больше — нет', async () => {
    const ok = mk(() => PHOTO_MAX_BYTES);
    expect((await encodeWithin(ok.render)).size).toBe(PHOTO_MAX_BYTES);
    const over = mk((side) => (side === 1024 ? PHOTO_MAX_BYTES + 1 : 1000));
    await encodeWithin(over.render);
    expect(over.calls.at(-1)![0]).toBe(800);
  });

  it('на 1024 px не уложилось даже при 0,5 — уменьшает сторону', async () => {
    const { render, calls } = mk((side) => (side === 1024 ? 900_000 : 250_000));
    const r = await encodeWithin(render);
    expect(r.size).toBe(250_000);
    expect(calls.filter((c) => c[0] === 1024)).toHaveLength(8);
    expect(calls.at(-1)).toEqual([800, 0.85]);
  });

  it('не уложилось нигде — PhotoError', async () => {
    const { render } = mk(() => 5_000_000);
    await expect(encodeWithin(render)).rejects.toBeInstanceOf(PhotoError);
  });

  it('кодировщик вернул null — PhotoError сразу', async () => {
    let n = 0;
    await expect(
      encodeWithin(async () => {
        n++;
        return null;
      }),
    ).rejects.toThrow(/сжать/);
    expect(n).toBe(1);
  });

  it('свой предел', async () => {
    const { render } = mk((_, q) => Math.round(q * 1000));
    expect((await encodeWithin(render, 600)).size).toBe(600);
  });
});

describe('pickedFileError', () => {
  const f = (type: string, name = 'a', size = 1000) => ({ type, name, size });

  it('картинки подходят', () => {
    expect(pickedFileError(f('image/jpeg', 'IMG_1.jpg'))).toBeNull();
    expect(pickedFileError(f('image/png'))).toBeNull();
    expect(pickedFileError(f('image/heic', 'IMG_1.HEIC'))).toBeNull();
  });

  it('HEIC без типа — по расширению', () => {
    expect(pickedFileError(f('', 'IMG_2.heic'))).toBeNull();
    expect(pickedFileError(f('', 'IMG_2.HEIF'))).toBeNull();
  });

  it('не картинка', () => {
    expect(pickedFileError(f('application/pdf', 'a.pdf'))).toContain('картинка');
    expect(pickedFileError(f('', 'notes.txt'))).toContain('картинка');
    expect(pickedFileError(f('text/plain', 'a.jpg'))).toContain('картинка');
  });

  it('svg не подходит', () => {
    expect(pickedFileError(f('image/svg+xml', 'a.svg'))).toContain('SVG');
  });

  it('пустой и слишком большой файл', () => {
    expect(pickedFileError(f('image/jpeg', 'a.jpg', 0))).toBe('Файл пустой.');
    expect(pickedFileError(f('image/jpeg', 'a.jpg', PHOTO_SOURCE_MAX_BYTES + 1))).toContain('большой');
    expect(pickedFileError(f('image/jpeg', 'a.jpg', PHOTO_SOURCE_MAX_BYTES))).toBeNull();
  });
});

describe('sniffImageType', () => {
  const bytes = (...v: number[]) => Uint8Array.from(v);
  const ascii = (s: string) => [...s].map((c) => c.charCodeAt(0));

  it('JPEG, PNG, WebP', () => {
    expect(sniffImageType(bytes(0xff, 0xd8, 0xff, 0xe0))).toBe('image/jpeg');
    expect(sniffImageType(bytes(0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0))).toBe('image/png');
    expect(sniffImageType(bytes(...ascii('RIFF'), 1, 2, 3, 4, ...ascii('WEBPVP8L')))).toBe('image/webp');
  });

  it('остальное — null', () => {
    expect(sniffImageType(bytes(...ascii('GIF89a')))).toBeNull();
    expect(sniffImageType(bytes(...ascii('RIFF'), 1, 2, 3, 4, ...ascii('WAVEfmt ')))).toBeNull();
    expect(sniffImageType(bytes(...ascii('<svg xmlns')))).toBeNull();
    expect(sniffImageType(bytes(0xff, 0xd8))).toBeNull();
    expect(sniffImageType(new Uint8Array())).toBeNull();
  });
});

describe('photoUrl', () => {
  it('версия в адресе', () => {
    expect(photoUrl(12, 1792000000)).toBe('/api/products/12/photo?v=1792000000');
  });

  it('нет фото — null', () => {
    expect(photoUrl(12, null)).toBeNull();
    expect(photoUrl(12, undefined)).toBeNull();
  });
});
