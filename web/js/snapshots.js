// Tab-ul Istoric: instantanee salvate ca fișiere într-un folder, redenumire, comparare.

import { api } from './api.js';
import { $, el, toast, errorToast, confirmDialog, formDialog, promptDialog, fmtDate, timeAgo } from './ui.js';
import { state, on, LS } from './state.js';
import { iconLabel } from './editor/icons.js';

let ctx;
let snaps = [];

export function initSnapshots(c) {
  ctx = c;
  on('tab', (t) => { if (t === 'history') refresh(); });
  on('project', () => { if (state.tab === 'history') refresh(); });
  $('#snapSaveBtn').addEventListener('click', () => quickSnapshot());
  $('#snapDirBtn').addEventListener('click', changeDir);
  $('#snapOpenBtn').addEventListener('click', () => api.openSnapshotFolder().catch(errorToast));
  $('#snapRefreshBtn').addEventListener('click', refresh);
  $('#cmpBtn').addEventListener('click', compare);
}

async function refresh() {
  try {
    const r = await api.snapshots();
    snaps = r.snapshots;
    $('#snapDir').textContent = r.dir;
    renderTable();
    fillCompare();
  } catch (e) { errorToast(e); }
}

// quickSnapshot: salvează harta, apoi un instantaneu cu nume sugerat (redenumibil).
export async function quickSnapshot() {
  if (!state.project) return;
  const d = new Date();
  const p2 = (n) => String(n).padStart(2, '0');
  const suggested = `${d.getFullYear()}-${p2(d.getMonth() + 1)}-${p2(d.getDate())} ${state.project.name}`;
  const r = await formDialog('Salvează instantaneu', [
    { name: 'name', label: 'Nume', value: suggested, required: true, hint: 'Devine numele fișierului. Îl poți redenumi oricând.' },
    { name: 'folder', label: 'Subfolder (opțional)', value: LS.get('snapFolder.' + state.project.id, state.project.name), placeholder: 'ex. Restaurant RP' },
    { name: 'note', label: 'Observații', type: 'textarea', placeholder: 'ex. Înainte de schimbarea routerului din bar' },
  ], { ok: 'Salvează', intro: 'Instantaneul conține toată locația: dispozitive, Wi-Fi, hărți, survey. Peste câteva luni îl poți compara cu starea de atunci.' });
  if (!r) return;
  try {
    if (state.editor.dirty) await ctx.saveMap({ auto: true });
    LS.set('snapFolder.' + state.project.id, r.folder);
    const info = await api.saveSnapshot(state.project.id, r);
    toast(`Instantaneu salvat: ${info.path}`, 'ok', 5000);
    if (state.tab === 'history') refresh();
  } catch (e) { errorToast(e); }
}

async function changeDir() {
  const cur = await api.snapshotDir();
  const dir = await promptDialog('Folderul instantaneelor', 'Cale completă', cur.dir, { ok: 'Folosește', placeholder: cur.default });
  if (!dir) return;
  try {
    const r = await api.setSnapshotDir(dir);
    toast(`Folder: ${r.dir}`, 'ok');
    refresh();
  } catch (e) { errorToast(e); }
}

function renderTable() {
  const t = $('#snapTable');
  const head = el('thead', {}, el('tr', {}, ['Nume', 'Folder', 'Locație', 'Data', 'Dispozitive', 'Wi-Fi', 'Hărți', 'Observații', ''].map((h) => el('th', {}, h))));
  const body = el('tbody');
  for (const s of snaps) {
    body.append(el('tr', {},
      el('td', {}, el('b', {}, s.name)),
      el('td', { class: 'muted' }, s.folder || '—'),
      el('td', {}, s.project_name),
      el('td', { title: timeAgo(s.taken_at) }, fmtDate(s.taken_at)),
      el('td', {}, s.devices),
      el('td', {}, s.wifi),
      el('td', {}, s.maps),
      el('td', { class: 'muted small' }, s.note || ''),
      el('td', {}, el('div', { class: 'actions' },
        el('button', { title: 'Compară cu starea curentă', onclick: () => { setCompare('snap:' + s.path, 'current:' + state.project.id); compare(); } }, 'Compară cu acum'),
        el('button', { onclick: () => window.open(api.snapshotReportURL(s.path), '_blank') }, 'Raport'),
        el('button', { onclick: () => rename(s) }, 'Redenumește'),
        el('button', { title: 'Descarcă fișierul .netmap.json', onclick: () => { location.href = api.snapshotFileURL(s.path, true); } }, '⤓'),
        el('button', { title: 'Creează un proiect nou din acest instantaneu', onclick: () => restore(s) }, 'Restaurează'),
        el('button', { class: 'danger-text', title: 'Șterge fișierul', onclick: () => remove(s) }, '✕'))),
    ));
  }
  t.replaceChildren(head, body);
  if (!snaps.length) {
    t.append(el('tbody', {}, el('tr', {}, el('td', { colspan: 9 }, el('div', { class: 'empty' },
      el('b', {}, 'Niciun instantaneu salvat încă'),
      'Apasă „Salvează instantaneu acum” (sau butonul „Instantaneu” de sus) ca să păstrezi starea de azi a rețelei. Peste 3 luni o compari cu ce găsești atunci.')))));
  }
}

async function rename(s) {
  const name = await promptDialog('Redenumește instantaneul', 'Nume nou', s.name, { ok: 'Redenumește' });
  if (!name || name === s.name) return;
  try {
    await api.renameSnapshot(s.path, name);
    toast('Redenumit.', 'ok');
    refresh();
  } catch (e) { errorToast(e); }
}

async function remove(s) {
  const ok = await confirmDialog(`Ștergi definitiv fișierul „${s.path}”?`, { title: 'Șterge instantaneul', ok: 'Șterge', danger: true });
  if (!ok) return;
  try {
    await api.deleteSnapshot(s.path);
    refresh();
  } catch (e) { errorToast(e); }
}

async function restore(s) {
  const name = await promptDialog('Restaurează ca proiect nou', 'Numele noului proiect', `${s.project_name} (${s.name})`, { ok: 'Restaurează' });
  if (!name) return;
  try {
    const p = await api.restoreSnapshot(s.path, name);
    await ctx.reloadProjects();
    await ctx.loadProject(p.id);
    toast(`Locația „${p.name}” a fost creată din instantaneu. Locația originală nu a fost modificată.`, 'ok', 6000);
  } catch (e) { errorToast(e); }
}

// ---------- comparare ----------

function sourceOptions() {
  const opts = [];
  for (const p of state.projects) opts.push({ value: 'current:' + p.id, label: `● Starea curentă — ${p.name}` });
  for (const s of snaps) opts.push({ value: 'snap:' + s.path, label: `${fmtDate(s.taken_at, false)} — ${s.name}` });
  return opts;
}

function fillCompare() {
  const a = $('#cmpA'), b = $('#cmpB');
  const va = a.value, vb = b.value;
  const opts = sourceOptions();
  a.replaceChildren(...opts.map((o) => el('option', { value: o.value }, o.label)));
  b.replaceChildren(...opts.map((o) => el('option', { value: o.value }, o.label)));
  // implicit: cel mai vechi instantaneu al proiectului curent → starea curentă
  const mine = snaps.filter((s) => s.project_name === state.project?.name);
  a.value = opts.some((o) => o.value === va) ? va : (mine.length ? 'snap:' + mine[mine.length - 1].path : opts[0]?.value);
  b.value = opts.some((o) => o.value === vb) ? vb : 'current:' + state.project?.id;
}

function setCompare(a, b) {
  $('#cmpA').value = a;
  $('#cmpB').value = b;
}

async function compare() {
  const a = $('#cmpA').value, b = $('#cmpB').value;
  const box = $('#cmpResult');
  if (!a || !b) return;
  if (a === b) { toast('Alege două surse diferite.'); return; }
  box.replaceChildren(el('p', { class: 'muted' }, 'Se compară…'));
  try {
    const d = await api.compare(a, b);
    renderDiff(box, d);
    box.scrollIntoView({ behavior: 'smooth', block: 'start' });
  } catch (e) {
    box.replaceChildren();
    errorToast(e);
  }
}

function renderDiff(box, d) {
  const card = (cls, n, label) => el('div', { class: `cmp-card ${cls}` }, el('b', {}, n), el('span', { class: 'muted small' }, label));
  const chgCount = d.devices_changed.length;
  box.replaceChildren(
    el('p', {}, el('b', {}, d.a.label), el('span', { class: 'muted' }, ` (${fmtDate(d.a.taken_at)})`), '  →  ', el('b', {}, d.b.label), el('span', { class: 'muted' }, ` (${fmtDate(d.b.taken_at)})`)),
    el('div', { class: 'cmp-summary' },
      card('', `${d.a.devices} → ${d.b.devices}`, 'dispozitive în inventar'),
      card('add', '+' + d.devices_added.length, 'dispozitive noi'),
      card('rem', '−' + d.devices_removed.length, 'dispozitive dispărute'),
      card('chg', chgCount, 'dispozitive modificate'),
      card('', `${d.a.wifi} → ${d.b.wifi}`, 'BSSID-uri Wi-Fi'),
      card('add', '+' + d.wifi_added.length, 'rețele Wi-Fi noi'),
      card('rem', '−' + d.wifi_removed.length, 'rețele Wi-Fi dispărute'),
      card('', `${d.a.bt || 0} → ${d.b.bt || 0}`, 'Bluetooth (cu nume)')),
  );

  const devTable = (list, withChanges) => el('table', { class: 'grid' },
    el('thead', {}, el('tr', {}, ['Nume', 'Tip', 'IP', 'MAC', 'Vendor', withChanges ? 'Modificări' : ''].map((h) => el('th', {}, h)))),
    el('tbody', {}, list.map((x) => el('tr', {},
      el('td', {}, el('b', {}, x.name)), el('td', {}, iconLabel(x.type)), el('td', { class: 'mono' }, x.ip), el('td', { class: 'mono' }, x.mac), el('td', {}, x.vendor),
      el('td', {}, withChanges ? changes(x.changes) : '')))));

  const sec = (title, n, content) => (n ? el('div', { class: 'cmp-sec' }, el('h4', {}, `${title} (${n})`), el('div', { class: 'table-wrap' }, content)) : null);
  box.append(...[
    sec('Dispozitive noi', d.devices_added.length, devTable(d.devices_added)),
    sec('Dispozitive dispărute', d.devices_removed.length, devTable(d.devices_removed)),
    sec('Dispozitive modificate', d.devices_changed.length, devTable(d.devices_changed, true)),
  ].filter(Boolean));
  const wifiTable = (list, withChanges) => el('table', { class: 'grid' },
    el('thead', {}, el('tr', {}, ['SSID', 'BSSID', 'Bandă', 'Canal', withChanges ? 'Modificări' : ''].map((h) => el('th', {}, h)))),
    el('tbody', {}, list.map((x) => el('tr', {}, el('td', {}, x.ssid || '<ascuns>'), el('td', { class: 'mono' }, x.bssid), el('td', {}, x.band + ' GHz'), el('td', {}, x.channel),
      el('td', {}, withChanges ? changes(x.changes) : '')))));
  box.append(...[
    sec('Rețele Wi-Fi noi', d.wifi_added.length, wifiTable(d.wifi_added)),
    sec('Rețele Wi-Fi dispărute', d.wifi_removed.length, wifiTable(d.wifi_removed)),
    sec('Rețele Wi-Fi modificate', d.wifi_changed.length, wifiTable(d.wifi_changed, true)),
  ].filter(Boolean));
  const btTable = (list, withChanges) => el('table', { class: 'grid' },
    el('thead', {}, el('tr', {}, ['Nume', 'Tip', 'MAC', withChanges ? 'Modificări' : ''].map((h) => el('th', {}, h)))),
    el('tbody', {}, list.map((x) => el('tr', {}, el('td', {}, el('b', {}, x.name)), el('td', {}, iconLabel(x.type)), el('td', { class: 'mono' }, x.mac),
      el('td', {}, withChanges ? changes(x.changes) : '')))));
  box.append(...[
    sec('Bluetooth nou', (d.bt_added || []).length, btTable(d.bt_added || [])),
    sec('Bluetooth dispărut', (d.bt_removed || []).length, btTable(d.bt_removed || [])),
    sec('Bluetooth modificat', (d.bt_changed || []).length, btTable(d.bt_changed || [], true)),
  ].filter(Boolean));
  if (d.maps.length) {
    const st = { added: 'hartă nouă', removed: 'hartă ștearsă', changed: 'modificată', same: 'neschimbată' };
    box.append(el('div', { class: 'cmp-sec' }, el('h4', {}, 'Hărți'),
      ...d.maps.map((m) => el('div', { class: 'small', style: { margin: '6px 0' } },
        el('b', {}, m.name), ` — ${st[m.status]}`,
        m.status !== 'added' && m.status !== 'removed' ? ` · echipamente ${m.nodes_before} → ${m.nodes_after} · cabluri ${m.links_before} → ${m.links_after}` : '',
        m.added_nodes?.length ? el('div', {}, el('span', { class: 'tag new' }, 'adăugate'), ' ', m.added_nodes.join(', ')) : null,
        m.removed_nodes?.length ? el('div', {}, el('span', { class: 'tag removed' }, 'scoase'), ' ', m.removed_nodes.join(', ')) : null,
        m.moved_nodes?.length ? el('div', {}, el('span', { class: 'tag' }, 'mutate'), ' ', m.moved_nodes.join(', ')) : null))));
    // hărțile una lângă alta
    const names = [...new Set(d.maps.map((m) => m.name))];
    for (const name of names) {
      const sa = d.svg_a?.[name], sb = d.svg_b?.[name];
      if (!sa && !sb) continue;
      box.append(el('div', { class: 'cmp-maps' },
        el('div', {}, el('div', { class: 'cap' }, `${name} — ${d.a.label}`), el('div', { html: sa || '<p class="empty">—</p>' })),
        el('div', {}, el('div', { class: 'cap' }, `${name} — ${d.b.label}`), el('div', { html: sb || '<p class="empty">—</p>' }))));
    }
  }
  if (!d.devices_added.length && !d.devices_removed.length && !d.devices_changed.length && !d.wifi_added.length && !d.wifi_removed.length && !d.wifi_changed.length && !(d.bt_added || []).length && !(d.bt_removed || []).length && d.maps.every((m) => m.status === 'same')) {
    box.append(el('p', { class: 'muted' }, 'Nicio diferență.'));
  }
}

function changes(list) {
  return el('div', {}, (list || []).map((c) => el('div', { class: 'chg' }, el('b', {}, c.label + ': '),
    el('span', { class: 'before' }, c.before || '∅'), ' → ', el('span', { class: 'after' }, c.after || '∅'))));
}
