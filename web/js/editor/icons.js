// Icon-urile SVG (definite în /icons.json, comune cu raportul generat de server),
// pre-randate ca imagini și păstrate în cache per culoare.

let DATA = { categories: [], icons: {} };
const cache = new Map();
let onReady = () => {};

export async function loadIcons() {
  const res = await fetch('/icons.json');
  DATA = await res.json();
  return DATA;
}

export const iconCategories = () => DATA.categories;
export const iconDefs = () => DATA.icons;
export const iconDef = (key) => DATA.icons[key] || DATA.icons.necunoscut;
export const iconLabel = (key) => (DATA.icons[key] || {}).label || key;

// Tipurile de dispozitiv (din scanner) au aceleași chei ca icon-urile.
export function iconForType(t) {
  return DATA.icons[t] ? t : 'necunoscut';
}

export function iconSVG(key, color = '#1a73e8', size = 48) {
  const d = iconDef(key);
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48" width="${size}" height="${size}" color="${color}">${d ? d.svg : ''}</svg>`;
}

// setIconReadyCallback: editorul se re-randează când o imagine termină de încărcat.
export function setIconReadyCallback(fn) { onReady = fn; }

// iconImage întoarce o imagine (posibil încă neîncărcată) randată la rezoluție mare.
export function iconImage(key, color) {
  const k = key + '|' + color;
  let img = cache.get(k);
  if (!img) {
    img = new Image();
    img.onload = () => onReady();
    img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(iconSVG(key, color, 192));
    cache.set(k, img);
  }
  return img;
}
