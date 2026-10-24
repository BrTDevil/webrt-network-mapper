// Tab-ul Dispozitive: tabel sortabil/filtrabil cu editare inline.

import { api } from './api.js';
import { $, el, toast, errorToast, confirmDialog, formDialog, timeAgo, debounce } from './ui.js';
import { state, on, emit, placedDeviceIds, deviceName, deviceType, LS } from './state.js';
import { iconSVG, iconLabel, iconDefs, iconCategories, iconForType } from './editor/icons.js';
import { showNodeOfDevice } from './mapview.js';

let ctx;
let sort = LS.get('devSort', { key: 'ip', asc: true });

const COLS = [
  { key: 'status', label: '' },
  { key: 'icon', label: '' },
  { key: 'name', label: 'Nume' },
  { key: 'type', label: 'Tip' },
  { key: 'ip', label: 'IP' },
  { key: 'mac', label: 'MAC' },
  { key: 'vendor', label: 'Vendor' },
  { key: 'hostname', label: 'Hostname' },
  { key: 'ports', label: 'Porturi' },
  { key: 'location', label: 'Locație' },
  { key: 'notes', label: 'Observații' },
  { key: 'placed', label: 'Pe hartă' },
  { key: 'seen', label: 'Ultima vedere' },
  { key: 'actions', label: '' },
];

export function initDevices(c) {
  ctx = c;
  on('devices', render);
  on('devices-edited', render);
  on('map', render);
  on('tab', (t) => { if (t === 'devices') render(); });
  $('#devSearch').addEventListener('input', debounce(render, 150));
  $('#devStatus').addEventListener('change', render);
  $('#addDeviceBtn').addEventListener('click', addManual);
}

function ipNum(ip) {
  const p = (ip || '').split('.').map(Number);
  return p.length === 4 ? ((p[0] * 256 + p[1]) * 256 + p[2]) * 256 + p[3] : Infinity;
}

function ports(d) {
  try { return JSON.parse(d.open_ports || '[]') || []; } catch { return []; }
}

function typeOptions(current, detected) {
  const defs = iconDefs();
  return [
    el('option', { value: '', selected: !current }, `automat: ${iconLabel(detected || 'necunoscut')}`),
    ...iconCategories().map((cat) => el('optgroup', { label: cat },
      Object.entries(defs).filter(([, d]) => d.cat === cat).map(([k, d]) => el('option', { value: k, selected: current === k }, d.label)))),
  ];
}

function render() {
  if (state.tab !== 'devices') return;
  const table = $('#devTable');
  const q = $('#devSearch').value.trim().toLowerCase();
  const st = $('#devStatus').value;
  const placed = placedDeviceIds();
  let list = state.devices.filter((d) => {
    if (st === 'online' && !d.online) return false;
    if (st === 'offline' && d.online) return false;
    if (st === 'new' && !d.new) return false;
    if (st === 'unplaced' && placed.has(d.id)) return false;
    if (st === 'unknown' && !(deviceType(d) === 'necunoscut' && !d.custom_name)) return false;
    if (!q) return true;
    return [d.custom_name, d.hostname, d.last_ip, d.mac, d.vendor, d.location, d.notes, iconLabel(deviceType(d))].some((v) => (v || '').toLowerCase().includes(q));
  });
  const key = {
    status: (d) => (d.online ? 0 : 1), name: (d) => deviceName(d).toLowerCase(), type: (d) => iconLabel(deviceType(d)),
    ip: (d) => ipNum(d.last_ip), mac: (d) => d.mac, vendor: (d) => (d.vendor || '').toLowerCase(), hostname: (d) => d.hostname || '',
    ports: (d) => ports(d).length, location: (d) => (d.location || '').toLowerCase(), notes: (d) => d.notes || '',
    placed: (d) => (placed.has(d.id) ? 0 : 1), seen: (d) => d.last_seen || '',
  }[sort.key] || ((d) => ipNum(d.last_ip));
  list.sort((a, b) => {
    const x = key(a), y = key(b);
    return (x < y ? -1 : x > y ? 1 : 0) * (sort.asc ? 1 : -1);
  });

  const on = state.devices.filter((d) => d.online).length;
  const nw = state.devices.filter((d) => d.new).length;
  const ls = state.lastScan;
  $('#devInfo').textContent = state.devices.length
    ? `${state.devices.length} în inventar · ${on} online · ${state.devices.length - on} offline${nw ? ` · ${nw} noi` : ''}` +
      (ls ? ` · ultimul scan: ${timeAgo(ls.finished_at)} (${ls.subnet}, ${ls.method})` : '')
    : '';

  const head = el('thead', {}, el('tr', {}, COLS.map((c) => el('th', {
    class: sort.key === c.key ? `sorted ${sort.asc ? 'asc' : ''}` : '',
    onclick: () => {
      if (c.key === 'actions' || c.key === 'icon') return;
      sort = { key: c.key, asc: sort.key === c.key ? !sort.asc : true };
      LS.set('devSort', sort);
      render();
    },
  }, c.label))));

  const body = el('tbody');
  for (const d of list) {
    const patch = async (fields) => {
      try {
        const nd = await api.patchDevice(d.id, fields);
        Object.assign(d, { custom_name: nd.custom_name, device_type: nd.device_type, location: nd.location, notes: nd.notes, color: nd.color });
        state.editor?.requestRender();
        emit('devices-edited');
      } catch (e) { errorToast(e); }
    };
    const t = iconForType(deviceType(d));
    const isPlaced = placed.has(d.id);
    body.append(el('tr', { class: `${d.online ? '' : 'offline'} ${d.new ? 'new' : ''}` },
      el('td', { title: d.online ? 'Online la ultimul scan' : 'Offline / nu a răspuns la ultimul scan' }, el('span', { class: `dot ${d.online ? 'on' : ''}` })),
      el('td', {}, el('span', { html: iconSVG(t, d.color || 'var(--accent)', 26) })),
      el('td', {}, el('input', { value: d.custom_name || '', placeholder: d.hostname || d.last_ip || '', onchange: (e) => patch({ custom_name: e.target.value }) }),
        d.new ? el('span', { class: 'tag new' }, 'NOU') : null),
      el('td', {}, el('select', { onchange: (e) => patch({ device_type: e.target.value }).then(render) }, typeOptions(d.device_type, d.detected_type))),
      el('td', { class: 'mono' }, d.last_ip || '—'),
      el('td', { class: 'mono' }, d.mac.startsWith('MANUAL-') ? '—' : d.mac),
      el('td', {}, d.vendor || ''),
      el('td', {}, d.hostname || ''),
      el('td', { class: 'mono', title: ports(d).map((p) => p).join(', ') }, ports(d).join(', ')),
      el('td', {}, el('input', { value: d.location || '', placeholder: '—', onchange: (e) => patch({ location: e.target.value }) })),
      el('td', {}, el('input', { value: d.notes || '', placeholder: '—', onchange: (e) => patch({ notes: e.target.value }) })),
      el('td', {}, isPlaced ? 'da' : el('span', { class: 'muted' }, 'nu')),
      el('td', { title: d.last_seen }, timeAgo(d.last_seen)),
      el('td', {}, el('div', { class: 'actions' },
        el('input', { type: 'color', value: d.color || '#1a73e8', title: 'Culoare pe hartă', onchange: (e) => patch({ color: e.target.value }) }),
        el('button', { title: isPlaced ? 'Arată pe hartă' : 'Plasează pe hartă (în centru)', onclick: () => showOnMap(d, isPlaced) }, isPlaced ? 'Arată pe hartă' : 'Pune pe hartă'),
        el('button', { class: 'danger-text', title: 'Șterge din inventar', onclick: () => remove(d) }, '✕'))),
    ));
  }
  table.replaceChildren(head, body);
  if (!state.devices.length) {
    table.append(el('tbody', {}, el('tr', {}, el('td', { colspan: COLS.length }, el('div', { class: 'empty' }, el('b', {}, 'Inventarul e gol'), 'Apasă „Scan” (sus, dreapta) ca să descoperi dispozitivele din rețea, sau adaugă unul manual.')))));
  }
}

function showOnMap(d, isPlaced) {
  import('./app.js').then(({ switchTab }) => {
    switchTab('map');
    setTimeout(() => {
      if (isPlaced) showNodeOfDevice(d.id);
      else {
        const ed = state.editor;
        ed.hooks.onDropDevice(d.id, ed.snapPos(ed.toWorld(ed.cssW / 2, ed.cssH / 2)));
      }
    }, 30);
  });
}

async function remove(d) {
  const ok = await confirmDialog(`Ștergi „${deviceName(d)}” (${d.last_ip || d.mac}) din inventar?\n\nIstoricul observațiilor se pierde. Dacă dispozitivul e încă în rețea, va reapărea la următorul scan (fără numele și observațiile tale).`,
    { title: 'Șterge dispozitivul', ok: 'Șterge', danger: true });
  if (!ok) return;
  try {
    await api.deleteDevice(d.id);
    // Nodurile de pe hartă rămân, dar nu mai sunt legate de inventar.
    const ed = state.editor;
    if (ed.scene.nodes.some((n) => n.device_id === d.id)) {
      ed.change('Dezleagă dispozitiv', (s) => { for (const n of s.nodes) if (n.device_id === d.id) { n.label = n.label || deviceName(d); delete n.device_id; } });
    }
    await ctx.reloadInventory();
    toast('Dispozitiv șters din inventar.');
  } catch (e) { errorToast(e); }
}

async function addManual() {
  const defs = iconDefs();
  const r = await formDialog('Dispozitiv manual', [
    { name: 'name', label: 'Nume', required: true, placeholder: 'ex. Switch bucătărie' },
    { name: 'type', label: 'Tip', type: 'select', value: 'switch', options: Object.entries(defs).map(([k, d]) => ({ value: k, label: `${d.cat} — ${d.label}` })) },
    { name: 'ip', label: 'IP (opțional)', placeholder: '192.168.1.50' },
    { name: 'mac', label: 'MAC (opțional)', placeholder: 'AA:BB:CC:DD:EE:FF' },
  ], { ok: 'Adaugă', intro: 'Pentru echipamente care nu apar la scan (switch-uri nemanaged, prize de rețea, dispozitive oprite). Dacă dai MAC-ul, scanările viitoare îl vor recunoaște.' });
  if (!r) return;
  try {
    await api.createDevice(state.project.id, { mac: r.mac.trim(), ip: r.ip.trim(), name: r.name.trim(), type: r.type });
    await ctx.reloadInventory();
    toast('Dispozitiv adăugat. Îl găsești în lista „Neplasate” din tab-ul Hartă.', 'ok');
  } catch (e) { errorToast(e); }
}
