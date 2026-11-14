// The bar between the panels (F-008): drag to share the width (or the height, with the panels on top of each other),
// double-click for half and half, right-click for a list of fixed shares (50/50, 60/40, 70/30 and the other way round).
// The share is kept in the session. The pure parts (clamping, the list of shares, the bar's orientation) are exported
// so they can be tested without a page.

import { app } from './app.js';
import { showList } from './popup.js';

// clampSplit keeps a share between 10% and 90% (a missing or broken one is half and half).
export function clampSplit(r) {
  const n = Number(r);
  return Math.max(0.1, Math.min(0.9, Number.isFinite(n) && n !== 0 ? n : 0.5));
}

// SPLIT_CHOICES are the fixed shares the right-click menu offers: the left (or top) panel's part first.
export const SPLIT_CHOICES = [0.5, 0.6, 0.7, 0.8, 0.4, 0.3, 0.2];

// splitItems is the right-click menu: one line per share, "60/40" and so on, the current share marked. vertical: the
// panels are on top of each other, so the labels say top/bottom.
export function splitItems(current, vertical = false) {
  const cur = clampSplit(current);
  return SPLIT_CHOICES.map((r) => {
    const a = Math.round(r * 100), b = 100 - a;
    const on = Math.abs(cur - r) < 0.005;
    return { label: `${on ? '✓ ' : ''}${a}/${b}`, detail: vertical ? 'top/bottom' : 'left/right', value: r };
  });
}

// barOrientation is the separator's aria-orientation: the bar stands upright between panels side by side and lies flat
// between panels on top of each other.
export const barOrientation = (vertical) => (vertical ? 'horizontal' : 'vertical');

export function applySplit(r) {
  r = clampSplit(r);
  app.split = r;
  app.left.el.style.flex = `${r} 1 0`;
  app.right.el.style.flex = `${1 - r} 1 0`;
}

// syncSplitter makes the bar's orientation and tooltip follow the panel arrangement (called after Ctrl+Shift+H).
export function syncSplitter() {
  const bar = document.getElementById('splitter');
  const box = document.getElementById('panels');
  if (!bar || !box) return;
  const vertical = box.classList.contains('horizontal');
  bar.setAttribute('aria-orientation', barOrientation(vertical));
  bar.setAttribute('aria-label', vertical ? 'Panel height' : 'Panel width');
  bar.title = `Drag to change the panel ${vertical ? 'heights' : 'widths'} · double-click for half and half · right-click for more`;
}

export function initSplitter(initial) {
  const bar = document.getElementById('splitter');
  const box = document.getElementById('panels');
  applySplit(initial);
  syncSplitter();
  bar.addEventListener('mousedown', (ev) => {
    if (ev.button !== 0) return;
    ev.preventDefault();
    bar.classList.add('drag');
    const r = box.getBoundingClientRect();
    const vertical = box.classList.contains('horizontal');
    const move = (e) => applySplit(vertical ? (e.clientY - r.top) / r.height : (e.clientX - r.left) / r.width);
    const up = () => {
      bar.classList.remove('drag');
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
      app.saveSession();
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  });
  bar.addEventListener('dblclick', () => { applySplit(0.5); app.saveSession(); });
  bar.addEventListener('contextmenu', async (ev) => {
    ev.preventDefault();
    ev.stopPropagation();
    const vertical = box.classList.contains('horizontal');
    const res = await showList({ title: 'Panel sizes', items: splitItems(app.split, vertical), anchor: { x: ev.clientX, y: ev.clientY }, kind: 'splitter' });
    if (res?.item?.value) { applySplit(res.item.value); app.saveSession(); }
  });
}
