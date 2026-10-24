// Tab-ul Bluetooth: dispozitivele din apropiere descoperite pasiv.

import { api } from './api.js';
import { $, el, toast, errorToast, confirmDialog, timeAgo, debounce } from './ui.js';
import { state, on, emit, btName, btType, placedBtIds, deviceName, LS } from './state.js';
import { iconSVG, iconLabel, iconDefs, iconCategories, iconForType } from './editor/icons.js';
import { signalColor } from './wifi.js';

let ctx;

export function initBluetooth(c) {
  ctx = c;
  on('bluetooth', render);
  on('devices', render);
  on('map', render);
  on('tab', (t) => { if (t === 'bluetooth') render(); });
  $('#btSearch').addEventListener('input', debounce(render, 150));
  $('#btNamedOnly').checked = LS.get('btNamedOnly', true);
  $('#btNamedOnly').addEventListener('change', (e) => { LS.set('btNamedOnly', e.target.checked); render(); });
  $('#btVisibleOnly').addEventListener('change', render);
  $('#btScanBtn').addEventListener('click', () => ctx.startScan({ lan: false, wifi: false, bluetooth: true }));
}

function typeOptions(current, detected) {
  const defs = iconDefs();
  return [
    el('option', { value: '', selected: !current }, `automat: ${iconLabel(detected || 'bluetooth')}`),
    ...iconCategories().map((cat) => el('optgroup', { label: cat },
      Object.entries(defs).filter(([, d]) => d.cat === cat).map(([k, d]) => el('option', { value: k, selected: current === k }, d.label)))),
  ];
}

function render() {
  if (state.tab !== 'bluetooth') return;
  const q = $('#btSearch').value.trim().toLowerCase();
  const namedOnly = $('#btNamedOnly').checked;
  const visibleOnly = $('#btVisibleOnly').checked;
  const placed = placedBtIds();
  const all = state.bluetooth;
  const anon = all.filter((b) => !b.name && !b.custom_name).length;
  const list = all.filter((b) => {
    if (namedOnly && !b.name && !b.custom_name && !placed.has(b.id)) return false;
    if (visibleOnly && !b.visible) return false;
    if (!q) return true;
    return [b.name, b.custom_name, b.mac, b.manufacturer, b.location, b.notes, iconLabel(btType(b))].some((v) => (v || '').toLowerCase().includes(q));
  }).sort((a, b) => (b.rssi ?? -200) - (a.rssi ?? -200));

  $('#btInfo').textContent = all.length
    ? `${all.length} dispozitive văzute · ${all.length - anon} cu nume · ${anon} anonime (adrese aleatoare, de obicei telefoane/ceasuri din jur)${namedOnly && anon ? ' — ascunse' : ''}`
    : '';

  const table = $('#btTable');
  const head = el('thead', {}, el('tr', {}, ['', '', 'Nume', 'Tip', 'MAC', 'Producător', 'Semnal', 'Același cu (LAN)', 'Locație', 'Observații', 'Văzut', ''].map((h) => el('th', {}, h))));
  const body = el('tbody');
  const devOpts = () => [el('option', { value: '' }, '— niciunul —'), ...state.devices.map((d) => el('option', { value: d.id }, `${deviceName(d)}${d.last_ip && d.last_ip !== deviceName(d) ? ' · ' + d.last_ip : ''}`))];
  for (const b of list) {
    const patch = async (fields) => {
      try {
        const nb = await api.patchBluetooth(b.id, fields);
        Object.assign(b, nb, { rssi: b.rssi, visible: b.visible });
        state.editor?.requestRender();
        render();
      } catch (e) { errorToast(e); }
    };
    const t = iconForType(btType(b));
    const sel = el('select', { onchange: (e) => patch(e.target.value ? { device_id: +e.target.value, link_status: 'confirmat' } : { link_status: 'none' }) }, devOpts());
    sel.value = b.device_id && b.link_status !== 'respins' ? String(b.device_id) : '';
    const link = el('div', { class: 'bssid-assoc' }, sel);
    if (b.link_status === 'inferat' && b.device_id) {
      link.append(el('span', { class: 'tag inferat', title: 'Același nume ca un dispozitiv din LAN' }, 'inferat'),
        el('button', { class: 'mini', onclick: () => patch({ link_status: 'confirmat', device_id: b.device_id }) }, 'Confirmă'),
        el('button', { class: 'mini', onclick: () => patch({ link_status: 'respins' }) }, 'Respinge'));
    }
    const isPlaced = placed.has(b.id);
    body.append(el('tr', { class: b.visible ? '' : 'offline' },
      el('td', { title: b.visible ? 'Văzut la ultimul scan Bluetooth' : 'Nu a fost văzut la ultimul scan' }, el('span', { class: 'dot', style: { background: b.visible ? '#7b1fa2' : '' } })),
      el('td', {}, el('span', { html: iconSVG(t, '#7b1fa2', 26) })),
      el('td', {}, el('input', { value: b.custom_name || '', placeholder: b.name || '(fără nume)', onchange: (e) => patch({ custom_name: e.target.value }) })),
      el('td', {}, el('select', { onchange: (e) => patch({ device_type: e.target.value }) }, typeOptions(b.device_type, b.detected_type))),
      el('td', { class: 'mono' }, b.mac, b.address_type === 'random' ? el('div', { class: 'muted small', title: 'Adresă aleatoare: se poate schimba periodic (confidențialitate)' }, 'aleatoare') : null),
      el('td', {}, b.manufacturer || ''),
      el('td', { style: { color: signalColor(b.rssi), fontWeight: 600 } }, b.rssi != null ? `${b.rssi} dBm` : '—'),
      el('td', {}, link),
      el('td', {}, el('input', { value: b.location || '', placeholder: '—', onchange: (e) => patch({ location: e.target.value }) })),
      el('td', {}, el('input', { value: b.notes || '', placeholder: '—', onchange: (e) => patch({ notes: e.target.value }) })),
      el('td', { title: b.last_seen }, timeAgo(b.last_seen)),
      el('td', {}, el('div', { class: 'actions' },
        el('button', { onclick: () => placeOnMap(b, isPlaced) }, isPlaced ? 'Arată pe hartă' : 'Pune pe hartă'),
        el('button', { class: 'danger-text', title: 'Șterge din listă', onclick: () => remove(b) }, '✕'))),
    ));
  }
  table.replaceChildren(head, body);
  if (!list.length) {
    table.append(el('tbody', {}, el('tr', {}, el('td', { colspan: 12 }, el('div', { class: 'empty' },
      el('b', {}, all.length ? 'Niciun dispozitiv pentru filtrul curent' : 'Niciun dispozitiv Bluetooth'),
      all.length ? 'Debifează „Doar cu nume” ca să vezi și dispozitivele anonime.' : 'Apasă „Scan Bluetooth”. Aplicația doar ascultă ~12 secunde anunțurile din jur — nu se conectează și nu împerechează nimic.')))));
  }
}

function placeOnMap(b, isPlaced) {
  import('./app.js').then(({ switchTab }) => {
    switchTab('map');
    const ed = state.editor;
    setTimeout(() => {
      // Același echipament ca unul din LAN, deja pe hartă → îl arătăm pe acela.
      const sameNode = b.device_id && b.link_status !== 'respins' ? ed.scene.nodes.find((n) => n.device_id === b.device_id) : null;
      const node = ed.scene.nodes.find((n) => n.bt_id === b.id) || sameNode;
      if (node) {
        ed.select([{ type: 'node', id: node.id }]);
        ed.centerOn([node.x, node.y], Math.max(ed.view.zoom, 1));
        if (!isPlaced && sameNode) toast(`„${btName(b)}” e același echipament cu „${deviceName(state.devMap.get(b.device_id))}”, deja pe hartă.`);
        return;
      }
      const p = ed.snapPos(ed.toWorld(ed.cssW / 2, ed.cssH / 2));
      ed.addNode({ icon: iconForType(btType(b)), bt_id: b.id, label: '', color: '#7b1fa2' }, p);
      emit('devices-edited');
      toast('Pus pe hartă. Pentru legătură folosește unealta Cablu, tipul „Bluetooth”.');
    }, 30);
  });
}

async function remove(b) {
  const ok = await confirmDialog(`Ștergi „${btName(b)}” din lista Bluetooth? Dacă e încă în apropiere, reapare la următorul scan.`, { title: 'Șterge dispozitivul Bluetooth', ok: 'Șterge', danger: true });
  if (!ok) return;
  try {
    await api.deleteBluetooth(b.id);
    const ed = state.editor;
    if (ed.scene.nodes.some((n) => n.bt_id === b.id)) {
      ed.change('Dezleagă Bluetooth', (s) => { for (const n of s.nodes) if (n.bt_id === b.id) { n.label = n.label || btName(b); delete n.bt_id; } });
    }
    await ctx.reloadInventory();
  } catch (e) { errorToast(e); }
}
