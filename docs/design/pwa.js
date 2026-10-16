/*
 * garage — раздел 7 макетов: иконка приложения и заставка запуска (PWA).
 * Подключается после основного скрипта mockups.html и пользуется его
 * глобальными хелперами: phone(), ico(). Рисует в #pwa.
 */
(() => {
  const root = document.getElementById("pwa");
  if (!root || typeof phone !== "function") return;

  // Пути к иконкам из web/public — те же файлы, что отдаёт сервер
  const ICON_SVG = "../../web/public/icon.svg";
  const ICON_IOS = "../../web/public/apple-touch-icon.png";

  // Ячейка главного экрана iOS: квадрат 60 px со скруглением, как маска iOS,
  // и подпись под ним. Чужие приложения — заглушки цвета поверхности с иконкой.
  const cell = (label, inner, own = false) => `
<div style="display:grid;justify-items:center;gap:6px">
  <div style="width:60px;height:60px;border-radius:14px;overflow:hidden;display:grid;place-items:center;
    background:var(--surface);font-size:28px;color:var(--text-muted);
    ${own ? "box-shadow:var(--sh-sm);outline:var(--bw) solid var(--line-key)" : ""}">${inner}</div>
  <span style="font-size:var(--fs-12);font-weight:${own ? "var(--fw-bold)" : "var(--fw-medium)"};
    color:${own ? "var(--text)" : "var(--text-muted)"}">${label}</span>
</div>`;

  const app = (label, icon) => cell(label, ico(icon));
  const ours = cell("garage", `<img src="${ICON_IOS}" alt="Иконка garage" width="60" height="60" style="display:block">`, true);

  // Сетка 4 × N, как на главном экране; наша иконка — во втором ряду
  const grid = [
    app("Камера", "camera"), app("Фото", "photo"), app("Карты", "map-2"), app("Погода", "cloud"),
    app("Заметки", "notes"), ours, app("Часы", "clock"), app("Почта", "mail"),
    app("Музыка", "music"), app("Файлы", "folder"), app("Wallet", "wallet"), app("VPN", "shield-lock"),
  ].join("");

  // Док внизу экрана — четыре закреплённых приложения
  const dock = `
<div style="margin-top:auto;padding:14px 10px;border:var(--bw) solid var(--line);border-radius:28px;
  background:var(--surface-raised);display:grid;grid-template-columns:repeat(4,1fr);gap:8px">
  ${["phone", "message", "world", "settings"].map((i) => `<div style="height:60px;border-radius:14px;display:grid;place-items:center;
    background:var(--surface);font-size:28px;color:var(--text-muted)">${ico(i)}</div>`).join("")}
</div>`;

  const home = `
<div style="flex:1;display:flex;flex-direction:column;gap:22px;padding-top:18px">
  <div style="display:grid;grid-template-columns:repeat(4,1fr);row-gap:22px">${grid}</div>
  <p class="g-muted" style="margin:0;font-size:var(--fs-12);text-align:center">
    Иконка — PNG 180 × 180 без прозрачности, углы скругляет iOS</p>
  ${dock}
</div>`;

  // Заставка запуска: фон темы, по центру логотип и подпись.
  // На iOS без apple-touch-startup-image показывается фон background_color из manifest.
  const splash = `
<div style="flex:1;display:grid;place-items:center;align-content:center;gap:10px;margin:-8px -16px -16px;background:var(--bg)">
  <div class="logo" style="font-size:44px;line-height:var(--lh-tight)">garage<b>.</b></div>
  <span class="g-muted" style="font-size:var(--fs-12);font-weight:var(--fw-semibold);letter-spacing:0.04em">Учёт автохимии</span>
</div>`;

  // Иконка крупно в трёх размерах — на светлой (фон темы) и тёмной (инверсия) подложке.
  // Подложки берутся из токенов --bg и --text, поэтому одна всегда светлая, другая тёмная.
  const sizes = [180, 64, 32];
  const plate = (bg, fg, cap) => `
<div style="padding:var(--sp-6);border:var(--bw) solid var(--line);border-radius:var(--r-card);box-shadow:var(--sh-md);
  background:${bg};color:${fg};display:grid;gap:var(--sp-3)">
  <span style="font-size:var(--fs-12);font-weight:var(--fw-bold);opacity:0.75">${cap}</span>
  <div style="display:flex;align-items:flex-end;gap:var(--sp-6)">
    ${sizes.map((s) => `<div style="display:grid;justify-items:center;gap:var(--sp-2)">
      <img src="${ICON_SVG}" alt="Иконка garage ${s} px" width="${s}" height="${s}" style="display:block">
      <span style="font-size:var(--fs-12);font-weight:var(--fw-semibold)">${s} px</span></div>`).join("")}
  </div>
</div>`;

  const big = `
<figure style="margin:0"><figcaption class="mk-cap">Иконка icon.svg — 180, 64 и 32 px</figcaption>
<div style="display:grid;gap:var(--sp-5)">
  ${plate("var(--bg)", "var(--text)", "На фоне темы")}
  ${plate("var(--text)", "var(--bg)", "На инверсном фоне")}
  <div style="display:flex;gap:var(--sp-4);align-items:center">
    <img src="${ICON_IOS}" alt="Иконка для iOS" width="64" height="64" style="display:block;border-radius:14px">
    <span class="g-muted" style="font-size:var(--fs-12);max-width:300px">apple-touch-icon.png и icon-192/512.png —
      из icon-maskable.svg: сплошной фон, бутылка в центральных 80 %</span>
  </div>
</div></figure>`;

  root.innerHTML =
    phone("Главный экран iPhone", home) +
    phone("Заставка при запуске", splash) +
    big;
})();
