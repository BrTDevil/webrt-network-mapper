// Export: PNG, SVG, CSV, JSON, HTML, PDF (print) și import de backup.

import { api } from './api.js';
import { $, $$, el, toast, errorToast, dropdown, download, promptDialog, today } from './ui.js';
import { state, on } from './state.js';

let ctx;

export function initExport(c) {
  ctx = c;
  on('import', importBackup);
  $('#pdfBtn').addEventListener('click', () => printReport());
  $('#previewBtn').addEventListener('click', preview);
  $('#htmlBtn').addEventListener('click', () => go('html'));
  $('#csvBtn').addEventListener('click', () => go('csv'));
  $('#jsonBtn').addEventListener('click', () => go('json'));
  $('#svgBtn').addEventListener('click', exportSVG);
  $('#importBtn').addEventListener('click', importBackup);
  for (const b of $$('[data-png]')) b.addEventListener('click', () => exportPNG(+b.dataset.png));
}

export function exportMenu(anchor) {
  dropdown(anchor, [
    { label: 'Raport PDF (tab nou)', action: () => printReport() },
    { label: 'Raport HTML (fișier)', action: () => go('html') },
    '-',
    { label: 'Hartă PNG 2×', action: () => exportPNG(2) },
    { label: 'Hartă PNG 4×', action: () => exportPNG(4) },
    { label: 'Hartă SVG', action: exportSVG },
    '-',
    { label: 'Dispozitive CSV', action: () => go('csv') },
    { label: 'Backup JSON', action: () => go('json') },
    { label: 'Importă backup…', action: importBackup },
  ]);
}

async function go(format) {
  if (!state.project) return;
  await import('./app.js').then((m) => m.saveMap({ auto: true }));
  location.href = api.exportURL(state.project.id, format, { author: $('#reportAuthor').value });
}

// printReport deschide raportul într-un tab nou (fără tipărire automată).
// Tab-ul se deschide imediat, la click, ca să nu fie blocat ca pop-up;
// adresa se setează după ce harta a fost salvată.
async function printReport() {
  if (!state.project) return;
  const win = window.open('about:blank', '_blank');
  await import('./app.js').then((m) => m.saveMap({ auto: true }));
  const url = api.exportURL(state.project.id, 'html', { inline: '1', author: $('#reportAuthor').value });
  if (win) win.location.href = url;
  else location.href = url;
}

async function preview() {
  await import('./app.js').then((m) => m.saveMap({ auto: true }));
  const f = $('#reportFrame');
  f.hidden = false;
  f.src = api.exportURL(state.project.id, 'html', { inline: '1', author: $('#reportAuthor').value });
}

function fileBase() {
  const n = (state.project?.name || 'harta') + (state.maps.length > 1 ? ' - ' + state.map.name : '');
  return n.replace(/[\\/:*?"<>|]+/g, '-');
}

async function mapSVG() {
  const scene = state.editor.exportScene();
  return api.renderSVG({ scene_json: JSON.stringify(scene), background: state.map.background, project_id: state.project.id, legend: true });
}

async function exportSVG() {
  try {
    const svg = await mapSVG();
    download(`${fileBase()} ${today()}.svg`, svg, 'image/svg+xml');
  } catch (e) { errorToast(e); }
}

// PNG: rasterizăm SVG-ul generat de server (aceeași randare ca în raport, cu legendă).
async function exportPNG(scale) {
  try {
    const svg = await mapSVG();
    const m = svg.match(/width="([\d.]+)" height="([\d.]+)"/);
    const w = m ? +m[1] : 1200, h = m ? +m[2] : 800;
    const max = 16000;
    const s = Math.min(scale, max / w, max / h);
    const img = new Image();
    const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
    await new Promise((res, rej) => { img.onload = res; img.onerror = () => rej(new Error('Nu am putut randa harta.')); img.src = url; });
    const c = document.createElement('canvas');
    c.width = Math.round(w * s);
    c.height = Math.round(h * s);
    const g = c.getContext('2d');
    g.fillStyle = '#fff';
    g.fillRect(0, 0, c.width, c.height);
    g.drawImage(img, 0, 0, c.width, c.height);
    URL.revokeObjectURL(url);
    c.toBlob((blob) => download(`${fileBase()} ${today()}@${scale}x.png`, blob), 'image/png');
  } catch (e) { errorToast(e); }
}

async function importBackup() {
  const fi = $('#fileInput');
  fi.accept = '.json,application/json';
  fi.value = '';
  fi.onchange = async () => {
    const f = fi.files[0];
    if (!f) return;
    const text = await f.text();
    let parsed;
    try { parsed = JSON.parse(text); } catch { toast('Fișierul nu este JSON valid.', 'error'); return; }
    if (parsed.format !== 'webrt-network-mapper') { toast('Fișierul nu este un backup webrt-network-mapper.', 'error'); return; }
    const name = await promptDialog('Importă backup', 'Numele locației noi', `${parsed.project?.name || 'Locație'} (importat)`, { ok: 'Importă' });
    if (!name) return;
    try {
      const p = await api.importBackup(text, name);
      await ctx.reloadProjects();
      await ctx.loadProject(p.id);
      toast(`Importat ca „${p.name}”.`, 'ok');
    } catch (e) { errorToast(e); }
  };
  fi.click();
}
