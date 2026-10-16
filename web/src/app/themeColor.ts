// Цвет строки состояния следует теме (И-2.2): <meta name="theme-color"> получает
// значение токена --bg текущей темы. В index.html две метки с media — они работают
// до запуска скрипта; дальше остаётся одна, без media, и её меняет этот код
// при смене системной темы и атрибута data-theme.

function currentBg(): string {
  return getComputedStyle(document.documentElement).getPropertyValue('--bg').trim();
}

function apply(): void {
  const bg = currentBg();
  if (!bg) return;
  const metas = [...document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')];
  let meta = metas.shift();
  for (const extra of metas) extra.remove();
  if (!meta) {
    meta = document.createElement('meta');
    meta.name = 'theme-color';
    document.head.append(meta);
  }
  meta.removeAttribute('media');
  if (meta.content !== bg) meta.content = bg;
}

export function syncThemeColor(): void {
  apply();
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', apply);
  new MutationObserver(apply).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
}
