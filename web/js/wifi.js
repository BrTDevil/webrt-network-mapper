// Tab-ul Wi-Fi: grupare pe SSID, asociere BSSID ↔ AP (inferat / confirmat), grafic de canale.

import { api } from './api.js';
import { $, el, errorToast } from './ui.js';
import { state, on, deviceName } from './state.js';

let ctx;

export function initWifi(c) {
  ctx = c;
  on('wifi', render);
  on('devices', render);
  on('tab', (t) => { if (t === 'wifi') render(); });
  $('#wifiVisibleOnly').addEventListener('change', render);
  $('#wifiScanBtn').addEventListener('click', () => ctx.startScan({ lan: false, wifi: true }));
}

function macNum(m) {
  return parseInt((m || '').replace(/[:-]/g, ''), 16);
}

// Același echipament, radio diferit (2.4/5 GHz): BSSID-uri care diferă doar la final.
export function sameRadio(a, b) {
  if (a === b) return false;
  const LA = 0x020000000000;
  const x = macNum(a) % 2 ** 48, y = macNum(b) % 2 ** 48;
  const cx = x - (Math.floor(x / LA) % 2 ? LA : 0), cy = y - (Math.floor(y / LA) % 2 ? LA : 0);
  return Math.floor(cx / 256) === Math.floor(cy / 256) || Math.floor(cx / 16) === Math.floor(cy / 16);
}

export function signalColor(dbm) {
  if (dbm == null) return '#9aa0a6';
  if (dbm >= -55) return '#1e8e3e';
  if (dbm >= -67) return '#7cb342';
  if (dbm >= -75) return '#f9ab00';
  if (dbm >= -82) return '#f57c00';
  return '#d93025';
}

function render() {
  if (state.tab !== 'wifi') return;
  const visibleOnly = $('#wifiVisibleOnly').checked;
  const nets = state.wifi.filter((w) => !visibleOnly || w.visible);
  const groups = new Map();
  for (const w of nets) {
    const k = w.ssid || '';
    if (!groups.has(k)) groups.set(k, []);
    groups.get(k).push(w);
  }
  const box = $('#wifiGroups');
  box.replaceChildren();
  if (!nets.length) {
    box.append(el('div', { class: 'empty' }, el('b', {}, 'Nicio rețea Wi-Fi'), 'Apasă „Scan Wi-Fi”. Pe Linux e nevoie de NetworkManager (nmcli).'));
  }
  const keys = [...groups.keys()].sort((a, b) => (a === '') - (b === '') || a.localeCompare(b));
  const devOpts = () => [el('option', { value: '' }, '— neasociat —'), ...state.devices.map((d) => el('option', { value: d.id }, `${deviceName(d)}${d.last_ip && d.last_ip !== deviceName(d) ? ' · ' + d.last_ip : ''}`))];

  for (const ssid of keys) {
    const list = groups.get(ssid).sort((a, b) => (b.signal_dbm ?? -200) - (a.signal_dbm ?? -200));
    // număr estimat de echipamente = grupuri de BSSID-uri „pereche”
    const clusters = [];
    for (const w of list) {
      const c = clusters.find((cl) => cl.some((x) => sameRadio(x.bssid, w.bssid)));
      if (c) c.push(w); else clusters.push([w]);
    }
    const g = el('div', { class: 'ssid-group' },
      el('div', { class: 'ssid-head' },
        el('b', {}, ssid || '<SSID ascuns>'),
        el('span', { class: 'muted' }, `${list.length} BSSID-uri → probabil ${clusters.length} ${clusters.length === 1 ? 'echipament' : 'echipamente'}`),
        el('span', { class: 'muted small', style: { marginLeft: 'auto' } }, [...new Set(list.map((w) => w.security))].join(', '))));
    for (const w of list) {
      const pair = state.wifi.find((x) => x.id !== w.id && sameRadio(x.bssid, w.bssid));
      const dev = w.ap_device_id ? state.devMap.get(w.ap_device_id) : null;
      const patch = async (fields) => {
        try {
          const nw = await api.patchWifi(w.id, fields);
          Object.assign(w, nw, { signal_dbm: w.signal_dbm, visible: w.visible });
          render();
        } catch (e) { errorToast(e); }
      };
      const sel = el('select', { onchange: (e) => patch(e.target.value ? { ap_device_id: +e.target.value, ap_link_status: 'confirmat' } : { ap_link_status: 'none' }) }, devOpts());
      sel.value = w.ap_device_id && w.ap_link_status !== 'respins' ? String(w.ap_device_id) : '';
      const assoc = el('div', { class: 'bssid-assoc' }, '→ ', sel);
      if (w.ap_link_status === 'inferat' && dev) {
        assoc.append(el('span', { class: 'tag inferat', title: 'Dedus din MAC-uri apropiate numeric — confirmă sau respinge' }, 'inferat'),
          el('button', { class: 'mini', onclick: () => patch({ ap_link_status: 'confirmat', ap_device_id: w.ap_device_id }) }, 'Confirmă'),
          el('button', { class: 'mini', onclick: () => patch({ ap_link_status: 'respins' }) }, 'Respinge'));
      } else if (w.ap_link_status === 'confirmat' && dev) {
        assoc.append(el('span', { class: 'tag confirmat' }, 'confirmat'));
      } else if (w.ap_link_status === 'respins') {
        assoc.append(el('span', { class: 'tag' }, 'respins'));
      }
      g.append(el('div', { class: `bssid-row ${w.visible ? '' : 'gone'}`, title: w.visible ? '' : 'Nu a fost văzut la ultimul scan' },
        el('span', { class: 'dot', style: { background: signalColor(w.signal_dbm) } }),
        el('span', { class: 'mono' }, w.bssid),
        el('span', {}, `ch ${w.channel || '?'}`),
        el('span', {}, `${w.band || '?'} GHz`),
        el('span', { style: { color: signalColor(w.signal_dbm), fontWeight: 600 } }, w.signal_dbm != null ? `${w.signal_dbm} dBm` : '—'),
        assoc,
        el('div', { class: 'bssid-meta' },
          el('input', { value: w.label || '', placeholder: 'Etichetă (ex. AP Sală)', onchange: (e) => patch({ label: e.target.value }) }),
          el('input', { value: w.location || '', placeholder: 'Locație', onchange: (e) => patch({ location: e.target.value }) }),
          el('input', { value: w.notes || '', placeholder: 'Observații', onchange: (e) => patch({ notes: e.target.value }) })),
        pair ? el('div', { class: 'bssid-meta pair' }, `↔ probabil același echipament cu ${pair.ssid || '<ascuns>'} ${pair.band} GHz (${pair.bssid})`) : null,
        w.vendor ? el('div', { class: 'bssid-meta pair' }, `Producător: ${w.vendor}`) : null,
      ));
    }
    box.append(g);
  }
  renderWarnings(nets);
  $('#chart24').replaceChildren(channelChart(nets.filter((w) => w.band === '2.4'), [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13], [1, 6, 11]));
  const ch5 = [...new Set(nets.filter((w) => w.band === '5').map((w) => w.channel))].sort((a, b) => a - b);
  $('#chart5').replaceChildren(ch5.length ? channelChart(nets.filter((w) => w.band === '5'), ch5, []) : el('p', { class: 'muted small' }, 'Nicio rețea pe 5 GHz.'));
}

function renderWarnings(nets) {
  const box = $('#wifiWarnings');
  box.replaceChildren();
  const bad = nets.filter((w) => w.band === '2.4' && w.channel && ![1, 6, 11].includes(w.channel) && w.visible);
  for (const w of bad) {
    box.append(el('div', { class: 'warn-box' }, `${w.ssid || '<ascuns>'} (${w.bssid}) e pe canalul ${w.channel} pe 2.4 GHz — se suprapune cu canalele vecine. Recomandat: 1, 6 sau 11.`));
  }
  // AP-uri proprii puternice pe același canal
  const strong = nets.filter((w) => w.visible && (w.signal_dbm ?? -100) >= -70 && w.channel);
  const byCh = new Map();
  for (const w of strong) {
    const k = `${w.band}/${w.channel}`;
    if (!byCh.has(k)) byCh.set(k, []);
    const l = byCh.get(k);
    if (!l.some((x) => sameRadio(x.bssid, w.bssid))) l.push(w);
  }
  for (const [k, l] of byCh) {
    if (l.length < 2) continue;
    const [band, ch] = k.split('/');
    box.append(el('div', { class: 'warn-box' }, `${l.length} AP-uri cu semnal puternic pe același canal ${ch} (${band} GHz): ${l.map((w) => `${w.ssid || '<ascuns>'} ${w.signal_dbm} dBm`).join(', ')}. Mută-le pe canale diferite ca să nu interfereze.`));
  }
  const inferred = nets.filter((w) => w.ap_link_status === 'inferat').length;
  if (inferred) box.append(el('div', { class: 'warn-box info' }, `${inferred} asocieri BSSID ↔ echipament sunt deduse automat (inferat). Confirmă-le sau respinge-le mai jos.`));
}

function channelChart(nets, channels, good) {
  const W = 330, H = 120, pad = 18;
  const counts = channels.map((ch) => nets.filter((w) => w.channel === ch));
  const max = Math.max(1, ...counts.map((c) => c.length));
  const bw = (W - pad) / channels.length;
  const parts = [];
  channels.forEach((ch, i) => {
    const list = counts[i];
    const h = (list.length / max) * (H - 34);
    const best = list.reduce((m, w) => Math.max(m, w.signal_dbm ?? -100), -200);
    const x = pad + i * bw;
    if (good.includes(ch)) parts.push(`<rect x="${x}" y="4" width="${bw - 2}" height="${H - 24}" fill="currentColor" opacity=".05"/>`);
    if (list.length) {
      parts.push(`<rect x="${x + 2}" y="${H - 20 - h}" width="${bw - 6}" height="${h}" rx="2" fill="${signalColor(best > -200 ? best : null)}"><title>Canal ${ch}: ${list.map((w) => `${w.ssid || '<ascuns>'} (${w.signal_dbm} dBm)`).join(', ')}</title></rect>`);
      parts.push(`<text x="${x + bw / 2 - 1}" y="${H - 24 - h}" font-size="10" text-anchor="middle" fill="currentColor">${list.length}</text>`);
    }
    parts.push(`<text x="${x + bw / 2 - 1}" y="${H - 6}" font-size="${channels.length > 16 ? 8 : 10}" text-anchor="middle" fill="currentColor" opacity="${good.includes(ch) ? 1 : 0.6}" font-weight="${good.includes(ch) ? 700 : 400}">${ch}</text>`);
  });
  return el('div', { html: `<svg viewBox="0 0 ${W} ${H}" width="100%" style="color:var(--text)">${parts.join('')}</svg>` });
}
