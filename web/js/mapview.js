// Tab-ul Hartă: editorul, unelte, paleta de icon-uri, dispozitive neplasate,
// panoul de proprietăți și bara de stare.

import { api } from './api.js';
import { $, $$, el, toast, errorToast, formDialog, promptDialog, debounce } from './ui.js';
import { state, emit, on, placedDeviceIds, deviceName, deviceType, btName, LS } from './state.js';
import { Editor } from './editor/canvas.js';
import { iconCategories, iconDefs, iconSVG, iconLabel, iconForType } from './editor/icons.js';
import { CABLE_TYPES, LAYERS, ROOM_COLORS, newId, polylineLength, layer, roomAt } from './editor/scene.js';

let ctx; // {saveMap, reloadInventory}
let ed;

const TOOL_DEFS = [
  { id: 'select', label: 'Select', key: 'V', svg: '<path d="M5 3l12 9-5 1 3 6-2 1-3-6-4 4z" fill="currentColor"/>' },
  { id: 'room', label: 'Cameră', key: 'R', svg: '<rect x="3" y="5" width="18" height="14" rx="1" fill="currentColor" fill-opacity=".2" stroke="currentColor" stroke-width="2"/>' },
  { id: 'wall', label: 'Perete', key: 'W', svg: '<path d="M3 20V6h11v8h7" fill="none" stroke="currentColor" stroke-width="4" stroke-linejoin="miter"/>' },
  { id: 'cable', label: 'Cablu', key: 'C', svg: '<circle cx="5" cy="6" r="2.5" fill="currentColor"/><circle cx="19" cy="18" r="2.5" fill="currentColor"/><path d="M5 6h7v12h7" fill="none" stroke="currentColor" stroke-width="2.2"/>' },
  { id: 'text', label: 'Text', key: 'T', svg: '<path d="M5 5h14M12 5v15M9 20h6" fill="none" stroke="currentColor" stroke-width="2.4"/>' },
  { id: 'door', label: 'Ușă', key: 'D', svg: '<path d="M4 20h16M6 20V6M6 6a14 14 0 0 1 14 14" fill="none" stroke="currentColor" stroke-width="2"/>' },
  { id: 'window', label: 'Fereastră', key: '', svg: '<rect x="3" y="9" width="18" height="6" fill="none" stroke="currentColor" stroke-width="2"/><path d="M3 12h18" stroke="currentColor" stroke-width="2"/>' },
  { id: 'survey', label: 'Survey', key: 'P', svg: '<circle cx="12" cy="12" r="3" fill="currentColor"/><circle cx="12" cy="12" r="7" fill="none" stroke="currentColor" stroke-width="2"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3" stroke="currentColor" stroke-width="2"/>' },
  { id: 'scale', label: 'Scară', key: '', svg: '<path d="M3 16h18M3 12v8M21 12v8M8 14v4M13 14v4M17 14v4" fill="none" stroke="currentColor" stroke-width="2"/>' },
  { id: 'eraser', label: 'Șterge', key: 'E', svg: '<path d="M8 20h12M4 15l9-9 6 6-7 7H8z" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/>' },
];

const HINTS = {
  select: '',
  room: 'Trage pentru a desena o cameră. Alt = fără snap.',
  wall: 'Click pentru puncte · dublu-click / Enter = termină · Shift = 0/45/90° · Backspace = anulează ultimul punct',
  cable: 'Click pe un port (sau nod) → click-uri pentru colțuri → click pe portul destinație. Enter / dublu-click = capăt liber · M = drept/ortogonal · Shift = 0/45/90°',
  text: 'Click pentru a plasa un text.',
  door: 'Click lângă un perete: ușa se aliniază automat. Shift = plasează mai multe.',
  window: 'Click lângă un perete: fereastra se aliniază automat.',
  survey: 'Click unde stai cu laptopul: se rulează un scan Wi-Fi și se salvează semnalul în acel punct.',
  scale: 'Trasează o linie pe o distanță cunoscută (ex. un perete), apoi introdu lungimea în metri.',
  eraser: 'Click pe un element pentru a-l șterge de pe hartă (inventarul nu e afectat).',
  place: 'Click pe hartă pentru a plasa echipamentul · Shift = plasează mai multe · Esc = renunță',
};

export function initMapView(c) {
  ctx = c;
  ed = new Editor($('#canvas'), $('#minimap'), {
    getDevice: (id) => state.devMap.get(id),
    getBluetooth: (id) => state.btMap.get(id),
    getSurvey: (id) => state.surveyMap.get(id),
    surveyBest,
    onDirty: (v) => emit('dirty', v),
    onSelection: () => renderProps(),
    onSelectionSoft: () => { if (!$('#props').contains(document.activeElement)) renderProps(); },
    onTool: (t) => renderTools(t),
    onHistory: () => {
      $('#undoBtn').disabled = !ed.history.canUndo;
      $('#redoBtn').disabled = !ed.history.canRedo;
      $('#undoBtn').title = ed.history.canUndo ? `Undo: ${ed.history.undoLabel} (Ctrl+Z)` : 'Undo (Ctrl+Z)';
      $('#redoBtn').title = ed.history.canRedo ? `Redo: ${ed.history.redoLabel} (Ctrl+Y)` : 'Redo (Ctrl+Y)';
    },
    onView: () => refreshStatus(),
    onCursor: (p) => {
      const upm = ed.scene.scale.unitsPerMeter;
      $('#cursorPos').textContent = upm ? `x: ${(p[0] / upm).toFixed(2)} m  y: ${(p[1] / upm).toFixed(2)} m` : `x: ${Math.round(p[0])}  y: ${Math.round(p[1])}`;
    },
    onChange: () => { renderUnplaced(); },
    onDropDevice: (id, p) => placeDevice(id, p),
    onNodesInRooms: (u) => nodesInRooms(u),
    onSurveyPoint: (p) => addSurveyPoint(p),
    onSurveyMoved: (sps) => {
      for (const sp of sps) if (sp.survey_point_id) api.patchSurveyPoint(sp.survey_point_id, { x: sp.x, y: sp.y }).catch(errorToast);
    },
    onScale: (len) => setScale(len),
    onEditProps: () => setTimeout(() => { const i = $('#props input, #props textarea'); if (i) { i.focus(); i.select?.(); } }, 30),
    onMessage: (m) => toast(m),
  });
  state.editor = ed;
  ed.history.onChange();
  ed.showIP = LS.get('showIP', true);
  ed.showLengths = LS.get('showLengths', false);

  renderTools('select');
  renderPalette();
  bindStatus();
  on('devices', () => { renderUnplaced(); renderProps(); ed.requestRender(); });
  on('bluetooth', () => { renderProps(); ed.requestRender(); });
  on('survey', () => ed.requestRender());
  on('map', () => { renderUnplaced(); renderProps(); refreshStatus(); });
  on('project', () => renderUnplaced());

  $('#autoLayoutBtn').addEventListener('click', async () => {
    const { runAutoLayout } = await import('./app.js');
    runAutoLayout();
  });
  $('#minimap').addEventListener('click', (e) => {
    const r = e.currentTarget.getBoundingClientRect();
    ed.minimapClick(e.clientX - r.left, e.clientY - r.top);
  });
  $('#minimap').hidden = !LS.get('minimap', true);
  $('#mmChk').checked = !$('#minimap').hidden;
  new MutationObserver(() => ed.readTheme()).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
}

export function refreshMapChrome() {
  refreshStatus();
  renderUnplaced();
  renderProps();
}

function surveyBest(spid) {
  const sp = state.surveyMap.get(spid);
  if (!sp || !sp.readings?.length) return '';
  const ssid = $('#heatSSID')?.value;
  const list = ssid ? sp.readings.filter((r) => r.ssid === ssid) : sp.readings;
  const r = list[0];
  return r ? `${r.ssid || '<ascuns>'} ${r.signal_dbm} dBm` : '';
}

// ---------- unelte ----------

function renderTools(active) {
  const box = $('#tools');
  if (!box.children.length) {
    for (const t of TOOL_DEFS) {
      box.append(el('button', { 'data-tool': t.id, title: `${t.label}${t.key ? ` (${t.key})` : ''}`, onclick: () => { ed.setTool(t.id); $('#canvas').focus(); } },
        el('span', { html: `<svg viewBox="0 0 24 24">${t.svg}</svg>` }), t.label, t.key ? el('kbd', {}, t.key) : null));
    }
  }
  for (const b of $$('#tools button')) b.classList.toggle('active', b.dataset.tool === active);
  for (const b of $$('.pal-item')) b.classList.toggle('active', active === 'place' && b.dataset.icon === ed.tool?.icon);
  $('#canvasHint').textContent = HINTS[active] || '';
  const opts = $('#toolOpts');
  opts.replaceChildren();
  if (active === 'cable') {
    opts.append(
      el('div', { class: 'row' }, 'Mod',
        el('span', { class: 'seg' },
          el('button', { class: ed.linkMode === 'orthogonal' ? 'on' : '', onclick: () => { ed.linkMode = 'orthogonal'; renderTools('cable'); } }, 'Ortogonal'),
          el('button', { class: ed.linkMode === 'straight' ? 'on' : '', onclick: () => { ed.linkMode = 'straight'; renderTools('cable'); } }, 'Drept')),
        el('span', { class: 'muted' }, '(M)')),
      el('div', { class: 'row' }, 'Tip',
        el('select', { onchange: (e) => { ed.cableType = e.target.value; } },
          Object.entries(CABLE_TYPES).map(([k, v]) => el('option', { value: k, selected: ed.cableType === k }, v.label)))),
    );
  }
  if (active === 'wall') {
    opts.append(el('div', { class: 'row' }, 'Grosime',
      el('input', { type: 'number', min: 2, max: 40, value: ed.wallThickness || 8, style: { width: '64px' }, onchange: (e) => { ed.wallThickness = +e.target.value || 8; } })));
  }
}

// ---------- paleta de icon-uri ----------

function renderPalette() {
  const box = $('#palette');
  const search = $('#iconSearch');
  const draw = () => {
    const q = search.value.trim().toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '');
    box.replaceChildren();
    const defs = iconDefs();
    for (const cat of iconCategories()) {
      const items = Object.entries(defs).filter(([k, d]) => d.cat === cat &&
        (!q || (d.label + ' ' + k + ' ' + cat).toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').includes(q)));
      if (!items.length) continue;
      box.append(el('div', { class: 'pal-cat' }, cat));
      const grid = el('div', { class: 'pal-grid' });
      for (const [k, d] of items) {
        const it = el('button', {
          class: 'pal-item', draggable: 'true', 'data-icon': k, title: d.label + ' — trage pe hartă sau click apoi click pe hartă',
          onclick: () => { ed.setTool('place', { icon: k }); renderTools('place'); $('#canvas').focus(); },
          ondragstart: (e) => {
            window.__netmapDrag = { icon: k };
            e.dataTransfer.setData('application/x-netmap', JSON.stringify({ kind: 'icon', icon: k }));
            e.dataTransfer.effectAllowed = 'copy';
            e.dataTransfer.setDragImage(it.querySelector('svg'), 15, 15);
          },
          ondragend: () => { window.__netmapDrag = null; ed.ghost = null; ed.requestRender(); },
        }, el('span', { html: iconSVG(k, 'var(--accent)', 30) }), d.label);
        grid.append(it);
      }
      box.append(grid);
    }
    if (!box.children.length) box.append(el('p', { class: 'muted small' }, 'Niciun icon găsit.'));
  };
  search.addEventListener('input', draw);
  draw();
}

// ---------- dispozitive neplasate ----------

function renderUnplaced() {
  const box = $('#unplaced');
  if (!box) return;
  const placed = placedDeviceIds();
  const list = state.devices.filter((d) => !placed.has(d.id));
  $('#unplacedCount').textContent = list.length || '';
  box.replaceChildren();
  if (!state.devices.length) {
    box.append(el('p', { class: 'muted small' }, 'Rulează un scan (butonul Scan) ca să apară aici dispozitivele din rețea.'));
    return;
  }
  if (!list.length) {
    box.append(el('p', { class: 'muted small' }, 'Toate dispozitivele sunt pe hartă.'));
    return;
  }
  list.sort((a, b) => Number(b.online) - Number(a.online));
  for (const d of list) {
    const icon = iconForType(deviceType(d));
    const it = el('div', {
      class: 'unplaced-item', draggable: 'true', title: 'Trage pe hartă (sau click pentru a-l plasa în centru)',
      ondragstart: (e) => {
        window.__netmapDrag = { icon, color: d.color };
        e.dataTransfer.setData('application/x-netmap', JSON.stringify({ kind: 'device', id: d.id }));
        e.dataTransfer.effectAllowed = 'copy';
      },
      ondragend: () => { window.__netmapDrag = null; ed.ghost = null; ed.requestRender(); },
      onclick: () => placeDevice(d.id, ed.snapPos(ed.toWorld(ed.cssW / 2, ed.cssH / 2))),
    },
    el('span', { html: iconSVG(icon, d.color || 'var(--accent)', 22) }),
    el('div', { class: 'txt' }, el('div', {}, el('span', { class: `dot ${d.online ? 'on' : ''}` }), ' ', deviceName(d)), el('div', { class: 'sub' }, d.last_ip || d.mac)));
    box.append(it);
  }
}

function placeDevice(id, p) {
  const d = state.devMap.get(id);
  if (!d) return;
  const existing = ed.scene.nodes.find((n) => n.device_id === id);
  if (existing) {
    ed.select([{ type: 'node', id: existing.id }]);
    ed.centerOn([existing.x, existing.y]);
    toast('Dispozitivul este deja pe hartă.');
    return;
  }
  ed.addNode({ icon: iconForType(deviceType(d)), device_id: id, label: '' }, freeSpot(p));
  renderUnplaced();
}

// freeSpot: dacă locul e ocupat de alt nod, caută cel mai apropiat loc liber (în pași de grilă).
function freeSpot(p) {
  const g = ed.scene.grid.size;
  const step = Math.max(g, 20) * 4;
  const busy = (q) => ed.scene.nodes.some((n) => Math.abs(n.x - q[0]) < 50 && Math.abs(n.y - q[1]) < 60);
  if (!busy(p)) return p;
  for (let r = 1; r < 12; r++) {
    for (let i = -r; i <= r; i++) {
      for (const q of [[p[0] + i * step, p[1] - r * step], [p[0] + i * step, p[1] + r * step], [p[0] - r * step, p[1] + i * step], [p[0] + r * step, p[1] + i * step]]) {
        if (!busy(q)) return q;
      }
    }
  }
  return p;
}

export function showNodeOfDevice(id) {
  const n = ed.scene.nodes.find((x) => x.device_id === id);
  if (!n) return false;
  ed.select([{ type: 'node', id: n.id }]);
  ed.centerOn([n.x, n.y], Math.max(ed.view.zoom, 1));
  return true;
}

// nodesInRooms: dispozitivul preia numele camerei ca locație (dacă nu a fost setată manual).
async function nodesInRooms(updates) {
  for (const u of updates) {
    const d = state.devMap.get(u.node.device_id);
    if (!d) continue;
    if (d.location && d.location !== u.prev) continue;
    try {
      const nd = await api.patchDevice(d.id, { location: u.room });
      Object.assign(d, { location: nd.location });
    } catch (e) { errorToast(e); }
  }
  renderProps();
  emit('devices-edited');
}

async function addSurveyPoint(p) {
  const label = await promptDialog('Punct survey', 'Unde te afli?', `Punct ${ed.scene.surveyPoints.length + 1}`, { placeholder: 'ex. Lângă bar', ok: 'Scanează aici' });
  if (!label) return;
  toast('Scanez rețelele Wi-Fi în acest punct…', 'info', 2500);
  try {
    const r = await api.createSurveyPoint(state.project.id, { label, x: p[0], y: p[1], map_id: state.map.id });
    ed.begin();
    ed.scene.surveyPoints.push({ id: newId('s'), survey_point_id: r.point.id, x: p[0], y: p[1], label });
    ed.commit('Punct survey');
    await ctx.reloadInventory();
    emit('survey-updated');
    if (r.warning) toast(r.warning, 'error', 6000);
    else toast(`${r.point.readings.length} rețele măsurate în „${label}”.`, 'ok');
  } catch (e) { errorToast(e); }
}

async function setScale(len) {
  const v = await promptDialog('Scara hărții', 'Lungimea liniei trasate, în metri', '', { placeholder: 'ex. 5', ok: 'Setează' });
  const m = parseFloat(String(v || '').replace(',', '.'));
  if (!m || m <= 0) return;
  ed.change('Scară', (s) => { s.scale.unitsPerMeter = Math.round((len / m) * 100) / 100; });
  toast(`Scara: 1 m = ${ed.scene.scale.unitsPerMeter} unități.`, 'ok');
  renderProps();
}

// ---------- bara de stare ----------

function bindStatus() {
  $('#zoomIn').onclick = () => ed.setZoom(ed.view.zoom * 1.25);
  $('#zoomOut').onclick = () => ed.setZoom(ed.view.zoom / 1.25);
  $('#fitBtn').onclick = () => ed.fitAll();
  $('#undoBtn').onclick = () => ed.undo();
  $('#redoBtn').onclick = () => ed.redo();
  $('#gridSize').onchange = (e) => { ed.change('Grilă', (s) => { s.grid.size = +e.target.value; }); };
  $('#gridVis').onchange = (e) => { ed.scene.grid.visible = e.target.checked; ed.setDirty(true); ed.requestRender(); };
  $('#snapChk').onchange = (e) => { ed.scene.grid.snap = e.target.checked; ed.setDirty(true); };
  $('#ipChk').checked = ed.showIP;
  $('#ipChk').onchange = (e) => { ed.showIP = e.target.checked; LS.set('showIP', ed.showIP); ed.requestRender(); };
  $('#lenChk').checked = ed.showLengths;
  $('#lenChk').onchange = (e) => {
    ed.showLengths = e.target.checked;
    LS.set('showLengths', ed.showLengths);
    if (ed.showLengths && !ed.scene.scale.unitsPerMeter) toast('Setează întâi scara hărții (unealta Scară).');
    ed.requestRender();
  };
  $('#mmChk').onchange = (e) => { $('#minimap').hidden = !e.target.checked; LS.set('minimap', e.target.checked); ed.requestRender(); };
}

function refreshStatus() {
  $('#zoomVal').textContent = Math.round(ed.view.zoom * 100) + '%';
  $('#gridSize').value = String(ed.scene.grid.size);
  $('#gridVis').checked = ed.scene.grid.visible !== false;
  $('#snapChk').checked = !!ed.scene.grid.snap;
}

// ---------- panoul de proprietăți ----------

function field(label, input, hint) {
  return el('label', { class: 'field' }, el('span', {}, label), input, hint ? el('div', { class: 'hint' }, hint) : null);
}

function input(value, onchange, attrs = {}) {
  return el('input', { type: 'text', value: value ?? '', ...attrs, onchange: (e) => onchange(attrs.type === 'number' ? parseFloat(e.target.value) : e.target.value) });
}

function select(options, value, onchange) {
  return el('select', { onchange: (e) => onchange(e.target.value) },
    options.map((o) => (o.group
      ? el('optgroup', { label: o.group }, o.options.map((x) => el('option', { value: x.value, selected: String(x.value) === String(value) }, x.label)))
      : el('option', { value: o.value, selected: String(o.value) === String(value) }, o.label))));
}

function iconOptions() {
  const defs = iconDefs();
  return iconCategories().map((cat) => ({
    group: cat,
    options: Object.entries(defs).filter(([, d]) => d.cat === cat).map(([k, d]) => ({ value: k, label: d.label })),
  }));
}

function renderProps() {
  const box = $('#props');
  if (!box || !ed) return;
  box.replaceChildren();
  box.className = 'sidebar right props';
  const objs = ed.selectedObjects();
  if (!objs.length) return renderMapProps(box);
  if (objs.length > 1) return renderMulti(box, objs);
  const { type, obj } = objs[0];
  const set = (label, fn) => ed.change(label, () => fn(obj));
  switch (type) {
    case 'node': return renderNodeProps(box, obj, set);
    case 'link': return renderLinkProps(box, obj, set);
    case 'room': {
      box.append(el('h3', {}, 'Cameră'),
        field('Nume', input(obj.name, (v) => set('Nume cameră', (o) => { o.name = v; }))),
        field('Culoare', el('div', { class: 'btn-row' },
          ...ROOM_COLORS.map((c) => el('button', { style: { background: c, width: '26px', height: '22px', padding: 0, outline: obj.color === c ? '2px solid var(--accent)' : '' }, title: c, onclick: () => set('Culoare cameră', (o) => { o.color = c; }) })),
          el('input', { type: 'color', value: obj.color || '#e8f0fe', onchange: (e) => set('Culoare cameră', (o) => { o.color = e.target.value; }) }))),
        el('div', { class: 'field-row' },
          field('X', input(Math.round(obj.x), (v) => set('Poziție', (o) => { o.x = v; }), { type: 'number' })),
          field('Y', input(Math.round(obj.y), (v) => set('Poziție', (o) => { o.y = v; }), { type: 'number' }))),
        el('div', { class: 'field-row' },
          field('Lățime', input(Math.round(obj.w), (v) => set('Dimensiune', (o) => { o.w = Math.max(10, v); }), { type: 'number' })),
          field('Înălțime', input(Math.round(obj.h), (v) => set('Dimensiune', (o) => { o.h = Math.max(10, v); }), { type: 'number' }))),
        sizeInMeters(obj.w, obj.h) || '',
        el('p', { class: 'muted small' }, 'Echipamentele plasate în cameră primesc automat locația = numele camerei.'),
        deleteBtn());
      return;
    }
    case 'wall': {
      const len = polylineLength(obj.points);
      box.append(el('h3', {}, 'Perete'),
        field('Grosime', input(obj.thickness || 8, (v) => set('Grosime perete', (o) => { o.thickness = Math.max(1, v || 8); }), { type: 'number', min: 1, max: 60 })),
        el('dl', { class: 'kv' }, el('dt', {}, 'Puncte'), el('dd', {}, obj.points.length), el('dt', {}, 'Lungime'), el('dd', {}, lengthStr(len))),
        el('p', { class: 'muted small' }, 'Trage de pătrățele pentru a muta colțurile. Selectează un colț și apasă Del pentru a-l elimina.'),
        deleteBtn());
      return;
    }
    case 'text': {
      box.append(el('h3', {}, 'Text'),
        field('Text', input(obj.text, (v) => set('Text', (o) => { o.text = v; }))),
        el('div', { class: 'field-row' },
          field('Mărime', input(obj.size || 14, (v) => set('Mărime text', (o) => { o.size = Math.max(6, v || 14); }), { type: 'number', min: 6, max: 120 })),
          field('Culoare', el('input', { type: 'color', value: obj.color || '#202124', onchange: (e) => set('Culoare text', (o) => { o.color = e.target.value; }) }))),
        el('label', { class: 'check' }, el('input', { type: 'checkbox', checked: !!obj.bold, onchange: (e) => set('Text îngroșat', (o) => { o.bold = e.target.checked; }) }), ' Îngroșat'),
        deleteBtn());
      return;
    }
    case 'door': {
      box.append(el('h3', {}, obj.kind === 'window' ? 'Fereastră' : 'Ușă'),
        field('Tip', select([{ value: 'door', label: 'Ușă' }, { value: 'window', label: 'Fereastră' }], obj.kind, (v) => set('Tip', (o) => { o.kind = v; }))),
        el('div', { class: 'field-row' },
          field('Lățime', input(obj.w || 40, (v) => set('Lățime', (o) => { o.w = Math.max(10, v); }), { type: 'number' })),
          field('Rotație °', input(obj.rotation || 0, (v) => set('Rotație', (o) => { o.rotation = v || 0; }), { type: 'number', step: 15 }))),
        el('div', { class: 'btn-row' }, el('button', { onclick: () => set('Oglindește', (o) => { o.rotation = ((o.rotation || 0) + 180) % 360; }) }, 'Întoarce 180°')),
        deleteBtn());
      return;
    }
    case 'survey': {
      const sp = state.surveyMap.get(obj.survey_point_id);
      box.append(el('h3', {}, 'Punct survey'),
        field('Etichetă', input(obj.label || sp?.label, (v) => {
          set('Etichetă survey', (o) => { o.label = v; });
          if (obj.survey_point_id) api.patchSurveyPoint(obj.survey_point_id, { label: v }).then(() => ctx.reloadInventory()).catch(errorToast);
        })));
      if (sp?.readings?.length) {
        box.append(el('div', { class: 'sec' }, `Semnale (${sp.readings.length})`),
          el('table', { class: 'grid' }, sp.readings.slice(0, 12).map((r) => el('tr', {}, el('td', {}, r.ssid || '<ascuns>'), el('td', { class: 'mono' }, r.band + ' GHz'), el('td', {}, r.signal_dbm + ' dBm')))));
      } else box.append(el('p', { class: 'muted small' }, 'Nicio măsurătoare pentru acest punct.'));
      box.append(el('p', { class: 'muted small' }, 'Ștergerea de pe hartă păstrează măsurătorile; le poți șterge din tab-ul Survey.'), deleteBtn());
      return;
    }
  }
}

function deleteBtn(label = 'Șterge de pe hartă') {
  return el('div', { class: 'btn-row' }, el('button', { class: 'danger-text', onclick: () => ed.deleteSelection() }, label));
}

function lengthStr(units) {
  const upm = ed.scene.scale.unitsPerMeter;
  return upm ? `${(units / upm).toFixed(2)} m` : `${Math.round(units)} unități (setează scara pentru metri)`;
}

function sizeInMeters(w, h) {
  const upm = ed.scene.scale.unitsPerMeter;
  if (!upm) return null;
  return el('p', { class: 'muted small' }, `${(w / upm).toFixed(2)} × ${(h / upm).toFixed(2)} m · ${((w * h) / upm / upm).toFixed(1)} m²`);
}

function renderNodeProps(box, n, set) {
  const d = state.devMap.get(n.device_id);
  box.append(el('h3', {}, el('span', { html: iconSVG(n.icon, n.color || d?.color || 'var(--accent)', 26) }), iconLabel(n.icon)));
  box.append(
    field('Etichetă pe hartă', input(n.label, (v) => set('Etichetă', (o) => { o.label = v; }), { placeholder: d ? deviceName(d) + ' (din inventar)' : 'ex. Casă bar' })),
    field('Icon', select(iconOptions(), n.icon, (v) => set('Icon', (o) => { o.icon = v; }))),
    el('div', { class: 'field-row' },
      field('Culoare', el('input', { type: 'color', value: n.color || d?.color || '#1a73e8', onchange: (e) => set('Culoare', (o) => { o.color = e.target.value; }) })),
      field('Mărime', select([0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3].map((x) => ({ value: x, label: x + '×' })), n.scale || 1, (v) => set('Mărime', (o) => { o.scale = +v; }))),
      field('Rotație', select([0, 90, 180, 270].map((x) => ({ value: x, label: x + '°' })), n.rotation || 0, (v) => set('Rotație', (o) => { o.rotation = +v; })))),
  );

  // legătura cu inventarul
  const placed = placedDeviceIds();
  const devOpts = [{ value: '', label: '— nelegat de inventar —' },
    ...state.devices.filter((x) => x.id === n.device_id || !placed.has(x.id)).map((x) => ({ value: x.id, label: `${deviceName(x)}${x.last_ip && deviceName(x) !== x.last_ip ? ' · ' + x.last_ip : ''}` }))];
  box.append(el('div', { class: 'sec' }, 'Dispozitiv din inventar'),
    field('Dispozitiv', select(devOpts, n.device_id || '', (v) => set('Leagă dispozitiv', (o) => {
      if (v) {
        o.device_id = +v;
        const nd = state.devMap.get(+v);
        if (nd && (o.icon === 'necunoscut' || !o.icon)) o.icon = iconForType(deviceType(nd));
      } else delete o.device_id;
    }))));

  if (d) {
    const patch = async (fields) => {
      try {
        const nd = await api.patchDevice(d.id, fields);
        Object.assign(d, nd, { online: d.online, new: d.new, open_ports: d.open_ports });
        ed.requestRender();
        emit('devices-edited');
      } catch (e) { errorToast(e); }
    };
    box.append(
      field('Nume (inventar)', input(d.custom_name, (v) => patch({ custom_name: v }), { placeholder: d.hostname || d.last_ip })),
      field('Tip (inventar)', select([{ value: '', label: `automat: ${iconLabel(d.detected_type || 'necunoscut')}` }, ...iconOptions()], d.device_type || '', async (v) => {
        const oldIcon = iconForType(deviceType(d));
        await patch({ device_type: v });
        if (n.icon === oldIcon) set('Icon', (o) => { o.icon = iconForType(deviceType(d)); });
        renderProps();
      })),
      field('Locație', input(d.location, (v) => patch({ location: v }), { placeholder: roomAt(ed.scene, [n.x, n.y])?.name || 'ex. Bar, Sală, Bucătărie' })),
      field('Observații', el('textarea', { rows: 2, onchange: (e) => patch({ notes: e.target.value }) }, d.notes || '')),
      el('dl', { class: 'kv' },
        el('dt', {}, 'Stare'), el('dd', {}, el('span', { class: `dot ${d.online ? 'on' : ''}` }), d.online ? ' online' : ` offline (văzut ${d.last_seen ? new Date(d.last_seen).toLocaleDateString('ro-RO') : '—'})`),
        el('dt', {}, 'IP'), el('dd', { class: 'mono' }, d.last_ip || '—'),
        el('dt', {}, 'MAC'), el('dd', { class: 'mono' }, d.mac),
        el('dt', {}, 'Vendor'), el('dd', {}, d.vendor || '—'),
        el('dt', {}, 'Hostname'), el('dd', {}, d.hostname || '—'),
        el('dt', {}, 'Porturi'), el('dd', {}, portsStr(d.open_ports) || '—'),
        d.os_guess ? el('dt', {}, 'OS') : null, d.os_guess ? el('dd', {}, d.os_guess) : null,
      ),
    );
  }

  // dispozitiv Bluetooth
  const bt = n.bt_id ? state.btMap.get(n.bt_id) : null;
  if (bt) {
    const patchBt = async (fields) => {
      try {
        const nb = await api.patchBluetooth(bt.id, fields);
        Object.assign(bt, nb, { rssi: bt.rssi, visible: bt.visible });
        ed.requestRender();
      } catch (e) { errorToast(e); }
    };
    box.append(el('div', { class: 'sec' }, 'Dispozitiv Bluetooth'),
      field('Nume', input(bt.custom_name, (v) => patchBt({ custom_name: v }), { placeholder: bt.name || bt.mac })),
      field('Locație', input(bt.location, (v) => patchBt({ location: v }))),
      el('dl', { class: 'kv' },
        el('dt', {}, 'Stare'), el('dd', {}, bt.visible ? 'văzut la ultimul scan' : `nevăzut (ultima dată ${bt.last_seen ? new Date(bt.last_seen).toLocaleDateString('ro-RO') : '—'})`),
        el('dt', {}, 'Nume anunțat'), el('dd', {}, bt.name || '—'),
        el('dt', {}, 'MAC'), el('dd', { class: 'mono' }, bt.mac + (bt.address_type === 'random' ? ' (aleatoare)' : '')),
        el('dt', {}, 'Producător'), el('dd', {}, bt.manufacturer || '—'),
        el('dt', {}, 'Semnal'), el('dd', {}, bt.rssi != null ? `${bt.rssi} dBm` : '—')));
  }

  // BSSID-uri emise de acest echipament
  const wifiNets = state.wifi;
  if (wifiNets.length && ['router', 'router_ap', 'ap', 'modem'].includes(n.icon) || (n.wifi_bssids || []).length) {
    const sel = new Set(n.wifi_bssids || []);
    box.append(el('div', { class: 'sec' }, 'Rețele Wi-Fi emise'),
      el('div', { class: 'wifi-checks' }, wifiNets.map((w) => el('label', { class: 'check' },
        el('input', { type: 'checkbox', checked: sel.has(w.bssid), onchange: (e) => set('Wi-Fi nod', (o) => {
          const s = new Set(o.wifi_bssids || []);
          e.target.checked ? s.add(w.bssid) : s.delete(w.bssid);
          o.wifi_bssids = [...s];
        }) }),
        `${w.ssid || '<ascuns>'} · ${w.band} GHz · `, el('span', { class: 'mono' }, w.bssid.slice(-8))))));
  }
  box.append(el('div', { class: 'btn-row' },
    el('button', { onclick: () => set('Rotește', (o) => { o.rotation = ((o.rotation || 0) + 90) % 360; }) }, '↻ 90°'),
    el('button', { class: 'danger-text', onclick: () => ed.deleteSelection() }, 'Șterge de pe hartă')),
  el('p', { class: 'muted small' }, 'Ștergerea de pe hartă nu șterge dispozitivul din inventar.'));
}

function portsStr(js) {
  try { return (JSON.parse(js || '[]') || []).join(', '); } catch { return ''; }
}

function renderLinkProps(box, l, set) {
  const len = polylineLength(l.route || []);
  const nodeName = (end) => {
    if (!end.node) return 'capăt liber';
    const n = ed.scene.nodes.find((x) => x.id === end.node);
    return n ? `${ed.nodeLabel(n) || iconLabel(n.icon)} (${{ top: 'sus', right: 'dreapta', bottom: 'jos', left: 'stânga' }[end.port] || end.port})` : '—';
  };
  box.append(el('h3', {}, 'Cablu'),
    field('Tip', select(Object.entries(CABLE_TYPES).map(([k, v]) => ({ value: k, label: v.label })), l.type, (v) => set('Tip cablu', (o) => { o.type = v; }))),
    field('Traseu', el('span', { class: 'seg' },
      el('button', { class: l.mode === 'orthogonal' ? 'on' : '', onclick: () => set('Mod cablu', (o) => { o.mode = 'orthogonal'; }) }, 'Ortogonal'),
      el('button', { class: l.mode !== 'orthogonal' ? 'on' : '', onclick: () => set('Mod cablu', (o) => { o.mode = 'straight'; }) }, 'Drept'))),
    field('Etichetă', input(l.label, (v) => set('Etichetă cablu', (o) => { o.label = v; }), { placeholder: 'ex. Cat6, 15 m' })),
    el('div', { class: 'field-row' },
      field('Port sursă', input(l.from_port, (v) => set('Port', (o) => { o.from_port = v; }), { placeholder: 'ex. SW port 4' })),
      field('Port destinație', input(l.to_port, (v) => set('Port', (o) => { o.to_port = v; }), { placeholder: 'ex. LAN' }))),
    field('Status', select([{ value: 'confirmat', label: 'Confirmat' }, { value: 'inferat', label: 'Inferat (dedus, neverificat)' }], l.status, (v) => set('Status cablu', (o) => {
      o.status = v;
      if (v === 'inferat' && o.type === 'ethernet') o.type = 'inferat';
    }))),
    field('Observații', el('textarea', { rows: 2, onchange: (e) => set('Observații cablu', (o) => { o.notes = e.target.value; }) }, l.notes || '')),
    el('dl', { class: 'kv' },
      el('dt', {}, 'De la'), el('dd', {}, nodeName(l.from)),
      el('dt', {}, 'La'), el('dd', {}, nodeName(l.to)),
      el('dt', {}, 'Lungime'), el('dd', {}, lengthStr(len)),
      el('dt', {}, 'Colțuri'), el('dd', {}, String(l.points.length))),
    el('p', { class: 'muted small' }, 'Trage de pătrățele pentru a muta colțurile · Alt+click pe cablu = colț nou · selectează un colț + Del = elimină · capetele se pot muta pe alt port.'),
    el('div', { class: 'btn-row' },
      el('button', { onclick: () => set('Resetează traseu', (o) => { o.points = []; }) }, 'Traseu automat'),
      el('button', { onclick: () => set('Inversează', (o) => { [o.from, o.to] = [o.to, o.from]; o.points.reverse(); [o.from_port, o.to_port] = [o.to_port, o.from_port]; }) }, 'Inversează'),
      el('button', { class: 'danger-text', onclick: () => ed.deleteSelection() }, 'Șterge')));
}

function renderMulti(box, objs) {
  const counts = {};
  for (const o of objs) counts[o.type] = (counts[o.type] || 0) + 1;
  const names = { node: 'echipamente', link: 'cabluri', room: 'camere', wall: 'pereți', text: 'texte', door: 'uși/ferestre', survey: 'puncte survey' };
  const a = (k, label, title) => el('button', { title, onclick: () => ed.align(k) }, label);
  box.append(el('h3', {}, `${objs.length} elemente selectate`),
    el('p', { class: 'muted small' }, Object.entries(counts).map(([k, v]) => `${v} ${names[k]}`).join(', ')),
    el('div', { class: 'sec' }, 'Aliniere'),
    el('div', { class: 'align-grid' },
      a('left', '⇤ Stânga', 'Aliniază la stânga'), a('center', '↔ Centru', 'Centrează orizontal'), a('right', 'Dreapta ⇥', 'Aliniază la dreapta'), a('dist-h', '⇹ Distr. H', 'Distribuie orizontal (min. 3)'),
      a('top', '⤒ Sus', 'Aliniază sus'), a('middle', '↕ Mijloc', 'Centrează vertical'), a('bottom', '⤓ Jos', 'Aliniază jos'), a('dist-v', '⇳ Distr. V', 'Distribuie vertical (min. 3)')),
  );
  const nodes = objs.filter((o) => o.type === 'node');
  if (nodes.length) {
    box.append(el('div', { class: 'sec' }, 'Echipamente'),
      field('Culoare', el('input', { type: 'color', value: nodes[0].obj.color || '#1a73e8', onchange: (e) => ed.change('Culoare', () => { for (const n of nodes) n.obj.color = e.target.value; }) })));
  }
  const links = objs.filter((o) => o.type === 'link');
  if (links.length) {
    box.append(el('div', { class: 'sec' }, 'Cabluri'),
      field('Tip', select([{ value: '', label: '—' }, ...Object.entries(CABLE_TYPES).map(([k, v]) => ({ value: k, label: v.label }))], '', (v) => v && ed.change('Tip cablu', () => { for (const l of links) l.obj.type = v; }))));
  }
  box.append(el('div', { class: 'btn-row' },
    el('button', { onclick: () => ed.duplicate() }, 'Duplică (Ctrl+D)'),
    el('button', { class: 'danger-text', onclick: () => ed.deleteSelection() }, 'Șterge de pe hartă')));
}

function renderMapProps(box) {
  const s = ed.scene;
  box.append(el('h3', {}, 'Hartă: ' + (state.map?.name || '')));
  box.append(el('p', { class: 'muted small' }, 'Selectează un element ca să-i vezi proprietățile. Dublu-click = editare rapidă.'));

  box.append(el('div', { class: 'sec' }, 'Scară'),
    el('p', { class: 'small' }, s.scale.unitsPerMeter ? `1 m = ${s.scale.unitsPerMeter} unități (grilă ${s.grid.size} = ${(s.grid.size / s.scale.unitsPerMeter).toFixed(2)} m)` : 'Nesetată'),
    el('div', { class: 'btn-row' },
      el('button', { onclick: () => ed.setTool('scale') }, 'Trasează o distanță cunoscută'),
      s.scale.unitsPerMeter ? el('button', { onclick: () => ed.change('Scară', (sc) => { sc.scale.unitsPerMeter = 0; }) }, 'Elimină') : null));

  box.append(el('div', { class: 'sec' }, 'Plan de fundal'));
  if (state.map?.background) {
    const bg = s.background || {};
    box.append(
      field('Opacitate', el('input', { type: 'range', min: 0.05, max: 1, step: 0.05, value: bg.opacity ?? 0.5, oninput: (e) => { s.background.opacity = +e.target.value; ed.requestRender(); }, onchange: () => ed.setDirty(true) })),
      el('div', { class: 'field-row' },
        field('X', input(Math.round(bg.x || 0), (v) => ed.change('Fundal', () => { s.background.x = v; }), { type: 'number', disabled: layer(s, 'background').locked })),
        field('Y', input(Math.round(bg.y || 0), (v) => ed.change('Fundal', () => { s.background.y = v; }), { type: 'number', disabled: layer(s, 'background').locked })),
        field('Lățime', input(Math.round(bg.w || 0), (v) => ed.change('Fundal', () => { const r = s.background.h / s.background.w; s.background.w = v; s.background.h = v * r; }), { type: 'number', disabled: layer(s, 'background').locked }))),
      el('div', { class: 'btn-row' },
        el('button', { onclick: () => uploadBackground() }, 'Înlocuiește'),
        el('button', { class: 'danger-text', onclick: async () => {
          const { setBackground } = await import('./app.js');
          await setBackground('');
          ed.change('Elimină fundal', () => { delete s.background; });
          renderProps();
        } }, 'Elimină')));
  } else {
    box.append(el('p', { class: 'muted small' }, 'Încarcă planul locației (PNG/JPG) ca să trasezi pereții peste el.'),
      el('div', { class: 'btn-row' }, el('button', { onclick: () => uploadBackground() }, 'Încarcă plan…')));
  }

  box.append(el('div', { class: 'sec' }, 'Straturi'));
  for (const L of LAYERS) {
    const st = layer(s, L.id);
    box.append(el('div', { class: 'layer-row' }, L.label,
      el('button', { class: st.visible ? 'on' : '', title: 'Vizibil', onclick: () => { ed.change('Strat', () => { st.visible = !st.visible; }); renderProps(); } }, st.visible ? '👁 vizibil' : 'ascuns'),
      el('button', { class: st.locked ? 'on' : '', title: 'Blocat (nu se poate selecta/modifica)', onclick: () => { ed.change('Strat', () => { st.locked = !st.locked; }); renderProps(); } }, st.locked ? '🔒 blocat' : 'liber')));
  }

  box.append(el('div', { class: 'sec' }, 'Tipuri de cablu'),
    el('div', {}, Object.values(CABLE_TYPES).map((c) => el('div', { class: 'small', style: { display: 'flex', alignItems: 'center', gap: '8px', margin: '3px 0' } },
      el('span', { html: `<svg width="34" height="8"><line x1="1" y1="4" x2="33" y2="4" stroke="${c.color}" stroke-width="3" stroke-dasharray="${c.dash.join(' ')}"/></svg>` }), c.label))));

  box.append(el('div', { class: 'sec' }, 'Scurtături'),
    el('div', { class: 'small muted', html: 'V select · R cameră · W perete · C cablu · T text · D ușă · P survey · E radieră<br>M drept/ortogonal · Shift 0/45/90° · G grilă · S snap · F încadrează<br>Space+drag / click-mijloc = pan · rotița = zoom<br>Ctrl+Z/Y undo/redo · Ctrl+C/V/D · Del șterge · Ctrl+S salvează' }));
}

function uploadBackground() {
  const fi = $('#fileInput');
  fi.accept = 'image/png,image/jpeg,image/webp,image/svg+xml';
  fi.value = '';
  fi.onchange = async () => {
    const f = fi.files[0];
    if (!f) return;
    if (f.size > 12 * 1024 * 1024) { toast('Imaginea e prea mare (max 12 MB).', 'error'); return; }
    const uri = await new Promise((res) => { const r = new FileReader(); r.onload = () => res(r.result); r.readAsDataURL(f); });
    const img = new Image();
    await new Promise((res) => { img.onload = res; img.src = uri; });
    const { setBackground } = await import('./app.js');
    await setBackground(uri);
    const w = Math.min(img.naturalWidth, 2000);
    const h = (w * img.naturalHeight) / img.naturalWidth;
    ed.change('Plan de fundal', (s) => {
      if (!s.background) s.background = { x: 0, y: 0, w, h, opacity: 0.5 };
      else s.background.h = (s.background.w * img.naturalHeight) / img.naturalWidth;
    });
    ed.fitAll();
    renderProps();
    toast('Plan încărcat. Salvează harta (Ctrl+S) ca să-l păstrezi.', 'ok');
  };
  fi.click();
}
