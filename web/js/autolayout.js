// Generarea automată a hărții din inventar: Internet → gateway → coloane pe categorii,
// fiecare dispozitiv legat de gateway. Legăturile sunt deduse, deci marcate „inferat”.

import { state, deviceType } from './state.js';
import { iconForType } from './editor/icons.js';
import { newId, sceneBounds } from './editor/scene.js';

// Categorii (ordinea coloanelor de la stânga la dreapta).
const GROUPS = [
  { id: 'net', label: 'Rețea / AP-uri', types: ['router', 'router_ap', 'ap', 'switch', 'modem', 'firewall', 'patch_panel', 'rack'] },
  { id: 'srv', label: 'Stații & servere', types: ['server', 'nas', 'pc', 'pc_bd', 'terminal', 'monitor'] },
  { id: 'pos', label: 'POS / casă', types: ['casa_de_marcat', 'terminal_pos', 'imprimanta_bonuri', 'imprimanta_bucatarie', 'kiosk', 'scanner_coduri', 'cantar'] },
  { id: 'per', label: 'Periferice', types: ['imprimanta', 'camera', 'camera_dome', 'nvr', 'telefon_ip', 'boxa', 'tv', 'ups'] },
  { id: 'mob', label: 'Mobile & Wi-Fi', types: ['telefon', 'tableta', 'tableta_ospatar', 'terminal_ospatar', 'laptop'] },
  { id: 'unk', label: 'Neidentificate', types: [] },
];

// Tipuri conectate de obicei prin Wi-Fi.
const WIRELESS = new Set(['telefon', 'tableta', 'tableta_ospatar', 'terminal_ospatar', 'laptop']);

const COL_W = 240;     // lățimea unei coloane
const ROW_H = 120;     // distanța verticală între dispozitive (multiplu de grilă)
const MAX_ROWS = 4;    // după atâtea dispozitive se începe o coloană nouă

function isWireless(d) {
  const t = deviceType(d);
  if (WIRELESS.has(t)) return true;
  if ((d.vendor || '').startsWith('MAC privat')) return true; // telefoane cu MAC aleator
  return /espressif|tuya|shelly|sonoff/i.test(d.vendor || '');
}

function groupOf(d) {
  const t = deviceType(d);
  return GROUPS.find((g) => g.types.includes(t)) || GROUPS[GROUPS.length - 1];
}

function isGateway(d) {
  const gw = state.lastScan?.gateway || state.info?.network?.gateway;
  return gw && d.last_ip === gw;
}

// autoLayout plasează dispozitivele neplasate și le leagă de gateway.
// Pe o hartă goală construiește tot (Internet + gateway + coloane).
// elsewhere = dispozitivele deja plasate pe alte hărți; nu sunt puse și pe aceasta.
// Întoarce numărul de dispozitive plasate.
export function autoLayout(ed, elsewhere = new Set()) {
  const s = ed.scene;
  const placed = new Set(elsewhere);
  for (const n of s.nodes) if (n.device_id) placed.add(n.device_id);
  const todo = state.devices.filter((d) => !placed.has(d.id));
  if (!todo.length) return 0;

  const g = s.grid.size || 20;
  const snap = (v) => Math.round(v / g) * g;
  // BSSID-urile emise de fiecare dispozitiv (asocieri Wi-Fi inferate/confirmate).
  const bssids = new Map();
  for (const w of state.wifi) {
    if (w.ap_device_id && w.ap_link_status !== 'respins') {
      if (!bssids.has(w.ap_device_id)) bssids.set(w.ap_device_id, []);
      bssids.get(w.ap_device_id).push(w.bssid);
    }
  }

  ed.begin();

  // 1. Gateway: existent pe hartă sau nou.
  let gwNode = s.nodes.find((n) => n.device_id && isGateway(state.devMap.get(n.device_id) || {}));
  const gwDev = todo.find(isGateway);
  let originX, originY;
  const empty = !s.nodes.length;
  if (gwNode) {
    originX = gwNode.x;
    originY = gwNode.y;
  } else {
    const b = empty ? null : sceneBounds(s);
    originX = b ? snap(b.x + b.w + 200) : 0;
    originY = b ? snap(b.y + 180) : 0;
    if (gwDev) {
      gwNode = node(gwDev, originX, originY, 'router', bssids);
      s.nodes.push(gwNode);
    }
  }
  // Internet deasupra gateway-ului (doar dacă nu există deja).
  if (gwNode && !s.nodes.some((n) => n.icon === 'internet')) {
    const inet = { id: newId('n'), icon: 'internet', x: gwNode.x, y: snap(gwNode.y - 160), rotation: 0, scale: 1, label: 'Internet / ISP' };
    s.nodes.push(inet);
    s.links.push(link(inet.id, 'bottom', gwNode.id, 'top', [], 'inferat'));
  }

  // 2. Restul, pe coloane de categorii.
  const rest = todo.filter((d) => d !== gwDev);
  const byGroup = GROUPS.map((gr) => ({ gr, list: rest.filter((d) => groupOf(d) === gr) }))
    .filter((x) => x.list.length);
  const columns = [];
  for (const { gr, list } of byGroup) {
    list.sort((a, b) => ipNum(a.last_ip) - ipNum(b.last_ip));
    for (let i = 0; i < list.length; i += MAX_ROWS) {
      columns.push({ label: i === 0 ? gr.label : '', list: list.slice(i, i + MAX_ROWS) });
    }
  }
  const total = columns.length * COL_W;
  const startX = gwNode && empty ? snap(originX - total / 2 + COL_W / 2) : originX;
  const busY = snap((gwNode ? gwNode.y : originY) + 120);

  columns.forEach((col, ci) => {
    const gutter = snap(startX + ci * COL_W - 70);
    if (col.label) s.texts.push({ id: newId('t'), x: gutter + 8, y: busY + 44, text: col.label, size: 13, bold: true, color: '#5f6368' });
    col.list.forEach((d, ri) => {
      const x = snap(startX + ci * COL_W);
      const y = snap(busY + 110 + ri * ROW_H);
      const n = node(d, x, y, null, bssids);
      s.nodes.push(n);
      if (gwNode) {
        // Traseu în „pieptene”: jos din gateway → magistrală orizontală → pe marginea coloanei → în portul stâng.
        s.links.push(link(gwNode.id, 'bottom', n.id, 'left', [[gutter, busY], [gutter, y]], isWireless(d) ? 'wifi' : 'inferat'));
      }
    });
  });

  s.autoLayoutDone = true;
  ed.commit('Aranjare automată');
  return todo.length;
}

function node(d, x, y, forceIcon, bssids) {
  let icon = forceIcon || iconForType(deviceType(d));
  // Un router care nu e gateway, dar emite Wi-Fi = router folosit ca amplificator.
  if (!forceIcon && icon === 'router' && !isGateway(d)) icon = 'router_ap';
  const n = { id: newId('n'), icon, x, y, rotation: 0, scale: 1, label: '', device_id: d.id };
  const b = bssids.get(d.id);
  if (b?.length) n.wifi_bssids = b;
  return n;
}

function link(fromNode, fromPort, toNode, toPort, points, type) {
  return {
    id: newId('l'), from: { node: fromNode, port: fromPort }, to: { node: toNode, port: toPort },
    points, mode: 'orthogonal', type, label: '', status: 'inferat',
  };
}

function ipNum(ip) {
  const p = (ip || '').split('.').map(Number);
  return p.length === 4 ? ((p[0] * 256 + p[1]) * 256 + p[2]) * 256 + p[3] : Infinity;
}

