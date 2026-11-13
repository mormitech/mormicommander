// The bar between the panels (F-008): drag to share the width (or the height, with the panels on top of each other),
// double-click for half and half. The share is kept in the session.

import { app } from './app.js';

export function applySplit(r) {
  r = Math.max(0.1, Math.min(0.9, r || 0.5));
  app.split = r;
  app.left.el.style.flex = `${r} 1 0`;
  app.right.el.style.flex = `${1 - r} 1 0`;
}

export function initSplitter(initial) {
  const bar = document.getElementById('splitter');
  const box = document.getElementById('panels');
  applySplit(initial);
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
}
