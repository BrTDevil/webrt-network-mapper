// Utilitare DOM: elemente, dialoguri, notificări.

export function el(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k === 'class') e.className = v;
    else if (k === 'style' && typeof v === 'object') Object.assign(e.style, v);
    else if (k.startsWith('on') && typeof v === 'function') e.addEventListener(k.slice(2), v);
    else if (k === 'html') e.innerHTML = v;
    else if (k in e && typeof v !== 'string') e[k] = v;
    else e.setAttribute(k, v === true ? '' : v);
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false) continue;
    e.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return e;
}

export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

export function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

// ---------- notificări ----------

export function toast(msg, kind = 'info', ms = 3500) {
  let box = $('#toasts');
  if (!box) {
    box = el('div', { id: 'toasts' });
    document.body.append(box);
  }
  const t = el('div', { class: `toast ${kind}` }, msg);
  box.append(t);
  setTimeout(() => t.classList.add('hide'), ms);
  setTimeout(() => t.remove(), ms + 400);
}

export function errorToast(e) {
  console.error(e);
  toast(e?.message || String(e), 'error', 6000);
}

// ---------- dialoguri ----------

// modal deschide un dialog; build(body, close) populează conținutul.
// Întoarce o promisiune rezolvată cu valoarea dată la close().
export function modal(title, build, { wide = false, actions } = {}) {
  return new Promise((resolve) => {
    const back = el('div', { class: 'modal-back' });
    const body = el('div', { class: 'modal-body' });
    const foot = el('div', { class: 'modal-foot' });
    const box = el('div', { class: `modal${wide ? ' wide' : ''}`, role: 'dialog', 'aria-modal': 'true' },
      el('div', { class: 'modal-head' }, el('h3', {}, title), el('button', { class: 'icon-btn', title: 'Închide', onclick: () => close(undefined) }, '✕')),
      body, foot);
    back.append(box);
    let done = false;
    const close = (v) => {
      if (done) return;
      done = true;
      back.remove();
      document.removeEventListener('keydown', onKey, true);
      resolve(v);
    };
    const onKey = (e) => {
      if (e.key === 'Escape') { e.stopPropagation(); close(undefined); }
    };
    document.addEventListener('keydown', onKey, true);
    back.addEventListener('mousedown', (e) => { if (e.target === back) close(undefined); });
    build(body, close, foot);
    if (actions) for (const a of actions(close)) foot.append(a);
    if (!foot.children.length) foot.remove();
    document.body.append(back);
    const f = box.querySelector('[autofocus], input, select, textarea, button.primary');
    if (f) setTimeout(() => { f.focus(); if (f.select && f.tagName === 'INPUT') f.select(); }, 20);
  });
}

export function confirmDialog(message, { title = 'Confirmare', ok = 'Confirmă', danger = false, requireText } = {}) {
  return modal(title, (body, close, foot) => {
    body.append(el('p', { class: 'pre' }, message));
    let input;
    if (requireText) {
      body.append(el('p', { class: 'muted' }, `Scrie „${requireText}” pentru a confirma:`));
      input = el('input', { type: 'text', autofocus: true });
      body.append(input);
    }
    const okBtn = el('button', { class: danger ? 'danger' : 'primary', onclick: () => {
      if (requireText && input.value.trim() !== requireText) { input.classList.add('invalid'); return; }
      close(true);
    } }, ok);
    foot.append(el('button', { onclick: () => close(false) }, 'Renunță'), okBtn);
    if (input) input.addEventListener('keydown', (e) => { if (e.key === 'Enter') okBtn.click(); });
  }).then((v) => v === true);
}

// formDialog: fields = [{name, label, type, value, options, placeholder, required, hint}]
export function formDialog(title, fields, { ok = 'Salvează', wide = false, intro } = {}) {
  return modal(title, (body, close, foot) => {
    const form = el('form', { class: 'form', onsubmit: (e) => { e.preventDefault(); submit(); } });
    if (intro) form.append(el('p', { class: 'muted pre' }, intro));
    const inputs = {};
    for (const f of fields) {
      let input;
      if (f.type === 'select') {
        input = el('select', { name: f.name }, f.options.map((o) => el('option', { value: o.value, selected: String(o.value) === String(f.value) }, o.label)));
      } else if (f.type === 'textarea') {
        input = el('textarea', { name: f.name, rows: f.rows || 3, placeholder: f.placeholder || '' }, f.value || '');
      } else if (f.type === 'checkbox') {
        input = el('input', { type: 'checkbox', name: f.name, checked: !!f.value });
      } else {
        input = el('input', { type: f.type || 'text', name: f.name, value: f.value ?? '', placeholder: f.placeholder || '', step: f.step, min: f.min, max: f.max });
      }
      if (f.required) input.required = true;
      inputs[f.name] = input;
      const label = f.type === 'checkbox'
        ? el('label', { class: 'check' }, input, ' ', f.label)
        : el('label', {}, el('span', {}, f.label), input);
      form.append(label);
      if (f.hint) form.append(el('div', { class: 'hint' }, f.hint));
    }
    body.append(form);
    const submit = () => {
      if (!form.reportValidity()) return;
      const out = {};
      for (const [k, i] of Object.entries(inputs)) out[k] = i.type === 'checkbox' ? i.checked : (i.type === 'number' ? parseFloat(i.value) : i.value);
      close(out);
    };
    foot.append(el('button', { type: 'button', onclick: () => close(undefined) }, 'Renunță'), el('button', { class: 'primary', onclick: submit }, ok));
    setTimeout(() => { const first = form.querySelector('input,select,textarea'); if (first) { first.focus(); first.select?.(); } }, 20);
  }, { wide });
}

export async function promptDialog(title, label, value = '', opts = {}) {
  const r = await formDialog(title, [{ name: 'v', label, value, required: true, placeholder: opts.placeholder }], { ok: opts.ok || 'OK', intro: opts.intro });
  return r ? r.v.trim() : null;
}

// ---------- meniuri ----------

export function dropdown(anchor, items) {
  closeMenus();
  const r = anchor.getBoundingClientRect();
  const m = el('div', { class: 'menu' });
  for (const it of items) {
    if (it === '-') { m.append(el('div', { class: 'menu-sep' })); continue; }
    m.append(el('button', { class: it.danger ? 'danger-text' : '', disabled: it.disabled, onclick: () => { closeMenus(); it.action(); } },
      it.label, it.hint ? el('span', { class: 'menu-hint' }, it.hint) : null));
  }
  document.body.append(m);
  const w = m.offsetWidth;
  m.style.left = Math.min(r.left, window.innerWidth - w - 8) + 'px';
  m.style.top = r.bottom + 4 + 'px';
  setTimeout(() => document.addEventListener('mousedown', outside, true), 0);
  function outside(e) {
    if (!m.contains(e.target)) closeMenus();
  }
  m._outside = outside;
}

export function closeMenus() {
  for (const m of $$('.menu')) {
    document.removeEventListener('mousedown', m._outside, true);
    m.remove();
  }
}

// ---------- diverse ----------

export function download(filename, blobOrText, type = 'application/octet-stream') {
  const blob = blobOrText instanceof Blob ? blobOrText : new Blob([blobOrText], { type });
  const a = el('a', { href: URL.createObjectURL(blob), download: filename });
  document.body.append(a);
  a.click();
  setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 1000);
}

export function fmtDate(iso, withTime = true) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  const p = (n) => String(n).padStart(2, '0');
  const s = `${p(d.getDate())}.${p(d.getMonth() + 1)}.${d.getFullYear()}`;
  return withTime ? `${s} ${p(d.getHours())}:${p(d.getMinutes())}` : s;
}

export function timeAgo(iso) {
  if (!iso) return '—';
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 60) return 'acum';
  if (s < 3600) return `acum ${Math.round(s / 60)} min`;
  if (s < 86400) return `acum ${Math.round(s / 3600)} h`;
  const d = Math.round(s / 86400);
  return d === 1 ? 'ieri' : `acum ${d} zile`;
}

export function debounce(fn, ms) {
  let t;
  return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); };
}

export function today() {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}
