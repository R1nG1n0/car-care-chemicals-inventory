/*
 * garage — живой прототип анимаций (раздел 8 в mockups.html).
 * Берёт глобальные хелперы страницы: card, ico, fmt, total, openLine, tabbar,
 * search, scrShopping, scrTemplates. Само движение описано в
 * web/src/styles/motion.css, здесь — только жесты, FLIP и смена классов.
 * Свои атрибуты — data-p-*, чтобы не пересекаться с общим обработчиком страницы.
 */
(() => {
  "use strict";
  const root = document.getElementById("prototype");
  if (!root || typeof card !== "function") return;

  const RM = matchMedia("(prefers-reduced-motion: reduce)");
  const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const ms = (v) => (v.endsWith("ms") ? parseFloat(v) : parseFloat(v) * 1000) || 0;
  const SWIPE_AT = 112; // порог свайпа, px
  const FLICK = 0.11; // скорость «рывка», px/мс

  const CATS = { blue: "Шампунь", berry: "Активная пена", petrol: "Стёкла", slate: "Шины и диски" };
  // Sonax: 410 мл при минимуме 300 — списание 150 мл опускает его ниже порога
  const DATA = [
    { id: "a", n: "Koch Chemie Gsf", cat: "blue", u: "мл", o: 320, cap: 1000, s: 2, min: 500, p: "Коробка 3" },
    { id: "b", n: "Gyeon Q²M Foam", cat: "berry", u: "мл", o: 80, cap: 1000, s: 0, min: 200, p: "Полка 2" },
    { id: "c", n: "Sonax Glas Klar", cat: "petrol", u: "мл", o: 410, cap: 500, s: 0, min: 300, p: "Полка 1" },
    { id: "d", n: "Koch Chemie Felgenblitz", cat: "slate", u: "мл", o: 650, cap: 1000, s: 1, min: 400, p: "Коробка 3" },
  ];
  const isLow = (i) => total(i) < i.min;
  const state = { q: "", f: "all" };
  const visible = () => {
    const q = state.q.trim().toLowerCase();
    return DATA.filter((i) => (state.f === "all" || isLow(i)) &&
      (!q || `${i.n} ${CATS[i.cat]} ${i.p}`.toLowerCase().includes(q)));
  };

  // Разметка из своей строки (данные макета, не пользовательские)
  const html = (s) => {
    const t = document.createElement("template");
    t.innerHTML = s.trim();
    return t.content.firstElementChild;
  };

  /* ---------- Каркас: телефон и инструкция ---------- */
  root.innerHTML = `
<figure style="margin:0"><figcaption class="mk-cap">Живой прототип · телефон</figcaption>
<div class="ph">
  <div class="ph__bar"><span>9:41</span><span>VPN ● 87%</span></div>
  <div data-p-views style="flex:1;position:relative;overflow:hidden"></div>
  ${tabbar(0)}
  <div data-p-toasts style="position:absolute;left:16px;right:16px;bottom:88px"></div>
  <div data-p-layer></div>
</div></figure>
<div style="max-width:440px;display:grid;gap:12px;align-content:start">
  <p class="mk-cap" style="margin:0">Что попробовать</p>
  <ul class="mk-p" style="margin:0;padding-left:18px;display:grid;gap:8px">
    <li>Нижнее меню «Остатки → Купить → Шаблоны»: экран въезжает по порядку вкладок, назад — с другой стороны. Можно жать быстро — переходы прерываются.</li>
    <li>Поиск: наберите «koch» — лишние карточки тают, оставшиеся съезжают на место. Чипы «Все / Мало» перестраивают список так же.</li>
    <li>Свайп карточки влево (на компьютере — тянуть мышью): дальше 112 px или резким рывком открывается шторка расхода, иначе карточка возвращается.</li>
    <li>Шторку можно смахнуть вниз за ручку, закрыть нажатием на фон или Esc.</li>
    <li>Порог «мало»: на <b>Sonax Glas Klar</b> (410 мл, минимум 300) спишите 150 мл (50 → +50 → +50). Шкала плавно уменьшится, появится наклейка «мало!». «Отменить» вернёт остаток и снимет наклейку.</li>
    <li>Полоска на уведомлении — 10 секунд на отмену. Если переключиться на другую вкладку браузера, она встаёт на паузу.</li>
    <li>С системной настройкой «Уменьшить движение» остаётся только смена прозрачности.</li>
  </ul>
  <div><button type="button" class="g-btn g-press" data-p-replay>${ico("refresh")} Повторить появление</button></div>
</div>`;

  const [views, layer, toasts] = ["views", "layer", "toasts"].map((k) => root.querySelector(`[data-p-${k}]`));
  const tabs = [...root.querySelectorAll(".g-tabbar a")];
  tabs.forEach((a, k) => { a.dataset.pTab = String(k); });
  const ORDER = [0, 1, 3]; // Остатки, Купить, Шаблоны — порядок задаёт направление
  let cur = 0;
  const currentView = () => views.querySelector(".g-view:not(.is-leaving)");

  /* ---------- Экраны ---------- */
  function viewEl(tab, dir) {
    const v = document.createElement("div");
    v.className = "g-view ph__scr";
    v.style.cssText = "position:absolute;inset:0;overflow:hidden auto;scrollbar-width:none";
    if (dir) v.dataset.dir = dir;
    if (tab === 0) buildHome(v);
    else v.innerHTML = tab === 1 ? scrShopping : scrTemplates;
    return v;
  }

  function buildHome(v) {
    v.innerHTML = `
<div class="between"><h1 class="g-title">Остатки</h1><span class="g-badge g-badge--danger" data-p-badge></span></div>
${search}
<div class="g-filters" role="group" aria-label="Фильтр">
  <button type="button" class="g-filter" data-p-filter="all" style="font-family:inherit;cursor:pointer">Все&nbsp;<span data-p-count="all"></span></button>
  <button type="button" class="g-filter" data-p-filter="low" style="font-family:inherit;cursor:pointer">Мало&nbsp;<span data-p-count="low"></span></button>
</div>
<div class="stack g-flip g-stagger" data-p-list style="position:relative"></div>
<p class="g-muted" data-p-empty hidden style="margin:0;text-align:center">Ничего не нашлось</p>`;
    const inp = v.querySelector('input[type="search"]');
    inp.value = state.q;
    inp.setAttribute("aria-label", "Поиск по остаткам");
    inp.addEventListener("input", () => { state.q = inp.value; refresh(); });
    v.querySelectorAll("[data-p-filter]").forEach((b) => b.addEventListener("click", () => { state.f = b.dataset.pFilter; refresh(); }));
    const list = v.querySelector("[data-p-list]");
    visible().forEach((i) => list.appendChild(makeCard(i)));
    settleStagger(list);
    paintChrome(v);
  }

  // Шапка: число «мало», счётчики и состояние чипов, пустой результат
  function paintChrome(v) {
    const low = DATA.filter(isLow).length;
    const badge = v.querySelector("[data-p-badge]");
    if (!badge) return;
    badge.textContent = `${low} мало`;
    badge.hidden = low === 0;
    v.querySelector('[data-p-count="all"]').textContent = DATA.length;
    v.querySelector('[data-p-count="low"]').textContent = low;
    v.querySelectorAll("[data-p-filter]").forEach((b) => {
      const on = b.dataset.pFilter === state.f;
      b.setAttribute("aria-pressed", String(on));
      b.style.color = on ? "var(--bg)" : "var(--text)";
    });
    v.querySelector("[data-p-empty]").hidden = visible().length > 0;
  }

  // Очередь играет только при входе: потом класс снимается, чтобы новые
  // карточки из поиска появлялись по правилам FLIP, а не очередью
  function settleStagger(list) {
    clearTimeout(list.pTimer);
    list.pTimer = setTimeout(() => list.classList.remove("g-stagger"),
      ms(css("--dur-enter")) + 8 * ms(css("--stagger")));
  }

  function makeCard(i) {
    const wrap = html(`<div class="g-swipe" data-p-id="${i.id}">
      <div class="g-swipe__action">${ico("droplet")} Расход</div>
      ${card({ ...i, c: CATS[i.cat], low: isLow(i) })}</div>`);
    bindSwipe(wrap, i);
    return wrap;
  }

  /* ---------- Смена раздела ---------- */
  function go(tab) {
    if (tab === cur || !ORDER.includes(tab)) return;
    const dir = ORDER.indexOf(tab) > ORDER.indexOf(cur) ? "forward" : "back";
    views.querySelectorAll(".g-view.is-leaving").forEach((v) => v.remove());
    const old = currentView();
    if (old) {
      old.classList.add("is-leaving");
      old.inert = true;
      setTimeout(() => old.remove(), ms(css("--dur-fast")) + 40);
    }
    views.appendChild(viewEl(tab, dir));
    cur = tab;
    tabs.forEach((a) => (+a.dataset.pTab === tab ? a.setAttribute("aria-current", "page") : a.removeAttribute("aria-current")));
  }
  root.querySelector(".g-tabbar").addEventListener("click", (e) => {
    const a = e.target.closest("[data-p-tab]");
    if (a) go(+a.dataset.pTab);
  });

  function replay() {
    if (cur !== 0) return go(0); // вход в раздел сам проигрывает очередь
    const list = currentView().querySelector("[data-p-list]");
    list.classList.add("g-stagger");
    list.replaceChildren(...visible().map(makeCard));
    settleStagger(list);
  }
  root.querySelector("[data-p-replay]").addEventListener("click", replay);

  /* ---------- Поиск и фильтры: FLIP ---------- */
  function refresh() {
    const v = currentView();
    const list = v && v.querySelector("[data-p-list]");
    if (!list) return;
    list.classList.remove("g-stagger");
    paintChrome(v);
    const want = visible();
    const ids = new Set(want.map((i) => i.id));
    const kids = [...list.children].filter((el) => !el.classList.contains("is-exiting"));
    // First: где карточки видны сейчас (с учётом незаконченного сдвига)
    const first = new Map(kids.map((el) => [el, el.getBoundingClientRect()]));
    const base = list.getBoundingClientRect();
    kids.forEach((el) => el.pFlip && el.pFlip.cancel());

    // Уходящие выпадают из потока и тают на своём месте
    kids.filter((el) => !ids.has(el.dataset.pId)).forEach((el) => {
      const r = first.get(el);
      Object.assign(el.style, {
        position: "absolute", top: `${r.top - base.top}px`, left: `${r.left - base.left}px`, width: `${r.width}px`,
      });
      el.classList.add("is-exiting");
      setTimeout(() => el.remove(), 200);
    });

    // Новые встают на своё место по порядку данных
    const kept = new Map(kids.filter((el) => ids.has(el.dataset.pId)).map((el) => [el.dataset.pId, el]));
    const entering = [];
    let prev = null;
    want.forEach((i) => {
      let el = kept.get(i.id);
      if (!el) {
        el = makeCard(i);
        el.classList.add("is-entering");
        list.insertBefore(el, prev ? prev.nextSibling : list.firstChild);
        entering.push(el);
      }
      prev = el;
    });
    void list.offsetWidth; // фиксируем начальное состояние новых карточек
    entering.forEach((el) => el.classList.remove("is-entering"));
    if (RM.matches) return;

    // Last → Invert → Play: оставшиеся съезжают на новое место
    const ease = css("--ease-in-out");
    kept.forEach((el) => {
      const a = first.get(el), b = el.getBoundingClientRect();
      const dx = a.left - b.left, dy = a.top - b.top;
      if (Math.abs(dx) + Math.abs(dy) < 0.5) return;
      el.pFlip = el.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: "none" }],
        { duration: 200, easing: ease });
    });
  }

  /* ---------- Свайп карточки ---------- */
  // Дальше порога — сопротивление ×0.3, вправо — почти не тянется
  const swipeOffset = (dx) => (dx > 0 ? dx * 0.15 : dx < -SWIPE_AT ? -SWIPE_AT + (dx + SWIPE_AT) * 0.3 : dx);

  function bindSwipe(wrap, item) {
    const el = wrap.querySelector(".g-card");
    el.setAttribute("draggable", "false");
    el.addEventListener("dragstart", (e) => e.preventDefault());
    let g = null;
    el.addEventListener("pointerdown", (e) => {
      if (e.button !== 0 || wrap.classList.contains("is-exiting")) return;
      g = { id: e.pointerId, x: e.clientX, y: e.clientY, t: e.timeStamp, on: false, dx: 0 };
    });
    el.addEventListener("pointermove", (e) => {
      if (!g || e.pointerId !== g.id) return;
      const dx = e.clientX - g.x, dy = e.clientY - g.y;
      if (!g.on) {
        if (Math.hypot(dx, dy) < 6) return;
        if (Math.abs(dx) <= Math.abs(dy)) { g = null; return; } // вертикальный жест — прокрутка
        g.on = true;
        el.setPointerCapture(e.pointerId);
        el.classList.add("is-dragging");
      }
      g.dx = dx;
      el.style.transform = `translateX(${swipeOffset(dx)}px)`;
    });
    const end = (e) => {
      if (!g || e.pointerId !== g.id) return;
      const done = g; g = null;
      if (!done.on) return;
      el.classList.remove("is-dragging");
      el.style.transform = ""; // доводка до места — переходом из motion.css
      const v = -done.dx / Math.max(1, e.timeStamp - done.t);
      if (e.type === "pointerup" && (-done.dx > SWIPE_AT || (v > FLICK && -done.dx > 24))) openSheet(item);
    };
    el.addEventListener("pointerup", end);
    el.addEventListener("pointercancel", end);
  }

  /* ---------- Шторка расхода ---------- */
  let sheet = null;

  function openSheet(item) {
    closeSheet();
    const max = total(item);
    const scrim = html('<div class="g-scrim" style="position:absolute;inset:0;background:rgba(10,10,12,.55)"></div>');
    const el = html(`
<section class="g-sheet" role="dialog" aria-modal="true" aria-label="Расход: ${item.n}" style="position:absolute;left:0;right:0;bottom:0">
  <div data-p-grab style="margin:-8px 0 0;padding:8px 0 2px;cursor:grab;touch-action:none"><div class="g-sheet__grab"></div></div>
  <div style="margin-bottom:10px"><div class="g-card__name">${item.n}</div><div class="g-row__sub">${CATS[item.cat]} · ${item.p}</div></div>
  <div class="g-qty">
    <label class="g-label" for="p-qty">Количество</label>
    <div class="g-qty__value"><input id="p-qty" inputmode="numeric" autocomplete="off"><b>${item.u}</b></div>
    <div class="g-qty__steps">${[-50, -10, 10, 50].map((s) => `<button type="button" class="g-btn g-press" data-p-step="${s}" aria-label="${s > 0 ? "Плюс" : "Минус"} ${Math.abs(s)} ${item.u}">${s > 0 ? "+" : "−"}${Math.abs(s)}</button>`).join("")}</div>
    <div class="g-qty__hint">во вскрытой ${fmt(item.o)} ${item.u} · всего ${fmt(max)} ${item.u} · мин. ${fmt(item.min)}</div>
  </div>
  <button type="button" class="g-btn g-btn--cta g-btn--block g-press" data-p-commit style="margin-top:14px"></button>
</section>`);
    layer.append(scrim, el);
    const inp = el.querySelector("input"), commit = el.querySelector("[data-p-commit]");
    const val = () => Math.max(0, Math.min(max, parseInt(inp.value, 10) || 0));
    const label = () => {
      commit.textContent = `Списать ${fmt(val())} ${item.u}`;
      commit.setAttribute("aria-disabled", String(val() === 0));
    };
    inp.value = Math.min(50, max);
    label();
    inp.addEventListener("input", label);
    el.addEventListener("click", (e) => {
      const b = e.target.closest("[data-p-step]");
      if (b) {
        inp.value = Math.max(0, Math.min(max, val() + +b.dataset.pStep));
        label();
      } else if (e.target.closest("[data-p-commit]") && val() > 0) {
        consume(item, val());
      }
    });
    scrim.addEventListener("click", closeSheet);
    bindSheetDrag(el, scrim);
    sheet = { el, scrim };
    commit.focus({ preventScroll: true });
  }

  function closeSheet() {
    if (!sheet) return;
    const { el, scrim } = sheet; sheet = null;
    el.classList.remove("is-dragging");
    scrim.classList.remove("is-dragging");
    // При уменьшенном движении шторка гаснет там, где её отпустили
    if (!RM.matches) el.style.transform = "";
    scrim.style.opacity = "";
    el.classList.add("is-leaving");
    scrim.classList.add("is-leaving");
    setTimeout(() => { el.remove(); scrim.remove(); }, 240);
  }

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && sheet) closeSheet();
  });

  function bindSheetDrag(el, scrim) {
    const grab = el.querySelector("[data-p-grab]");
    let g = null;
    grab.addEventListener("pointerdown", (e) => {
      if (e.button !== 0) return;
      g = { id: e.pointerId, y: e.clientY, t: e.timeStamp, dy: 0 };
      grab.setPointerCapture(e.pointerId);
      el.classList.add("is-dragging");
      scrim.classList.add("is-dragging");
    });
    grab.addEventListener("pointermove", (e) => {
      if (!g || e.pointerId !== g.id) return;
      g.dy = e.clientY - g.y;
      // Вниз — за пальцем, вверх — с сильным сопротивлением
      const y = g.dy < 0 ? g.dy * 0.2 : g.dy;
      el.style.transform = `translateY(${y}px)`;
      scrim.style.opacity = String(Math.max(0, 1 - Math.max(0, y) / el.offsetHeight));
    });
    const end = (e) => {
      if (!g || e.pointerId !== g.id) return;
      const { dy, t } = g; g = null;
      const v = dy / Math.max(1, e.timeStamp - t);
      if (e.type === "pointerup" && (dy > el.offsetHeight * 0.25 || (v > FLICK && dy > 10))) return closeSheet();
      el.classList.remove("is-dragging");
      scrim.classList.remove("is-dragging");
      el.style.transform = "";
      scrim.style.opacity = "";
    };
    grab.addEventListener("pointerup", end);
    grab.addEventListener("pointercancel", end);
  }

  /* ---------- Списание, отмена, наклейка ---------- */
  function consume(item, n) {
    const prev = { o: item.o, s: item.s };
    let a = n;
    // Расход идёт из вскрытой; опустела — вскрывается запечатанная
    while (a > 0) {
      const t = Math.min(a, item.o);
      item.o -= t; a -= t;
      if (item.o === 0 && item.s > 0) { item.s--; item.o += item.cap; } else if (item.o === 0) break;
    }
    closeSheet();
    paint(item);
    showToast(item, n, prev);
  }

  // Число меняется сразу; шкала и наклейка — чуть позже, когда шторка уже уходит
  function paint(item) {
    const v = currentView();
    if (!v || !v.querySelector("[data-p-list]")) return;
    const wrap = v.querySelector(`.g-swipe[data-p-id="${item.id}"]:not(.is-exiting)`);
    if (wrap) {
      wrap.querySelector(".g-card__qty .g-num").textContent = fmt(total(item));
      wrap.querySelector(".g-card__open").textContent = openLine(item);
      const low = isLow(item);
      setTimeout(() => {
        const m = wrap.querySelector(".g-meter");
        m.classList.toggle("g-meter--low", low);
        m.setAttribute("aria-valuenow", String(item.o));
        m.firstElementChild.style.setProperty("--lv", (item.o / item.cap).toFixed(3));
        const c = wrap.querySelector(".g-card");
        const st = c.querySelector(".g-sticker:not(.is-leaving)");
        if (low && !st) {
          c.insertAdjacentHTML("beforeend", '<span class="g-sticker g-sticker--new">мало!</span>');
        } else if (!low && st) {
          st.classList.add("is-leaving");
          setTimeout(() => st.remove(), 200);
        }
      }, ms(css("--dur-fast")));
    }
    if (state.f === "low") refresh();
    else paintChrome(v);
  }

  let toast = null;

  function showToast(item, n, prev) {
    dropToast();
    const el = html(`<div class="g-toast" role="status" style="position:absolute;left:0;right:0;bottom:0">
      <span style="flex:1">Списано ${fmt(n)} ${item.u} · ${item.n}</span>
      <button type="button" class="g-btn g-btn--sm g-press">Отменить</button></div>`);
    toasts.appendChild(el);
    const t = { el, left: ms(css("--dur-undo")) || 10000, start: performance.now(), timer: 0 };
    t.timer = setTimeout(dropToast, t.left);
    toast = t;
    el.querySelector("button").addEventListener("click", () => {
      if (toast !== t) return;
      Object.assign(item, prev);
      dropToast();
      paint(item);
    });
    if (document.hidden) pauseToast();
  }

  function dropToast() {
    if (!toast) return;
    const { el, timer } = toast; toast = null;
    clearTimeout(timer);
    el.classList.add("is-leaving");
    setTimeout(() => el.remove(), 200);
  }

  // Пока вкладка браузера скрыта, время на отмену не идёт
  function pauseToast() {
    clearTimeout(toast.timer);
    toast.left -= performance.now() - toast.start;
    toast.el.dataset.paused = "";
  }
  document.addEventListener("visibilitychange", () => {
    if (!toast) return;
    if (document.hidden) return pauseToast();
    delete toast.el.dataset.paused;
    toast.start = performance.now();
    toast.timer = setTimeout(dropToast, toast.left);
  });

  /* ---------- Старт ---------- */
  views.appendChild(viewEl(0));
  // Прототип внизу страницы: очередь проигрывается, когда телефон стал виден
  const io = new IntersectionObserver((entries) => {
    if (!entries.some((en) => en.isIntersecting)) return;
    io.disconnect();
    replay();
  }, { threshold: 0.4 });
  io.observe(root.querySelector(".ph"));
})();
