// Client pentru API-ul local.

export class ApiError extends Error {
  constructor(status, message, data = null) {
    super(message);
    this.status = status;
    this.data = data; // corpul JSON al erorii (ex. { code, details })
  }
}

async function req(method, url, body, opts = {}) {
  const init = { method, headers: {} };
  if (body !== undefined) {
    if (body instanceof Blob || typeof body === 'string') {
      init.body = body;
      init.headers['Content-Type'] = 'application/json';
    } else {
      init.body = JSON.stringify(body);
      init.headers['Content-Type'] = 'application/json';
    }
  }
  let res;
  try {
    res = await fetch(url, init);
  } catch (e) {
    throw new ApiError(0, 'Serverul local nu răspunde. Rulează din nou „webrt-network-mapper web”.');
  }
  if (opts.raw) {
    if (!res.ok) throw new ApiError(res.status, await res.text());
    return res;
  }
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  if (!res.ok) throw new ApiError(res.status, (data && data.error) || text || res.statusText, data);
  return data;
}

const q = (params) => new URLSearchParams(params).toString();

export const api = {
  info: () => req('GET', '/api/info'),

  projects: () => req('GET', '/api/projects'),
  project: (id) => req('GET', `/api/projects/${id}`),
  createProject: (b) => req('POST', '/api/projects', b),
  updateProject: (id, b) => req('PATCH', `/api/projects/${id}`, b),
  deleteProject: (id) => req('DELETE', `/api/projects/${id}`),

  startScan: (pid, b) => req('POST', `/api/projects/${pid}/scan`, b),
  scans: (pid) => req('GET', `/api/projects/${pid}/scans`),
  scan: (id) => req('GET', `/api/scans/${id}`),
  cancelScan: (id) => req('POST', `/api/scans/${id}/cancel`),

  devices: (pid) => req('GET', `/api/projects/${pid}/devices`),
  createDevice: (pid, b) => req('POST', `/api/projects/${pid}/devices`, b),
  patchDevice: (id, b) => req('PATCH', `/api/devices/${id}`, b),
  deleteDevice: (id) => req('DELETE', `/api/devices/${id}`),

  wifi: (pid) => req('GET', `/api/projects/${pid}/wifi`),
  patchWifi: (id, b) => req('PATCH', `/api/wifi/${id}`, b),

  bluetooth: (pid) => req('GET', `/api/projects/${pid}/bluetooth`),
  patchBluetooth: (id, b) => req('PATCH', `/api/bluetooth/${id}`, b),
  deleteBluetooth: (id) => req('DELETE', `/api/bluetooth/${id}`),

  surveyPoints: (pid) => req('GET', `/api/projects/${pid}/survey-points`),
  createSurveyPoint: (pid, b) => req('POST', `/api/projects/${pid}/survey-points`, b),
  patchSurveyPoint: (id, b) => req('PATCH', `/api/survey-points/${id}`, b),
  deleteSurveyPoint: (id) => req('DELETE', `/api/survey-points/${id}`),

  maps: (pid) => req('GET', `/api/projects/${pid}/maps`),
  createMap: (pid, b) => req('POST', `/api/projects/${pid}/maps`, b),
  map: (id) => req('GET', `/api/maps/${id}`),
  saveMap: (id, b) => req('PUT', `/api/maps/${id}`, b),
  renameMap: (id, name) => req('PATCH', `/api/maps/${id}`, { name }),
  deleteMap: (id) => req('DELETE', `/api/maps/${id}`),

  exportURL: (pid, format, extra = {}) => `/api/projects/${pid}/export?${q({ format, ...extra })}`,
  importBackup: (text, name) => req('POST', `/api/import?${q({ name: name || '' })}`, text),
  renderSVG: (b) => req('POST', '/api/render/svg', b, { raw: true }).then((r) => r.text()),

  snapshotDir: () => req('GET', '/api/settings/snapshot-dir'),
  setSnapshotDir: (dir) => req('PUT', '/api/settings/snapshot-dir', { dir }),
  snapshots: () => req('GET', '/api/snapshots'),
  saveSnapshot: (pid, b) => req('POST', `/api/projects/${pid}/snapshots`, b),
  renameSnapshot: (path, name) => req('PATCH', `/api/snapshots?${q({ path })}`, { name }),
  deleteSnapshot: (path) => req('DELETE', `/api/snapshots?${q({ path })}`),
  snapshotFileURL: (path, download) => `/api/snapshots/file?${q({ path, download: download ? '1' : '0' })}`,
  snapshotReportURL: (path, print) => `/api/snapshots/report?${q({ path, print: print ? '1' : '0' })}`,
  restoreSnapshot: (path, name) => req('POST', '/api/snapshots/restore', { path, name }),
  openSnapshotFolder: () => req('POST', '/api/snapshots/open-folder'),
  compare: (a, b) => req('POST', '/api/compare', { a, b }),
};
