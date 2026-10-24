// Bootstrap: proiecte, hărți, tab-uri, salvare, scanare, scurtături globale.

import { api, ApiError } from './api.js';
import { $, $$, el, toast, errorToast, modal, confirmDialog, formDialog, promptDialog, dropdown, fmtDate } from './ui.js';
import { state, emit, on, setDevices, setWifi, setSurvey, setBluetooth, LS } from './state.js';
import { loadIcons } from './editor/icons.js';
import { initMapView, refreshMapChrome } from './mapview.js';
import { initDevices } from './devices.js';
import { initWifi } from './wifi.js';
import { initSurvey } from './survey.js';
import { initSnapshots, quickSnapshot } from './snapshots.js';
import { initExport, exportMenu } from './export.js';
import { autoLayout } from './autolayout.js';
import { initBluetooth } from './bluetooth.js';

let saving = false;

async function main() {
  applyTheme(LS.get('theme', null));
  try {
    await loadIcons();
    state.info = await api.info();
  } catch (e) {
    errorToast(e);
  }
  initMapView({ saveMap, reloadInventory });
  initDevices({ reloadInventory, switchTab });
  initWifi({ reloadInventory, startScan });
  initBluetooth({ reloadInventory, startScan });
  initSurvey({ reloadInventory, switchTab });
  initSnapshots({ loadProject, reloadProjects, saveMap });
  initExport({ reloadProjects, loadProject });

  bindTopbar();
  bindKeys();
  await reloadProjects();
  const last = LS.get('project', null);
  const p = state.projects.find((x) => x.id === last) || state.projects[0];
  if (p) await loadProject(p.id);
  else await firstRun();

  // autosalvare la 30 s dacă există modificări
  setInterval(() => { if (state.editor?.dirty && !saving) saveMap({ auto: true }); }, 30000);
  window.addEventListener('beforeunload', (e) => {
    if (state.editor?.dirty) { e.preventDefault(); e.returnValue = ''; }
  });
  switchTab(LS.get('tab', 'map'));
}

async function firstRun() {
  const r = await formDialog('Bun venit! Creează primul proiect', [
    { name: 'name', label: 'Numele locației', value: 'Restaurant', required: true, placeholder: 'ex. Restaurant RP' },
    { name: 'address', label: 'Adresă (opțional)' },
  ], { ok: 'Creează', intro: 'Un proiect = o locație (restaurant, birou, magazin). Conține inventarul rețelei, rețelele Wi-Fi și una sau mai multe hărți (etaje / zone).' });
  const p = await api.createProject({ name: r?.name || 'Rețeaua mea', address: r?.address || '' });
  await reloadProjects();
  await loadProject(p.id);
}

// ---------- temă ----------

function applyTheme(t) {
  if (!t) t = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  document.documentElement.dataset.theme = t;
  state.editor?.readTheme();
}

// ---------- tab-uri ----------

export function switchTab(tab) {
  if (!$(`#tab-${tab}`)) tab = 'map';
  state.tab = tab;
  LS.set('tab', tab);
  for (const b of $$('.tabs button')) b.classList.toggle('active', b.dataset.tab === tab);
  for (const p of $$('.tab-panel')) p.classList.toggle('active', p.id === `tab-${tab}`);
  $('#mapGroup').style.visibility = tab === 'map' ? '' : 'hidden';
  emit('tab', tab);
  if (tab === 'map') {
    state.editor?.resize();
    setTimeout(() => $('#canvas').focus(), 0);
  }
}

// ---------- proiecte și hărți ----------

async function reloadProjects() {
  state.projects = await api.projects();
  const sel = $('#projectSel');
  sel.replaceChildren(...state.projects.map((p) => el('option', { value: p.id }, p.name)));
  if (state.project) sel.value = state.project.id;
}

async function loadProject(id, mapId) {
  if (state.editor?.dirty && state.project && state.project.id !== id) {
    await saveMap({ auto: true });
  }
  try {
    const { project, last_lan_scan, ssid_filter } = await api.project(id);
    state.project = project;
    state.lastScan = last_lan_scan;
    state.ssidFilter = ssid_filter || '';
    LS.set('project', id);
    $('#projectSel').value = id;
    await reloadInventory();
    await reloadMaps(mapId ?? LS.get('map.' + id, null));
    emit('project');
  } catch (e) {
    errorToast(e);
  }
}

async function reloadInventory() {
  if (!state.project) return;
  const pid = state.project.id;
  const [devices, wifi, survey, bt] = await Promise.all([api.devices(pid), api.wifi(pid), api.surveyPoints(pid), api.bluetooth(pid)]);
  setDevices(devices);
  setWifi(wifi);
  setSurvey(survey);
  setBluetooth(bt);
  $('#devBadge').textContent = devices.length || '';
  $('#wifiBadge').textContent = wifi.length || '';
  $('#btBadge').textContent = bt.filter((b) => b.name || b.custom_name).length || '';
  state.editor?.requestRender();
}

async function reloadMaps(preferId) {
  state.maps = await api.maps(state.project.id);
  const sel = $('#mapSel');
  sel.replaceChildren(...state.maps.map((m) => el('option', { value: m.id }, m.name)));
  const m = state.maps.find((x) => x.id === preferId) || state.maps[0];
  if (m) await openMap(m.id);
}

async function openMap(id) {
  const m = await api.map(id);
  state.map = { id: m.id, name: m.name, version: m.version, background: m.background || '' };
  LS.set('map.' + state.project.id, m.id);
  $('#mapSel').value = m.id;
  let scene;
  try { scene = JSON.parse(m.scene_json); } catch { scene = {}; }
  const hasView = scene.view && (scene.view.panX || scene.view.panY || scene.view.zoom !== 1);
  await setBackground(state.map.background);
  state.editor.load(scene);
  if (!hasView) state.editor.fitAll();
  refreshMapChrome();
  emit('map');
  // Singura hartă a proiectului e goală, dar inventarul există (ex. scan din CLI): o generăm din prima.
  // Un etaj / o zonă adăugată ulterior pornește goală — nu primește inventarul celorlalte hărți.
  if (state.maps.length === 1 && !state.editor.scene.nodes.length && !state.editor.scene.autoLayoutDone && state.devices.length) await runAutoLayout();
}

// devicesOnOtherMaps: dispozitivele deja plasate pe celelalte hărți ale proiectului.
// Un dispozitiv stă pe un singur etaj, deci aranjarea automată nu îl mai pune și aici.
async function devicesOnOtherMaps() {
  const others = state.maps.filter((m) => m.id !== state.map.id);
  const maps = await Promise.all(others.map((m) => api.map(m.id)));
  const ids = new Set();
  for (const m of maps) {
    let scene;
    try { scene = JSON.parse(m.scene_json); } catch { continue; }
    for (const n of scene.nodes || []) if (n.device_id) ids.add(n.device_id);
  }
  return ids;
}

// runAutoLayout pune pe hartă dispozitivele neplasate, legate de router, și salvează.
export async function runAutoLayout({ quiet = false } = {}) {
  const ed = state.editor;
  const wasEmpty = !ed.scene.nodes.length;
  let elsewhere;
  try {
    elsewhere = await devicesOnOtherMaps();
  } catch (e) {
    errorToast(e);
    return 0;
  }
  const n = autoLayout(ed, elsewhere);
  if (!n) {
    if (!quiet) toast(elsewhere.size
      ? `Toate dispozitivele sunt deja plasate (${elsewhere.size} pe alte hărți / etaje).`
      : 'Toate dispozitivele sunt deja pe hartă.');
    return 0;
  }
  ed.fitAll();
  emit('devices-edited');
  await saveMap({ auto: true });
  toast(wasEmpty
    ? `Harta a fost generată automat: ${n} dispozitive legate de router. Legăturile sunt deduse (gri/verde punctat = „inferat”) — mută echipamentele unde sunt fizic și schimbă tipul cablului când îl confirmi. Ctrl+Z anulează.`
    : `${n} dispozitive noi au fost plasate și legate de router.`, 'ok', 9000);
  return n;
}

export async function setBackground(uri) {
  state.map.background = uri || '';
  if (!uri) { state.editor.bgImage = null; return; }
  const img = new Image();
  await new Promise((res) => { img.onload = res; img.onerror = res; img.src = uri; });
  state.editor.bgImage = img;
  state.editor.requestRender();
}

// saveMap salvează scena curentă (cu optimistic locking pe versiune).
export async function saveMap({ auto = false, force = false } = {}) {
  const ed = state.editor;
  if (!state.map || saving) return;
  if (!ed.dirty && !force) { if (!auto) toast('Nimic de salvat.'); return; }
  saving = true;
  setSaveState('Se salvează…');
  try {
    const scene = JSON.stringify(ed.scene);
    const r = await api.saveMap(state.map.id, { scene_json: scene, version: state.map.version, background: state.map.background });
    state.map.version = r.version;
    ed.setDirty(false);
    setSaveState('Salvat ' + new Date().toLocaleTimeString('ro-RO', { hour: '2-digit', minute: '2-digit' }));
    if (!auto) toast('Harta a fost salvată.', 'ok');
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      saving = false;
      await versionConflict();
      return;
    }
    setSaveState('Eroare la salvare', true);
    errorToast(e);
  } finally {
    saving = false;
  }
}

async function versionConflict() {
  const choice = await modal('Harta a fost modificată în altă parte', (body, close, foot) => {
    body.append(el('p', {}, 'Harta a fost salvată între timp din alt tab sau altă fereastră. Ce vrei să faci?'));
    foot.append(
      el('button', { onclick: () => close('reload') }, 'Reîncarcă versiunea salvată'),
      el('button', { class: 'primary', onclick: () => close('overwrite') }, 'Suprascrie cu versiunea mea'),
    );
  });
  if (choice === 'reload') {
    await openMap(state.map.id);
  } else if (choice === 'overwrite') {
    const m = await api.map(state.map.id);
    state.map.version = m.version;
    await saveMap({ force: true });
  }
}

function setSaveState(txt, unsaved = false) {
  const s = $('#saveState');
  s.textContent = txt;
  s.classList.toggle('unsaved', unsaved);
}

on('dirty', (d) => {
  if (d) setSaveState('● Nesalvat', true);
});

// ---------- bara de sus ----------

function bindTopbar() {
  for (const b of $$('.tabs button')) b.addEventListener('click', () => switchTab(b.dataset.tab));
  $('#projectSel').addEventListener('change', (e) => loadProject(+e.target.value));
  $('#projectSel').addEventListener('dblclick', () => renameProject());
  $('#mapSel').addEventListener('change', async (e) => {
    if (state.editor.dirty) await saveMap({ auto: true });
    await openMap(+e.target.value);
  });
  $('#saveBtn').addEventListener('click', () => saveMap());
  $('#scanBtn').addEventListener('click', () => startScan());
  $('#snapBtn').addEventListener('click', () => quickSnapshot());
  $('#exportBtn').addEventListener('click', (e) => exportMenu(e.currentTarget));
  $('#themeBtn').addEventListener('click', () => {
    const t = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    LS.set('theme', t);
    applyTheme(t);
  });

  $('#projectMenuBtn').addEventListener('click', (e) => dropdown(e.currentTarget, [
    { label: 'Locație nouă (altă rețea, pornește de la zero)…', action: newProject },
    { label: 'Redenumește locația…', action: renameProject },
    { label: 'Editează locația (adresă, observații)…', action: editProject },
    { label: 'Importă backup / instantaneu…', action: () => emit('import') },
    '-',
    { label: 'Șterge locația (tot: inventar, Wi-Fi, hărți)…', danger: true, action: deleteProject },
  ]));
  $('#mapMenuBtn').addEventListener('click', (e) => dropdown(e.currentTarget, [
    { label: `Etaj / zonă nouă în „${state.project?.name || ''}”…`, action: newMap },
    { label: 'Redenumește harta…', action: renameMap },
    { label: 'Duplică harta…', action: duplicateMap },
    '-',
    { label: 'Șterge harta…', danger: true, disabled: state.maps.length < 2, action: deleteMap },
  ]));
}

// newProject creează o locație goală și o deschide; întoarce locația sau undefined.
async function newProject({ intro } = {}) {
  const r = await formDialog('Locație nouă', [
    { name: 'name', label: 'Numele locației', required: true, placeholder: 'ex. Restaurant RP' },
    { name: 'address', label: 'Adresă' },
    { name: 'notes', label: 'Observații', type: 'textarea' },
  ], { ok: 'Creează', intro: intro || 'O locație are inventarul, rețelele Wi-Fi și hărțile ei, separate de celelalte locații. Pornește goală.' });
  if (!r) return;
  try {
    if (state.editor.dirty) await saveMap({ auto: true });
    const p = await api.createProject(r);
    await reloadProjects();
    await loadProject(p.id);
    toast(`Locația „${p.name}” a fost creată.`, 'ok');
    return p;
  } catch (e) { errorToast(e); }
}

async function renameProject() {
  const p = state.project;
  const name = await promptDialog('Redenumește locația', 'Nume nou', p.name, { ok: 'Redenumește' });
  if (!name || name === p.name) return;
  try {
    state.project = await api.updateProject(p.id, { name, address: p.address, notes: p.notes });
    await reloadProjects();
    emit('project');
    toast(`Locația se numește acum „${name}”.`, 'ok');
  } catch (e) { errorToast(e); }
}

async function editProject() {
  const p = state.project;
  const r = await formDialog('Editează locația', [
    { name: 'name', label: 'Nume', value: p.name, required: true },
    { name: 'address', label: 'Adresă', value: p.address },
    { name: 'notes', label: 'Observații', type: 'textarea', value: p.notes },
  ]);
  if (!r) return;
  try {
    state.project = await api.updateProject(p.id, r);
    await reloadProjects();
    emit('project');
  } catch (e) { errorToast(e); }
}

async function deleteProject() {
  const p = state.project;
  const ok = await confirmDialog(`Locația „${p.name}” va fi ștearsă definitiv, cu tot inventarul, rețelele Wi-Fi, Bluetooth, punctele de survey și hărțile.\n\nInstantaneele salvate în folder NU sunt afectate.`,
    { title: 'Șterge locația', ok: 'Șterge definitiv', danger: true, requireText: p.name });
  if (!ok) return;
  try {
    await api.deleteProject(p.id);
    state.editor.setDirty(false);
    state.project = null;
    await reloadProjects();
    if (state.projects.length) await loadProject(state.projects[0].id);
    else await firstRun();
    toast('Locație ștearsă.');
  } catch (e) { errorToast(e); }
}

async function newMap() {
  const name = await promptDialog(`Etaj / zonă nouă în „${state.project.name}”`, 'Nume (etaj / zonă)', '', {
    placeholder: 'ex. Terasă, Etaj 1',
    intro: 'Aceeași locație și aceeași rețea, altă hartă. Pentru alt loc (altă rețea) folosește „Locație nouă” din meniul locației.',
  });
  if (!name) return;
  try {
    if (state.editor.dirty) await saveMap({ auto: true });
    const m = await api.createMap(state.project.id, { name });
    await reloadMaps(m.id);
  } catch (e) { errorToast(e); }
}

async function renameMap() {
  const name = await promptDialog('Redenumește harta', 'Nume', state.map.name);
  if (!name) return;
  try {
    await api.renameMap(state.map.id, name);
    state.map.name = name;
    await reloadMaps(state.map.id);
  } catch (e) { errorToast(e); }
}

async function duplicateMap() {
  const name = await promptDialog('Duplică harta', 'Numele copiei', state.map.name + ' (copie)');
  if (!name) return;
  try {
    const scene = JSON.parse(JSON.stringify(state.editor.scene));
    const m = await api.createMap(state.project.id, { name, scene_json: JSON.stringify(scene) });
    if (state.map.background) await api.saveMap(m.id, { scene_json: JSON.stringify(scene), version: m.version, background: state.map.background });
    await reloadMaps(m.id);
  } catch (e) { errorToast(e); }
}

async function deleteMap() {
  const ok = await confirmDialog(`Harta „${state.map.name}” va fi ștearsă. Dispozitivele din inventar nu sunt afectate.`, { title: 'Șterge harta', ok: 'Șterge', danger: true });
  if (!ok) return;
  try {
    await api.deleteMap(state.map.id);
    state.editor.setDirty(false);
    await reloadMaps();
  } catch (e) { errorToast(e); }
}

// ---------- scanare ----------

export async function startScan(opts = { lan: true, wifi: true, bluetooth: false }) {
  if (!state.project) return;
  try { state.info = await api.info(); } catch { /* păstrează info vechi */ }
  const consent = LS.get('consent', false);
  const net = state.info?.network;
  const cur = state.info?.wifi;
  // Rețelele Wi-Fi cunoscute (din scanările anterioare) pentru „scanează altă rețea”.
  const ssids = [...new Set(state.wifi.map((w) => w.ssid).filter(Boolean))].filter((x) => x !== cur?.ssid).sort();

  const result = await modal('Scanează', (body, close, foot) => {
    const chk = (name, label, value) => {
      const i = el('input', { type: 'checkbox', checked: !!value });
      return { i, row: el('label', { class: 'check' }, i, ' ', label) };
    };
    const lan = chk('lan', 'LAN — dispozitive din rețea (IP, MAC, producător, porturi)', opts.lan);
    const wf = chk('wifi', 'Wi-Fi — rețele vizibile (nume, canal, semnal)', opts.wifi);
    const bt = chk('bt', 'Bluetooth — dispozitive din apropiere (pasiv, ~12 s)', opts.bluetooth);

    // rețeaua pe care se face scanul LAN
    const netSel = el('select', {},
      el('option', { value: '' }, cur ? `Rețeaua curentă: ${cur.ssid}` : (net ? `Rețeaua curentă (${net.interface}, ${net.subnet})` : 'Rețeaua curentă')),
      ...ssids.map((x) => el('option', { value: x }, `Altă rețea: ${x}`)),
      el('option', { value: '__other' }, 'Altă rețea (scriu numele)…'));
    const otherName = el('input', { type: 'text', placeholder: 'Numele rețelei (SSID)', hidden: true });
    const pwd = el('input', { type: 'password', placeholder: 'Parola rețelei (lasă gol dacă laptopul o are salvată)', autocomplete: 'new-password', hidden: true });
    const pwdHint = el('div', { class: 'hint', hidden: true },
      'Laptopul se deconectează temporar de la rețeaua curentă, se conectează la cea aleasă cu parola ta, scanează, apoi revine singur. Parola e folosită doar pentru conectare: nu se salvează nicăieri și nu se ghicește nimic.');
    const syncNet = () => {
      const other = netSel.value !== '';
      otherName.hidden = netSel.value !== '__other';
      pwd.hidden = !other;
      pwdHint.hidden = !other;
    };
    netSel.addEventListener('change', syncNet);

    const filter = el('input', { type: 'text', value: state.ssidFilter, placeholder: 'ex. RP  (se potrivește cu RP, RP_5G, RP-Terasa)' });
    const iface = el('input', { type: 'text', placeholder: net ? `automat (${net.interface})` : 'automat' });
    const subnet = el('input', { type: 'text', placeholder: net ? `automat (${net.subnet})` : 'automat' });
    const cons = consent ? null : chk('consent', 'Confirm că administrez aceste rețele sau am acordul proprietarului.', false);

    body.append(el('div', { class: 'form' },
      cur || net ? el('p', { class: 'muted small pre' }, `Calculator: ${net ? `${net.ip} pe ${net.interface}` : '—'}${cur ? ` · conectat la Wi-Fi „${cur.ssid}”` : ''}${net?.gateway ? ` · gateway ${net.gateway}` : ''}`) : null,
      lan.row, wf.row, bt.row,
      el('label', {}, el('span', {}, 'Rețeaua pentru scanul LAN'), el('div', { class: 'net-choice' }, netSel, otherName, pwd, pwdHint)),
      el('label', {}, el('span', {}, 'Wi-Fi: doar rețelele mele (după nume)'), filter),
      el('div', { class: 'hint' }, 'Termeni separați prin virgulă. Gol = toate rețelele din jur (inclusiv ale vecinilor). Se reține pentru această locație.'),
      el('details', {}, el('summary', { class: 'muted small' }, 'Avansat'),
        el('label', {}, el('span', {}, 'Interfață'), iface),
        el('label', {}, el('span', {}, 'Subnet'), subnet)),
      cons ? cons.row : null,
      el('p', { class: 'muted small' }, 'Doar descoperire: ARP, ping, nume, porturi comune (TCP connect), anunțuri Wi-Fi/Bluetooth. Nu se încearcă autentificări pe dispozitive.'),
    ));
    const go = el('button', { class: 'primary', onclick: () => {
      if (cons && !cons.i.checked) { toast('Trebuie să confirmi că ai dreptul să scanezi rețeaua.', 'error'); return; }
      const target = netSel.value === '__other' ? otherName.value.trim() : netSel.value;
      if (netSel.value === '__other' && !target) { otherName.classList.add('invalid'); return; }
      const r = {
        lan: lan.i.checked, wifi: wf.i.checked, bluetooth: bt.i.checked,
        interface: iface.value.trim(), subnet: subnet.value.trim(), ssid_filter: filter.value.trim(),
        connect_ssid: target, password: target ? pwd.value : '',
      };
      pwd.value = '';
      close(r);
    } }, 'Pornește scanul');
    foot.append(el('button', { onclick: () => close() }, 'Renunță'), go);
  }, { wide: true });
  if (!result) return;
  LS.set('consent', true);
  if (!result.lan && !result.wifi && !result.bluetooth) return;
  if (result.connect_ssid && !result.lan) { toast('Bifează LAN ca să scanezi altă rețea.', 'error'); return; }
  state.ssidFilter = result.ssid_filter;
  await runScan(result);
}

async function runScan(body) {
  let ids;
  try {
    ({ scan_ids: ids } = await api.startScan(state.project.id, body));
  } catch (e) {
    if (e.data?.code === 'other_network') return otherNetwork(e.data.details, body);
    errorToast(e);
    return;
  }
  body = { ...body, password: '' }; // parola nu mai stă în memoria paginii

  let cancelled = false;
  let needsConfirm = null;
  await modal('Scanare în curs', (b, close, foot) => {
    const rows = ids.map((id) => {
      const bar = el('div');
      const msg = el('div', { class: 'muted small' }, 'Pornește…');
      const title = el('b', {}, '…');
      b.append(el('div', {}, title, el('div', { class: 'progress' }, bar), msg));
      return { id, bar, msg, title };
    });
    const cancelBtn = el('button', { onclick: async () => { cancelled = true; for (const id of ids) await api.cancelScan(id).catch(() => {}); } }, 'Oprește');
    const closeBtn = el('button', { class: 'primary', disabled: true, onclick: () => close() }, 'Închide');
    foot.append(cancelBtn, closeBtn);
    const tick = async () => {
      let allDone = true;
      for (const r of rows) {
        try {
          const s = await api.scan(r.id);
          r.title.textContent = s.type === 'lan' ? `LAN${s.subnet ? ' — ' + s.subnet : ''}` : s.type === 'bluetooth' ? 'Bluetooth' : 'Wi-Fi';
          r.bar.style.width = s.progress + '%';
          r.msg.textContent = s.message || (s.status === 'running' ? 'Se scanează…' : '');
          if (s.status === 'running') allDone = false;
          if (s.status === 'error') r.msg.classList.add('danger-text');
          if (s.status === 'needs_confirm') needsConfirm = s;
          if (s.status === 'done' && s.method) r.msg.textContent += s.method !== 'arp' ? ' — pentru ARP complet vezi README (setcap).' : '';
        } catch { /* ignoră */ }
      }
      if (allDone) {
        closeBtn.disabled = false;
        cancelBtn.disabled = true;
        await reloadInventory();
        const p = await api.project(state.project.id);
        state.lastScan = p.last_lan_scan;
        emit('scanned');
        if (needsConfirm) close();
        return;
      }
      setTimeout(tick, 700);
    };
    tick();
  });
  if (needsConfirm) {
    const ok = await confirmDialog(needsConfirm.message + '\n\nScanarea unui subnet mare durează mult și generează mult trafic.', { title: 'Subnet mare', ok: 'Scanează oricum' });
    if (ok) await runScan({ ...body, wifi: false, bluetooth: false, allow_large: true });
    return;
  }
  if (!cancelled) {
    const newCount = state.devices.filter((d) => d.new).length;
    toast(`Scan terminat: ${state.devices.filter((d) => d.online).length} dispozitive online${newCount ? `, ${newCount} noi` : ''}.`, 'ok');
    // Primul scan: harta se construiește singură. Scanările următoare adaugă doar dispozitivele noi.
    const hasDevicesOnMap = state.editor.scene.nodes.some((n) => n.device_id);
    if (body.lan && (!hasDevicesOnMap || newCount)) {
      switchTab('map');
      await runAutoLayout({ quiet: true });
    }
  }
}

// otherNetwork: routerul rețelei curente nu a mai fost văzut în locația deschisă.
// Nu amestecăm inventarele: utilizatorul alege unde merge scanul.
async function otherNetwork(d, body) {
  const match = d.matches?.[0];
  const choice = await modal('Altă rețea decât a locației', (b, close, foot) => {
    b.append(el('p', { class: 'pre' },
      `Laptopul e conectat la o rețea care nu a mai fost scanată în locația „${d.project.name}” ` +
      `(router ${d.gateway_ip}, ${d.gateway_mac}).\n\n` +
      (match
        ? `Rețeaua asta aparține locației „${match.name}”.`
        : 'Dacă ești în alt loc, creează o locație nouă: pornește de la zero, fără nimic din „' + d.project.name + '”.')));
    foot.append(el('button', { onclick: () => close() }, 'Renunță'));
    foot.append(el('button', { onclick: () => close('here'), title: 'Ex. al doilea router în aceeași clădire' }, `Scanează totuși în „${d.project.name}”`));
    if (match) foot.append(el('button', { class: 'primary', onclick: () => close('switch') }, `Deschide „${match.name}” și scanează`));
    foot.append(el('button', { class: match ? '' : 'primary', onclick: () => close('new') }, 'Locație nouă și scanează'));
  }, { wide: true });
  if (choice === 'here') return runScan({ ...body, allow_other_network: true });
  if (choice === 'switch') {
    await loadProject(match.id);
    return runScan(body);
  }
  if (choice === 'new') {
    const p = await newProject({ intro: 'Scanul pornește imediat după creare, în locația nouă.' });
    if (p) return runScan(body);
  }
}

// ---------- tastatură ----------

function bindKeys() {
  document.addEventListener('keydown', (e) => {
    const ctrl = e.ctrlKey || e.metaKey;
    if (ctrl && e.key.toLowerCase() === 's') {
      e.preventDefault();
      saveMap();
      return;
    }
    if (document.querySelector('.modal-back')) return;
    const t = e.target;
    const typing = t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable);
    if (typing) {
      if (e.key === 'Escape') t.blur();
      return;
    }
    if (state.tab !== 'map') return;
    if (e.key === ' ') {
      if (!state.editor.spaceDown) {
        state.editor.spaceDown = true;
        $('#canvas').style.cursor = 'grab';
      }
      e.preventDefault();
      return;
    }
    if (state.editor.key(e)) e.preventDefault();
  });
  document.addEventListener('keyup', (e) => {
    if (e.key === ' ' && state.editor) {
      state.editor.spaceDown = false;
      $('#canvas').style.cursor = state.editor.tool?.cursor || 'default';
    }
  });
}

main();
