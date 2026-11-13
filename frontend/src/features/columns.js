// Column widths: drag the line between two column titles to resize (F-090). The name column takes whatever room is
// left, so dragging a border resizes the column to its right and the name column gives or takes the difference; the
// border follows the mouse either way. Double-click a border to give that column its default width back.
//
// Widths are saved in the settings ("columnWidths") as soon as a drag ends, per set of columns (the titles shown), so
// the standard columns and each custom column set keep their own widths, and both panels share them. Saving the
// settings or the position (cm_SaveSettings / cm_SavePosition) writes the settings file, which includes them.

import { getSetting, setSetting } from '../core/settings.js';

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
    const s = sig(header);
    const rest = { ...all() };
    delete rest[s];
    setSetting('columnWidths', rest);
    for (const p of app.panels) if (sig(p.$.header) === s) { p.el.style.removeProperty('--cols'); p.renderHeader?.(); app.emit('panel', p, { moved: false }); }
  });
  // Views and column sets redraw the header, and the views feature resets the panel's column template on every folder
  // change (without touching the titles); lay the saved widths over whatever they set, whenever either happens.
  new MutationObserver(() => apply(panel)).observe(header, { childList: true });
  new MutationObserver(() => apply(panel)).observe(panel.el, { attributes: true, attributeFilter: ['style'] });
  app.on('panel', (p) => { if (p === panel) queueMicrotask(() => apply(panel)); });
  apply(panel);
}

export function init(app) {
  for (const p of app.panels) wire(app, p);
}
