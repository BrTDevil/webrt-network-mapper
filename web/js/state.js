// Starea comună a aplicației și un mic bus de evenimente.

export const state = {
  info: null,
  projects: [],
  project: null,       // {id, name, address, notes}
  lastScan: null,
  maps: [],
  map: null,           // {id, name, version, background}
  devices: [],
  devMap: new Map(),   // id → device
  wifi: [],
  bluetooth: [],
  btMap: new Map(),    // id → dispozitiv Bluetooth
  ssidFilter: '',
  survey: [],
  surveyMap: new Map(),
  editor: null,
  tab: 'map',
};

const listeners = new Map();

export function on(evt, fn) {
  if (!listeners.has(evt)) listeners.set(evt, new Set());
  listeners.get(evt).add(fn);
}

export function emit(evt, data) {
  for (const fn of listeners.get(evt) || []) {
    try { fn(data); } catch (e) { console.error(e); }
  }
}

export function setDevices(list) {
  state.devices = list;
  state.devMap = new Map(list.map((d) => [d.id, d]));
  emit('devices');
}

export function setWifi(list) {
  state.wifi = list;
  emit('wifi');
}

export function setBluetooth(list) {
  state.bluetooth = list;
  state.btMap = new Map(list.map((b) => [b.id, b]));
  emit('bluetooth');
}

export function btName(b) {
  if (!b) return '';
  return b.custom_name || b.name || b.mac;
}

export function btType(b) {
  return (b && (b.device_type || b.detected_type)) || 'bluetooth';
}

export function placedBtIds() {
  const ids = new Set();
  if (state.editor) for (const n of state.editor.scene.nodes) if (n.bt_id) ids.add(n.bt_id);
  return ids;
}

export function setSurvey(list) {
  state.survey = list;
  state.surveyMap = new Map(list.map((s) => [s.id, s]));
  emit('survey');
}

// placedDeviceIds: dispozitivele aflate pe harta curentă.
export function placedDeviceIds() {
  const ids = new Set();
  if (state.editor) for (const n of state.editor.scene.nodes) if (n.device_id) ids.add(n.device_id);
  return ids;
}

export function deviceName(d) {
  if (!d) return '';
  return d.custom_name || d.hostname || d.last_ip || d.mac;
}

export function deviceType(d) {
  return (d && (d.device_type || d.detected_type)) || 'necunoscut';
}

export const LS = {
  get(k, def) { try { const v = localStorage.getItem('netmap.' + k); return v === null ? def : JSON.parse(v); } catch { return def; } },
  set(k, v) { try { localStorage.setItem('netmap.' + k, JSON.stringify(v)); } catch {} },
};
