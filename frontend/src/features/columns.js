// Column widths: drag the line between two column titles to resize (F-090). The name column takes whatever room is
// left, so dragging a border resizes the column to its right and the name column gives or takes the difference; the
// border follows the mouse either way. Double-click a border to give that column its default width back.
//
// Show > Fit column widths to contents (cm_ColumnsFitWidth), Equal column widths (cm_ColumnsEqualWidth) and Default
// column widths (cm_ColumnsDefaultWidth), also in the header's right-click menu, set every column of the active panel
// at once (F-673). Fitting measures the title and the rows on screen (the panel draws only those).
//
// Widths are saved in the settings ("columnWidths") as soon as a drag ends, per set of columns (the titles shown), so
// the standard columns and each custom column set keep their own widths, and both panels share them. Saving the
// settings or the position (cm_SaveSettings / cm_SavePosition) writes the settings file, which includes them.

import { getSetting, setSetting } from '../core/settings.js';
import { command } from '../core/registry.js';
import { equalWidths, fitWidths } from './views-cols-lib.js';

const GRAB = 5;      // pixels either side of a border that start a resize
const MIN = 28;      // narrowest a column can get
const NAME_MIN = 80; // the name column never gets narrower than this

const all = () => getSetting('columnWidths', {}) || {};
const spans = (header) => [...header.children].filter((s) => s.tagName === 'SPAN' && !s.hidden && getComputedStyle(s).display !== 'none');
const sig = (header) => spans(header).map((s) => s.dataset.col || s.dataset.vcol || s.textContent.trim()).join('|');

// apply lays the saved widths over the panel's column template (the name column stays flexible).
function apply(panel) {
  const header = panel.$.header;
  if (!header || header.hidden || header.classList.contains('mc-col-dragging')) return;
  const w = all()[sig(header)];
  if (!Array.isArray(w)) return;
  const tpl = [`minmax(${NAME_MIN}px, 1fr)`, ...w.slice(1).map((px) => `${Math.max(MIN, Math.round(px))}px`)];
  if (tpl.length !== spans(header).length) return; // the columns changed since the widths were saved
  const want = tpl.join(' ');
  if (panel.el.style.getPropertyValue('--cols').trim() !== want) panel.el.style.setProperty('--cols', want);
}

// borderAt finds the column whose left border is under the pointer (never the name column, which has none to drag).
function borderAt(header, x) {
  const ss = spans(header);
  for (let k = 1; k < ss.length; k++) {
    const left = ss[k].getBoundingClientRect().left - 4; // (the grid has a column gap of 8px; the border is in its middle)
    if (Math.abs(x - left) <= GRAB) return k;
  }
  return -1;
}

function wire(app, panel) {
  const header = panel.$.header;
  let justDragged = false;
  header.addEventListener('mousemove', (ev) => {
    if (!header.classList.contains('mc-col-dragging')) header.classList.toggle('mc-col-edge', borderAt(header, ev.clientX) > 0);
  });
  header.addEventListener('mouseleave', () => header.classList.remove('mc-col-edge'));
  header.addEventListener('pointerdown', (ev) => {
    if (ev.button !== 0) return;
    const k = borderAt(header, ev.clientX);
    if (k < 1) return;
    ev.preventDefault();
    ev.stopPropagation();
    const ss = spans(header);
    const widths = ss.map((s) => s.getBoundingClientRect().width);
    const nameRoom = widths[0] - NAME_MIN;
    const startX = ev.clientX, start = widths[k];
    header.setPointerCapture(ev.pointerId);
    header.classList.add('mc-col-dragging');
    const move = (e) => {
      // Moving the border left makes column k wider; the name column pays for it, down to its minimum.
      const grow = Math.min(nameRoom, startX - e.clientX);
      widths[k] = Math.max(MIN, start + grow);
      const s = sig(header);
      for (const p of app.panels) if (sig(p.$.header) === s) p.el.style.setProperty('--cols', [`minmax(${NAME_MIN}px, 1fr)`, ...widths.slice(1).map((px) => `${Math.round(px)}px`)].join(' '));
    };
    const up = () => {
      header.removeEventListener('pointermove', move);
      header.removeEventListener('pointerup', up);
      header.removeEventListener('pointercancel', up);
      header.classList.remove('mc-col-dragging');
      justDragged = true;
      setTimeout(() => { justDragged = false; }, 0);
      setSetting('columnWidths', { ...all(), [sig(header)]: widths.map((px) => Math.round(px)) });
    };
    header.addEventListener('pointermove', move);
    header.addEventListener('pointerup', up);
    header.addEventListener('pointercancel', up);
  });
  // A drag ends with a click on a title, which would sort by that column; swallow it.
  header.addEventListener('click', (ev) => { if (justDragged || borderAt(header, ev.clientX) > 0) { ev.stopImmediatePropagation(); ev.preventDefault(); } }, true);
  header.addEventListener('dblclick', (ev) => {
    if (borderAt(header, ev.clientX) < 1) return;
    resetWidths(app, header);
  });
  // Views and column sets redraw the header, and the views feature resets the panel's column template on every folder
  // change (without touching the titles); lay the saved widths over whatever they set, whenever either happens.
  new MutationObserver(() => apply(panel)).observe(header, { childList: true });
  new MutationObserver(() => apply(panel)).observe(panel.el, { attributes: true, attributeFilter: ['style'] });
  app.on('panel', (p) => { if (p === panel) queueMicrotask(() => apply(panel)); });
  apply(panel);
}

// resetWidths forgets the saved widths of the header's columns: the view's own widths come back.
function resetWidths(app, header) {
  const s = sig(header);
  const rest = { ...all() };
  delete rest[s];
  setSetting('columnWidths', rest);
  for (const p of app.panels) if (sig(p.$.header) === s) { p.el.style.removeProperty('--cols'); p.renderHeader?.(); app.emit('panel', p, { moved: false }); }
}

// saveWidths keeps widths (one per column, the name column first) for the header's columns and shows them in every
// panel with the same columns.
function saveWidths(app, header, widths) {
  const s = sig(header);
  setSetting('columnWidths', { ...all(), [s]: widths.map((px) => Math.round(px)) });
  for (const p of app.panels) if (sig(p.$.header) === s) apply(p);
}

// usableHeader is the active panel's header when it shows columns besides the name, or null (with a message).
function usableHeader(app) {
  const panel = app.cur, header = panel?.$?.header;
  if (!header || header.hidden || spans(header).length < 2) { app.flash?.('This view has no columns to size.', 'err'); return null; }
  return header;
}

let measureCtx = null;
function measurer(el) {
  if (measureCtx === null) measureCtx = document.createElement('canvas').getContext?.('2d') || false;
  if (!measureCtx) return (t) => t.length * 7;
  measureCtx.font = getComputedStyle(el).font || '13px system-ui';
  return (t) => measureCtx.measureText(t).width;
}

// fitToContents sizes each column (not the name) to its widest title or cell on screen.
function fitToContents(app) {
  const header = usableHeader(app);
  if (!header) return false;
  const ss = spans(header);
  const texts = ss.map((s) => [s.textContent.trim()]);
  const rows = [...app.cur.$.rows.children];
  for (const r of rows) {
    const cells = [...r.children].filter((c) => c.tagName === 'SPAN' && getComputedStyle(c).display !== 'none');
    if (cells.length !== ss.length) continue; // (a row of another layout)
    cells.forEach((c, k) => texts[k].push(c.textContent));
  }
  const w = fitWidths(texts, measurer(rows[0] || header), { min: MIN });
  w[0] = ss[0].getBoundingClientRect().width; // (the name column takes the room left either way)
  saveWidths(app, header, w);
  return true;
}

// equalize gives every column, the name column included, the same width.
function equalize(app) {
  const header = usableHeader(app);
  if (!header) return false;
  const n = spans(header).length;
  saveWidths(app, header, equalWidths(header.clientWidth || header.getBoundingClientRect().width, n, 8, MIN));
  return true;
}

command({ name: 'cm_ColumnsFitWidth', label: 'Fit column widths to contents', menu: 'Show', menuAfter: 'cm_SrcCustomViewMenu', run: fitToContents }); // ours
command({ name: 'cm_ColumnsEqualWidth', label: 'Equal column widths', menu: 'Show', menuAfter: 'cm_ColumnsFitWidth', run: equalize }); // ours
command({
  name: 'cm_ColumnsDefaultWidth', label: 'Default column widths', menu: 'Show', menuAfter: 'cm_ColumnsEqualWidth', // ours
  run: (app) => { const h = usableHeader(app); if (!h) return false; resetWidths(app, h); return true; },
});

export function init(app) {
  for (const p of app.panels) wire(app, p);
}
