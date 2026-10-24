// Uneltele editorului. Fiecare unealtă primește evenimente în coordonate de lume.

import { newId, dist, normRect, rectsIntersect, itemBox, LAYER_OF, COLL, layer, nearestPort, nodeBox, portPos, projectOnSegment, ROOM_COLORS, computeRoute, orthogonalize } from './scene.js';
import { snapPoint, collectSnapPoints, constrain45, alignGuides, snapToGrid } from './snap.js';
import { moveObj } from './canvas.js';

// ---------- utilitare comune ----------

function snapOpts(ed, extra = {}) {
  return { scene: ed.scene, zoom: ed.view.zoom, grid: ed.scene.grid.size, snapOn: ed.scene.grid.snap, ...extra };
}

function locked(ed, type) {
  const L = layer(ed.scene, LAYER_OF[type]);
  if (L.locked || !L.visible) {
    ed.hooks.onMessage?.(`Stratul „${labelOfLayer(LAYER_OF[type])}” este ${L.locked ? 'blocat' : 'ascuns'}.`);
    return true;
  }
  return false;
}

function labelOfLayer(id) {
  return { background: 'Fundal', walls: 'Pereți & camere', links: 'Cabluri', nodes: 'Echipamente', survey: 'Wi-Fi & survey', labels: 'Etichete' }[id] || id;
}

function drawPolyline(ctx, pts, color, width, dash = []) {
  if (pts.length < 2) return;
  ctx.strokeStyle = color;
  ctx.lineWidth = width;
  ctx.lineJoin = 'round';
  ctx.lineCap = 'round';
  ctx.setLineDash(dash);
  ctx.beginPath();
  ctx.moveTo(...pts[0]);
  for (const p of pts.slice(1)) ctx.lineTo(...p);
  ctx.stroke();
  ctx.setLineDash([]);
}

// ---------- Select ----------

class SelectTool {
  constructor(ed) {
    this.ed = ed;
    this.cursor = 'default';
    this.drag = null;
  }

  down(p, e) {
    const ed = this.ed;
    // 1. mânere
    const h = ed.hitHandle(p);
    if (h) {
      ed.begin();
      if (h.kind === 'waypoint' || h.kind === 'wallpoint') ed.handle = { kind: h.kind, id: h.obj.id, index: h.index };
      this.drag = { kind: h.kind, h, start: p, orig: JSON.parse(JSON.stringify(h.obj)) };
      if (h.kind === 'rotate') {
        h.obj.rotation = ((h.obj.rotation || 0) + 90) % 360;
        ed.commit('Rotește');
        this.drag = null;
        ed.selectionChanged(false);
      }
      return;
    }
    // 2. Alt + click pe cablu = adaugă punct intermediar
    const hit = ed.hitTest(p);
    if (hit && hit.type === 'link' && e.altKey && ed.isSelected('link', hit.id)) {
      const l = hit.obj;
      ed.begin();
      insertWaypoint(ed, l, snapPoint(p, snapOpts(ed)).p);
      ed.commit('Adaugă punct');
      return;
    }
    if (hit) {
      if (e.shiftKey) {
        ed.select([{ type: hit.type, id: hit.id }], true);
        return;
      }
      if (!ed.isSelected(hit.type, hit.id)) ed.select([{ type: hit.type, id: hit.id }]);
      ed.handle = null;
      // pornește mutarea
      const objs = ed.selectedObjects().filter((o) => !layer(ed.scene, LAYER_OF[o.type]).locked);
      if (!objs.length) return;
      const nodeIds = new Set(objs.filter((o) => o.type === 'node').map((o) => o.id));
      // Cablurile dintre două noduri mutate își mută și punctele intermediare.
      const links = ed.scene.links.filter((l) => !objs.some((o) => o.type === 'link' && o.id === l.id) &&
        l.from.node && l.to.node && nodeIds.has(l.from.node) && nodeIds.has(l.to.node) && l.points.length);
      const ref = refPoint(hit.type, hit.obj);
      this.drag = {
        kind: 'move', start: p, ref, moved: false,
        items: objs.map((o) => ({ type: o.type, obj: o.obj, orig: JSON.parse(JSON.stringify(o.obj)) })),
        links: links.map((l) => ({ obj: l, orig: JSON.parse(JSON.stringify(l)) })),
        nodeIds,
      };
      ed.begin();
      return;
    }
    // 3. spațiu gol: selecție dreptunghiulară
    if (!e.shiftKey) ed.clearSelection();
    this.drag = { kind: 'marquee', start: p, additive: e.shiftKey };
  }

  move(p, e) {
    const ed = this.ed;
    const d = this.drag;
    if (!d) {
      const h = ed.hitHandle(p);
      if (h) ed.canvas.style.cursor = h.kind === 'rotate' ? 'alias' : h.kind === 'scale' ? 'nwse-resize' : 'move';
      else ed.canvas.style.cursor = ed.hitTest(p) ? 'move' : 'default';
      return;
    }
    const noSnap = e.altKey;
    const sp = (q) => (noSnap ? q : snapPoint(q, snapOpts(ed)).p);
    switch (d.kind) {
      case 'move': {
        let target = [d.ref[0] + p[0] - d.start[0], d.ref[1] + p[1] - d.start[1]];
        if (!noSnap && ed.scene.grid.snap) target = snapToGrid(target, ed.scene.grid.size);
        ed.guides = [];
        if (d.items.length === 1 && d.items[0].type === 'node') {
          const g = alignGuides(target, ed.scene, { zoom: ed.view.zoom, excludeNodes: d.nodeIds });
          target = g.p;
          ed.guides = g.guides;
        }
        const dx = target[0] - d.ref[0], dy = target[1] - d.ref[1];
        if (Math.abs(p[0] - d.start[0]) + Math.abs(p[1] - d.start[1]) > 2 / ed.view.zoom) d.moved = true;
        for (const it of d.items) {
          Object.assign(it.obj, JSON.parse(JSON.stringify(it.orig)));
          moveObj(it.type, it.obj, dx, dy);
        }
        for (const l of d.links) {
          l.obj.points = l.orig.points.map((q) => [q[0] + dx, q[1] + dy]);
        }
        break;
      }
      case 'marquee':
        ed.marquee = normRect(d.start[0], d.start[1], p[0], p[1]);
        break;
      case 'scale': {
        const o = d.h.obj;
        const half = Math.max(Math.abs(p[0] - o.x), Math.abs(p[1] - o.y));
        o.scale = Math.round(Math.min(3, Math.max(0.5, half / 24 - 0.3)) * 4) / 4;
        break;
      }
      case 'room-corner': {
        const o = d.h.obj, orig = d.orig;
        const q = sp(p);
        let x1 = orig.x, y1 = orig.y, x2 = orig.x + orig.w, y2 = orig.y + orig.h;
        if (d.h.corner.includes('w')) x1 = q[0]; else x2 = q[0];
        if (d.h.corner.includes('n')) y1 = q[1]; else y2 = q[1];
        Object.assign(o, normRect(x1, y1, x2, y2));
        break;
      }
      case 'wallpoint': {
        const w = d.h.obj;
        let q = snapPoint(p, snapOpts(ed, { points: noSnap ? null : collectSnapPoints(ed.scene, { excludeWall: w.id, ports: false }) })).p;
        const prev = w.points[d.h.index - 1] || w.points[d.h.index + 1];
        if (e.shiftKey && prev) q = constrain45(prev, q);
        w.points[d.h.index] = q;
        break;
      }
      case 'waypoint': {
        const l = d.h.obj;
        let q = sp(p);
        l.points[d.h.index] = q;
        break;
      }
      case 'link-end': {
        const l = d.h.obj;
        const port = ed.hitPort(p);
        if (port) l[d.h.end] = { node: port.node, port: port.port };
        else {
          const q = sp(p);
          l[d.h.end] = { x: q[0], y: q[1] };
        }
        break;
      }
    }
    ed.selectionChanged(false);
  }

  up(p, e) {
    const ed = this.ed;
    const d = this.drag;
    this.drag = null;
    ed.guides = [];
    if (!d) return;
    if (d.kind === 'marquee') {
      const m = ed.marquee;
      ed.marquee = null;
      if (!m || m.w * m.h < 4) return;
      const items = [];
      for (const [type, coll] of Object.entries(COLL)) {
        const L = layer(ed.scene, LAYER_OF[type]);
        if (!L.visible || L.locked) continue;
        // camerele se selectează doar dacă sunt cuprinse complet
        for (const o of ed.scene[coll]) {
          const b = itemBox(ed.scene, type, o);
          const inside = b.x >= m.x && b.y >= m.y && b.x + b.w <= m.x + m.w && b.y + b.h <= m.y + m.h;
          if (type === 'room' ? inside : rectsIntersect(b, m)) items.push({ type, id: o.id });
        }
      }
      ed.select(items, d.additive);
      return;
    }
    const labels = { move: 'Mută', scale: 'Redimensionează', 'room-corner': 'Redimensionează camera', wallpoint: 'Mută punct perete', waypoint: 'Mută punct cablu', 'link-end': 'Mută capăt cablu' };
    if (d.kind === 'move' && !d.moved) {
      ed._before = null;
      return;
    }
    ed.commit(labels[d.kind] || 'Modifică');
    if (d.kind === 'move') {
      ed.afterMove(d.items.filter((i) => i.type === 'node').map((i) => i.obj));
      const sps = d.items.filter((i) => i.type === 'survey').map((i) => i.obj);
      if (sps.length) ed.hooks.onSurveyMoved?.(sps);
    }
    ed.selectionChanged(false);
  }

  dblclick(p) {
    const hit = this.ed.hitTest(p);
    if (hit) {
      this.ed.select([{ type: hit.type, id: hit.id }]);
      this.ed.hooks.onEditProps?.(hit);
    }
  }

  cancel() {
    const ed = this.ed;
    if (this.drag && this.drag.kind === 'move') {
      for (const it of this.drag.items) Object.assign(it.obj, it.orig);
    }
    this.drag = null;
    ed.marquee = null;
  }
}

function refPoint(type, o) {
  switch (type) {
    case 'wall': return o.points[0].slice();
    case 'link': return (o.points[0] || o.route?.[0] || [0, 0]).slice();
    default: return [o.x, o.y];
  }
}

// insertWaypoint adaugă un punct intermediar pe segmentul cel mai apropiat.
function insertWaypoint(ed, l, p) {
  const r = l.route || [];
  const pts = l.points;
  // Găsește poziția în lista de puncte comparând distanța pe traseu.
  const along = (q) => {
    let acc = 0, best = Infinity, bestAcc = 0;
    for (let i = 1; i < r.length; i++) {
      const pr = projectOnSegment(q, r[i - 1], r[i]);
      const d = dist(q, pr.point);
      const segLen = dist(r[i - 1], r[i]);
      if (d < best) { best = d; bestAcc = acc + pr.t * segLen; }
      acc += segLen;
    }
    return bestAcc;
  };
  const tp = along(p);
  let idx = pts.length;
  for (let i = 0; i < pts.length; i++) {
    if (along(pts[i]) > tp) { idx = i; break; }
  }
  pts.splice(idx, 0, p);
}

// ---------- Plasare icon (click în paletă, apoi click pe hartă) ----------

class PlaceTool {
  constructor(ed, opts) {
    this.ed = ed;
    this.icon = opts.icon || 'necunoscut';
    this.cursor = 'copy';
  }
  move(p, e) {
    this.ed.ghost = { icon: this.icon, p: this.ed.snapPos(p, e.altKey) };
  }
  down(p, e) {
    this.ed.ghost = null;
    this.ed.addNode({ icon: this.icon }, this.ed.snapPos(p, e.altKey));
    if (!e.shiftKey) this.ed.setTool('select');
  }
  cancel() { this.ed.ghost = null; }
}

// ---------- Perete ----------

class WallTool {
  constructor(ed) {
    this.ed = ed;
    this.cursor = 'crosshair';
    this.points = [];
    this.cur = null;
    this.keepTransaction = false;
  }
  point(p, e) {
    const ed = this.ed;
    let q = snapPoint(p, snapOpts(ed, { points: e.altKey ? null : collectSnapPoints(ed.scene, { ports: false }) })).p;
    const last = this.points[this.points.length - 1];
    if (last && e.shiftKey) q = constrain45(last, q);
    return q;
  }
  down(p, e) {
    if (locked(this.ed, 'wall')) return;
    const q = this.point(p, e);
    const last = this.points[this.points.length - 1];
    if (last && dist(last, q) < 0.5) { this.finish(); return; }
    this.points.push(q);
  }
  move(p, e) { this.cur = this.point(p, e); }
  dblclick() { this.finish(); }
  key(e) {
    if (e.key === 'Enter') { this.finish(); return true; }
    if (e.key === 'Escape' && this.points.length) { this.cancel(); return true; }
    if (e.key === 'Backspace' && this.points.length) { this.points.pop(); return true; }
    return false;
  }
  finish() {
    const ed = this.ed;
    // dublu-click adaugă ultimul punct de două ori
    const pts = this.points.filter((p, i, a) => i === 0 || dist(p, a[i - 1]) > 0.5);
    this.points = [];
    if (pts.length < 2) return;
    ed.begin();
    const w = { id: newId('w'), points: pts, thickness: ed.wallThickness || 8 };
    ed.scene.walls.push(w);
    ed.commit('Adaugă perete');
    ed.select([{ type: 'wall', id: w.id }]);
  }
  cancel() { this.points = []; this.cur = null; }
  draw(ctx) {
    const ed = this.ed;
    const pts = this.cur ? [...this.points, this.cur] : this.points;
    drawPolyline(ctx, pts, ed.theme.wall, ed.wallThickness || 8);
    if (this.cur) {
      ctx.fillStyle = ed.theme.select;
      ctx.beginPath(); ctx.arc(this.cur[0], this.cur[1], 3 / ed.view.zoom, 0, Math.PI * 2); ctx.fill();
      showLength(ed, ctx, this.points[this.points.length - 1], this.cur);
    }
  }
}

function showLength(ed, ctx, a, b) {
  if (!a || !b) return;
  const upm = ed.scene.scale.unitsPerMeter;
  const l = dist(a, b);
  const txt = upm ? `${(l / upm).toFixed(2)} m` : `${Math.round(l)} u`;
  ed.haloText(txt, (a[0] + b[0]) / 2, (a[1] + b[1]) / 2 - 8 / ed.view.zoom, `${11 / ed.view.zoom}px ${ed.font()}`, ed.theme.select);
}

// ---------- Cameră ----------

class RoomTool {
  constructor(ed) {
    this.ed = ed;
    this.cursor = 'crosshair';
    this.start = null;
    this.rect = null;
  }
  down(p, e) {
    if (locked(this.ed, 'room')) return;
    this.start = snapPoint(p, snapOpts(this.ed, { snapOn: this.ed.scene.grid.snap && !e.altKey })).p;
  }
  move(p, e) {
    if (!this.start) return;
    const q = snapPoint(p, snapOpts(this.ed, { snapOn: this.ed.scene.grid.snap && !e.altKey })).p;
    this.rect = normRect(this.start[0], this.start[1], q[0], q[1]);
  }
  up() {
    const ed = this.ed, r = this.rect;
    this.start = null;
    this.rect = null;
    if (!r || r.w < 10 || r.h < 10) return;
    ed.begin();
    const room = { id: newId('r'), name: 'Cameră ' + (ed.scene.rooms.length + 1), ...r, color: ROOM_COLORS[ed.scene.rooms.length % ROOM_COLORS.length] };
    ed.scene.rooms.push(room);
    ed.commit('Adaugă cameră');
    ed.select([{ type: 'room', id: room.id }]);
    ed.setTool('select');
    ed.hooks.onEditProps?.({ type: 'room', id: room.id, obj: room });
  }
  cancel() { this.start = null; this.rect = null; }
  draw(ctx) {
    if (!this.rect) return;
    const r = this.rect, ed = this.ed;
    ctx.fillStyle = 'rgba(26,115,232,.12)';
    ctx.strokeStyle = ed.theme.select;
    ctx.lineWidth = 1.5 / ed.view.zoom;
    ctx.fillRect(r.x, r.y, r.w, r.h);
    ctx.strokeRect(r.x, r.y, r.w, r.h);
    const upm = ed.scene.scale.unitsPerMeter;
    const txt = upm ? `${(r.w / upm).toFixed(1)} × ${(r.h / upm).toFixed(1)} m` : `${Math.round(r.w)} × ${Math.round(r.h)}`;
    ed.haloText(txt, r.x + r.w / 2, r.y + r.h / 2, `${12 / ed.view.zoom}px ${ed.font()}`, ed.theme.select);
  }
}

// ---------- Cablu ----------

class CableTool {
  constructor(ed) {
    this.ed = ed;
    this.cursor = 'crosshair';
    this.showPorts = true;
    this.from = null;      // {node, port} | {x, y}
    this.points = [];
    this.cur = null;       // punct liber curent
    this.hoverPort = null;
  }

  resolve(p, e) {
    const ed = this.ed;
    const port = ed.hitPort(p, null);
    if (port) return { end: { node: port.node, port: port.port }, p: port.p };
    // click pe corpul unui nod = cel mai apropiat port
    const hit = ed.hitTest(p, { types: ['node'] });
    if (hit && this.from) {
      const side = nearestPort(hit.obj, this.lastPoint() || p);
      return { end: { node: hit.id, port: side }, p: portPos(hit.obj, side).p };
    }
    if (hit && !this.from) {
      const side = nearestPort(hit.obj, p);
      return { end: { node: hit.id, port: side }, p: portPos(hit.obj, side).p };
    }
    const pts = e.altKey ? null : collectSnapPoints(ed.scene, { ports: false });
    let q = snapPoint(p, snapOpts(ed, { points: pts, snapOn: ed.scene.grid.snap && !e.altKey })).p;
    const last = this.lastPoint();
    if (last && e.shiftKey) q = constrain45(last, q);
    return { end: null, p: q };
  }

  lastPoint() {
    if (this.points.length) return this.points[this.points.length - 1];
    if (!this.from) return null;
    if (this.from.node) {
      const n = this.ed.scene.nodes.find((x) => x.id === this.from.node);
      return n ? portPos(n, this.from.port).p : null;
    }
    return [this.from.x, this.from.y];
  }

  down(p, e) {
    if (locked(this.ed, 'link')) return;
    const r = this.resolve(p, e);
    if (!this.from) {
      this.from = r.end || { x: r.p[0], y: r.p[1] };
      return;
    }
    if (r.end) {
      if (this.from.node === r.end.node && this.from.port === r.end.port) return;
      this.finish(r.end);
      return;
    }
    const last = this.lastPoint();
    if (last && dist(last, r.p) < 0.5) { this.finish(); return; }
    this.points.push(r.p);
  }

  move(p, e) {
    const r = this.resolve(p, e);
    this.cur = r.p;
    this.curEnd = r.end;
  }

  dblclick() { this.finish(); }

  key(e) {
    if (e.key === 'Enter') { this.finish(); return true; }
    if (e.key === 'Escape' && this.from) { this.cancel(); return true; }
    if (e.key === 'Backspace' && this.points.length) { this.points.pop(); return true; }
    return false;
  }

  finish(toEnd) {
    const ed = this.ed;
    if (!this.from) return;
    let to = toEnd;
    let pts = this.points.slice();
    if (!to) {
      // Capăt liber: ultimul punct devine capătul.
      const last = pts.pop();
      if (!last) { this.cancel(); return; }
      to = { x: last[0], y: last[1] };
    }
    if (ed.linkMode === 'orthogonal') pts = pts.filter((q, i, a) => i === 0 || dist(q, a[i - 1]) > 0.5);
    const link = {
      id: newId('l'), from: this.from, to, points: pts, mode: ed.linkMode, type: ed.cableType || 'ethernet',
      label: '', status: 'confirmat',
    };
    this.from = null;
    this.points = [];
    ed.begin();
    ed.scene.links.push(link);
    ed.commit('Adaugă cablu');
    ed.select([{ type: 'link', id: link.id }]);
  }

  cancel() { this.from = null; this.points = []; this.cur = null; }

  draw(ctx) {
    const ed = this.ed;
    if (!this.from || !this.cur) return;
    const tmp = { from: this.from, to: this.curEnd || { x: this.cur[0], y: this.cur[1] }, points: this.points, mode: ed.linkMode };
    const route = computeRoute(ed.scene, tmp);
    const color = { ethernet: '#1a73e8', fibra: '#f57c00', wifi: '#2e9d4f', poe: '#d93025', inferat: '#80868b', bluetooth: '#7b1fa2' }[ed.cableType] || '#1a73e8';
    drawPolyline(ctx, route, color, 3, [6 / ed.view.zoom, 4 / ed.view.zoom]);
    ctx.fillStyle = color;
    for (const q of this.points) { ctx.beginPath(); ctx.arc(q[0], q[1], 3.5 / ed.view.zoom, 0, Math.PI * 2); ctx.fill(); }
    showLength(ed, ctx, this.lastPoint(), this.cur);
  }
}

// ---------- Text ----------

class TextTool {
  constructor(ed) { this.ed = ed; this.cursor = 'text'; }
  down(p, e) {
    const ed = this.ed;
    if (locked(ed, 'text')) return;
    const q = snapPoint(p, snapOpts(ed, { snapOn: ed.scene.grid.snap && !e.altKey })).p;
    ed.begin();
    const t = { id: newId('t'), x: q[0], y: q[1], text: 'Text', size: 14 };
    ed.scene.texts.push(t);
    ed.commit('Adaugă text');
    ed.select([{ type: 'text', id: t.id }]);
    ed.setTool('select');
    ed.hooks.onEditProps?.({ type: 'text', id: t.id, obj: t });
  }
}

// ---------- Ușă / fereastră ----------

class DoorTool {
  constructor(ed, opts) {
    this.ed = ed;
    this.kind = opts.kind || 'door';
    this.cursor = 'crosshair';
    this.preview = null;
  }
  place(p) {
    // Se lipește de cel mai apropiat perete, aliniată cu el.
    const ed = this.ed;
    let best = null, bd = 30 / ed.view.zoom;
    for (const w of ed.scene.walls) {
      for (let i = 1; i < w.points.length; i++) {
        const pr = projectOnSegment(p, w.points[i - 1], w.points[i]);
        const d = dist(p, pr.point);
        if (d < bd) {
          bd = d;
          const a = w.points[i - 1], b = w.points[i];
          best = { x: pr.point[0], y: pr.point[1], rotation: Math.round((Math.atan2(b[1] - a[1], b[0] - a[0]) * 180) / Math.PI) };
        }
      }
    }
    if (best) return best;
    const q = snapToGrid(p, ed.scene.grid.size);
    return { x: q[0], y: q[1], rotation: 0 };
  }
  move(p) { this.preview = this.place(p); }
  down(p, e) {
    const ed = this.ed;
    if (locked(ed, 'door')) return;
    const pos = this.place(p);
    ed.begin();
    const d = { id: newId('d'), kind: this.kind, w: this.kind === 'window' ? 60 : 40, ...pos };
    ed.scene.doors.push(d);
    ed.commit(this.kind === 'window' ? 'Adaugă fereastră' : 'Adaugă ușă');
    if (!e.shiftKey) {
      ed.setTool('select');
      ed.select([{ type: 'door', id: d.id }]);
    }
  }
  draw() {
    if (!this.preview) return;
    const ed = this.ed;
    ed.ctx.globalAlpha = 0.5;
    ed.drawDoor({ ...this.preview, kind: this.kind, w: this.kind === 'window' ? 60 : 40 });
    ed.ctx.globalAlpha = 1;
  }
  cancel() { this.preview = null; }
}

// ---------- Radieră ----------

class EraserTool {
  constructor(ed) { this.ed = ed; this.cursor = 'not-allowed'; }
  down(p) {
    const hit = this.ed.hitTest(p);
    if (!hit) return;
    this.ed.select([{ type: hit.type, id: hit.id }]);
    this.ed.deleteSelection();
  }
  move(p) {
    const hit = this.ed.hitTest(p);
    this.ed.eraseHover = hit;
  }
  draw(ctx) {
    const h = this.ed.eraseHover;
    if (!h) return;
    const b = itemBox(this.ed.scene, h.type, h.obj);
    ctx.strokeStyle = '#d93025';
    ctx.lineWidth = 2 / this.ed.view.zoom;
    ctx.strokeRect(b.x - 3, b.y - 3, b.w + 6, b.h + 6);
  }
  cancel() { this.ed.eraseHover = null; }
}

// ---------- Punct survey ----------

class SurveyTool {
  constructor(ed) { this.ed = ed; this.cursor = 'crosshair'; this.busy = false; }
  down(p, e) {
    const ed = this.ed;
    if (this.busy || locked(ed, 'survey')) return;
    const q = snapPoint(p, snapOpts(ed, { snapOn: ed.scene.grid.snap && !e.altKey })).p;
    this.busy = true;
    Promise.resolve(ed.hooks.onSurveyPoint?.(q)).finally(() => { this.busy = false; });
  }
  move(p) { this.cur = p; }
  draw(ctx) {
    if (!this.cur) return;
    ctx.strokeStyle = '#9334e6';
    ctx.lineWidth = 1.5 / this.ed.view.zoom;
    ctx.setLineDash([3, 3]);
    ctx.beginPath(); ctx.arc(this.cur[0], this.cur[1], 10, 0, Math.PI * 2); ctx.stroke();
    ctx.setLineDash([]);
  }
}

// ---------- Scară (linie pe o distanță cunoscută) ----------

class ScaleTool {
  constructor(ed) { this.ed = ed; this.cursor = 'crosshair'; this.a = null; this.b = null; }
  down(p, e) {
    const q = snapPoint(p, snapOpts(this.ed, { points: collectSnapPoints(this.ed.scene, {}), snapOn: !e.altKey })).p;
    if (!this.a) { this.a = q; return; }
    const a = this.a;
    let b = q;
    if (e.shiftKey) b = constrain45(a, b);
    this.a = null;
    this.b = null;
    const len = dist(a, b);
    if (len < 5) return;
    this.ed.hooks.onScale?.(len);
    this.ed.setTool('select');
  }
  move(p, e) {
    this.b = snapPoint(p, snapOpts(this.ed, { points: collectSnapPoints(this.ed.scene, {}), snapOn: !e.altKey })).p;
    if (this.a && e.shiftKey) this.b = constrain45(this.a, this.b);
  }
  draw(ctx) {
    if (!this.a || !this.b) return;
    drawPolyline(ctx, [this.a, this.b], '#e8388a', 2 / this.ed.view.zoom);
    showLength(this.ed, ctx, this.a, this.b);
  }
  cancel() { this.a = null; this.b = null; }
}

export const TOOLS = {
  select: SelectTool,
  place: PlaceTool,
  wall: WallTool,
  room: RoomTool,
  cable: CableTool,
  text: TextTool,
  door: DoorTool,
  window: class extends DoorTool { constructor(ed, o) { super(ed, { ...o, kind: 'window' }); } },
  eraser: EraserTool,
  survey: SurveyTool,
  scale: ScaleTool,
};

export { orthogonalize, nodeBox };
