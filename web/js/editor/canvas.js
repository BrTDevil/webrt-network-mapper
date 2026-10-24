// Editorul de hartă: randare Canvas 2D, zoom/pan, selecție, istoric, clipboard.

import {
  normalize, emptyScene, updateRoutes, sceneBounds, itemBox, nodeBox, nodeHalf, portPos, PORTS,
  CABLE_TYPES, LAYER_OF, COLL, layer, dist, distToSegment, rectContains, rectsIntersect, normRect,
  polylineMid, polylineLength, contentOf, newId, roomAt, findById,
} from './scene.js';
import { iconImage, iconLabel, setIconReadyCallback } from './icons.js';
import { History } from './history.js';
import { TOOLS } from './tools.js';

const MIN_ZOOM = 0.1, MAX_ZOOM = 4;

export class Editor {
  constructor(canvas, minimap, hooks = {}) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.minimap = minimap;
    this.hooks = hooks;
    this.scene = emptyScene();
    this.selection = [];          // [{type, id}]
    this.handle = null;           // sub-selecție: {kind:'waypoint'|'wallpoint', id, index}
    this.history = new History(200);
    this.tool = null;
    this.toolName = 'select';
    this.linkMode = 'orthogonal';
    this.cableType = 'ethernet';
    this.hover = null;
    this.mouse = [0, 0];          // lume
    this.mouseScreen = [0, 0];
    this.guides = [];
    this.ghost = null;            // previzualizare drag & drop: {icon, p}
    this.overlay = null;          // desen temporar al uneltei curente
    this.spaceDown = false;
    this.panning = null;
    this.dirty = false;
    this.showIP = true;
    this.bgImage = null;
    this.heatmap = null;          // {canvas, x, y, w, h}
    this.apEstimates = [];        // [{x, y, label}]
    this.theme = {};
    this.clipboard = null;
    this._raf = 0;
    this._before = null;

    setIconReadyCallback(() => this.requestRender());
    this.history.onChange = () => this.hooks.onHistory?.();
    this.readTheme();
    this.bindEvents();
    this.resize();
    new ResizeObserver(() => this.resize()).observe(canvas.parentElement);
    this.setTool('select');
  }

  // ---------- încărcare / stare ----------

  load(sceneObj, { keepView = false } = {}) {
    const view = this.scene.view;
    this.scene = normalize(sceneObj);
    if (keepView) this.scene.view = view;
    updateRoutes(this.scene);
    this.selection = [];
    this.handle = null;
    this.history.clear();
    this.setDirty(false);
    this.setTool('select');
    this.selectionChanged();
    this.requestRender();
  }

  get view() { return this.scene.view; }

  setDirty(v) {
    this.dirty = v;
    this.hooks.onDirty?.(v);
  }

  // begin/commit înconjoară orice modificare: ce e între ele devine un pas de undo.
  begin() {
    if (this._before === null) this._before = contentOf(this.scene);
  }

  commit(label) {
    if (this._before === null) return;
    updateRoutes(this.scene);
    const after = contentOf(this.scene);
    const before = this._before;
    this._before = null;
    if (before !== after) {
      this.history.push(label, before, after);
      this.setDirty(true);
      this.hooks.onChange?.();
    }
    this.requestRender();
  }

  // change: modificare atomică (folosită de panoul de proprietăți).
  change(label, fn) {
    this.begin();
    fn(this.scene);
    this.commit(label);
    this.selectionChanged(false);
  }

  restore(content) {
    const view = this.scene.view;
    this.scene = normalize(JSON.parse(content));
    this.scene.view = view;
    updateRoutes(this.scene);
    this.selection = this.selection.filter((s) => findById(this.scene, s.type, s.id));
    this.handle = null;
    this.setDirty(true);
    this.selectionChanged();
    this.hooks.onChange?.();
    this.requestRender();
  }

  undo() {
    this.cancelTool();
    const c = this.history.undo();
    if (c !== null) this.restore(c);
  }

  redo() {
    this.cancelTool();
    const c = this.history.redo();
    if (c !== null) this.restore(c);
  }

  // ---------- unelte ----------

  setTool(name, opts = {}) {
    this.cancelTool();
    const T = TOOLS[name] || TOOLS.select;
    this.toolName = name;
    this.tool = new T(this, opts);
    this.canvas.style.cursor = this.tool.cursor || 'default';
    this.hooks.onTool?.(name);
    this.requestRender();
  }

  cancelTool() {
    if (this.tool && this.tool.cancel) this.tool.cancel();
    this.overlay = null;
    this.guides = [];
    if (this._before !== null && this.tool && !this.tool.keepTransaction) this._before = null;
  }

  // ---------- coordonate și vedere ----------

  toWorld(sx, sy) {
    const v = this.view;
    return [(sx - v.panX) / v.zoom, (sy - v.panY) / v.zoom];
  }

  toScreen(x, y) {
    const v = this.view;
    return [x * v.zoom + v.panX, y * v.zoom + v.panY];
  }

  eventPoint(e) {
    const r = this.canvas.getBoundingClientRect();
    return [e.clientX - r.left, e.clientY - r.top];
  }

  zoomAt(sx, sy, factor) {
    const v = this.view;
    const z = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, v.zoom * factor));
    const [wx, wy] = this.toWorld(sx, sy);
    v.zoom = z;
    v.panX = sx - wx * z;
    v.panY = sy - wy * z;
    this.hooks.onView?.();
    this.requestRender();
  }

  setZoom(z) {
    this.zoomAt(this.cssW / 2, this.cssH / 2, z / this.view.zoom);
  }

  fitAll() {
    const b = sceneBounds(this.scene);
    if (!b) {
      this.view.zoom = 1;
      this.view.panX = 40;
      this.view.panY = 40;
    } else {
      const pad = 60;
      const z = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Math.min((this.cssW - 2 * pad) / Math.max(b.w, 1), (this.cssH - 2 * pad) / Math.max(b.h, 1))));
      this.view.zoom = Math.min(z, 2);
      this.view.panX = this.cssW / 2 - (b.x + b.w / 2) * this.view.zoom;
      this.view.panY = this.cssH / 2 - (b.y + b.h / 2) * this.view.zoom;
    }
    this.hooks.onView?.();
    this.requestRender();
  }

  centerOn(p, zoom) {
    if (zoom) this.view.zoom = zoom;
    this.view.panX = this.cssW / 2 - p[0] * this.view.zoom;
    this.view.panY = this.cssH / 2 - p[1] * this.view.zoom;
    this.hooks.onView?.();
    this.requestRender();
  }

  resize() {
    const parent = this.canvas.parentElement;
    const dpr = window.devicePixelRatio || 1;
    this.cssW = parent.clientWidth;
    this.cssH = parent.clientHeight;
    this.dpr = dpr;
    this.canvas.width = Math.max(1, Math.round(this.cssW * dpr));
    this.canvas.height = Math.max(1, Math.round(this.cssH * dpr));
    this.canvas.style.width = this.cssW + 'px';
    this.canvas.style.height = this.cssH + 'px';
    this.requestRender();
  }

  readTheme() {
    const cs = getComputedStyle(document.documentElement);
    const v = (n, d) => (cs.getPropertyValue(n).trim() || d);
    this.theme = {
      bg: v('--canvas-bg', '#fafbfc'),
      grid: v('--canvas-grid', '#e8eaed'),
      gridMajor: v('--canvas-grid-major', '#d2d6db'),
      wall: v('--canvas-wall', '#3c4043'),
      text: v('--canvas-text', '#202124'),
      muted: v('--canvas-muted', '#5f6368'),
      halo: v('--canvas-halo', '#ffffff'),
      accent: v('--accent', '#1a73e8'),
      select: v('--canvas-select', '#1a73e8'),
      guide: v('--canvas-guide', '#e8388a'),
    };
    this.requestRender();
  }

  // ---------- selecție ----------

  isSelected(type, id) {
    return this.selection.some((s) => s.type === type && s.id === id);
  }

  select(items, additive = false) {
    if (!additive) this.selection = [];
    for (const it of items) {
      const i = this.selection.findIndex((s) => s.type === it.type && s.id === it.id);
      if (i >= 0) { if (additive) this.selection.splice(i, 1); } else this.selection.push(it);
    }
    this.handle = null;
    this.selectionChanged();
  }

  clearSelection() {
    if (!this.selection.length && !this.handle) return;
    this.selection = [];
    this.handle = null;
    this.selectionChanged();
  }

  selectionChanged(notify = true) {
    this.requestRender();
    if (notify) this.hooks.onSelection?.(this.selectedObjects());
    else this.hooks.onSelectionSoft?.(this.selectedObjects());
  }

  selectedObjects() {
    return this.selection.map((s) => ({ ...s, obj: findById(this.scene, s.type, s.id) })).filter((s) => s.obj);
  }

  selectAll() {
    const items = [];
    for (const [type, coll] of Object.entries(COLL)) {
      const L = layer(this.scene, LAYER_OF[type]);
      if (!L.visible || L.locked) continue;
      for (const o of this.scene[coll]) items.push({ type, id: o.id });
    }
    this.select(items);
  }

  deleteSelection() {
    if (this.handle) {
      const h = this.handle;
      this.change('Șterge punct', (s) => {
        if (h.kind === 'waypoint') {
          const l = s.links.find((x) => x.id === h.id);
          if (l) l.points.splice(h.index, 1);
        } else if (h.kind === 'wallpoint') {
          const w = s.walls.find((x) => x.id === h.id);
          if (w && w.points.length > 2) w.points.splice(h.index, 1);
        }
      });
      this.handle = null;
      return;
    }
    if (!this.selection.length) return;
    const sel = this.selection.slice();
    this.begin();
    const nodeIds = new Set(sel.filter((s) => s.type === 'node').map((s) => s.id));
    const removedSurvey = [];
    for (const s of sel) {
      const coll = this.scene[COLL[s.type]];
      const i = coll.findIndex((o) => o.id === s.id);
      if (i >= 0) {
        if (s.type === 'survey') removedSurvey.push(coll[i]);
        coll.splice(i, 1);
      }
    }
    // Cablurile legate de noduri șterse rămân, cu capătul liber în poziția portului.
    for (const l of this.scene.links) {
      for (const end of ['from', 'to']) {
        if (l[end]?.node && nodeIds.has(l[end].node)) {
          const idx = end === 'from' ? 0 : (l.route?.length || 1) - 1;
          const p = l.route?.[idx];
          l[end] = p ? { x: p[0], y: p[1] } : { x: 0, y: 0 };
        }
      }
    }
    this.selection = [];
    this.commit('Șterge');
    this.selectionChanged();
    if (removedSurvey.length) this.hooks.onSurveyRemoved?.(removedSurvey);
  }

  // ---------- clipboard ----------

  copy() {
    const objs = this.selectedObjects();
    if (!objs.length) return false;
    const nodeIds = new Set(objs.filter((o) => o.type === 'node').map((o) => o.id));
    const items = objs.filter((o) => o.type !== 'survey' && (o.type !== 'link' || this.linkInside(o.obj, nodeIds)));
    // Includem automat cablurile dintre nodurile copiate.
    for (const l of this.scene.links) {
      if (!items.some((i) => i.type === 'link' && i.id === l.id) && l.from.node && l.to.node && nodeIds.has(l.from.node) && nodeIds.has(l.to.node)) {
        items.push({ type: 'link', id: l.id, obj: l });
      }
    }
    this.clipboard = JSON.stringify(items.map((i) => ({ type: i.type, obj: i.obj })));
    return true;
  }

  linkInside(l, nodeIds) {
    return (!l.from.node || nodeIds.has(l.from.node)) && (!l.to.node || nodeIds.has(l.to.node));
  }

  paste(offset) {
    if (!this.clipboard) return;
    const items = JSON.parse(this.clipboard);
    const d = offset ?? this.scene.grid.size * 2;
    const idMap = new Map();
    this.begin();
    const sel = [];
    for (const it of items) {
      const o = it.obj;
      const nid = newId(it.type[0]);
      idMap.set(o.id, nid);
      o.id = nid;
      if (it.type === 'wall') o.points = o.points.map((p) => [p[0] + d, p[1] + d]);
      else if (it.type === 'link') {
        o.points = o.points.map((p) => [p[0] + d, p[1] + d]);
      } else { o.x += d; o.y += d; }
      if (it.type === 'node') delete o.device_id; // un dispozitiv apare o singură dată pe hartă
    }
    for (const it of items) {
      const o = it.obj;
      if (it.type === 'link') {
        for (const end of ['from', 'to']) {
          if (o[end].node) o[end].node = idMap.get(o[end].node);
          else { o[end].x += d; o[end].y += d; }
        }
      }
      this.scene[COLL[it.type]].push(o);
      sel.push({ type: it.type, id: o.id });
    }
    this.commit('Lipește');
    this.select(sel);
    this.clipboard = JSON.stringify(items.map((i) => ({ type: i.type, obj: i.obj })));
  }

  duplicate() {
    if (this.copy()) this.paste();
  }

  // ---------- aliniere / distribuire ----------

  align(kind) {
    const objs = this.selectedObjects().filter((o) => ['node', 'text', 'room', 'survey', 'door'].includes(o.type));
    if (objs.length < 2) return;
    const boxes = objs.map((o) => ({ o, b: itemBox(this.scene, o.type, o.obj) }));
    const minX = Math.min(...boxes.map((x) => x.b.x)), maxX = Math.max(...boxes.map((x) => x.b.x + x.b.w));
    const minY = Math.min(...boxes.map((x) => x.b.y)), maxY = Math.max(...boxes.map((x) => x.b.y + x.b.h));
    const move = (o, dx, dy) => { o.obj.x += dx; o.obj.y += dy; };
    this.begin();
    if (kind === 'dist-h' || kind === 'dist-v') {
      if (objs.length < 3) { this._before = null; return; }
      const h = kind === 'dist-h';
      boxes.sort((a, b) => (h ? a.b.x + a.b.w / 2 - (b.b.x + b.b.w / 2) : a.b.y + a.b.h / 2 - (b.b.y + b.b.h / 2)));
      const c0 = h ? boxes[0].b.x + boxes[0].b.w / 2 : boxes[0].b.y + boxes[0].b.h / 2;
      const last = boxes[boxes.length - 1];
      const c1 = h ? last.b.x + last.b.w / 2 : last.b.y + last.b.h / 2;
      const step = (c1 - c0) / (boxes.length - 1);
      boxes.forEach((x, i) => {
        const c = h ? x.b.x + x.b.w / 2 : x.b.y + x.b.h / 2;
        const target = c0 + step * i;
        move(x.o, h ? target - c : 0, h ? 0 : target - c);
      });
    } else {
      for (const { o, b } of boxes) {
        switch (kind) {
          case 'left': move(o, minX - b.x, 0); break;
          case 'right': move(o, maxX - (b.x + b.w), 0); break;
          case 'center': move(o, (minX + maxX) / 2 - (b.x + b.w / 2), 0); break;
          case 'top': move(o, 0, minY - b.y); break;
          case 'bottom': move(o, 0, maxY - (b.y + b.h)); break;
          case 'middle': move(o, 0, (minY + maxY) / 2 - (b.y + b.h / 2)); break;
        }
      }
    }
    this.commit('Aliniere');
    this.afterMove(objs.filter((o) => o.type === 'node').map((o) => o.obj));
  }

  // afterMove: nodurile legate de dispozitive preiau locația camerei în care sunt.
  afterMove(nodes) {
    const updates = [];
    for (const n of nodes) {
      const r = roomAt(this.scene, [n.x, n.y]);
      const name = r ? r.name : '';
      if (name && n.roomName !== name) {
        updates.push({ node: n, room: name, prev: n.roomName || '' });
      }
    }
    if (!updates.length) return;
    for (const u of updates) u.node.roomName = u.room;
    this.hooks.onNodesInRooms?.(updates);
  }

  // ---------- hit testing ----------

  hitTolerance() { return 6 / this.view.zoom; }

  // hitTest întoarce elementul de sus de sub punct (respectă straturile).
  hitTest(p, { types, ignoreLocked = true } = {}) {
    const s = this.scene;
    const ok = (type) => {
      if (types && !types.includes(type)) return false;
      const L = layer(s, LAYER_OF[type]);
      return L.visible && (!ignoreLocked || !L.locked);
    };
    const tol = this.hitTolerance();
    const ctx = this.ctx;
    if (ok('text')) {
      for (let i = s.texts.length - 1; i >= 0; i--) {
        const t = s.texts[i];
        ctx.font = `${t.bold ? 700 : 400} ${t.size || 14}px ${this.font()}`;
        const w = ctx.measureText(t.text || '').width;
        const b = { x: t.x - 2, y: t.y - (t.size || 14), w: w + 4, h: (t.size || 14) * 1.3 };
        if (rectContains(b, p)) return { type: 'text', id: t.id, obj: t };
      }
    }
    if (ok('survey')) {
      for (let i = s.surveyPoints.length - 1; i >= 0; i--) {
        const sp = s.surveyPoints[i];
        if (dist(p, [sp.x, sp.y]) < 10 + tol) return { type: 'survey', id: sp.id, obj: sp };
      }
    }
    if (ok('node')) {
      for (let i = s.nodes.length - 1; i >= 0; i--) {
        const n = s.nodes[i];
        const b = nodeBox(n);
        if (rectContains({ x: b.x - tol, y: b.y - tol, w: b.w + 2 * tol, h: b.h + 2 * tol }, p)) return { type: 'node', id: n.id, obj: n };
        // eticheta
        if (rectContains({ x: n.x - 50, y: b.y + b.h, w: 100, h: 30 }, p) && this.nodeLabel(n)) return { type: 'node', id: n.id, obj: n };
      }
    }
    if (ok('link')) {
      for (let i = s.links.length - 1; i >= 0; i--) {
        const l = s.links[i];
        const r = l.route || [];
        for (let j = 1; j < r.length; j++) {
          if (distToSegment(p, r[j - 1], r[j]) < tol + 2 / this.view.zoom) return { type: 'link', id: l.id, obj: l, segment: j - 1 };
        }
      }
    }
    if (ok('door')) {
      for (let i = s.doors.length - 1; i >= 0; i--) {
        const d = s.doors[i];
        if (dist(p, [d.x, d.y]) < (d.w || 40) / 2 + tol) return { type: 'door', id: d.id, obj: d };
      }
    }
    if (ok('wall')) {
      for (let i = s.walls.length - 1; i >= 0; i--) {
        const w = s.walls[i];
        for (let j = 1; j < w.points.length; j++) {
          if (distToSegment(p, w.points[j - 1], w.points[j]) < (w.thickness || 8) / 2 + tol) return { type: 'wall', id: w.id, obj: w, segment: j - 1 };
        }
      }
    }
    if (ok('room')) {
      for (let i = s.rooms.length - 1; i >= 0; i--) {
        const r = s.rooms[i];
        if (rectContains(r, p)) return { type: 'room', id: r.id, obj: r };
      }
    }
    return null;
  }

  // hitHandle: mânerele elementelor selectate (rotire, scalare, colțuri, puncte).
  hitHandle(p) {
    const tol = 8 / this.view.zoom;
    const objs = this.selectedObjects();
    if (objs.length === 1) {
      const { type, obj } = objs[0];
      if (layer(this.scene, LAYER_OF[type]).locked) return null;
      if (type === 'node') {
        const b = nodeBox(obj);
        const off = 14 / this.view.zoom;
        if (dist(p, [obj.x, b.y - off]) < tol) return { kind: 'rotate', obj };
        if (dist(p, [b.x + b.w + off / 2, b.y + b.h + off / 2]) < tol) return { kind: 'scale', obj };
      }
      if (type === 'room') {
        const cs = [[obj.x, obj.y, 'nw'], [obj.x + obj.w, obj.y, 'ne'], [obj.x, obj.y + obj.h, 'sw'], [obj.x + obj.w, obj.y + obj.h, 'se']];
        for (const [x, y, c] of cs) if (dist(p, [x, y]) < tol) return { kind: 'room-corner', obj, corner: c };
      }
      if (type === 'wall') {
        for (let i = 0; i < obj.points.length; i++) if (dist(p, obj.points[i]) < tol) return { kind: 'wallpoint', obj, index: i };
      }
      if (type === 'link') {
        for (let i = 0; i < obj.points.length; i++) if (dist(p, obj.points[i]) < tol) return { kind: 'waypoint', obj, index: i };
        const r = obj.route || [];
        if (r.length && !obj.from.node && dist(p, r[0]) < tol) return { kind: 'link-end', obj, end: 'from' };
        if (r.length && !obj.to.node && dist(p, r[r.length - 1]) < tol) return { kind: 'link-end', obj, end: 'to' };
        // Capetele legate pot fi mutate pe alt port / nod.
        if (r.length && obj.from.node && dist(p, r[0]) < tol) return { kind: 'link-end', obj, end: 'from' };
        if (r.length && obj.to.node && dist(p, r[r.length - 1]) < tol) return { kind: 'link-end', obj, end: 'to' };
      }
    }
    return null;
  }

  // hitPort: portul unui nod sub cursor (raza 8 px pe ecran).
  hitPort(p, exclude) {
    const tol = 9 / this.view.zoom;
    if (!layer(this.scene, 'nodes').visible) return null;
    for (let i = this.scene.nodes.length - 1; i >= 0; i--) {
      const n = this.scene.nodes[i];
      if (exclude && exclude === n.id) continue;
      for (const s of PORTS) {
        if (dist(p, portPos(n, s).p) < tol) return { node: n.id, port: s, p: portPos(n, s).p };
      }
    }
    return null;
  }

  // ---------- date din inventar ----------

  device(id) { return id ? this.hooks.getDevice?.(id) : null; }

  nodeLabel(n) {
    if (n.label) return n.label;
    if (n.bt_id) {
      const b = this.hooks.getBluetooth?.(n.bt_id);
      if (b) return b.custom_name || b.name || b.mac;
    }
    const d = this.device(n.device_id);
    if (d) return d.custom_name || d.hostname || d.last_ip || d.mac;
    return '';
  }

  nodeSublabel(n) {
    if (!this.showIP) return '';
    const d = this.device(n.device_id);
    if (d && d.last_ip && this.nodeLabel(n) !== d.last_ip) return d.last_ip;
    return '';
  }

  // exportScene: copie cu etichetele rezolvate (pentru randarea SVG pe server).
  exportScene() {
    updateRoutes(this.scene);
    const s = JSON.parse(JSON.stringify(this.scene));
    for (const n of s.nodes) {
      const orig = this.scene.nodes.find((x) => x.id === n.id);
      n.label = this.nodeLabel(orig);
      n.sublabel = this.nodeSublabel(orig);
      if (!n.color) {
        const d = this.device(n.device_id);
        if (d && d.color) n.color = d.color;
      }
    }
    for (const sp of s.surveyPoints) {
      const info = this.hooks.getSurvey?.(sp.survey_point_id);
      if (info && !sp.label) sp.label = info.label;
    }
    return s;
  }

  // ---------- evenimente ----------

  bindEvents() {
    const c = this.canvas;
    c.addEventListener('pointerdown', (e) => this.onDown(e));
    c.addEventListener('pointermove', (e) => this.onMove(e));
    c.addEventListener('pointerup', (e) => this.onUp(e));
    c.addEventListener('pointercancel', (e) => this.onUp(e));
    c.addEventListener('dblclick', (e) => this.tool?.dblclick?.(this.toWorld(...this.eventPoint(e)), e));
    c.addEventListener('wheel', (e) => {
      e.preventDefault();
      const [sx, sy] = this.eventPoint(e);
      if (e.ctrlKey || !e.shiftKey) {
        const f = Math.exp(-e.deltaY * (e.deltaMode === 1 ? 0.05 : 0.0015));
        this.zoomAt(sx, sy, f);
      } else {
        this.view.panX -= e.deltaX || e.deltaY;
        this.requestRender();
      }
    }, { passive: false });
    c.addEventListener('contextmenu', (e) => e.preventDefault());
    c.addEventListener('pointerleave', () => { this.hover = null; this.requestRender(); });

    // Drag & drop din paletă și din lista de dispozitive.
    c.addEventListener('dragover', (e) => {
      const t = [...e.dataTransfer.types];
      if (!t.includes('application/x-netmap')) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'copy';
      const p = this.toWorld(...this.eventPoint(e));
      const data = window.__netmapDrag;
      this.ghost = { icon: data?.icon || 'necunoscut', color: data?.color, p: this.snapPos(p, e.altKey) };
      this.requestRender();
    });
    c.addEventListener('dragleave', () => { this.ghost = null; this.requestRender(); });
    c.addEventListener('drop', (e) => {
      e.preventDefault();
      this.ghost = null;
      let data;
      try { data = JSON.parse(e.dataTransfer.getData('application/x-netmap')); } catch { return; }
      const p = this.snapPos(this.toWorld(...this.eventPoint(e)), e.altKey);
      if (data.kind === 'icon') this.addNode({ icon: data.icon }, p);
      if (data.kind === 'device') this.hooks.onDropDevice?.(data.id, p);
      this.requestRender();
    });
  }

  snapPos(p, noSnap) {
    if (noSnap || !this.scene.grid.snap) return p;
    const g = this.scene.grid.size;
    return [Math.round(p[0] / g) * g, Math.round(p[1] / g) * g];
  }

  addNode(props, p) {
    const L = layer(this.scene, 'nodes');
    if (L.locked) { this.hooks.onMessage?.('Stratul „Echipamente” este blocat.'); return null; }
    const n = { id: newId('n'), icon: 'necunoscut', x: p[0], y: p[1], rotation: 0, scale: 1, label: '', ...props };
    this.begin();
    this.scene.nodes.push(n);
    this.commit('Adaugă echipament');
    this.select([{ type: 'node', id: n.id }]);
    this.afterMove([n]);
    return n;
  }

  onDown(e) {
    this.canvas.setPointerCapture(e.pointerId);
    const sp = this.eventPoint(e);
    if (e.button === 1 || (e.button === 0 && this.spaceDown) || e.button === 2 && this.toolName === 'select' && e.shiftKey) {
      this.panning = { sx: sp[0], sy: sp[1], px: this.view.panX, py: this.view.panY };
      this.canvas.style.cursor = 'grabbing';
      return;
    }
    if (e.button === 2) {
      // Click dreapta: termină/anulează operația curentă.
      if (this.tool?.finish) this.tool.finish();
      else this.tool?.cancel?.();
      this.requestRender();
      return;
    }
    if (e.button !== 0) return;
    this.tool?.down?.(this.toWorld(...sp), e);
    this.requestRender();
  }

  onMove(e) {
    const sp = this.eventPoint(e);
    this.mouseScreen = sp;
    if (this.panning) {
      this.view.panX = this.panning.px + sp[0] - this.panning.sx;
      this.view.panY = this.panning.py + sp[1] - this.panning.sy;
      this.hooks.onView?.();
      this.requestRender();
      return;
    }
    const p = this.toWorld(...sp);
    this.mouse = p;
    this.hooks.onCursor?.(p);
    this.tool?.move?.(p, e);
    this.requestRender();
  }

  onUp(e) {
    if (this.panning) {
      this.panning = null;
      this.canvas.style.cursor = this.spaceDown ? 'grab' : (this.tool?.cursor || 'default');
      return;
    }
    this.tool?.up?.(this.toWorld(...this.eventPoint(e)), e);
    this.requestRender();
  }

  // key întoarce true dacă a consumat tasta.
  key(e) {
    const k = e.key;
    const ctrl = e.ctrlKey || e.metaKey;
    if (this.tool?.key?.(e)) return true;
    if (ctrl) {
      switch (k.toLowerCase()) {
        case 'z': e.shiftKey ? this.redo() : this.undo(); return true;
        case 'y': this.redo(); return true;
        case 'c': this.copy(); return true;
        case 'v': this.paste(); return true;
        case 'd': this.duplicate(); return true;
        case 'a': this.selectAll(); return true;
      }
      return false;
    }
    if (e.altKey) return false;
    switch (k) {
      case 'Delete':
      case 'Backspace': this.deleteSelection(); return true;
      case 'Escape': this.cancelTool(); this.setTool('select'); this.clearSelection(); return true;
      case 'ArrowLeft': case 'ArrowRight': case 'ArrowUp': case 'ArrowDown': {
        if (!this.selection.length) return false;
        const g = this.scene.grid.size * (e.shiftKey ? 5 : 1);
        const d = { ArrowLeft: [-g, 0], ArrowRight: [g, 0], ArrowUp: [0, -g], ArrowDown: [0, g] }[k];
        this.moveSelection(d[0], d[1]);
        return true;
      }
    }
    const map = { v: 'select', w: 'wall', r: 'room', c: 'cable', t: 'text', e: 'eraser', d: 'door', p: 'survey' };
    const low = k.toLowerCase();
    if (map[low] && !e.shiftKey) { this.setTool(map[low]); return true; }
    switch (low) {
      case 'm':
        this.linkMode = this.linkMode === 'orthogonal' ? 'straight' : 'orthogonal';
        if (this.toolName !== 'cable') {
          const links = this.selectedObjects().filter((o) => o.type === 'link');
          if (links.length) this.change('Mod cablu', () => { for (const l of links) l.obj.mode = l.obj.mode === 'orthogonal' ? 'straight' : 'orthogonal'; });
        }
        this.hooks.onTool?.(this.toolName);
        this.requestRender();
        return true;
      case 'g':
        this.scene.grid.visible = !this.scene.grid.visible;
        this.setDirty(true);
        this.hooks.onView?.();
        this.requestRender();
        return true;
      case 's':
        this.scene.grid.snap = !this.scene.grid.snap;
        this.setDirty(true);
        this.hooks.onView?.();
        return true;
      case 'f': this.fitAll(); return true;
      case '+': case '=': this.setZoom(this.view.zoom * 1.2); return true;
      case '-': this.setZoom(this.view.zoom / 1.2); return true;
      case '0': this.setZoom(1); return true;
    }
    return false;
  }

  moveSelection(dx, dy) {
    const objs = this.selectedObjects().filter((o) => !layer(this.scene, LAYER_OF[o.type]).locked);
    if (!objs.length) return;
    this.begin();
    for (const o of objs) moveObj(o.type, o.obj, dx, dy);
    this.commit('Mută');
    this.selectionChanged(false);
    this.afterMove(objs.filter((o) => o.type === 'node').map((o) => o.obj));
    this.hooks.onSurveyMoved?.(objs.filter((o) => o.type === 'survey').map((o) => o.obj));
  }

  // ---------- randare ----------

  requestRender() {
    if (this._raf) return;
    this._raf = requestAnimationFrame(() => {
      this._raf = 0;
      this.render();
    });
  }

  font() { return 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'; }

  render() {
    const ctx = this.ctx, s = this.scene, v = this.view, t = this.theme, dpr = this.dpr;
    updateRoutes(s);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.fillStyle = t.bg;
    ctx.fillRect(0, 0, this.cssW, this.cssH);
    ctx.setTransform(dpr * v.zoom, 0, 0, dpr * v.zoom, dpr * v.panX, dpr * v.panY);
    const vis = (id) => layer(s, id).visible;
    const view = { x: -v.panX / v.zoom, y: -v.panY / v.zoom, w: this.cssW / v.zoom, h: this.cssH / v.zoom };

    // fundal
    if (vis('background') && this.bgImage && s.background && s.background.w) {
      ctx.globalAlpha = s.background.opacity ?? 0.5;
      ctx.drawImage(this.bgImage, s.background.x, s.background.y, s.background.w, s.background.h);
      ctx.globalAlpha = 1;
    }
    if (s.grid.visible) this.drawGrid(view);

    if (vis('walls')) {
      for (const r of s.rooms) this.drawRoom(r);
      for (const w of s.walls) this.drawWall(w);
      for (const d of s.doors) this.drawDoor(d);
    }
    if (vis('survey') && this.heatmap) {
      const h = this.heatmap;
      ctx.globalAlpha = 0.45;
      ctx.imageSmoothingEnabled = true;
      ctx.drawImage(h.canvas, h.x, h.y, h.w, h.h);
      ctx.globalAlpha = 1;
    }
    if (vis('links')) for (const l of s.links) this.drawLink(l);
    if (vis('nodes')) for (const n of s.nodes) this.drawNode(n, view);
    if (vis('survey')) {
      for (const sp of s.surveyPoints) this.drawSurvey(sp);
      for (const a of this.apEstimates) this.drawAPEstimate(a);
    }
    if (vis('labels')) for (const tx of s.texts) this.drawText(tx);

    this.drawSelection();
    if (this.tool?.showPorts) this.drawPorts();
    this.tool?.draw?.(ctx);
    this.drawGuides();
    if (this.ghost) {
      const g = this.ghost;
      ctx.globalAlpha = 0.55;
      const img = iconImage(g.icon, g.color || t.accent);
      if (img.complete) ctx.drawImage(img, g.p[0] - 24, g.p[1] - 24, 48, 48);
      ctx.globalAlpha = 1;
    }
    this.drawRuler();
    this.drawMinimap();
  }

  px(n) { return n / this.view.zoom; }

  drawGrid(view) {
    const ctx = this.ctx, g = this.scene.grid.size, z = this.view.zoom;
    if (g * z < 5) return;
    const x0 = Math.floor(view.x / g) * g, y0 = Math.floor(view.y / g) * g;
    const x1 = view.x + view.w, y1 = view.y + view.h;
    ctx.lineWidth = this.px(1);
    for (const major of [false, true]) {
      ctx.strokeStyle = major ? this.theme.gridMajor : this.theme.grid;
      ctx.beginPath();
      for (let x = x0; x <= x1; x += g) {
        const isMajor = Math.round(x / g) % 5 === 0;
        if (isMajor !== major) continue;
        ctx.moveTo(x, view.y); ctx.lineTo(x, y1);
      }
      for (let y = y0; y <= y1; y += g) {
        const isMajor = Math.round(y / g) % 5 === 0;
        if (isMajor !== major) continue;
        ctx.moveTo(view.x, y); ctx.lineTo(x1, y);
      }
      ctx.stroke();
    }
  }

  drawRoom(r) {
    const ctx = this.ctx;
    const c = r.color || '#e8f0fe';
    ctx.globalAlpha = 0.55;
    ctx.fillStyle = c;
    ctx.fillRect(r.x, r.y, r.w, r.h);
    ctx.globalAlpha = 1;
    ctx.strokeStyle = c;
    ctx.lineWidth = 1.5;
    ctx.strokeRect(r.x, r.y, r.w, r.h);
    if (r.name) {
      ctx.font = `600 18px ${this.font()}`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillStyle = this.theme.muted;
      ctx.globalAlpha = 0.75;
      ctx.fillText(r.name, r.x + r.w / 2, r.y + r.h / 2);
      ctx.globalAlpha = 1;
    }
  }

  drawWall(w) {
    if (w.points.length < 2) return;
    const ctx = this.ctx;
    ctx.strokeStyle = this.theme.wall;
    ctx.lineWidth = w.thickness || 8;
    ctx.lineJoin = 'miter';
    ctx.lineCap = 'square';
    ctx.beginPath();
    ctx.moveTo(...w.points[0]);
    for (const p of w.points.slice(1)) ctx.lineTo(...p);
    ctx.stroke();
  }

  drawDoor(d) {
    const ctx = this.ctx, w = d.w || 40;
    ctx.save();
    ctx.translate(d.x, d.y);
    ctx.rotate(((d.rotation || 0) * Math.PI) / 180);
    ctx.fillStyle = this.theme.bg;
    ctx.fillRect(-w / 2, -5, w, 10);
    ctx.strokeStyle = this.theme.wall;
    ctx.lineWidth = 1.5;
    if (d.kind === 'window') {
      ctx.strokeRect(-w / 2, -5, w, 10);
      ctx.beginPath(); ctx.moveTo(-w / 2, 0); ctx.lineTo(w / 2, 0); ctx.stroke();
    } else {
      ctx.beginPath();
      ctx.moveTo(-w / 2, 0); ctx.lineTo(-w / 2, -w);
      ctx.arc(-w / 2, 0, w, -Math.PI / 2, 0);
      ctx.stroke();
    }
    ctx.restore();
  }

  drawLink(l) {
    const r = l.route;
    if (!r || r.length < 2) return;
    const ctx = this.ctx;
    const st = CABLE_TYPES[l.type] || CABLE_TYPES.inferat;
    ctx.strokeStyle = st.color;
    ctx.lineWidth = 3;
    ctx.lineJoin = 'round';
    ctx.lineCap = 'round';
    ctx.setLineDash(st.dash);
    ctx.beginPath();
    ctx.moveTo(...r[0]);
    for (const p of r.slice(1)) ctx.lineTo(...p);
    ctx.stroke();
    ctx.setLineDash([]);
    // capete libere
    for (const [end, p] of [['from', r[0]], ['to', r[r.length - 1]]]) {
      if (!l[end].node) {
        ctx.fillStyle = st.color;
        ctx.beginPath(); ctx.arc(p[0], p[1], 3.5, 0, Math.PI * 2); ctx.fill();
      }
    }
    let label = l.label || '';
    if (this.showLengths && this.scene.scale.unitsPerMeter > 0) {
      const m = polylineLength(r) / this.scene.scale.unitsPerMeter;
      label = (label ? label + ' · ' : '') + m.toFixed(1) + ' m';
    }
    if (label) {
      const [mx, my] = polylineMid(r);
      this.haloText(label, mx, my - 6, `11px ${this.font()}`, st.color, 'center');
    }
  }

  haloText(text, x, y, font, color, align = 'center') {
    const ctx = this.ctx;
    ctx.font = font;
    ctx.textAlign = align;
    ctx.textBaseline = 'alphabetic';
    ctx.lineWidth = 3;
    ctx.lineJoin = 'round';
    ctx.strokeStyle = this.theme.halo;
    ctx.strokeText(text, x, y);
    ctx.fillStyle = color;
    ctx.fillText(text, x, y);
  }

  drawNode(n, view) {
    const b = nodeBox(n);
    if (!rectsIntersect({ x: b.x - 60, y: b.y - 10, w: b.w + 120, h: b.h + 50 }, view)) return;
    const ctx = this.ctx;
    const d = this.device(n.device_id);
    const color = n.color || d?.color || this.theme.accent;
    const img = iconImage(n.icon, color);
    if (img.complete && img.naturalWidth) {
      ctx.save();
      ctx.translate(n.x, n.y);
      ctx.rotate(((n.rotation || 0) * Math.PI) / 180);
      ctx.drawImage(img, -b.w / 2, -b.h / 2, b.w, b.h);
      ctx.restore();
    }
    if (n.bt_id && !n.device_id) {
      // punct mov = văzut prin Bluetooth la ultimul scan
      const bt = this.hooks.getBluetooth?.(n.bt_id);
      ctx.fillStyle = bt?.visible ? '#7b1fa2' : '#9aa0a6';
      ctx.strokeStyle = this.theme.halo;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.arc(b.x + b.w - 2, b.y + 2, 5, 0, Math.PI * 2);
      ctx.fill();
      ctx.stroke();
    }
    if (n.device_id) {
      const on = d?.online;
      ctx.fillStyle = on ? '#1e8e3e' : '#9aa0a6';
      ctx.strokeStyle = this.theme.halo;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.arc(b.x + b.w - 2, b.y + 2, 5, 0, Math.PI * 2);
      ctx.fill();
      ctx.stroke();
      if (d?.new) {
        this.haloText('NOU', b.x + b.w + 4, b.y + 6, `700 9px ${this.font()}`, '#d93025', 'left');
      }
    }
    if (layer(this.scene, 'labels').visible && this.view.zoom > 0.25) {
      let y = b.y + b.h + 14;
      const label = this.nodeLabel(n);
      if (label) {
        this.haloText(label, n.x, y, `600 12px ${this.font()}`, this.theme.text);
        y += 14;
      }
      const sub = this.nodeSublabel(n);
      if (sub) this.haloText(sub, n.x, y, `10.5px ${this.font()}`, this.theme.muted);
    }
  }

  drawSurvey(sp) {
    const ctx = this.ctx;
    ctx.fillStyle = 'rgba(147,52,230,.2)';
    ctx.strokeStyle = '#9334e6';
    ctx.lineWidth = 2;
    ctx.beginPath(); ctx.arc(sp.x, sp.y, 8, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
    ctx.fillStyle = '#9334e6';
    ctx.beginPath(); ctx.arc(sp.x, sp.y, 2.5, 0, Math.PI * 2); ctx.fill();
    const info = this.hooks.getSurvey?.(sp.survey_point_id);
    let txt = sp.label || info?.label || '';
    const best = this.hooks.surveyBest?.(sp.survey_point_id);
    if (best) txt = (txt ? txt + ' · ' : '') + best;
    if (txt) this.haloText(txt, sp.x + 12, sp.y + 4, `11px ${this.font()}`, '#7627bb', 'left');
  }

  drawAPEstimate(a) {
    const ctx = this.ctx;
    ctx.strokeStyle = '#9334e6';
    ctx.setLineDash([4, 4]);
    ctx.lineWidth = 1.5;
    ctx.beginPath(); ctx.arc(a.x, a.y, 18, 0, Math.PI * 2); ctx.stroke();
    ctx.setLineDash([]);
    ctx.beginPath(); ctx.moveTo(a.x - 6, a.y); ctx.lineTo(a.x + 6, a.y); ctx.moveTo(a.x, a.y - 6); ctx.lineTo(a.x, a.y + 6); ctx.stroke();
    this.haloText(a.label, a.x, a.y - 24, `600 10.5px ${this.font()}`, '#7627bb');
  }

  drawText(t) {
    const ctx = this.ctx;
    ctx.font = `${t.bold ? 700 : 400} ${t.size || 14}px ${this.font()}`;
    ctx.textAlign = 'left';
    ctx.textBaseline = 'alphabetic';
    ctx.fillStyle = t.color || this.theme.text;
    ctx.fillText(t.text || '', t.x, t.y);
  }

  drawSelection() {
    const ctx = this.ctx, z = this.view.zoom, c = this.theme.select;
    const hs = 7 / z;
    const square = (p, fill = '#fff') => {
      ctx.fillStyle = fill;
      ctx.strokeStyle = c;
      ctx.lineWidth = 1.5 / z;
      ctx.fillRect(p[0] - hs / 2, p[1] - hs / 2, hs, hs);
      ctx.strokeRect(p[0] - hs / 2, p[1] - hs / 2, hs, hs);
    };
    const objs = this.selectedObjects();
    for (const { type, obj } of objs) {
      ctx.strokeStyle = c;
      ctx.lineWidth = 1.5 / z;
      if (type === 'link') {
        const r = obj.route || [];
        if (r.length < 2) continue;
        ctx.globalAlpha = 0.35;
        ctx.lineWidth = 9 / z;
        ctx.beginPath(); ctx.moveTo(...r[0]); for (const p of r.slice(1)) ctx.lineTo(...p); ctx.stroke();
        ctx.globalAlpha = 1;
        if (objs.length === 1) {
          obj.points.forEach((p, i) => square(p, this.handle?.kind === 'waypoint' && this.handle.index === i ? c : '#fff'));
          ctx.fillStyle = c;
          for (const p of [r[0], r[r.length - 1]]) { ctx.beginPath(); ctx.arc(p[0], p[1], 4 / z, 0, Math.PI * 2); ctx.fill(); }
        }
        continue;
      }
      if (type === 'wall') {
        const pts = obj.points;
        ctx.globalAlpha = 0.35;
        ctx.lineWidth = (obj.thickness || 8) + 6 / z;
        ctx.beginPath(); ctx.moveTo(...pts[0]); for (const p of pts.slice(1)) ctx.lineTo(...p); ctx.stroke();
        ctx.globalAlpha = 1;
        if (objs.length === 1) pts.forEach((p, i) => square(p, this.handle?.kind === 'wallpoint' && this.handle.index === i ? c : '#fff'));
        continue;
      }
      let b = itemBox(this.scene, type, obj);
      if (type === 'text') {
        ctx.font = `${obj.bold ? 700 : 400} ${obj.size || 14}px ${this.font()}`;
        b = { x: obj.x - 2, y: obj.y - (obj.size || 14), w: ctx.measureText(obj.text || '').width + 4, h: (obj.size || 14) * 1.3 };
      }
      ctx.setLineDash([4 / z, 3 / z]);
      ctx.strokeRect(b.x - 3 / z, b.y - 3 / z, b.w + 6 / z, b.h + 6 / z);
      ctx.setLineDash([]);
      if (objs.length === 1 && type === 'node') {
        const off = 14 / z;
        ctx.beginPath(); ctx.moveTo(obj.x, b.y - 3 / z); ctx.lineTo(obj.x, b.y - off); ctx.stroke();
        ctx.fillStyle = '#fff';
        ctx.beginPath(); ctx.arc(obj.x, b.y - off, 5 / z, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
        square([b.x + b.w + off / 2, b.y + b.h + off / 2]);
      }
      if (objs.length === 1 && type === 'room') {
        for (const p of [[b.x, b.y], [b.x + b.w, b.y], [b.x, b.y + b.h], [b.x + b.w, b.y + b.h]]) square(p);
      }
    }
    if (this.marquee) {
      const m = this.marquee;
      ctx.fillStyle = 'rgba(26,115,232,.08)';
      ctx.strokeStyle = c;
      ctx.lineWidth = 1 / z;
      ctx.setLineDash([4 / z, 3 / z]);
      ctx.fillRect(m.x, m.y, m.w, m.h);
      ctx.strokeRect(m.x, m.y, m.w, m.h);
      ctx.setLineDash([]);
    }
  }

  drawPorts() {
    const ctx = this.ctx, z = this.view.zoom;
    const p = this.mouse;
    const near = 70 / z + 30;
    for (const n of this.scene.nodes) {
      if (dist([n.x, n.y], p) > near + nodeHalf(n)) continue;
      for (const s of PORTS) {
        const q = portPos(n, s).p;
        const hot = dist(q, p) < 9 / z;
        ctx.fillStyle = hot ? this.theme.select : '#fff';
        ctx.strokeStyle = this.theme.select;
        ctx.lineWidth = 1.5 / z;
        ctx.beginPath(); ctx.arc(q[0], q[1], (hot ? 6 : 4.5) / z, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
      }
    }
  }

  drawGuides() {
    if (!this.guides.length) return;
    const ctx = this.ctx, z = this.view.zoom;
    ctx.strokeStyle = this.theme.guide;
    ctx.lineWidth = 1 / z;
    ctx.setLineDash([3 / z, 3 / z]);
    ctx.beginPath();
    for (const g of this.guides) { ctx.moveTo(...g.from); ctx.lineTo(...g.to); }
    ctx.stroke();
    ctx.setLineDash([]);
  }

  drawRuler() {
    const upm = this.scene.scale.unitsPerMeter;
    if (!upm) return;
    const ctx = this.ctx, dpr = this.dpr, z = this.view.zoom;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    let meters = 1;
    for (const m of [0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 50, 100]) if (m * upm * z <= 140) meters = m;
    const len = meters * upm * z;
    const x = 16, y = this.cssH - 18;
    ctx.fillStyle = this.theme.bg;
    ctx.globalAlpha = 0.85;
    ctx.fillRect(x - 6, y - 22, len + 12, 30);
    ctx.globalAlpha = 1;
    ctx.strokeStyle = this.theme.text;
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(x, y - 5); ctx.lineTo(x, y + 4); ctx.moveTo(x, y); ctx.lineTo(x + len, y); ctx.moveTo(x + len, y - 5); ctx.lineTo(x + len, y + 4);
    ctx.stroke();
    ctx.font = `11px ${this.font()}`;
    ctx.fillStyle = this.theme.text;
    ctx.textAlign = 'center';
    ctx.fillText(`${meters} m`, x + len / 2, y - 8);
  }

  drawMinimap() {
    const mm = this.minimap;
    if (!mm || mm.hidden) return;
    const b = sceneBounds(this.scene);
    const dpr = this.dpr;
    const W = mm.clientWidth, H = mm.clientHeight;
    if (mm.width !== W * dpr) { mm.width = W * dpr; mm.height = H * dpr; }
    const c = mm.getContext('2d');
    c.setTransform(dpr, 0, 0, dpr, 0, 0);
    c.fillStyle = this.theme.bg;
    c.fillRect(0, 0, W, H);
    const v = this.view;
    const vw = { x: -v.panX / v.zoom, y: -v.panY / v.zoom, w: this.cssW / v.zoom, h: this.cssH / v.zoom };
    const all = b ? normRect(Math.min(b.x, vw.x), Math.min(b.y, vw.y), Math.max(b.x + b.w, vw.x + vw.w), Math.max(b.y + b.h, vw.y + vw.h)) : vw;
    const sc = Math.min((W - 8) / all.w, (H - 8) / all.h);
    const ox = 4 - all.x * sc + ((W - 8) - all.w * sc) / 2, oy = 4 - all.y * sc + ((H - 8) - all.h * sc) / 2;
    this._mm = { sc, ox, oy };
    c.save();
    c.translate(ox, oy);
    c.scale(sc, sc);
    for (const r of this.scene.rooms) { c.fillStyle = r.color || '#e8f0fe'; c.fillRect(r.x, r.y, r.w, r.h); }
    c.strokeStyle = this.theme.wall;
    c.lineWidth = Math.max(1 / sc, 4);
    for (const w of this.scene.walls) { c.beginPath(); c.moveTo(...w.points[0]); for (const p of w.points.slice(1)) c.lineTo(...p); c.stroke(); }
    c.fillStyle = this.theme.accent;
    for (const n of this.scene.nodes) c.fillRect(n.x - 12, n.y - 12, 24, 24);
    c.strokeStyle = this.theme.select;
    c.lineWidth = 2 / sc;
    c.strokeRect(vw.x, vw.y, vw.w, vw.h);
    c.restore();
  }

  minimapClick(mx, my) {
    if (!this._mm) return;
    const { sc, ox, oy } = this._mm;
    this.centerOn([(mx - ox) / sc, (my - oy) / sc]);
  }
}

// moveObj mută un element cu (dx, dy).
export function moveObj(type, o, dx, dy) {
  switch (type) {
    case 'wall': o.points = o.points.map((p) => [p[0] + dx, p[1] + dy]); break;
    case 'link':
      o.points = o.points.map((p) => [p[0] + dx, p[1] + dy]);
      for (const end of ['from', 'to']) if (!o[end].node) { o[end].x += dx; o[end].y += dy; }
      break;
    default: o.x += dx; o.y += dy;
  }
}
