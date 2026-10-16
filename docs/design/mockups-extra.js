/*
 * garage — дополнительные макеты: состояния экранов и формы.
 * Подключается после основного скрипта mockups.html и использует его хелперы:
 * ITEMS, fmt, ico, card, tabbar, phone, back, filters, homeHead, homeList.
 * Компоненты — из states.css. Правило «одна горчичная кнопка на экран»:
 * на экранах с горчичной кнопкой нижнее меню (с горчичным «+») не показывается.
 */
(() => {
  const $ = (id) => document.getElementById(id);
  if (!$("phones-states") || !$("phones-forms")) return;

  /* ---------- Общие куски ---------- */

  // Пустое состояние: иконка, заголовок, пояснение, кнопки, приглушённая строка
  const empty = ({ icon, title, text, actions = "", meta = "", error = false }) => `
<section class="g-empty${error ? " g-empty--error" : ""}" aria-labelledby="e-${icon}">
  <span class="g-empty__icon">${ico(icon)}</span>
  <h2 class="g-empty__title" id="e-${icon}">${title}</h2>
  <p class="g-empty__text">${text}</p>
  ${actions ? `<div class="g-empty__actions">${actions}</div>` : ""}
  ${meta ? `<div class="g-empty__meta">${meta}</div>` : ""}
</section>`;

  // Поиск с уже введённым запросом и кнопкой очистки 44×44
  const searchWith = (q) => `
<div class="g-search" role="search"><span class="g-search__icon">${ico("search")}</span>
  <input class="g-input" type="search" value="${q}" aria-label="Поиск позиций" style="padding-right:48px">
  <button class="g-btn g-btn--icon g-btn--ghost" aria-label="Очистить поиск" style="position:absolute;right:0;top:0">${ico("x")}</button>
</div>`;

  // Подложка загрузки по форме карточки на главной
  const skel = (w) => `
<div class="g-skel" aria-hidden="true">
  <div class="g-skel__strip"></div>
  <div class="g-skel__body">
    <div class="g-skel__line" style="width:${w}%"></div>
    <div class="g-skel__num"></div>
    <div class="g-skel__bar"></div>
    <div class="g-skel__line g-skel__line--short"></div>
  </div>
</div>`;

  /* ---------- Состояния ---------- */

  // 1. Сервер недоступен: VPN выключен
  const scrOffline = `
<div class="logo">garage<b>.</b></div>
${empty({
  icon: "wifi-off",
  error: true,
  title: "Сервер недоступен",
  text: "Сайт работает только через VPN. Проверьте, что AmneziaWG включён.",
  actions: `<button class="g-btn g-btn--cta g-btn--block g-press">${ico("refresh")} Повторить</button>`,
  meta: "Последняя попытка: 12:04",
})}`;

  // 2. Связь пропала во время работы: полоса над главной
  const banner = `
<div class="g-banner g-banner--danger" role="alert">
  ${ico("wifi-off")}<span class="g-banner__text">Нет связи — изменения не сохранятся</span>
  <button class="g-btn g-btn--sm g-press">Повторить</button>
</div>`;
  const scrLost = banner + homeHead + homeList(false);

  // 3. Первый вход: справочники заполнены, позиций ещё нет
  const scrFirst = `
<h1 class="g-title">Остатки</h1>
${empty({
  icon: "package",
  title: "Здесь будут остатки",
  text: "Добавьте первую покупку — остатки появятся сами.",
  actions: `<button class="g-btn g-btn--block g-press">${ico("plus")} Добавить покупку</button>
            <button class="g-btn g-btn--block g-press">Создать позицию без покупки</button>`,
})}`;

  // 4. Поиск ничего не нашёл
  const scrNotFound = `
<div class="between"><h1 class="g-title">Остатки</h1><span class="g-badge g-badge--danger">2 мало</span></div>
${searchWith("полироль 3М")}
${empty({
  icon: "search-off",
  title: "Ничего не нашлось",
  text: "Позиции «полироль 3М» нет в каталоге. Проверьте написание или создайте её.",
  actions: `<button class="g-btn g-btn--block g-press">Создать позицию «полироль 3М»</button>`,
})}`;

  // 5. Загрузка: подложки вместо карточек, список помечен aria-busy
  const scrLoading = `
<h1 class="g-title">Остатки</h1>
<div class="g-search" role="search"><span class="g-search__icon">${ico("search")}</span><input class="g-input" type="search" placeholder="Шампунь, Koch, штрихкод"></div>
<div class="chips" aria-hidden="true"><span class="g-skel__chip"></span><span class="g-skel__chip"></span><span class="g-skel__chip" style="width:96px"></span></div>
<div class="stack" aria-busy="true" aria-label="Остатки загружаются">${[58, 44, 66, 50].map(skel).join("")}</div>`;

  // 6. Пустой список покупок
  const scrNoShopping = `
<h1 class="g-title">Купить</h1>
${empty({
  icon: "shopping-cart-check",
  title: "Всё куплено",
  text: "Позиции ниже минимального остатка появятся здесь сами. Остальное добавьте вручную.",
  actions: `<button class="g-btn g-btn--block g-press">${ico("plus")} Добавить в список</button>`,
})}`;

  $("phones-states").innerHTML =
    phone("Нет связи — VPN выключен", scrOffline) +
    phone("Связь пропала во время работы", scrLost, { tab: 0 }) +
    phone("Первый вход — пустая главная", scrFirst, { tab: 0 }) +
    phone("Поиск ничего не нашёл", scrNotFound, { tab: 0 }) +
    phone("Загрузка", scrLoading, { tab: 0 }) +
    phone("Пустой список покупок", scrNoShopping, { tab: 1 });

  /* ---------- Формы ---------- */

  // Восемь цветов категорий с подписями для экранного диктора
  const CATS = [
    ["blue", "синий", "Шампуни"], ["berry", "ягодный", "Активная пена"], ["olive", "оливковый", "Очистители"],
    ["petrol", "петроль", "Стёкла"], ["ochre", "охра", "Полироли и защита"], ["plum", "сливовый", "Кожа и пластик"],
    ["slate", "сланцевый", "Шины и диски"], ["umber", "умбра", "Микрофибра"],
  ];

  // Выбор категории в форме позиции: чипы с цветной точкой
  const catPick = (cur) => `
<div class="g-catpick g-catpick--scroll" role="radiogroup" aria-labelledby="np-cat">
  ${CATS.map(([c, , n]) => `<button type="button" class="g-catpick__opt" role="radio" aria-checked="${c === cur}" tabindex="${c === cur ? 0 : -1}" style="--cat:var(--cat-${c})"><span class="g-catpick__dot"></span>${n}</button>`).join("")}
</div>`;

  // Выбор цвета: восемь квадратов-радиокнопок
  const swatches = (cur, label) => `
<div class="g-swatches" role="radiogroup" aria-label="${label}">
  ${CATS.map(([c, name]) => `<button type="button" class="g-swatch" role="radio" aria-checked="${c === cur}" aria-label="${name}" tabindex="${c === cur ? 0 : -1}" style="--sw:var(--cat-${c})">${ico("check")}</button>`).join("")}
</div>`;

  // 1. Новая позиция (ФТ-3): прокручиваемая форма, кнопка закреплена внизу
  const scrProduct = `
<div class="between">${back("Отмена")}<span class="g-label">Новая позиция</span></div>
<form class="g-form" style="flex:1 1 0" onsubmit="return false">
  <div class="g-form__body">
    <div class="g-field"><label class="g-label" for="np-name">Название</label><input class="g-input" id="np-name" value="Green Star"></div>
    <div class="g-field">
      <label class="g-label" for="np-brand">Бренд <span class="g-field__opt">· необязательно</span></label>
      <input class="g-input" id="np-brand" value="Ko" role="combobox" aria-expanded="true" aria-controls="np-brand-list" aria-autocomplete="list">
      <ul class="g-suggest" id="np-brand-list" role="listbox" aria-label="Введённые ранее">
        <li class="g-suggest__opt" role="option" aria-selected="true">Koch Chemie <small>3 позиции</small></li>
      </ul>
    </div>
    <div class="g-field"><span class="g-label" id="np-cat">Категория</span>${catPick("olive")}</div>
    <div class="g-field"><span class="g-label" id="np-unit">Единица</span>
      <div class="g-seg" role="group" aria-labelledby="np-unit"><button type="button" aria-pressed="true">мл</button><button type="button" aria-pressed="false">шт</button></div>
    </div>
    <div class="two">
      <div class="g-field"><label class="g-label" for="np-size">Размер упаковки, мл</label><input class="g-input" id="np-size" inputmode="numeric" value="1000"></div>
      <div class="g-field"><label class="g-label" for="np-min">Мин. остаток, мл</label><input class="g-input" id="np-min" inputmode="numeric" value="500"></div>
    </div>
    <div class="g-field">
      <label class="g-label" for="np-code">Штрихкод <span class="g-field__opt">· необязательно</span></label>
      <div class="g-field__row">
        <input class="g-input" id="np-code" inputmode="numeric" placeholder="4007218000123">
        <button type="button" class="g-btn g-btn--sm" aria-disabled="true" aria-describedby="np-code-hint">${ico("barcode")} Сканировать</button>
      </div>
      <span class="g-field__hint" id="np-code-hint">Сканер появится в версии 1.1</span>
    </div>
    <div class="g-field">
      <label class="g-label" for="np-note">Заметка <span class="g-field__opt">· необязательно</span></label>
      <textarea class="g-input g-input--area" id="np-note" placeholder="Разведение, особенности">1:10 для арок, 1:50 для кузова</textarea>
    </div>
  </div>
  <div class="g-form__foot"><button class="g-btn g-btn--cta g-btn--block g-press">Сохранить позицию</button></div>
</form>`;

  // 2. Справочники (ФТ-2): категории, одна строка раскрыта для правки
  const catRow = (c, n, count) => `
<div class="g-row"><button class="g-handle" aria-label="Перетащить ${n}">${ico("grip-vertical")}</button>
  <span class="g-cat-sq" style="--cat:var(--cat-${c})"></span>
  <div class="g-row__main"><div class="g-row__title">${n}</div><div class="g-row__sub">${count}</div></div>
  <button class="g-btn g-btn--icon g-btn--ghost" aria-label="Меню категории ${n}">${ico("dots-vertical")}</button>
</div>`;

  const catEdit = (c, n) => `
<div class="g-row g-row--edit"><button class="g-handle" aria-label="Перетащить ${n}">${ico("grip-vertical")}</button>
  <div class="g-row__main">
    <input class="g-input" value="${n}" aria-label="Название категории">
  </div>
  <button class="g-btn g-btn--icon g-btn--accent g-press" aria-label="Сохранить">${ico("check")}</button>
  <div class="g-row__extra"><span class="g-label">Цвет</span>${swatches(c, `Цвет категории ${n}`)}</div>
</div>`;

  const scrCategories = `
<div class="between"><h1 class="g-title">Справочники</h1><button class="g-btn g-btn--sm g-press">${ico("plus")} Категория</button></div>
<div class="g-seg" role="group" aria-label="Справочник"><button type="button" aria-pressed="true">Категории</button><button type="button" aria-pressed="false">Места</button><button type="button" aria-pressed="false">Магазины</button></div>
<div class="g-list">
  ${catRow("blue", "Шампуни", "4 позиции")}
  ${catRow("berry", "Активная пена", "2 позиции")}
  ${catEdit("ochre", "Полироли и защита")}
  ${catRow("petrol", "Стёкла", "1 позиция")}
</div>
<a href="#" class="between" style="font-size:13px;font-weight:700;padding:0 4px;min-height:44px;color:var(--text-muted);text-decoration:none"><span>Архив · 1 категория</span>${ico("chevron-right")}</a>`;

  // 3. Профиль (ФТ-1): пароль, шаги кнопок количества, приглашения, выход
  const scrProfile = `
<div class="between">${back("Ещё")}<span class="g-label">Профиль</span></div>
<div class="g-list">
  <div class="g-row"><div class="g-row__main"><div class="g-row__sub">Логин</div><div class="g-row__title">ringino</div></div>${ico("user")}</div>
  <a href="#" class="g-row g-row--link"><div class="g-row__main g-row__title">Сменить пароль</div><span class="g-row__go">${ico("chevron-right")}</span></a>
</div>
<section class="g-section" aria-labelledby="pf-steps">
  <span class="g-label" id="pf-steps">Шаги кнопок количества, мл</span>
  <div class="two">
    <div class="g-field"><label class="g-label" for="pf-s1">Малый</label><input class="g-input" id="pf-s1" inputmode="numeric" value="10"></div>
    <div class="g-field"><label class="g-label" for="pf-s2">Большой</label><input class="g-input" id="pf-s2" inputmode="numeric" value="50"></div>
  </div>
</section>
<section class="g-section" aria-labelledby="pf-inv">
  <span class="g-label" id="pf-inv">Приглашения</span>
  <article class="g-invite" aria-label="Активное приглашение">
    <div class="g-invite__head"><b>Одноразовая ссылка</b><span class="g-badge g-badge--accent">активно</span></div>
    <p class="g-invite__link">garage.north-flow.online/invite/7f3c9a1e5b2d48c6a0f1e7d3</p>
    <div class="g-invite__meta">Создано 14.10 · действует ещё 6 дней</div>
    <div class="g-invite__actions">
      <button class="g-btn g-btn--sm g-press">${ico("copy")} Копировать</button>
      <button class="g-btn g-btn--sm g-btn--ghost g-btn--danger-text">${ico("link-off")} Отозвать</button>
    </div>
  </article>
  <button class="g-btn g-btn--cta g-btn--block g-press">${ico("user-plus")} Создать приглашение</button>
</section>
<button class="g-btn g-btn--ghost g-btn--block g-btn--danger-text">${ico("logout")} Выйти на всех устройствах</button>`;

  $("phones-forms").innerHTML =
    phone("Новая позиция — форма прокручивается", scrProduct) +
    phone("Справочники — правка категории", scrCategories, { tab: 4 }) +
    phone("Профиль и приглашения", scrProfile);

  // Выбранная категория в строке с прокруткой — в поле зрения
  document.querySelectorAll(".g-catpick--scroll").forEach((row) => {
    const sel = row.querySelector('[aria-checked="true"]');
    if (sel) row.scrollLeft = sel.offsetLeft - row.offsetLeft - 40;
  });

  /* ---------- Поведение: радиогруппы (цвет, категория) ---------- */
  const pick = (radio) => {
    const group = radio.closest('[role="radiogroup"]');
    group.querySelectorAll('[role="radio"]').forEach((r) => {
      const on = r === radio;
      r.setAttribute("aria-checked", String(on));
      r.tabIndex = on ? 0 : -1;
    });
  };
  const scope = [$("phones-states"), $("phones-forms")];
  scope.forEach((root) => {
    root.addEventListener("click", (e) => {
      const radio = e.target.closest('[role="radio"]');
      if (radio) pick(radio);
    });
    // Стрелки перемещают выбор внутри радиогруппы, как у нативных радиокнопок
    root.addEventListener("keydown", (e) => {
      const radio = e.target.closest('[role="radio"]');
      if (!radio || !["ArrowRight", "ArrowDown", "ArrowLeft", "ArrowUp"].includes(e.key)) return;
      e.preventDefault();
      const all = [...radio.closest('[role="radiogroup"]').querySelectorAll('[role="radio"]')];
      const step = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : -1;
      const next = all[(all.indexOf(radio) + step + all.length) % all.length];
      pick(next);
      next.focus();
    });
  });
})();
