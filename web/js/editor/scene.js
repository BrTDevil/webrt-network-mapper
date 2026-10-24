// Modelul scenei: structura scene_json, rutarea cablurilor, geometrie și hit-testing.

export const SCHEMA = 1;

export const LAYERS = [
  { id: 'background', label: 'Fundal' },
  { id: 'walls', label: 'Pereți & camere' },
  { id: 'links', label: 'Cabluri' },
  { id: 'nodes', label: 'Echipamente' },
  { id: 'survey', label: 'Wi-Fi & survey' },
  { id: 'labels', label: 'Etichete' },
];

// Stratul fiecărui tip de element.
export const LAYER_OF = { room: 'walls', wall: 'walls', door: 'walls', link: 'links', node: 'nodes', survey: 'survey', text: 'labels' };

// Colecția din scenă pentru fiecare tip.
export const COLL = { room: 'rooms', wall: 'walls', door: 'doors', link: 'links', node: 'nodes', survey: 'surveyPoints', text: 'texts' };

export const CABLE_TYPES = {
  ethernet: { label: 'Ethernet', color: '#1a73e8', dash: [] },
  fibra: { label: 'Fibră', color: '#f57c00', dash: [] },
  wifi: { label: 'Wi-Fi / wireless', color: '#2e9d4f', dash: [2, 5] },
  poe: { label: 'Alimentare / PoE', color: '#d93025', dash: [9, 5] },
  bluetooth: { label: 'Bluetooth', color: '#7b1fa2', dash: [1, 5] },
  inferat: { label: 'Necunoscut / inferat', color: '#80868b', dash: [2, 5] },
};

export const ROOM_COLORS = ['#e8f0fe', '#e6f4ea', '#fef7e0', '#fce8e6', '#f3e8fd', '#e4f7fb', '#f1f3f4'];

export const NODE_SIZE = 48;

export function emptyScene() {
  return normalize({});
}

// normalize completează câmpurile lipsă (scene vechi sau importate).
export function normalize(s) {
  s = s || {};
  s.schema = SCHEMA;
  s.grid = Object.assign({ size: 20, snap: true, visible: true }, s.grid);
  s.scale = Object.assign({ unitsPerMeter: 0 }, s.scale);
  s.view = Object.assign({ zoom: 1, panX: 0, panY: 0 }, s.view);
  const layers = Array.isArray(s.layers) ? s.layers : [];
  s.layers = LAYERS.map((l) => Object.assign({ id: l.id, visible: true, locked: false }, layers.find((x) => x.id === l.id)));
  for (const k of ['rooms', 'walls', 'nodes', 'links', 'texts', 'doors', 'surveyPoints']) {
    if (!Array.isArray(s[k])) s[k] = [];
  }
  for (const n of s.nodes) {
    n.rotation = n.rotation || 0;
    n.scale = n.scale || 1;
  }
  for (const l of s.links) {
    l.points = l.points || [];
    l.mode = l.mode || 'orthogonal';
    l.type = l.type || 'ethernet';
    l.status = l.status || 'confirmat';
  }
  for (const w of s.walls) w.thickness = w.thickness || 8;
  return s;
}

let idCounter = 0;
export function newId(prefix) {
  idCounter = (idCounter + 1) % 1e6;
  return prefix + Date.now().toString(36) + idCounter.toString(36) + Math.random().toString(36).slice(2, 5);
}

export function findById(scene, type, id) {
  return scene[COLL[type]].find((o) => o.id === id);
}

export function layer(scene, id) {
  return scene.layers.find((l) => l.id === id) || { visible: true, locked: false };
}

// ---------- geometrie ----------

export const dist = (a, b) => Math.hypot(a[0] - b[0], a[1] - b[1]);

export function distToSegment(p, a, b) {
  const dx = b[0] - a[0], dy = b[1] - a[1];
  const l2 = dx * dx + dy * dy;
  if (!l2) return dist(p, a);
  let t = ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / l2;
  t = Math.max(0, Math.min(1, t));
  return dist(p, [a[0] + t * dx, a[1] + t * dy]);
}

export function projectOnSegment(p, a, b) {
  const dx = b[0] - a[0], dy = b[1] - a[1];
  const l2 = dx * dx + dy * dy;
  if (!l2) return { point: a.slice(), t: 0 };
  let t = ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / l2;
  t = Math.max(0, Math.min(1, t));
  return { point: [a[0] + t * dx, a[1] + t * dy], t };
}

export function polylineLength(pts) {
  let l = 0;
  for (let i = 1; i < pts.length; i++) l += dist(pts[i - 1], pts[i]);
  return l;
}

export function polylineMid(pts) {
  let half = polylineLength(pts) / 2;
  for (let i = 1; i < pts.length; i++) {
    const seg = dist(pts[i - 1], pts[i]);
    if (half <= seg && seg > 0) {
      const t = half / seg;
      return [pts[i - 1][0] + (pts[i][0] - pts[i - 1][0]) * t, pts[i - 1][1] + (pts[i][1] - pts[i - 1][1]) * t];
    }
    half -= seg;
  }
  return pts[0] || [0, 0];
}

export function rectContains(r, p) {
  return p[0] >= r.x && p[0] <= r.x + r.w && p[1] >= r.y && p[1] <= r.y + r.h;
}

export function rectsIntersect(a, b) {
  return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;
}

export function normRect(x1, y1, x2, y2) {
  return { x: Math.min(x1, x2), y: Math.min(y1, y2), w: Math.abs(x2 - x1), h: Math.abs(y2 - y1) };
}

// ---------- noduri și porturi ----------

export const PORTS = ['top', 'right', 'bottom', 'left'];
const PORT_VEC = { top: [0, -1], right: [1, 0], bottom: [0, 1], left: [-1, 0] };

export function nodeHalf(n) {
  return (NODE_SIZE * (n.scale || 1)) / 2;
}

export function nodeBox(n) {
  const h = nodeHalf(n);
  return { x: n.x - h, y: n.y - h, w: 2 * h, h: 2 * h };
}

// Portul „side” se rotește odată cu nodul (pași de 90°).
function rotatedPort(n, side) {
  const steps = Math.round(((n.rotation || 0) % 360 + 360) % 360 / 90);
  return PORTS[(PORTS.indexOf(side) + steps) % 4];
}

// portPos: poziția în lume și direcția ieșirii din port.
export function portPos(n, side) {
  const real = rotatedPort(n, side);
  const v = PORT_VEC[real];
  const h = nodeHalf(n) + 2;
  return { p: [n.x + v[0] * h, n.y + v[1] * h], dir: real === 'left' || real === 'right' ? 'h' : 'v', side: real };
}

// nearestPort alege portul nodului cel mai apropiat de un punct.
export function nearestPort(n, p) {
  let best = 'top', bd = Infinity;
  for (const s of PORTS) {
    const d = dist(portPos(n, s).p, p);
    if (d < bd) { bd = d; best = s; }
  }
  return best;
}

// ---------- rutarea cablurilor ----------

// endpoint rezolvă un capăt de cablu: legat de un port de nod sau punct liber.
export function endpoint(scene, end, nodeMap) {
  if (end && end.node) {
    const n = nodeMap ? nodeMap.get(end.node) : scene.nodes.find((x) => x.id === end.node);
    if (n) return portPos(n, end.port || 'top');
  }
  if (end && typeof end.x === 'number') return { p: [end.x, end.y], dir: null };
  return null;
}

// computeRoute calculează traseul complet al unui cablu.
// Modul ortogonal produce doar segmente orizontale/verticale; colțurile se
// calculează automat în L sau Z, ținând cont de direcția porturilor.
export function computeRoute(scene, link, nodeMap) {
  const a = endpoint(scene, link.from, nodeMap);
  const b = endpoint(scene, link.to, nodeMap);
  if (!a || !b) return [];
  const pts = [a.p, ...(link.points || []).map((p) => [p[0], p[1]]), b.p];
  if (link.mode !== 'orthogonal') return pts;
  return orthogonalize(pts, a.dir, b.dir);
}

export function orthogonalize(pts, startDir, endDir) {
  const out = [pts[0]];
  let prev = startDir; // orientarea ultimului segment: 'h' | 'v' | null
  for (let i = 0; i < pts.length - 1; i++) {
    const a = out[out.length - 1];
    const b = pts[i + 1];
    const last = i === pts.length - 2;
    if (Math.abs(a[0] - b[0]) < 0.01 || Math.abs(a[1] - b[1]) < 0.01) {
      out.push(b);
      prev = Math.abs(a[1] - b[1]) < 0.01 ? 'h' : 'v';
      continue;
    }
    let first; // orientarea primului segment dintre a și b
    if (last && endDir) {
      if (i === 0 && startDir && startDir === endDir) {
        // Z: ieșire și intrare pe aceeași orientare.
        if (startDir === 'h') {
          const mx = (a[0] + b[0]) / 2;
          out.push([mx, a[1]], [mx, b[1]], b);
        } else {
          const my = (a[1] + b[1]) / 2;
          out.push([a[0], my], [b[0], my], b);
        }
        prev = startDir;
        continue;
      }
      first = endDir === 'h' ? 'v' : 'h';
      if (i === 0 && startDir) first = startDir;
    } else if (i === 0 && startDir) {
      first = startDir;
    } else {
      first = prev === 'h' ? 'v' : 'h';
    }
    out.push(first === 'h' ? [b[0], a[1]] : [a[0], b[1]], b);
    prev = first === 'h' ? 'v' : 'h';
  }
  return simplify(out);
}

// simplify elimină punctele duplicate și cele coliniare.
export function simplify(pts) {
  const out = [];
  for (const p of pts) {
    const q = out[out.length - 1];
    if (q && Math.abs(q[0] - p[0]) < 0.01 && Math.abs(q[1] - p[1]) < 0.01) continue;
    out.push(p);
  }
  for (let i = out.length - 2; i > 0; i--) {
    const a = out[i - 1], b = out[i], c = out[i + 1];
    const cross = (b[0] - a[0]) * (c[1] - b[1]) - (b[1] - a[1]) * (c[0] - b[0]);
    const dot = (b[0] - a[0]) * (c[0] - b[0]) + (b[1] - a[1]) * (c[1] - b[1]);
    if (Math.abs(cross) < 0.01 && dot >= 0) out.splice(i, 1);
  }
  return out;
}

// updateRoutes recalculează și memorează traseele (câmpul route e folosit la export).
export function updateRoutes(scene) {
  const nodeMap = new Map(scene.nodes.map((n) => [n.id, n]));
  for (const l of scene.links) l.route = computeRoute(scene, l, nodeMap);
}

// ---------- limite ----------

export function sceneBounds(scene) {
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  const add = (x, y) => { minX = Math.min(minX, x); minY = Math.min(minY, y); maxX = Math.max(maxX, x); maxY = Math.max(maxY, y); };
  for (const r of scene.rooms) { add(r.x, r.y); add(r.x + r.w, r.y + r.h); }
  for (const w of scene.walls) for (const p of w.points) add(p[0], p[1]);
  for (const n of scene.nodes) { const h = nodeHalf(n); add(n.x - h, n.y - h); add(n.x + h, n.y + h + 28); }
  for (const l of scene.links) for (const p of l.route || []) add(p[0], p[1]);
  for (const t of scene.texts) { add(t.x, t.y - (t.size || 14)); add(t.x + (t.text || '').length * (t.size || 14) * 0.6, t.y); }
  for (const d of scene.doors) { add(d.x - (d.w || 40), d.y - (d.w || 40)); add(d.x + (d.w || 40), d.y + (d.w || 40)); }
  for (const s of scene.surveyPoints) { add(s.x - 10, s.y - 10); add(s.x + 10, s.y + 10); }
  if (scene.background && scene.background.w) { add(scene.background.x, scene.background.y); add(scene.background.x + scene.background.w, scene.background.y + scene.background.h); }
  if (minX === Infinity) return null;
  return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
}

// Bounding box pentru un element (folosit la selecție dreptunghiulară și aliniere).
export function itemBox(scene, type, o) {
  switch (type) {
    case 'node': return nodeBox(o);
    case 'room': return { x: o.x, y: o.y, w: o.w, h: o.h };
    case 'text': {
      const s = o.size || 14;
      return { x: o.x, y: o.y - s, w: Math.max(10, (o.text || '').length * s * 0.58), h: s * 1.25 };
    }
    case 'door': { const w = o.w || 40; return { x: o.x - w / 2, y: o.y - w / 2, w, h: w }; }
    case 'survey': return { x: o.x - 9, y: o.y - 9, w: 18, h: 18 };
    case 'wall':
    case 'link': {
      const pts = type === 'wall' ? o.points : (o.route || []);
      if (!pts.length) return { x: 0, y: 0, w: 0, h: 0 };
      const xs = pts.map((p) => p[0]), ys = pts.map((p) => p[1]);
      return normRect(Math.min(...xs), Math.min(...ys), Math.max(...xs), Math.max(...ys));
    }
  }
  return { x: 0, y: 0, w: 0, h: 0 };
}

// roomAt întoarce camera (cea mai mică) care conține punctul.
export function roomAt(scene, p) {
  let best = null;
  for (const r of scene.rooms) {
    if (rectContains(r, p) && (!best || r.w * r.h < best.w * best.h)) best = r;
  }
  return best;
}

// cloneScene: copie profundă (fără câmpuri de UI).
export function serialize(scene) {
  return JSON.stringify(scene);
}

// sceneContent: starea relevantă pentru undo (fără vedere).
export function contentOf(scene) {
  const { view, ...rest } = scene;
  return JSON.stringify(rest);
}
