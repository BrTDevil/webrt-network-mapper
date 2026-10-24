// Snap la grilă, la porturi, la capete/mijloace de segmente, constrângere unghiulară
// și ghidaje de aliniere.

import { portPos, PORTS, dist, layer } from './scene.js';

export function snapToGrid(p, size) {
  return [Math.round(p[0] / size) * size, Math.round(p[1] / size) * size];
}

// constrain45: fixează direcția a→p la 0/45/90°.
export function constrain45(a, p) {
  const dx = p[0] - a[0], dy = p[1] - a[1];
  const step = Math.PI / 4;
  const snapped = Math.round(Math.atan2(dy, dx) / step) * step;
  const ux = Math.round(Math.cos(snapped) * 1e9) / 1e9;
  const uy = Math.round(Math.sin(snapped) * 1e9) / 1e9;
  // Proiecția cursorului pe direcția aleasă.
  const l = (dx * ux + dy * uy) / (ux * ux + uy * uy);
  return [a[0] + ux * l, a[1] + uy * l];
}

// Puncte „magnetice”: porturi de noduri, capete și mijloace de pereți/cabluri.
export function collectSnapPoints(scene, opts = {}) {
  const pts = [];
  if (layer(scene, 'nodes').visible && opts.ports !== false) {
    for (const n of scene.nodes) {
      if (opts.excludeNodes && opts.excludeNodes.has(n.id)) continue;
      for (const s of PORTS) pts.push({ p: portPos(n, s).p, kind: 'port', node: n.id, port: s });
    }
  }
  if (layer(scene, 'walls').visible) {
    for (const w of scene.walls) {
      if (opts.excludeWall === w.id) continue;
      w.points.forEach((p, i) => {
        pts.push({ p, kind: 'end' });
        if (i > 0) pts.push({ p: [(p[0] + w.points[i - 1][0]) / 2, (p[1] + w.points[i - 1][1]) / 2], kind: 'mid' });
      });
    }
  }
  if (layer(scene, 'links').visible) {
    for (const l of scene.links) {
      if (opts.excludeLink === l.id) continue;
      const r = l.route || [];
      r.forEach((p, i) => {
        if (i === 0 || i === r.length - 1) pts.push({ p, kind: 'end' });
        if (i > 0) pts.push({ p: [(p[0] + r[i - 1][0]) / 2, (p[1] + r[i - 1][1]) / 2], kind: 'mid' });
      });
    }
  }
  return pts;
}

// snapPoint aplică, în ordine: puncte magnetice (rază în px pe ecran), grilă.
export function snapPoint(p, { scene, zoom, grid, snapOn, points, radiusPx = 10 }) {
  if (snapOn && points) {
    let best = null, bd = radiusPx / zoom;
    for (const sp of points) {
      const d = dist(sp.p, p);
      if (d < bd) { bd = d; best = sp; }
    }
    if (best) return { p: best.p.slice(), snap: best };
  }
  if (snapOn) return { p: snapToGrid(p, grid), snap: null };
  return { p: p.slice(), snap: null };
}

// alignGuides: linii de aliniere cu alte noduri / puncte (toleranță în px).
export function alignGuides(p, scene, { zoom, excludeNodes, extra = [] }) {
  const tol = 5 / zoom;
  const guides = [];
  let x = null, y = null;
  const cands = [];
  for (const n of scene.nodes) if (!excludeNodes || !excludeNodes.has(n.id)) cands.push([n.x, n.y]);
  for (const e of extra) cands.push(e);
  for (const c of cands) {
    if (x === null && Math.abs(c[0] - p[0]) < tol) { x = c[0]; guides.push({ axis: 'x', v: c[0], from: c, to: p }); }
    if (y === null && Math.abs(c[1] - p[1]) < tol) { y = c[1]; guides.push({ axis: 'y', v: c[1], from: c, to: p }); }
  }
  return { p: [x ?? p[0], y ?? p[1]], guides };
}
