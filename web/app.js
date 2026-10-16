'use strict';

/* TrustTunnel Panel — одностраничный интерфейс без фреймворков и сборки. */

const $app = document.getElementById('app');
const S = {
  session: null,
  state: { servers: [], clients: [] },
  theme: localStorage.getItem('ttp-theme') || 'auto',
  q: '',
  serverFilter: '',
};

/* ---------------- утилиты DOM ---------------- */

function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v == null || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
      else if (k === 'value') el.value = v;
      else if (k === 'checked') el.checked = !!v;
      else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
      else el.setAttribute(k, v === true ? '' : v);
    }
  }
  append(el, kids);
  return el;
}

function append(el, kids) {
  for (const k of kids) {
    if (k == null || k === false) continue;
    if (Array.isArray(k)) append(el, k);
    else el.appendChild(k instanceof Node ? k : document.createTextNode(String(k)));
  }
}

const ICONS = {
  plus: 'M12 5v14M5 12h14',
  server: 'M4 4h16v6H4zM4 14h16v6H4zM8 7h.01M8 17h.01',
  users: 'M16 19v-1a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v1M9 10a3 3 0 1 0 0-6 3 3 0 0 0 0 6M22 19v-1a4 4 0 0 0-3-3.9M16 4.1a3 3 0 0 1 0 5.8',
  settings: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z',
  refresh: 'M21 12a9 9 0 1 1-2.6-6.4M21 4v5h-5',
  trash: 'M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6',
  edit: 'M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z',
  share: 'M4 12v8h16v-8M16 6l-4-4-4 4M12 2v13',
  log: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zM14 2v6h6M8 13h8M8 17h6',
  power: 'M18.4 6.6a9 9 0 1 1-12.8 0M12 2v10',
  download: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3',
  copy: 'M9 9h11v11H9zM5 15H4V4h11v1',
  search: 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16zM21 21l-4.3-4.3',
  sun: 'M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10zM12 1v2M12 21v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M1 12h2M21 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4',
  moon: 'M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z',
  auto: 'M12 3a9 9 0 1 0 0 18zM12 3a9 9 0 0 1 0 18',
  logout: 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9',
  back: 'M19 12H5M12 19l-7-7 7-7',
  activity: 'M22 12h-4l-3 9L9 3l-3 9H2',
  check: 'M20 6 9 17l-5-5',
  alert: 'M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0zM12 9v4M12 17h.01',
  info: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 16v-4M12 8h.01',
  shield: 'M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z',
  clock: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 6v6l4 2',
  key: 'M21 2l-2 2m-7.6 7.6a5.5 5.5 0 1 1-7.8 7.8 5.5 5.5 0 0 1 7.8-7.8zm0 0L15.5 7.5m0 0 3 3L22 7l-3-3m-3.5 3.5L19 4',
  close: 'M18 6 6 18M6 6l12 12',
  download2: 'M12 3v12M7 10l5 5 5-5M5 21h14',
};

function icon(name, cls) {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '2');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');
  if (cls) svg.setAttribute('class', cls);
  const p = document.createElementNS(ns, 'path');
  p.setAttribute('d', ICONS[name] || '');
  svg.appendChild(p);
  return svg;
}

function btn(text, opts = {}) {
  const b = h('button', { class: 'btn ' + (opts.kind || ''), type: opts.type || 'button', title: opts.title, onclick: opts.onclick, disabled: opts.disabled },
    opts.icon ? icon(opts.icon) : null, text ? h('span', null, text) : null);
  return b;
}

/** Блокирует кнопку и показывает спиннер, пока выполняется действие. */
async function busy(b, fn) {
  if (b.disabled) return;
  const old = Array.from(b.childNodes);
  b.disabled = true;
  b.replaceChildren(h('span', { class: 'spinner' }), ...old.filter(n => n.nodeName === 'SPAN'));
  try { return await fn(); }
  finally { b.disabled = false; b.replaceChildren(...old); }
}

/* ---------------- API ---------------- */

async function api(method, path, body) {
  const opt = { method, credentials: 'same-origin', headers: { 'X-Requested-With': 'ttpanel' } };
  if (body !== undefined) { opt.headers['Content-Type'] = 'application/json'; opt.body = JSON.stringify(body); }
  const r = await fetch('api' + path, opt);
  let data = null;
  try { data = await r.json(); } catch (_) { }
  if (r.status === 401 && path !== '/login') {
    S.session.user = '';
    closeAllModals();
    render();
    throw new Error('Сессия истекла — войдите снова');
  }
  if (!r.ok) throw new Error((data && data.error) || ('Ошибка ' + r.status));
  return data;
}

async function loadState() {
  S.state = await api('GET', '/state');
}

/** После изменения клиентов служба перезапускается через ~2 с — обновим статус сервера чуть позже. */
function refreshSoon(serverId) {
  setTimeout(async () => {
    try { await api('POST', `/servers/${serverId}/refresh`); await loadState(); if (!document.querySelector('.overlay')) render(); } catch (_) { }
  }, 5000);
}

/* ---------------- уведомления и окна ---------------- */

function toast(msg, kind) {
  const t = h('div', { class: 'toast ' + (kind || '') }, msg);
  document.getElementById('toasts').appendChild(t);
  setTimeout(() => t.remove(), kind === 'err' ? 7000 : 3500);
}

function closeAllModals() { document.querySelectorAll('.overlay').forEach(o => o.remove()); }

function modal({ title, body, foot, wide, onclose }) {
  const close = () => { ov.remove(); document.removeEventListener('keydown', esc); if (onclose) onclose(); };
  const esc = e => { if (e.key === 'Escape') close(); };
  const box = h('div', { class: 'modal' + (wide ? ' wide' : ''), role: 'dialog', 'aria-modal': 'true' },
    h('div', { class: 'modal-head' }, h('h2', null, title), btn('', { kind: 'ghost icon sm', icon: 'close', title: 'Закрыть', onclick: close })),
    h('div', { class: 'modal-body' }, body),
    foot ? h('div', { class: 'modal-foot' }, foot) : null);
  const ov = h('div', { class: 'overlay', onmousedown: e => { if (e.target === ov) close(); } }, box);
  document.body.appendChild(ov);
  document.addEventListener('keydown', esc);
  setTimeout(() => { const f = box.querySelector('input:not([type=hidden]):not([readonly]), select, textarea'); if (f) f.focus(); }, 30);
  return { close, box };
}

function confirmDialog(title, text, okText, danger) {
  return new Promise(resolve => {
    let done = false;
    const finish = v => { if (!done) { done = true; resolve(v); } };
    const ok = btn(okText || 'OK', { kind: danger ? 'danger solid' : 'primary', onclick: () => { finish(true); m.close(); } });
    const m = modal({
      title, body: h('p', { class: 'muted' }, text),
      foot: [btn('Отмена', { onclick: () => m.close() }), ok],
      onclose: () => finish(false),
    });
    setTimeout(() => ok.focus(), 40);
  });
}

function field(label, input, hint) {
  return h('label', { class: 'field' }, h('span', null, label), input, hint ? h('span', { class: 'hint' }, hint) : null);
}

/* ---------------- форматирование ---------------- */

const AVATAR_COLORS = ['#2563eb', '#7c3aed', '#db2777', '#059669', '#d97706', '#0891b2'];
function avatar(name, cls, dotColor) {
  let hash = 0;
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) | 0;
  const a = h('div', { class: 'avatar ' + (cls || '') }, (name[0] || '?').toUpperCase());
  a.style.background = AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length];
  if (dotColor) { const d = h('span', { class: 'dot' }); d.style.background = dotColor; a.appendChild(d); }
  return a;
}

function fmtDate(iso) {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric' });
}
function isoDay(iso) {
  const d = new Date(iso);
  return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
}
function fmtUptime(sec) {
  if (!sec) return '—';
  const d = Math.floor(sec / 86400), hr = Math.floor(sec % 86400 / 3600), m = Math.floor(sec % 3600 / 60);
  return d ? `${d} д ${hr} ч` : hr ? `${hr} ч ${m} мин` : `${m} мин`;
}
function fmtMB(kb) { return kb >= 1048576 ? (kb / 1048576).toFixed(1) + ' ГБ' : Math.round(kb / 1024) + ' МБ'; }
function fmtBytes(b) {
  b = b || 0;
  const u = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
  let i = 0;
  while (b >= 1024 && i < u.length - 1) { b /= 1024; i++; }
  return (i === 0 ? Math.round(b) : b < 10 ? b.toFixed(2) : b < 100 ? b.toFixed(1) : Math.round(b)) + ' ' + u[i];
}
/** Скорость в битах — так её привыкли видеть (как у провайдера). */
function fmtRate(bps) {
  const bits = (bps || 0) * 8;
  if (bits < 1000) return '0 Кбит/с';
  if (bits < 1e6) return Math.round(bits / 1000) + ' Кбит/с';
  return (bits / 1e6).toFixed(bits < 1e7 ? 2 : 1) + ' Мбит/с';
}
function fmtAgo(iso) {
  if (!iso) return 'ещё не подключался';
  const s = Math.max(0, (Date.now() - new Date(iso)) / 1000);
  if (s < 90) return 'был в сети только что';
  if (s < 3600) return `был ${Math.round(s / 60)} мин назад`;
  if (s < 86400) return `был ${Math.round(s / 3600)} ч назад`;
  return 'был ' + new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' });
}
function liveOf(c) { return (S.state.live && S.state.live[c.id]) || null; }
function statsOn(serverId) { const s = serverById(serverId); return !!(s && s.status && s.status.metrics === 'on'); }
function daysLeft(iso) { return iso ? Math.ceil((new Date(iso) - Date.now()) / 86400000) : null; }

function statusOf(s) {
  const st = s.status;
  if (!st) return { text: 'Проверка…', cls: 'grey pulse', color: 'var(--grey)' };
  if (!st.reachable) return { text: 'Недоступен', cls: 'red', color: 'var(--red)' };
  if (!st.sshOk) return { text: 'Нет доступа SSH', cls: 'red', color: 'var(--red)' };
  if (!st.installed) return { text: 'Не установлен', cls: 'grey', color: 'var(--grey)' };
  if (st.active === 'active') return { text: 'Работает', cls: 'green', color: 'var(--green)' };
  if (st.active === 'activating') return { text: 'Запускается', cls: 'orange pulse', color: 'var(--orange)' };
  return { text: 'Остановлен', cls: 'red', color: 'var(--red)' };
}

function serverById(id) { return S.state.servers.find(s => s.id === id); }
function clientsOf(id) { return S.state.clients.filter(c => c.serverId === id); }

/* ---------------- тема ---------------- */

const mq = matchMedia('(prefers-color-scheme: dark)');
function applyTheme() {
  const dark = S.theme === 'dark' || (S.theme === 'auto' && mq.matches);
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  document.querySelector('meta[name=theme-color]').setAttribute('content', dark ? '#1d2027' : '#ffffff');
}
mq.addEventListener('change', applyTheme);
function cycleTheme() {
  S.theme = { auto: 'light', light: 'dark', dark: 'auto' }[S.theme];
  localStorage.setItem('ttp-theme', S.theme);
  applyTheme();
  render();
  toast({ auto: 'Тема: как в системе', light: 'Тема: светлая', dark: 'Тема: тёмная' }[S.theme]);
}

/* ---------------- маршрутизация и отрисовка ---------------- */

function route() {
  const p = (location.hash.slice(1) || '/').split('/').filter(Boolean);
  if (p[0] === 's' && p[1]) return { page: 'server', id: p[1] };
  if (p[0] === 'clients') return { page: 'clients' };
  if (p[0] === 'settings') return { page: 'settings' };
  return { page: 'home' };
}

function render() {
  if (!S.session) return;
  if (S.session.setup) return renderAuth(true);
  if (!S.session.user) return renderAuth(false);

  // сохраняем фокус и положение курсора в поле (например, в поиске) при перерисовке
  const act = document.activeElement;
  const focusId = act && act.id, selStart = act && act.selectionStart;

  const r = route();
  let page;
  if (r.page === 'server') page = pageServer(r.id);
  else if (r.page === 'clients') page = pageClients();
  else if (r.page === 'settings') page = pageSettings();
  else page = pageHome();

  const navLink = (href, text, active, ic) => h('a', { href, class: active ? 'active' : '' }, icon(ic), h('span', null, text));
  const themeIcon = { auto: 'auto', light: 'sun', dark: 'moon' }[S.theme];
  $app.replaceChildren(
    h('header', { class: 'topbar' }, h('div', { class: 'topbar-in' },
      h('a', { class: 'brand', href: '#/' }, h('img', { src: 'icon.svg', alt: '' }), h('span', null, 'TrustTunnel Panel')),
      h('nav', { class: 'nav' },
        navLink('#/', 'Серверы', r.page === 'home' || r.page === 'server', 'server'),
        navLink('#/clients', 'Клиенты', r.page === 'clients', 'users'),
        navLink('#/settings', 'Настройки', r.page === 'settings', 'settings')),
      h('div', { class: 'top-actions' },
        btn('', { kind: 'ghost icon', icon: themeIcon, title: 'Сменить тему', onclick: cycleTheme }),
        btn('', { kind: 'ghost icon', icon: 'logout', title: 'Выйти', onclick: logout })))),
    h('main', null, page));

  if (focusId) {
    const el = document.getElementById(focusId);
    if (el) { el.focus(); if (selStart != null && el.setSelectionRange) try { el.setSelectionRange(selStart, selStart); } catch (_) { } }
  }
  S.sig = structureSig();
  S.renderedAt = Date.now();
  updateLive();
}

/* ---------------- живая статистика ---------------- */

function plural(n, one, few, many) {
  const m10 = n % 10, m100 = n % 100;
  return m10 === 1 && m100 !== 11 ? one : m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14) ? few : many;
}

function serverLive(sid) {
  let online = 0, rx = 0, tx = 0, total = 0;
  for (const c of clientsOf(sid)) {
    const l = liveOf(c);
    if (l && l.online) { online++; rx += l.rxRate; tx += l.txRate; }
    total += (c.trafficIn || 0) + (c.trafficOut || 0);
  }
  return { online, rx, tx, total };
}

/** Обновляет только ячейки статистики (data-live) — без перерисовки страницы, чтобы не мигало. */
function updateLive() {
  const byId = new Map(S.state.clients.map(c => [c.id, c]));
  document.querySelectorAll('[data-live]').forEach(el => {
    const [kind, id] = el.dataset.live.split(':');
    if (kind === 'dash-online') {
      const on = S.state.servers.some(s => s.status && s.status.metrics === 'on');
      el.textContent = on ? String(S.state.clients.filter(c => { const l = liveOf(c); return l && l.online; }).length) : '—';
      return;
    }
    if (kind === 'dash-traffic') {
      el.textContent = fmtBytes(S.state.clients.reduce((n, c) => n + (c.trafficIn || 0) + (c.trafficOut || 0), 0));
      return;
    }
    if (kind.startsWith('srv-')) {
      const on = statsOn(id), v = serverLive(id);
      if (kind === 'srv-online') el.textContent = on ? String(v.online) : '—';
      else if (kind === 'srv-speed') el.textContent = on ? `↓ ${fmtRate(v.rx)}  ↑ ${fmtRate(v.tx)}` : '—';
      else if (kind === 'srv-traffic') el.textContent = on || v.total ? fmtBytes(v.total) : '—';
      return;
    }
    const c = byId.get(id);
    if (!c) return;
    const l = liveOf(c), on = statsOn(c.serverId), online = !!(on && c.enabled && l && l.online);
    switch (kind) {
      case 'dot':
        el.classList.toggle('hidden', !on);
        el.style.background = online ? 'var(--green)' : 'var(--grey)';
        el.title = online ? 'в сети' : 'не в сети';
        break;
      case 'status':
        if (!on) { el.textContent = ''; break; }
        if (!c.enabled) { el.className = 'small muted'; el.textContent = 'доступ отключён'; break; }
        if (online) {
          el.className = 'small online';
          el.textContent = 'в сети' + (l.sessions > 1 ? ` · ${l.sessions} ${plural(l.sessions, 'сеанс', 'сеанса', 'сеансов')}` : '') + (l.ip ? ' · ' + l.ip : '');
        } else { el.className = 'small muted'; el.textContent = fmtAgo(c.lastSeen); }
        break;
      case 'speed':
        if (!online) el.replaceChildren(h('span', { class: 'muted' }, '—'));
        else el.replaceChildren(
          h('div', { class: 'rate', title: 'скачивание' }, '↓ ' + fmtRate(l.rxRate)),
          h('div', { class: 'rate up', title: 'отдача' }, '↑ ' + fmtRate(l.txRate)));
        break;
      case 'traffic': {
        const total = (c.trafficIn || 0) + (c.trafficOut || 0);
        if (!total) { el.replaceChildren(h('span', { class: 'muted' }, '—')); break; }
        el.replaceChildren(h('div', { class: 'strong' }, fmtBytes(total)),
          h('div', { class: 'small muted' }, `↓ ${fmtBytes(c.trafficOut)} · ↑ ${fmtBytes(c.trafficIn)}`));
        break;
      }
    }
  });
}

/** «Отпечаток» структуры: если он не изменился, достаточно обновить статистику, а не всю страницу. */
function structureSig() {
  return JSON.stringify([
    S.state.servers.map(s => [s.id, s.name, s.publicAddress, s.status && [s.status.reachable, s.status.sshOk, s.status.installed, s.status.active, s.status.metrics]]),
    S.state.clients.map(c => { const l = liveOf(c); return [c.id, c.username, c.enabled, c.note, c.expiresAt, !!(l && l.online)]; }),
    S.state.statsErrors,
  ]);
}

/* ---------------- вход и первый запуск ---------------- */

function renderAuth(setup) {
  const user = h('input', { type: 'text', id: 'u', autocomplete: 'username', value: setup ? 'admin' : '' });
  const pass = h('input', { type: 'password', id: 'p', autocomplete: setup ? 'new-password' : 'current-password' });
  const pass2 = setup ? h('input', { type: 'password', autocomplete: 'new-password' }) : null;
  const err = h('div', { class: 'err' });
  const submit = btn(setup ? 'Создать и войти' : 'Войти', { kind: 'primary', type: 'submit' });
  const form = h('form', {
    onsubmit: async e => {
      e.preventDefault();
      err.textContent = '';
      if (setup && pass.value !== pass2.value) { err.textContent = 'Пароли не совпадают'; return; }
      await busy(submit, async () => {
        try {
          await api('POST', setup ? '/setup' : '/login', { username: user.value.trim(), password: pass.value });
          S.session = await api('GET', '/session');
          await loadState();
          render();
        } catch (ex) { err.textContent = ex.message; }
      });
    },
  },
    field('Логин', user),
    field('Пароль', pass, setup ? 'Не короче 8 символов' : null),
    setup ? field('Повторите пароль', pass2) : null,
    err, submit);
  $app.replaceChildren(h('div', { class: 'auth' }, h('div', { class: 'card' },
    h('img', { class: 'logo', src: 'icon.svg', alt: '' }),
    h('h1', null, setup ? 'Добро пожаловать' : 'TrustTunnel Panel'),
    h('p', null, setup ? 'Создайте учётную запись администратора панели' : 'Войдите, чтобы управлять серверами'),
    form)));
  setTimeout(() => (setup ? pass : user).focus(), 30);
}

async function logout() {
  await api('POST', '/logout');
  S.session.user = '';
  render();
}

/* ---------------- обзор ---------------- */

function pageHome() {
  const servers = S.state.servers, clients = S.state.clients;
  const online = servers.filter(s => s.status && s.status.active === 'active').length;
  const active = clients.filter(c => c.enabled).length;
  const soon = clients.filter(c => c.enabled && c.expiresAt && daysLeft(c.expiresAt) <= 7).length;

  const stat = (ic, v, l, live) => h('div', { class: 'card stat' }, h('div', { class: 'ico' }, icon(ic)),
    h('div', null, h('div', { class: 'v', 'data-live': live }, v), h('div', { class: 'l' }, l)));

  if (!servers.length) {
    return h('div', null,
      h('div', { class: 'card cta' },
        icon('shield', 'big'),
        h('h2', null, 'Добавьте первый сервер'),
        h('p', null, 'Укажите адрес VPS и пароль root — панель подключится по SSH, установит свой ключ и покажет состояние TrustTunnel. ' +
          'Если TrustTunnel ещё не установлен, панель поставит его сама.'),
        btn('Добавить сервер', { kind: 'primary', icon: 'plus', onclick: () => serverModal() })));
  }

  return h('div', null,
    h('div', { class: 'stats' },
      stat('server', `${online} из ${servers.length}`, 'серверов работает'),
      stat('activity', '—', `клиентов в сети (из ${active} активных)`, 'dash-online'),
      stat('download', '—', 'трафика всего', 'dash-traffic'),
      stat('clock', String(soon), 'истекают за 7 дней')),
    h('div', { class: 'page-head' },
      h('div', { class: 'grow' }, h('h1', null, 'Серверы')),
      btn('Обновить', { kind: 'ghost', icon: 'refresh', onclick: e => busy(e.currentTarget, refreshAll) }),
      btn('Добавить сервер', { kind: 'primary', icon: 'plus', onclick: () => serverModal() })),
    h('div', { class: 'grid' },
      servers.map(serverCard),
      h('button', { class: 'add-card', onclick: () => serverModal() }, h('div', null, icon('plus'), 'Добавить сервер'))));
}

async function refreshAll() {
  await Promise.all(S.state.servers.map(s => api('POST', `/servers/${s.id}/refresh`).catch(() => null)));
  await loadState();
  render();
}

function serverCard(s) {
  const st = statusOf(s), x = s.status || {};
  const memPct = x.memTotal ? Math.round(x.memUsed / x.memTotal * 100) : 0;
  const bar = h('div', { class: 'bar' }, h('i'));
  bar.firstChild.style.width = memPct + '%';
  const nClients = clientsOf(s.id).filter(c => c.enabled).length;
  return h('a', { class: 'card server-card', href: '#/s/' + s.id },
    h('div', { class: 'head' },
      avatar(s.name, '', st.color),
      h('div', { style: { minWidth: 0, flex: 1 } },
        h('div', { class: 'name' }, s.name),
        h('div', { class: 'addr' }, s.publicAddress || s.host)),
      h('span', { class: 'pill ' + st.cls }, st.text)),
    h('div', { class: 'metrics' },
      h('div', { class: 'metric' }, h('div', { class: 'l' }, 'Клиенты'), h('div', { class: 'v' }, String(nClients))),
      h('div', { class: 'metric' }, h('div', { class: 'l' }, 'В сети'), h('div', { class: 'v', 'data-live': 'srv-online:' + s.id }, '—')),
      h('div', { class: 'metric' }, h('div', { class: 'l' }, 'Пинг'), h('div', { class: 'v' }, x.reachable ? x.pingMs + ' мс' : '—'))),
    x.memTotal ? h('div', { class: 'small muted' }, `Память ${fmtMB(x.memUsed)} из ${fmtMB(x.memTotal)} · нагрузка ${x.load}`, bar) : null);
}

/* ---------------- сервер ---------------- */

function pageServer(id) {
  const s = serverById(id);
  if (!s) return h('div', { class: 'card empty' }, 'Сервер не найден. ', h('a', { href: '#/' }, 'К списку серверов'));
  const st = statusOf(s), x = s.status || {};
  const installed = x.installed;

  const kv = (l, v) => h('div', { class: 'metric' }, h('div', { class: 'l' }, l), h('div', { class: 'v' }, v));
  const memPct = x.memTotal ? Math.round(x.memUsed / x.memTotal * 100) : 0;
  const bar = h('div', { class: 'bar' }, h('i'));
  bar.firstChild.style.width = memPct + '%';

  const hero = h('div', { class: 'card' },
    h('div', { class: 'server-hero' },
      h('div', { class: 'info' },
        avatar(s.name, '', st.color),
        h('div', { style: { minWidth: 0 } },
          h('h1', null, s.name),
          h('div', { class: 'muted small' }, `${s.user}@${s.host}:${s.port}`, s.publicAddress ? ` · клиенты подключаются к ${s.publicAddress}` : '')),
        h('span', { class: 'pill ' + st.cls }, st.text)),
      h('div', { class: 'actions' },
        btn('', { kind: 'icon', icon: 'refresh', title: 'Обновить состояние', onclick: e => busy(e.currentTarget, () => refreshServer(s.id)) }),
        installed ? btn('Журнал', { icon: 'log', onclick: () => logsModal(s) }) : null,
        installed ? (x.active === 'active'
          ? btn('Перезапустить', { icon: 'power', onclick: e => serviceAction(e.currentTarget, s, 'restart') })
          : btn('Запустить', { kind: 'primary', icon: 'power', onclick: e => serviceAction(e.currentTarget, s, 'start') })) : null,
        btn('', { kind: 'icon', icon: 'edit', title: 'Изменить сервер', onclick: () => serverModal(s) }),
        btn('', { kind: 'icon danger', icon: 'trash', title: 'Удалить из панели', onclick: () => deleteServer(s) }))),
    x.sshOk ? h('div', { class: 'kv' },
      h('div', { class: 'metric' }, h('div', { class: 'l' }, 'Клиентов онлайн'), h('div', { class: 'v', 'data-live': 'srv-online:' + s.id }, '—')),
      h('div', { class: 'metric wide' }, h('div', { class: 'l' }, 'Скорость сейчас'), h('div', { class: 'v', 'data-live': 'srv-speed:' + s.id }, '—')),
      h('div', { class: 'metric' }, h('div', { class: 'l' }, 'Трафик всего'), h('div', { class: 'v', 'data-live': 'srv-traffic:' + s.id }, '—')),
      kv('Версия', x.version || '—'),
      kv('Порт VPN', x.port || '—'),
      kv('Пинг', x.pingMs + ' мс'),
      kv('Соединений', String(x.conns)),
      kv('Нагрузка', x.load || '—'),
      h('div', { class: 'metric' }, h('div', { class: 'l' }, 'Память'), h('div', { class: 'v' }, `${fmtMB(x.memUsed)} / ${fmtMB(x.memTotal)}`), bar),
      kv('Аптайм', fmtUptime(x.uptime))) : null);

  const parts = [h('div', { class: 'crumbs' }, h('a', { href: '#/' }, '← Все серверы')), hero];

  if (x.error) parts.push(h('div', { class: 'callout warn', style: { marginTop: '14px' } }, icon('alert'), h('div', null, x.error)));

  if (x.sshOk && !installed) {
    parts.push(h('div', { class: 'card cta', style: { marginTop: '18px' } },
      icon('shield', 'big'),
      h('h2', null, 'TrustTunnel ещё не установлен'),
      h('p', null, 'Панель скачает официальный сервер TrustTunnel, создаст конфигурацию, первого клиента и запустит службу. Обычно это занимает 1–2 минуты.'),
      btn('Установить TrustTunnel', { kind: 'primary', icon: 'download2', onclick: () => installModal(s) })));
    return h('div', null, parts);
  }

  if (installed && x.metrics && x.metrics !== 'on') {
    parts.push(h('div', { class: 'callout', style: { marginTop: '14px', alignItems: 'center' } }, icon('activity'),
      h('div', { style: { flex: 1 } }, h('b', null, 'Статистика клиентов выключена. '),
        'Включите её, чтобы видеть, кто сейчас в сети, текущую скорость и потраченный трафик. ',
        'Метрики будут доступны только локально на сервере (127.0.0.1), служба перезапустится на пару секунд.'),
      btn('Включить статистику', { kind: 'primary', icon: 'activity', onclick: e => enableStats(e.currentTarget, s) })));
  }
  const statsErr = S.state.statsErrors && S.state.statsErrors[s.id];
  if (statsErr) parts.push(h('div', { class: 'callout warn', style: { marginTop: '14px' } }, icon('alert'), h('div', null, statsErr)));

  parts.push(clientsSection(clientsOf(s.id), s));
  return h('div', null, parts);
}

async function enableStats(b, s) {
  if (!await confirmDialog('Включить статистику клиентов?', 'В конфиг сервера добавится раздел [metrics] (только 127.0.0.1), и служба TrustTunnel перезапустится — клиенты переподключатся за пару секунд.', 'Включить')) return;
  await busy(b, async () => {
    try {
      await api('POST', `/servers/${s.id}/metrics`);
      toast('Статистика включается — данные появятся через несколько секунд', 'ok');
      setTimeout(async () => { try { await loadState(); render(); } catch (_) { } }, 6000);
    } catch (e) { toast(e.message, 'err'); }
  });
}

async function refreshServer(id) {
  await api('POST', `/servers/${id}/refresh`);
  await loadState();
  render();
}

async function serviceAction(b, s, action) {
  if (action === 'restart' && !await confirmDialog('Перезапустить TrustTunnel?', 'Подключённые клиенты переподключатся через несколько секунд.', 'Перезапустить')) return;
  await busy(b, async () => {
    try {
      await api('POST', `/servers/${s.id}/service`, { action });
      toast(action === 'restart' ? 'Служба перезапущена' : 'Служба запущена', 'ok');
      await loadState(); render();
    } catch (e) { toast(e.message, 'err'); }
  });
}

async function deleteServer(s) {
  if (!await confirmDialog('Удалить сервер из панели?', `«${s.name}» и его клиенты пропадут из панели. На самом сервере ничего не удаляется — TrustTunnel продолжит работать.`, 'Удалить', true)) return;
  try {
    await api('DELETE', `/servers/${s.id}`);
    toast('Сервер удалён из панели', 'ok');
    location.hash = '#/';
    await loadState(); render();
  } catch (e) { toast(e.message, 'err'); }
}

/* ---------------- клиенты ---------------- */

function clientsSection(list, server) {
  const q = S.q.trim().toLowerCase();
  const isOn = c => { const l = liveOf(c); return c.enabled && l && l.online ? 1 : 0; };
  // сначала те, кто в сети, затем по времени последнего подключения, затем по дате создания
  const filtered = list.filter(c => (!q || c.username.toLowerCase().includes(q) || (c.note || '').toLowerCase().includes(q)) &&
    (server || !S.serverFilter || c.serverId === S.serverFilter))
    .slice().sort((a, b) => isOn(b) - isOn(a) ||
      (b.lastSeen ? Date.parse(b.lastSeen) : 0) - (a.lastSeen ? Date.parse(a.lastSeen) : 0) ||
      Date.parse(a.createdAt) - Date.parse(b.createdAt));
  const search = h('div', { class: 'search' }, icon('search'),
    h('input', { type: 'text', id: 'q', placeholder: 'Поиск по имени или заметке', value: S.q, oninput: e => { S.q = e.target.value; render(); } }));

  const head = h('div', { class: 'section-head' },
    h('div', { class: 'grow' }, h('h2', null, server ? `Клиенты · ${list.length}` : 'Все клиенты')),
    search,
    !server && S.state.servers.length > 1 ? h('select', {
      onchange: e => { S.serverFilter = e.target.value; render(); }, style: { width: '200px' },
    }, h('option', { value: '' }, 'Все серверы'), S.state.servers.map(s => h('option', { value: s.id, selected: S.serverFilter === s.id }, s.name))) : null,
    btn('Добавить клиента', { kind: 'primary', icon: 'plus', onclick: () => clientModal(null, server), disabled: !S.state.servers.length }));

  if (!filtered.length) {
    return h('div', null, head, h('div', { class: 'card empty' }, icon('users'),
      list.length ? 'Ничего не найдено' : 'Клиентов пока нет. Добавьте первого — панель выдаст ссылку и QR-код для подключения.'));
  }

  const rows = filtered.map(c => {
    const srv = serverById(c.serverId);
    const left = daysLeft(c.expiresAt);
    const exp = !c.expiresAt ? h('span', { class: 'muted' }, 'бессрочно')
      : h('span', { class: left < 0 ? 'expired' : left <= 7 ? 'soon' : '' }, (left < 0 ? 'истёк ' : 'до ') + fmtDate(c.expiresAt));
    const sw = h('input', { type: 'checkbox', checked: c.enabled });
    sw.addEventListener('change', () => toggleClient(c, sw));
    // ячейки со статистикой обновляются «вживую» через updateLive() без перерисовки таблицы
    const av = avatar(c.username, 'sm');
    const dot = h('span', { class: 'dot', 'data-live': 'dot:' + c.id });
    av.appendChild(dot);
    return h('tr', { class: c.enabled ? '' : 'off' },
      h('td', null, h('div', { class: 'user' }, av,
        h('div', { class: 'user-text' },
          h('div', null, c.username),
          h('div', { class: 'small', 'data-live': 'status:' + c.id })))),
      !server ? h('td', { class: 'hide-m' }, srv ? h('a', { href: '#/s/' + srv.id }, srv.name) : '—') : null,
      h('td', { class: 'hide-m num' }, h('div', { 'data-live': 'speed:' + c.id })),
      h('td', { class: 'num' }, h('div', { 'data-live': 'traffic:' + c.id })),
      h('td', { class: 'hide-m' }, h('div', { class: 'note', title: c.note || '' }, c.note || '—')),
      h('td', { class: 'hide-m' }, exp),
      h('td', null, h('label', { class: 'switch', title: c.enabled ? 'Отключить доступ' : 'Включить доступ' }, sw, h('span'))),
      h('td', { class: 'r' },
        btn('Подключить', { kind: 'sm primary', icon: 'share', title: 'Ссылка и QR-код', onclick: () => shareModal(c), disabled: !c.enabled }),
        ' ',
        btn('', { kind: 'sm icon ghost', icon: 'edit', title: 'Изменить', onclick: () => clientModal(c) }),
        btn('', { kind: 'sm icon ghost danger', icon: 'trash', title: 'Удалить', onclick: () => deleteClient(c) })));
  });

  return h('div', null, head,
    h('div', { class: 'card', style: { overflowX: 'auto' } },
      h('table', { class: 'table' },
        h('thead', null, h('tr', null,
          h('th', null, 'Клиент'), !server ? h('th', { class: 'hide-m' }, 'Сервер') : null,
          h('th', { class: 'hide-m num' }, 'Скорость'), h('th', { class: 'num' }, 'Трафик'),
          h('th', { class: 'hide-m' }, 'Заметка'), h('th', { class: 'hide-m' }, 'Срок'), h('th', null, 'Доступ'), h('th'))),
        h('tbody', null, rows))),
    h('div', { class: 'small muted', style: { marginTop: '8px' } },
      'Добавление, удаление и отключение клиентов применяются перезапуском службы — подключённые пользователи переподключатся за пару секунд.'));
}

function pageClients() {
  if (!S.state.servers.length) return h('div', { class: 'card cta' }, icon('server', 'big'), h('h2', null, 'Сначала добавьте сервер'),
    h('p', null, 'Клиенты создаются на конкретном сервере TrustTunnel.'), btn('Добавить сервер', { kind: 'primary', icon: 'plus', onclick: () => serverModal() }));
  return clientsSection(S.state.clients, null);
}

async function toggleClient(c, sw) {
  sw.disabled = true;
  try {
    await api('PUT', `/clients/${c.id}`, { note: c.note, enabled: sw.checked });
    toast(sw.checked ? `Доступ для ${c.username} включён` : `Доступ для ${c.username} отключён`, 'ok');
    await loadState();
    refreshSoon(c.serverId);
  } catch (e) { toast(e.message, 'err'); sw.checked = !sw.checked; }
  sw.disabled = false;
  render();
}

async function deleteClient(c) {
  if (!await confirmDialog('Удалить клиента?', `${c.username} больше не сможет подключиться. Это действие нельзя отменить.`, 'Удалить', true)) return;
  try {
    await api('DELETE', `/clients/${c.id}`);
    toast('Клиент удалён', 'ok');
    await loadState(); render();
    refreshSoon(c.serverId);
  } catch (e) { toast(e.message, 'err'); }
}

function clientModal(c, server) {
  const editing = !!c;
  const servers = S.state.servers.filter(s => s.status && s.status.installed);
  const name = h('input', { type: 'text', value: editing ? c.username : '', placeholder: 'например, ivan-phone', readonly: editing, maxlength: 32, autocomplete: 'off' });
  const sel = h('select', null, servers.map(s => h('option', { value: s.id, selected: (server ? server.id : (c && c.serverId)) === s.id }, s.name)));
  const note = h('input', { type: 'text', value: editing ? c.note : '', placeholder: 'кто это, какое устройство', maxlength: 120 });
  const date = h('input', { type: 'date', value: editing && c.expiresAt ? isoDay(c.expiresAt) : '' });
  const quick = (label, days) => h('button', {
    type: 'button', onclick: e => {
      if (!days) date.value = '';
      else { const d = new Date(); d.setDate(d.getDate() + days); date.value = isoDay(d); }
      quickBar.querySelectorAll('button').forEach(b => b.classList.toggle('on', b === e.currentTarget));
    },
  }, label);
  const quickBar = h('div', { class: 'seg', style: { marginTop: '6px' } }, quick('Бессрочно', 0), quick('30 дней', 30), quick('90 дней', 90), quick('1 год', 365));
  const err = h('div', { class: 'err' });
  const ok = btn(editing ? 'Сохранить' : 'Создать', { kind: 'primary', type: 'submit' });

  if (!editing && !servers.length) {
    modal({ title: 'Новый клиент', body: h('p', { class: 'muted' }, 'Нет серверов с установленным TrustTunnel.') });
    return;
  }
  const form = h('form', {
    onsubmit: async e => {
      e.preventDefault(); err.textContent = '';
      await busy(ok, async () => {
        try {
          if (editing) {
            await api('PUT', `/clients/${c.id}`, { note: note.value, expiresAt: date.value });
            toast('Сохранено', 'ok');
            m.close();
            await loadState(); render();
          } else {
            const r = await api('POST', '/clients', { serverId: sel.value, username: name.value.trim(), note: note.value, expiresAt: date.value || null });
            m.close();
            await loadState(); render();
            toast(`Клиент ${name.value.trim()} создан`, 'ok');
            refreshSoon(sel.value || (server && server.id));
            const created = S.state.clients.find(x => x.id === r.id);
            if (created) shareModal(created);
          }
        } catch (ex) { err.textContent = ex.message; }
      });
    },
  },
    field('Имя', name, editing ? 'Имя изменить нельзя — это логин на сервере' : 'Латиница, цифры, точка, дефис, подчёркивание'),
    !editing && !server ? field('Сервер', sel) : null,
    field('Заметка', note),
    h('label', { class: 'field' }, h('span', null, 'Действует до'), date, quickBar,
      h('span', { class: 'hint' }, 'После этой даты панель автоматически отключит доступ')),
    h('button', { type: 'submit', class: 'hidden' }),
    err);
  const m = modal({
    title: editing ? c.username : 'Новый клиент',
    body: form,
    foot: [h('div', { class: 'note' }, editing ? '' : 'Пароль сгенерируется автоматически. Сервер перезапустится на пару секунд.'),
      btn('Отмена', { onclick: () => m.close() }), ok],
  });
  ok.addEventListener('click', () => form.requestSubmit());
  ok.type = 'button';
}

function shareModal(c) {
  const srv = serverById(c.serverId);
  const qrWrap = h('div', { class: 'qr-wrap' }, h('div', { class: 'spinner' }));
  const link = h('input', { type: 'text', readonly: true, value: 'Получаю ссылку…', onfocus: e => e.target.select() });
  const copy = btn('Копировать', { kind: 'primary', icon: 'copy', disabled: true });
  copy.addEventListener('click', async () => {
    try { await navigator.clipboard.writeText(link.value); }
    catch (_) { link.select(); document.execCommand('copy'); }
    toast('Ссылка скопирована', 'ok');
  });
  const dl = h('a', { class: 'btn', href: `api/clients/${c.id}/config.toml`, download: '' }, icon('download'), h('span', null, 'Файл .toml'));

  api('GET', `/clients/${c.id}/link`).then(r => {
    link.value = r.link;
    copy.disabled = false;
    const img = h('img', { class: 'qr', alt: 'QR-код для подключения', src: `api/clients/${c.id}/qr.png?t=${Date.now()}` });
    img.addEventListener('load', () => qrWrap.replaceChildren(img));
    img.addEventListener('error', () => qrWrap.replaceChildren(h('div', { class: 'muted' }, 'Не удалось построить QR-код')));
  }).catch(e => { link.value = ''; qrWrap.replaceChildren(h('div', { class: 'err' }, e.message)); });

  const m = modal({
    title: `Подключение: ${c.username}`,
    body: [
      h('div', { class: 'muted small', style: { textAlign: 'center' } }, srv ? srv.name : ''),
      qrWrap,
      h('div', { class: 'linkbox' }, link, copy),
      h('div', { class: 'callout', style: { marginTop: '14px' } }, icon('info'), h('div', null,
        h('ol', { class: 'steps' },
          h('li', null, 'Телефон: установите приложение TrustTunnel и отсканируйте QR-код.'),
          h('li', null, 'Windows: в TrustTunnel нажмите «Добавить» → «Из буфера обмена» после копирования ссылки.'),
          h('li', null, 'Ссылка содержит логин и пароль — передавайте её только владельцу.')))),
    ],
    foot: [dl, btn('Готово', { onclick: () => m.close() })],
  });
}

/* ---------------- серверы: окна ---------------- */

function serverModal(s) {
  const editing = !!s;
  const name = h('input', { type: 'text', value: editing ? s.name : '', placeholder: 'например, Нидерланды' });
  const host = h('input', { type: 'text', value: editing ? s.host : '', placeholder: 'IP-адрес или домен', autocomplete: 'off' });
  const port = h('input', { type: 'number', value: editing ? s.port : 22, min: 1, max: 65535 });
  const user = h('input', { type: 'text', value: editing ? s.user : 'root' });
  const pass = h('input', { type: 'password', autocomplete: 'new-password', placeholder: editing ? '' : 'пароль SSH' });
  const pub = h('input', { type: 'text', value: editing ? s.publicAddress : '', placeholder: 'необязательно' });
  const err = h('div', { class: 'err' });
  const ok = btn(editing ? 'Сохранить' : 'Подключить', { kind: 'primary' });
  const submit = async () => {
    err.textContent = '';
    const body = { name: name.value, host: host.value, port: +port.value, user: user.value, publicAddress: pub.value };
    if (!editing) body.password = pass.value;
    await busy(ok, async () => {
      try {
        if (editing) {
          await api('PUT', `/servers/${s.id}`, body);
          await api('POST', `/servers/${s.id}/refresh`).catch(() => null);
          toast('Сервер сохранён', 'ok');
          m.close();
        } else {
          const r = await api('POST', '/servers', body);
          m.close();
          toast(r.installed ? `Сервер подключён. Импортировано клиентов: ${r.imported}` : 'Сервер подключён. Можно установить TrustTunnel.', 'ok');
          location.hash = '#/s/' + r.id;
        }
        await loadState(); render();
      } catch (ex) { err.textContent = ex.message; }
    });
  };
  ok.addEventListener('click', submit);
  const form = h('form', { onsubmit: e => { e.preventDefault(); submit(); } },
    field('Название', name),
    h('div', { class: 'row' }, field('Адрес сервера', host), h('div', { style: { flex: '0 0 110px' } }, field('Порт SSH', port))),
    h('div', { class: 'row' }, field('Пользователь', user),
      !editing ? field('Пароль', pass) : null),
    !editing ? h('div', { class: 'callout' }, icon('key'), h('div', null,
      'Пароль нужен один раз: панель войдёт по нему, установит свой SSH-ключ и дальше работает только по ключу. Пароль не сохраняется. ',
      'Если ключ панели уже добавлен на сервер, пароль можно не вводить.')) : null,
    field('Адрес для клиентов', pub, 'Если клиенты должны подключаться по домену или другому IP/порту, например vpn.example.com'),
    h('button', { type: 'submit', class: 'hidden' }),
    err);
  const m = modal({ title: editing ? 'Изменить сервер' : 'Новый сервер', body: form, foot: [btn('Отмена', { onclick: () => m.close() }), ok] });
}

function installModal(s) {
  let cert = 'self-signed';
  const hostname = h('input', { type: 'text', value: 'vpn.tt.internal' });
  const port = h('input', { type: 'number', value: 443, min: 1, max: 65535 });
  const email = h('input', { type: 'email', placeholder: 'you@example.com' });
  const first = h('input', { type: 'text', value: 'user1', maxlength: 32 });
  const emailField = field('E-mail для Let\'s Encrypt', email, 'Домен должен указывать на этот сервер, порт 80 — быть открыт');
  emailField.classList.add('hidden');
  const hostHint = h('span', { class: 'hint' }, 'Для самоподписанного сертификата подойдёт любое имя — сертификат попадёт в ссылку клиента автоматически.');
  const segBtn = (val, label) => h('button', {
    type: 'button', class: val === cert ? 'on' : '', onclick: e => {
      cert = val;
      seg.querySelectorAll('button').forEach(b => b.classList.toggle('on', b === e.currentTarget));
      emailField.classList.toggle('hidden', val !== 'letsencrypt');
      hostHint.textContent = val === 'letsencrypt' ? 'Ваш домен, направленный на этот сервер (A-запись).' : 'Для самоподписанного сертификата подойдёт любое имя — сертификат попадёт в ссылку клиента автоматически.';
      if (val === 'letsencrypt' && hostname.value === 'vpn.tt.internal') hostname.value = '';
    },
  }, label);
  const seg = h('div', { class: 'seg' }, segBtn('self-signed', 'Самоподписанный'), segBtn('letsencrypt', 'Let\'s Encrypt'));
  const err = h('div', { class: 'err' });
  const ok = btn('Установить', { kind: 'primary', icon: 'download2' });
  ok.addEventListener('click', () => busy(ok, async () => {
    err.textContent = '';
    try {
      const r = await api('POST', `/servers/${s.id}/install`, { hostname: hostname.value.trim(), port: +port.value, certType: cert, email: email.value.trim(), firstClient: first.value.trim() });
      m.close();
      jobModal(r.job, s);
    } catch (ex) { err.textContent = ex.message; }
  }));
  const m = modal({
    title: 'Установка TrustTunnel',
    body: [
      h('label', { class: 'field' }, h('span', null, 'Сертификат'), seg),
      h('label', { class: 'field' }, h('span', null, 'Имя хоста (TLS)'), hostname, hostHint),
      emailField,
      h('div', { class: 'row' }, field('Порт VPN', port, 'Обычно 443 — так трафик похож на HTTPS'), field('Первый клиент', first)),
      err],
    foot: [btn('Отмена', { onclick: () => m.close() }), ok],
  });
}

function jobModal(jobId, s) {
  const log = h('pre', { class: 'logbox mono' }, 'Запуск…');
  const state = h('div', { class: 'note' }, h('span', { class: 'spinner' }), ' Идёт установка, не закрывайте окно');
  let stopped = false;
  const done = btn('Готово', { kind: 'primary', disabled: true, onclick: () => m.close() });
  const m = modal({ title: 'Установка на ' + s.name, wide: true, body: log, foot: [state, done], onclose: () => { stopped = true; } });
  const tick = async () => {
    if (stopped) return;
    try {
      const j = await api('GET', `/jobs/${jobId}`);
      const atBottom = log.scrollTop + log.clientHeight >= log.scrollHeight - 30;
      log.textContent = j.log || '…';
      if (atBottom) log.scrollTop = log.scrollHeight;
      if (j.done) {
        done.disabled = false;
        state.replaceChildren(j.ok ? h('span', { class: 'pill green' }, 'Установлено') : h('span', { class: 'pill red' }, 'Ошибка: ' + j.error));
        await loadState(); render();
        toast(j.ok ? 'TrustTunnel установлен и запущен' : 'Установка не удалась', j.ok ? 'ok' : 'err');
        return;
      }
    } catch (e) { state.textContent = e.message; }
    setTimeout(tick, 1000);
  };
  tick();
}

function logsModal(s) {
  const log = h('pre', { class: 'logbox mono' }, 'Загрузка…');
  const load = async () => {
    try {
      const r = await api('GET', `/servers/${s.id}/logs`);
      log.textContent = r.log || 'Журнал пуст';
      log.scrollTop = log.scrollHeight;
    } catch (e) { log.textContent = e.message; }
  };
  const m = modal({
    title: 'Журнал TrustTunnel · ' + s.name, wide: true, body: log,
    foot: [btn('Обновить', { icon: 'refresh', onclick: e => busy(e.currentTarget, load) }), btn('Закрыть', { kind: 'primary', onclick: () => m.close() })],
  });
  load();
}

/* ---------------- настройки ---------------- */

function pageSettings() {
  const box = h('div', null, h('div', { class: 'page-head' }, h('div', { class: 'grow' }, h('h1', null, 'Настройки'))));
  const keyArea = h('textarea', { readonly: true, rows: 3, class: 'mono' }, '');
  const ver = h('span', null, '');
  api('GET', '/settings').then(r => { keyArea.value = r.publicKey; ver.textContent = r.version; });

  const cur = h('input', { type: 'password', autocomplete: 'current-password' });
  const nw = h('input', { type: 'password', autocomplete: 'new-password' });
  const err = h('div', { class: 'err' });
  const save = btn('Сменить пароль', { kind: 'primary' });
  save.addEventListener('click', () => busy(save, async () => {
    err.textContent = '';
    try {
      await api('POST', '/settings/password', { current: cur.value, new: nw.value });
      cur.value = nw.value = '';
      toast('Пароль изменён. Другие сеансы завершены.', 'ok');
    } catch (e) { err.textContent = e.message; }
  }));

  const section = (title, sub, ...content) => h('div', { class: 'card card-pad', style: { marginBottom: '14px' } },
    h('h2', null, title), sub ? h('div', { class: 'muted small', style: { margin: '4px 0 14px' } }, sub) : null, content);

  append(box, [
    section('Пароль администратора', 'Вход в панель: ' + (S.session.user || ''),
      h('div', { class: 'row' }, field('Текущий пароль', cur), field('Новый пароль', nw, 'Не короче 8 символов')), err, save),
    section('SSH-ключ панели', 'Панель управляет серверами по этому ключу. Его можно добавить на сервер вручную вместо ввода пароля:',
      keyArea,
      h('div', { style: { display: 'flex', gap: '8px', marginTop: '10px', flexWrap: 'wrap' } },
        btn('Копировать ключ', { icon: 'copy', onclick: async () => { await navigator.clipboard.writeText(keyArea.value).catch(() => { keyArea.select(); document.execCommand('copy'); }); toast('Ключ скопирован', 'ok'); } })),
      h('div', { class: 'muted small', style: { marginTop: '10px' } }, 'На сервере: добавьте строку в файл ', h('code', null, '~/.ssh/authorized_keys'), ' пользователя root.')),
    section('Резервная копия', 'Серверы, клиенты и их пароли в одном JSON-файле. Храните его в надёжном месте.',
      h('a', { class: 'btn', href: 'api/backup', download: '' }, icon('download'), h('span', null, 'Скачать копию'))),
    section('О панели', null, h('div', { class: 'muted' }, 'TrustTunnel Panel ', ver, ' · лёгкая панель для Raspberry Pi и любых Linux-машин · ',
      h('a', { href: 'https://trusttunnel.org/', target: '_blank', rel: 'noopener' }, 'trusttunnel.org'))),
  ]);
  return box;
}

/* ---------------- запуск ---------------- */

async function boot() {
  applyTheme();
  try {
    S.session = await api('GET', '/session');
    if (S.session.user) await loadState();
  } catch (e) {
    $app.replaceChildren(h('div', { class: 'auth' }, h('div', { class: 'card' }, h('h1', null, 'Панель недоступна'), h('p', null, e.message))));
    return;
  }
  render();
  window.addEventListener('hashchange', () => { closeAllModals(); window.scrollTo(0, 0); render(); });
  // Живое обновление раз в 3 с, пока вкладка видна: обычно меняются только цифры статистики —
  // их обновляем точечно; страницу целиком перерисовываем при изменении структуры или раз в 15 с.
  let polling = false;
  setInterval(async () => {
    if (polling || document.hidden || !S.session || !S.session.user) return;
    polling = true;
    try {
      await loadState();
      const modalOpen = !!document.querySelector('.overlay');
      if (!modalOpen && (structureSig() !== S.sig || Date.now() - S.renderedAt > 15000)) render();
      else updateLive();
    } catch (_) { }
    polling = false;
  }, 3000);
  document.addEventListener('visibilitychange', async () => {
    if (!document.hidden && S.session && S.session.user && !document.querySelector('.overlay')) {
      try { await loadState(); render(); } catch (_) { }
    }
  });
}

boot();
