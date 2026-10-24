// Tab-ul Survey: puncte de măsurare, heatmap IDW per SSID, poziția estimată a AP-urilor.

import { api } from './api.js';
import { $, el, errorToast, confirmDialog, fmtDate, toast } from './ui.js';
import { state, on, emit } from './state.js';
import { signalColor } from './wifi.js';

let ctx;

export function initSurvey(c) {
  ctx = c;
  on('survey', () => { fillSSIDs(); render(); updateOverlays(); });
  on('map', () => updateOverlays());
  on('tab', (t) => { if (t === 'survey') render(); });
  $('#heatSSID').addEventListener('change', () => { updateOverlays(); render(); });
  $('#apEstimate').addEventListener('change', updateOverlays);
  $('#surveyAddBtn').addEventListener('click', () => {
    ctx.switchTab('map');
    state.editor.setTool('survey');
    toast('Dă click pe hartă acolo unde te afli cu laptopul.');
  });
}

function fillSSIDs() {
  const sel = $('#heatSSID');
  const cur = sel.value;
  const ssids = [...new Set(state.survey.flatMap((p) => p.readings.map((r) => r.ssid)).filter(Boolean))].sort();
  sel.replaceChildren(el('option', { value: '' }, '— fără heatmap —'), ...ssids.map((s) => el('option', { value: s }, s)));
  if (ssids.includes(cur)) sel.value = cur;
}

// Punctele de pe harta curentă: poziția vine din scenă (mutabilă), măsurătorile din DB.
function pointsOnMap() {
  const out = [];
  for (const sp of state.editor.scene.surveyPoints) {
    const info = state.surveyMap.get(sp.survey_point_id);
    if (info) out.push({ x: sp.x, y: sp.y, info });
  }
  return out;
}

function bestFor(info, ssid) {
  const r = info.readings.filter((x) => x.ssid === ssid);
  return r.length ? Math.max(...r.map((x) => x.signal_dbm)) : null;
}

// Heatmap: interpolare IDW pe o grilă, randată semi-transparent sub icon-uri.
function buildHeatmap(ssid) {
  const pts = pointsOnMap().map((p) => ({ x: p.x, y: p.y, v: bestFor(p.info, ssid) ?? -95 }));
  if (pts.length < 2) return null;
  const xs = pts.map((p) => p.x), ys = pts.map((p) => p.y);
  const margin = 160;
  const x0 = Math.min(...xs) - margin, y0 = Math.min(...ys) - margin;
  const x1 = Math.max(...xs) + margin, y1 = Math.max(...ys) + margin;
  const cell = Math.max(10, Math.round(Math.max(x1 - x0, y1 - y0) / 120));
  const cols = Math.ceil((x1 - x0) / cell), rows = Math.ceil((y1 - y0) / cell);
  const c = document.createElement('canvas');
  c.width = cols;
  c.height = rows;
  const g = c.getContext('2d');
  const img = g.createImageData(cols, rows);
  for (let j = 0; j < rows; j++) {
    for (let i = 0; i < cols; i++) {
      const x = x0 + (i + 0.5) * cell, y = y0 + (j + 0.5) * cell;
      let num = 0, den = 0, exact = null;
      for (const p of pts) {
        const d2 = (p.x - x) ** 2 + (p.y - y) ** 2;
        if (d2 < 1) { exact = p.v; break; }
        const w = 1 / d2;
        num += w * p.v;
        den += w;
      }
      const v = exact ?? num / den;
      const [r, gg, b] = heatColor(v);
      // Transparență mai mare departe de puncte.
      const nearest = Math.min(...pts.map((p) => Math.hypot(p.x - x, p.y - y)));
      const a = Math.max(0, Math.min(1, 1.25 - nearest / (margin * 1.6)));
      const k = (j * cols + i) * 4;
      img.data[k] = r; img.data[k + 1] = gg; img.data[k + 2] = b; img.data[k + 3] = Math.round(a * 255);
    }
  }
  g.putImageData(img, 0, 0);
  return { canvas: c, x: x0, y: y0, w: cols * cell, h: rows * cell };
}

function heatColor(dbm) {
  // -90 (roșu) → -70 (galben) → -50 (verde)
  const t = Math.max(0, Math.min(1, (dbm + 90) / 40));
  if (t < 0.5) {
    const u = t / 0.5;
    return [217 + (249 - 217) * u, 48 + (171 - 48) * u, 37 + (0 - 37) * u];
  }
  const u = (t - 0.5) / 0.5;
  return [249 + (30 - 249) * u, 171 + (142 - 171) * u, 0 + (62 - 0) * u];
}

// Poziția estimată a fiecărui BSSID: centroid ponderat cu puterea semnalului.
function estimateAPs(ssidFilter) {
  const pts = pointsOnMap();
  if (pts.length < 2) return [];
  const byB = new Map();
  for (const p of pts) {
    for (const r of p.info.readings) {
      if (ssidFilter && r.ssid !== ssidFilter) continue;
      if (!byB.has(r.bssid)) byB.set(r.bssid, { r, obs: [] });
      byB.get(r.bssid).obs.push({ x: p.x, y: p.y, v: r.signal_dbm });
    }
  }
  const out = [];
  for (const [bssid, { r, obs }] of byB) {
    if (obs.length < 2) continue;
    let sw = 0, sx = 0, sy = 0;
    for (const o of obs) {
      const w = 10 ** (o.v / 10) * 1e9;
      sw += w; sx += w * o.x; sy += w * o.y;
    }
    const wifi = state.wifi.find((w) => w.bssid === bssid);
    out.push({ x: sx / sw, y: sy / sw, label: `${wifi?.label || r.ssid || '<ascuns>'} ${r.band}G`, bssid, max: Math.max(...obs.map((o) => o.v)) });
  }
  return out;
}

function updateOverlays() {
  const ed = state.editor;
  if (!ed) return;
  const ssid = $('#heatSSID').value;
  ed.heatmap = ssid ? buildHeatmap(ssid) : null;
  ed.apEstimates = $('#apEstimate').checked ? estimateAPs(ssid) : [];
  ed.requestRender();
}

function render() {
  if (state.tab !== 'survey') return;
  const box = $('#surveyList');
  box.replaceChildren();
  if (!state.survey.length) {
    box.append(el('div', { class: 'empty' }, el('b', {}, 'Niciun punct de survey'), 'Apasă „Punct survey pe hartă”, apoi click pe hartă acolo unde te afli.'));
    return;
  }
  const ssid = $('#heatSSID').value;
  const onMap = new Set(state.editor.scene.surveyPoints.map((s) => s.survey_point_id));
  for (const p of state.survey) {
    const readings = ssid ? p.readings.filter((r) => r.ssid === ssid) : p.readings;
    const card = el('div', { class: 'sp-card' },
      el('h4', {}, p.label || `Punct ${p.id}`,
        el('span', { class: 'muted small' }, fmtDate(p.created_at)),
        onMap.has(p.id) ? null : el('span', { class: 'tag' }, p.map_id && p.map_id !== state.map?.id ? 'pe altă hartă' : 'nu e pe hartă'),
        el('div', { class: 'actions' },
          onMap.has(p.id) ? el('button', { onclick: () => { ctx.switchTab('map'); const s = state.editor.scene.surveyPoints.find((x) => x.survey_point_id === p.id); state.editor.select([{ type: 'survey', id: s.id }]); state.editor.centerOn([s.x, s.y]); } }, 'Arată pe hartă') : null,
          el('button', { class: 'danger-text', onclick: () => remove(p) }, 'Șterge'))),
      readings.length
        ? el('table', { class: 'grid' }, readings.slice(0, 10).map((r) => el('tr', {},
          el('td', {}, r.ssid || '<ascuns>'), el('td', { class: 'mono' }, r.bssid), el('td', {}, `${r.band} GHz · ch ${r.channel}`),
          el('td', { style: { width: '45%' } }, el('span', { class: 'sig', style: { width: `${Math.max(4, (r.signal_dbm + 100) * 2)}px`, background: signalColor(r.signal_dbm) } }), `${r.signal_dbm} dBm`))))
        : el('p', { class: 'muted small' }, 'Nicio măsurătoare.'));
    box.append(card);
  }
  const est = estimateAPs(ssid);
  if (est.length) {
    box.append(el('h3', {}, 'Poziție estimată AP-uri'),
      el('p', { class: 'muted small' }, 'Estimare din punctele cu semnal maxim (centroid ponderat). Cu cât ai mai multe puncte, cu atât estimarea e mai bună. Bifează opțiunea de mai sus ca s-o vezi pe hartă.'),
      el('table', { class: 'grid' }, est.map((a) => el('tr', {}, el('td', {}, a.label), el('td', { class: 'mono' }, a.bssid), el('td', {}, `max ${a.max} dBm`)))));
  }
}

async function remove(p) {
  const ok = await confirmDialog(`Ștergi punctul „${p.label}” și măsurătorile lui?`, { title: 'Șterge punct survey', ok: 'Șterge', danger: true });
  if (!ok) return;
  try {
    await api.deleteSurveyPoint(p.id);
    const ed = state.editor;
    if (ed.scene.surveyPoints.some((s) => s.survey_point_id === p.id)) {
      ed.change('Șterge punct survey', (s) => { s.surveyPoints = s.surveyPoints.filter((x) => x.survey_point_id !== p.id); });
    }
    await ctx.reloadInventory();
  } catch (e) { errorToast(e); }
}
