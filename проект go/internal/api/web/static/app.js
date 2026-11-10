// Веб-интерфейс "Космопорта". Работает с тем же REST API, что и curl.
"use strict";

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

const POLL_MS = 1500;

const STEPS = [
  { status: "created", icon: "📝", label: "Принята" },
  { status: "processing", icon: "🛠️", label: "Подготовка" },
  { status: "approved", icon: "✅", label: "Одобрено" },
  { status: "launched", icon: "🚀", label: "В полёте" },
  { status: "completed", icon: "🪐", label: "Прибыли" },
];
const STATUS_TEXT = {
  created: "принята", processing: "подготовка", approved: "одобрена",
  launched: "в полёте", completed: "прибыли", rejected: "отклонена",
};
const PLANETS = {
  iss: "radial-gradient(circle at 35% 35%, #e5e7eb, #6b7280 60%, #1f2937)",
  moon: "radial-gradient(circle at 35% 35%, #f3f4f6, #9ca3af 55%, #374151)",
  mars: "radial-gradient(circle at 35% 35%, #fdba74, #c2410c 55%, #431407)",
  europa: "radial-gradient(circle at 35% 35%, #fef3c7, #d6b98c 50%, #78512c)",
  titan: "radial-gradient(circle at 35% 35%, #fde68a, #d97706 55%, #451a03)",
};

const state = {
  token: localStorage.getItem("cosmoport_token") || "",
  login: localStorage.getItem("cosmoport_login") || "",
  destinations: [],
  selected: null,
  orders: new Map(), // id -> order (для отслеживания смены статусов)
  pollTimer: null,
  authMode: "login",
};

// ---------------------------------------------------------------- API + консоль

async function api(method, path, { body, auth = true, token, quiet = false, raw = false } = {}) {
  const headers = {};
  const useToken = token !== undefined ? token : (auth ? state.token : "");
  if (useToken) headers.Authorization = "Bearer " + useToken;
  let payload;
  if (body !== undefined) {
    if (raw) { payload = body; headers["Content-Type"] = "text/plain; charset=utf-8"; }
    else { payload = JSON.stringify(body); headers["Content-Type"] = "application/json"; }
  }

  const started = performance.now();
  let res, text;
  try {
    res = await fetch(path, { method, headers, body: payload });
    text = await res.text();
  } catch (err) {
    logRequest({ method, path, status: "ERR", ms: performance.now() - started, headers, payload, response: String(err), quiet });
    setHealth(false);
    throw new Error("Сервер недоступен");
  }
  const ms = performance.now() - started;
  let data = text;
  try { data = JSON.parse(text); } catch { /* text/plain, например /test */ }

  logRequest({ method, path, status: res.status, ms, headers, payload, response: text, quiet });
  if (!res.ok) {
    const err = new Error((data && data.error) || `HTTP ${res.status}`);
    err.status = res.status;
    throw err;
  }
  return data;
}

function logRequest({ method, path, status, ms, headers, payload, response, quiet }) {
  if (quiet && !$("#show-polling").checked) return;
  const ok = typeof status === "number" && status < 400;
  const row = document.createElement("div");
  row.className = "log";
  const shownHeaders = { ...headers };
  if (shownHeaders.Authorization) shownHeaders.Authorization = shownHeaders.Authorization.slice(0, 32) + "…";
  row.innerHTML = `
    <span class="log__time">${time()}</span>
    <span class="log__method">${method}</span>
    <span>${esc(path)}</span>
    <span class="${ok ? "log__status--ok" : "log__status--bad"}">${status}</span>
    <span class="log__ms">${ms.toFixed(0)} мс</span>
    <div class="log__detail">Заголовки: ${esc(JSON.stringify(shownHeaders))}
${payload ? "Тело запроса: " + esc(String(payload)) + "\n" : ""}Ответ: ${esc(truncate(response, 600))}</div>`;
  row.addEventListener("click", () => row.classList.toggle("is-open"));
  prependLog(row);
}

function logEvent(text) {
  const row = document.createElement("div");
  row.className = "log log--event";
  row.innerHTML = `<span class="log__time">${time()}</span><span>${esc(text)}</span>`;
  prependLog(row);
}

function prependLog(row) {
  const body = $("#console-body");
  body.prepend(row);
  while (body.children.length > 200) body.lastChild.remove();
}

// ---------------------------------------------------------------- утилиты

const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const truncate = (s, n) => (s && s.length > n ? s.slice(0, n) + "…" : s || "");
const time = (d = new Date()) => d.toLocaleTimeString("ru-RU");
const money = (n) => new Intl.NumberFormat("ru-RU").format(n) + " ₡";
const km = (n) => new Intl.NumberFormat("ru-RU", { notation: n > 1e7 ? "compact" : "standard" }).format(n) + " км";

function decodeJWT(token) {
  try {
    const part = token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/");
    return JSON.parse(decodeURIComponent(escape(atob(part))));
  } catch { return null; }
}

function toast(title, sub = "", kind = "") {
  const el = document.createElement("div");
  el.className = "toast" + (kind ? " toast--" + kind : "");
  el.innerHTML = `${esc(title)}${sub ? `<small>${esc(sub)}</small>` : ""}`;
  $("#toasts").append(el);
  setTimeout(() => el.remove(), 4500);
  while ($("#toasts").children.length > 4) $("#toasts").firstChild.remove();
}

// ---------------------------------------------------------------- схема

function pulse(nodes, links) {
  nodes.forEach((n) => {
    const el = $(`[data-node="${n}"]`);
    el.classList.add("is-active");
    setTimeout(() => el.classList.remove("is-active"), 900);
  });
  links.forEach(([name, dir], i) => {
    const el = $(`[data-link="${name}"]`);
    setTimeout(() => {
      el.classList.remove("go", "back");
      void el.offsetWidth; // перезапуск анимации
      el.classList.add(dir);
    }, i * 350);
  });
}

function setCaption(text) { $("#diagram-caption").textContent = text; }

// ---------------------------------------------------------------- здоровье сервиса

let healthy = null;
function setHealth(ok) {
  const el = $("#health");
  el.classList.toggle("is-ok", ok);
  el.classList.toggle("is-down", !ok);
  $("#health-text").textContent = ok ? "api в строю" : "api недоступен";
  if (healthy !== null && healthy !== ok) {
    toast(ok ? "🟢 api снова доступен" : "🔴 api недоступен", ok ? "" : "Возможно, его остановили (graceful shutdown)", ok ? "ok" : "bad");
  }
  healthy = ok;
}

async function checkHealth() {
  try {
    const res = await fetch("/health", { cache: "no-store" });
    setHealth(res.ok);
  } catch { setHealth(false); }
}

// ---------------------------------------------------------------- вкладки

function initTabs() {
  $$(".tab").forEach((btn) => btn.addEventListener("click", () => {
    $$(".tab").forEach((b) => b.classList.toggle("is-active", b === btn));
    $$(".page").forEach((p) => p.classList.toggle("is-active", p.dataset.page === btn.dataset.tab));
    if (btn.dataset.tab === "labs") loadMessages();
    if (btn.dataset.tab === "monitoring") renderPanels();
  }));
}

// ---------------------------------------------------------------- авторизация

function initAuth() {
  $$(".segmented__btn").forEach((btn) => btn.addEventListener("click", () => {
    state.authMode = btn.dataset.mode;
    $$(".segmented__btn").forEach((b) => b.classList.toggle("is-active", b === btn));
    $("#auth-submit").textContent = state.authMode === "login" ? "Войти" : "Зарегистрироваться";
    $("#auth-hint").textContent = state.authMode === "login"
      ? "Нет аккаунта? Переключись на «Регистрация»."
      : "Пользователь сохранится в PostgreSQL, пароль — только в виде bcrypt-хэша.";
  }));

  $("#auth-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const form = new FormData(e.target);
    const creds = { login: form.get("login").trim(), password: form.get("password") };
    const btn = $("#auth-submit");
    btn.disabled = true;
    try {
      if (state.authMode === "register") {
        await api("POST", "/auth/register", { body: creds, auth: false });
        toast("👩‍🚀 Космонавт зарегистрирован", `Логин ${creds.login} сохранён в БД`, "ok");
        logEvent(`Пользователь ${creds.login} создан в таблице users`);
      }
      const res = await api("POST", "/auth/login", { body: creds, auth: false });
      state.token = res.token;
      state.login = creds.login;
      localStorage.setItem("cosmoport_token", state.token);
      localStorage.setItem("cosmoport_login", state.login);
      logEvent("Получен JWT-токен — дальше он отправляется в заголовке Authorization");
      e.target.reset();
      onLoggedIn();
    } catch (err) {
      const msg = err.status === 409 ? "Такой логин уже занят" : err.status === 401 ? "Неверный логин или пароль" : err.message;
      toast("Не получилось", msg, "bad");
    } finally {
      btn.disabled = false;
    }
  });

  $("#logout").addEventListener("click", logout);
}

function onLoggedIn() {
  const claims = decodeJWT(state.token);
  if (!claims || claims.exp * 1000 < Date.now()) { logout(); return; }

  $("#auth-forms").hidden = true;
  $("#auth-user").hidden = false;
  $("#user-login").textContent = state.login;
  $("#user-id").textContent = claims.sub;
  $("#token-exp").textContent = new Date(claims.exp * 1000).toLocaleString("ru-RU");
  $("#token-raw").textContent = state.token;
  $("#token-claims").textContent = "Содержимое токена (payload):\n" + JSON.stringify(claims, null, 2);
  updateBookingState();
  state.orders.clear();
  refreshOrders(true);
  clearInterval(state.pollTimer);
  state.pollTimer = setInterval(() => refreshOrders(false), POLL_MS);
}

function logout() {
  state.token = "";
  state.login = "";
  localStorage.removeItem("cosmoport_token");
  localStorage.removeItem("cosmoport_login");
  clearInterval(state.pollTimer);
  state.orders.clear();
  $("#auth-forms").hidden = false;
  $("#auth-user").hidden = true;
  $("#orders").innerHTML = `<div class="empty">Войди в систему, чтобы увидеть свои заявки.</div>`;
  $("#counters").innerHTML = "";
  updateBookingState();
}

// ---------------------------------------------------------------- направления и бронирование

async function loadDestinations() {
  try {
    state.destinations = await api("GET", "/destinations", { auth: false });
  } catch { return; }
  const box = $("#destinations");
  box.innerHTML = "";
  state.destinations.forEach((d) => {
    const el = document.createElement("button");
    el.type = "button";
    el.className = "dest";
    el.dataset.code = d.code;
    el.innerHTML = `
      <span class="dest__code">${esc(d.code)}</span>
      <div class="dest__planet" style="background:${PLANETS[d.code] || PLANETS.moon}"></div>
      <div class="dest__name">${esc(d.name)}</div>
      <div class="dest__meta">📏 ${km(d.distance_km)}<br>⏱️ ${d.flight_days} дн. в пути</div>
      <div class="dest__price">${money(d.price_per_seat)} / место</div>`;
    el.addEventListener("click", () => {
      state.selected = d;
      $$(".dest").forEach((x) => x.classList.toggle("is-selected", x === el));
      updateBookingState();
    });
    box.append(el);
  });
}

function updateBookingState() {
  const pax = Math.max(1, Math.min(10, Number($("#passengers").value) || 1));
  const d = state.selected;
  $("#booking-summary").innerHTML = d
    ? `${esc(d.name)} · ${pax} пасс. · итого <b>${money(d.price_per_seat * pax)}</b>`
    : "Выбери направление";
  $("#book-btn").disabled = !d || !state.token;
  $("#booking-hint").style.display = state.token ? "none" : "";
}

function initBooking() {
  $("#passengers").addEventListener("input", updateBookingState);
  $("#booking").addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!state.selected) return;
    const passengers = Math.max(1, Math.min(10, Number($("#passengers").value) || 1));
    const btn = $("#book-btn");
    btn.disabled = true;
    try {
      const order = await api("POST", "/orders", { body: { destination: state.selected.code, passengers } });
      state.orders.set(order.id, order);
      toast(`📝 Заявка #${order.id} создана`, `${state.selected.name} — сообщение отправлено в ЦУП`, "ok");
      logEvent(`api сохранил заявку #${order.id} в PostgreSQL и опубликовал её в очередь orders.created`);
      setCaption(`Заявка #${order.id}: браузер → api → PostgreSQL и RabbitMQ → ЦУП`);
      pulse(["browser", "api", "db", "mq", "mc"], [["browser-api", "go"], ["api-db", "go"], ["api-mq", "go"], ["mq-mc", "go"]]);
      refreshOrders(false);
    } catch (err) {
      toast("Не удалось создать заявку", err.message, "bad");
      if (err.status === 401) logout();
    } finally {
      updateBookingState();
    }
  });
}

// ---------------------------------------------------------------- заявки

async function refreshOrders(first) {
  if (!state.token) return;
  let orders;
  try {
    orders = await api("GET", "/orders?limit=30", { quiet: !first });
    setHealth(true);
  } catch (err) {
    if (err.status === 401) { toast("Сессия истекла", "Войди заново", "bad"); logout(); }
    return;
  }

  let changed = false;
  for (const o of orders) {
    const prev = state.orders.get(o.id);
    if (prev && prev.status !== o.status) {
      changed = true;
      onStatusChange(o);
    }
    state.orders.set(o.id, o);
  }
  renderOrders(orders, changed);
}

function onStatusChange(o) {
  const dest = destName(o.destination);
  const kind = o.status === "rejected" ? "bad" : o.status === "completed" ? "ok" : "";
  const icon = { processing: "🛠️", approved: "✅", launched: "🚀", completed: "🪐", rejected: "⛔" }[o.status] || "•";
  toast(`${icon} Заявка #${o.id}: ${STATUS_TEXT[o.status]}`, o.status_reason || dest, kind);
  logEvent(`ЦУП → RabbitMQ (orders.status) → api: заявка #${o.id} теперь «${o.status}»`);
  setCaption(`Заявка #${o.id}: ЦУП прислал статус «${o.status}» через RabbitMQ, api записал его в БД`);
  pulse(["mc", "mq", "api", "db"], [["mq-mc", "back"], ["api-mq", "back"], ["api-db", "go"]]);
  const card = $(`[data-order="${o.id}"]`);
  if (card) { card.classList.add("is-flash"); setTimeout(() => card.classList.remove("is-flash"), 1200); }
}

const destName = (code) => (state.destinations.find((d) => d.code === code) || { name: code }).name;

function renderOrders(orders) {
  const counts = {};
  orders.forEach((o) => { counts[o.status] = (counts[o.status] || 0) + 1; });
  $("#counters").innerHTML = orders.length
    ? `<span class="counter">всего<b>${orders.length}</b></span>` +
      Object.entries(counts).map(([s, n]) => `<span class="counter">${STATUS_TEXT[s] || s}<b>${n}</b></span>`).join("")
    : "";

  if (!orders.length) {
    $("#orders").innerHTML = `<div class="empty">Заявок пока нет. Выбери направление выше и нажми «Забронировать полёт».</div>`;
    return;
  }
  $("#orders").innerHTML = orders.map(renderOrder).join("");
}

function renderOrder(o) {
  const rejected = o.status === "rejected";
  const idx = rejected ? 1 : STEPS.findIndex((s) => s.status === o.status);
  const progress = Math.max(0, idx) / (STEPS.length - 1);

  const steps = STEPS.map((s, i) => {
    let cls = "";
    let icon = s.icon;
    let label = s.label;
    if (rejected && i === 1) { cls = "is-bad"; icon = "⛔"; label = "Отклонено"; }
    else if (rejected && i > 1) cls = "is-skipped";
    else if (i < idx) cls = "is-done";
    else if (i === idx) cls = o.status === "completed" ? "is-final" : "is-current";
    return `<div class="step ${cls}"><div class="step__dot">${icon}</div>${label}</div>`;
  }).join("");

  const updated = new Date(o.updated_at);
  return `
    <div class="order ${rejected ? "is-rejected" : ""}" data-order="${o.id}">
      <div>
        <div class="order__title">#${o.id} · ${esc(destName(o.destination))}<span class="badge badge--${o.status}">${o.status}</span></div>
        <div class="order__meta">👥 ${o.passengers} пасс. · ${money(o.price)}<br>создана ${time(new Date(o.created_at))} · обновлена ${time(updated)}</div>
      </div>
      <div>
        <div class="track"><div class="track__fill" style="transform:scaleX(${rejected ? 0.25 : progress})"></div>${steps}</div>
        ${o.status_reason ? `<div class="order__reason">💬 ${esc(o.status_reason)}</div>` : ""}
      </div>
    </div>`;
}

// ---------------------------------------------------------------- вкладка «Проверка ЛР»

function initLabs() {
  $("#btn-test").addEventListener("click", async () => {
    const layers = ["handler", "service", "repository"];
    layers.forEach((l, i) => setTimeout(() => $(`[data-layer="${l}"]`).classList.add("is-active"), i * 180));
    try {
      const text = await api("GET", "/test", { auth: false });
      $("#test-result").innerHTML = `<span class="code-ok">200 OK</span>\n${esc(text)}`;
    } catch (err) {
      $("#test-result").innerHTML = `<span class="code-bad">Ошибка</span> ${esc(err.message)}`;
    }
    setTimeout(() => layers.forEach((l) => $(`[data-layer="${l}"]`).classList.remove("is-active")), 1200);
  });

  $("#dbtest-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const text = $("#dbtest-text").value;
    try {
      const msg = await api("POST", "/dbtest", { body: text, auth: false, raw: true });
      toast("🗄️ Записано в PostgreSQL", `id = ${msg.id}`, "ok");
      $("#dbtest-text").value = "";
      loadMessages();
    } catch (err) {
      toast("Ошибка записи", err.message, "bad");
    }
  });

  $$("[data-probe]").forEach((btn) => btn.addEventListener("click", async () => {
    const kind = btn.dataset.probe;
    const token = kind === "none" ? "" : kind === "fake" ? "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.fake-signature" : state.token;
    if (kind === "real" && !state.token) { toast("Сначала войди в систему", "", "bad"); return; }
    try {
      const orders = await api("GET", "/orders?limit=5", { token });
      $("#probe-result").innerHTML = `<span class="code-ok">200 OK</span> — middleware пустил, получено заявок: ${orders.length}`;
    } catch (err) {
      $("#probe-result").innerHTML = `<span class="code-bad">${err.status || ""} ${err.status === 401 ? "Unauthorized" : ""}</span> — ${esc(err.message)}`;
    }
  }));
}

async function loadMessages() {
  try {
    const list = await api("GET", "/dbtest", { auth: false });
    $("#messages").innerHTML = list.length
      ? list.map((m) => `<li>${esc(m.text)}<span>#${m.id} · ${new Date(m.created_at).toLocaleString("ru-RU")}</span></li>`).join("")
      : `<li class="muted">Пока пусто</li>`;
  } catch { /* api недоступен */ }
}

// ---------------------------------------------------------------- вкладка «Мониторинг»

let links = null;
let panelsRendered = false;

async function loadLinks() {
  try {
    links = await api("GET", "/ui/config", { auth: false, quiet: true });
  } catch { return; }
  $("#tool-links").innerHTML = `
    <a class="btn btn--ghost" target="_blank" href="${links.grafana}/d/cosmoport">📊 Grafana</a>
    <a class="btn btn--ghost" target="_blank" href="${links.prometheus}/targets">🔥 Prometheus</a>
    <a class="btn btn--ghost" target="_blank" href="${links.rabbitmq}/#/queues">📨 RabbitMQ</a>`;
}

function renderPanels() {
  if (panelsRendered || !links) return;
  panelsRendered = true;
  const src = (id) => `${links.grafana}/d-solo/cosmoport/cosmoport?orgId=1&panelId=${id}&refresh=5s&from=now-15m&to=now&theme=dark`;
  const stats = [2, 3, 4, 5, 7, 12].map((id) => `<iframe loading="lazy" src="${src(id)}"></iframe>`).join("");
  const charts = [14, 20, 21, 23, 15, 26].map((id) => `<iframe loading="lazy" src="${src(id)}"></iframe>`).join("");
  $("#panels").innerHTML = `<div class="stat-row">${stats}</div>${charts}`;
}

// ---------------------------------------------------------------- старт

function init() {
  initTabs();
  initAuth();
  initBooking();
  initLabs();
  $("#console-clear").addEventListener("click", () => { $("#console-body").innerHTML = ""; });

  logEvent("Интерфейс загружен. Нажми на строку запроса, чтобы увидеть заголовки и ответ.");
  checkHealth();
  setInterval(checkHealth, 3000);
  loadLinks();
  loadDestinations().then(() => { if (state.token) onLoggedIn(); });
  updateBookingState();
}

init();
