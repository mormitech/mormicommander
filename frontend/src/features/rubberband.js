// Rubber-band selection (F-700, ours): with Options > Drag & drop > "Rubber-band selection on empty space" on (off by
// default), a left drag that starts on the empty space of a file list (below the last entry, beside the columns of the
// Brief view, between or below the tiles of Thumbnails) draws a rectangle and selects the entries it touches. Without
// keys it replaces the selection; with Ctrl, Cmd or Shift held at the press it adds to it. Near the list's edges the
// list scrolls while the button is held; Esc puts the selection back as it was. The tree view has no rubber band.
// The geometry is rubberband-lib.js's; each view gives its boxes through panel.bandGeo (features/views.js).

import './rubberband.css';
import { bandRect, bandSelection, hitIndices } from './rubberband-lib.js';

const MIN_MOVE = 4;   // pixels before a press becomes a band (a plain click on empty space stays a click)
const EDGE = 24;      // pixels from the list's edge where it scrolls
const STEP_MS = 50;

export const rubberBandOn = (app) => app.config?.values?.['DragDrop.RubberBand'] === '1';

// fallback geometry for a panel without views.js: one entry per row
const geoOf = (p) => (typeof p.bandGeo === 'function' ? p.bandGeo() : { kind: 'rows', rowH: p.rowH, width: p.$.list.clientWidth });

let band = null; // {p, start, end, before, add, el, active, timer, cx, cy}

function contentPoint(list, cx, cy) {
  const r = list.getBoundingClientRect();
  const x = Math.max(0, Math.min(list.scrollWidth, cx - r.left + list.scrollLeft));
  const y = Math.max(0, Math.min(list.scrollHeight, cy - r.top + list.scrollTop));
  return { x, y };
}

function update() {
  const b = band, p = b.p, list = p.$.list;
  b.end = contentPoint(list, b.cx, b.cy);
  const rect = bandRect(b.start, b.end);
  Object.assign(b.el.style, { left: `${rect.x}px`, top: `${rect.y}px`, width: `${rect.w}px`, height: `${rect.h}px` });
  const sel = bandSelection(p.entries, b.before, hitIndices(p.entries.length, geoOf(p), rect), b.add);
  const now = p.selected;
  if (sel.size === now.size && [...sel].every((n) => now.has(n))) return;
  p.selected.clear();
  for (const n of sel) p.selected.add(n);
  p.selectionChanged();
}

// edgeScroll scrolls the list while the pointer is held near or past one of its edges.
function edgeScroll() {
  if (!band?.active) return;
  const list = band.p.$.list, r = list.getBoundingClientRect();
  let dx = 0, dy = 0;
  if (band.cy < r.top + EDGE) dy = -1; else if (band.cy > r.bottom - EDGE) dy = 1;
  if (band.cx < r.left + EDGE) dx = -1; else if (band.cx > r.right - EDGE) dx = 1;
  if (!dx && !dy) return;
  const t0 = list.scrollTop, l0 = list.scrollLeft;
  list.scrollTop += dy * (band.p.rowH || 20);
  list.scrollLeft += dx * 40;
  if (list.scrollTop !== t0 || list.scrollLeft !== l0) update();
}

function end(cancel) {
  const b = band;
  band = null;
  if (!b) return;
  clearInterval(b.timer);
  b.el.remove();
  if (cancel && b.active) {
    b.p.selected.clear();
    for (const n of b.before) b.p.selected.add(n);
    b.p.selectionChanged();
  }
}

export function init(app) {
  for (const p of app.panels) {
    const list = p.$.list;
    list.addEventListener('mousedown', (ev) => {
      if (ev.button !== 0 || ev.detail > 1 || ev.altKey || !rubberBandOn(app) || app.demo) return;
      if (ev.target.closest?.('.row, .msg') || !geoOf(p)) return;
      const r = list.getBoundingClientRect();
      if (ev.clientX - r.left > list.clientWidth || ev.clientY - r.top > list.clientHeight) return; // the scroll bars
      ev.preventDefault(); // (no text selection while the band is drawn)
      end(true);
      const el = document.createElement('div');
      el.className = 'mc-rubberband';
      el.hidden = true;
      list.append(el);
      band = {
        p, el, active: false, cx: ev.clientX, cy: ev.clientY, timer: 0,
        start: contentPoint(list, ev.clientX, ev.clientY), before: new Set(p.selected),
        add: ev.ctrlKey || ev.metaKey || ev.shiftKey,
      };
    });
  }
  document.addEventListener('pointermove', (ev) => {
    if (!band) return;
    if (!(ev.buttons & 1)) { end(false); return; }
    band.cx = ev.clientX;
    band.cy = ev.clientY;
    if (!band.active) {
      const r = band.p.$.list.getBoundingClientRect();
      const s = { x: band.start.x - band.p.$.list.scrollLeft + r.left, y: band.start.y - band.p.$.list.scrollTop + r.top };
      if (Math.hypot(ev.clientX - s.x, ev.clientY - s.y) < MIN_MOVE) return;
      band.active = true;
      band.el.hidden = false;
      band.timer = setInterval(edgeScroll, STEP_MS);
    }
    update();
  });
  document.addEventListener('pointerup', (ev) => { if (band && ev.button === 0) end(false); });
  window.addEventListener('blur', () => end(false));
  document.addEventListener('keydown', (ev) => {
    if (!band?.active || ev.key !== 'Escape') return;
    ev.preventDefault();
    ev.stopImmediatePropagation();
    end(true);
  }, true);
}
