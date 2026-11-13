// Shared pieces of the Synchronize folders and Compare by content windows (FEATURES.md sections 25 and 26): a big window
// on top of the panels built on the core's modal(), so control mode sees and answers it like any dialog, with keys of
// its own; and a small right-click menu that is a dialog too (kind "menu"), so it can be read and answered the same way.
//
// The core's modal() swallows every key while it is the top dialog. A window here needs its own keys (arrows in the
// result list, F8 for the next difference…), so it listens on the window in the capture phase *before* the modal is
// made: its listener runs first and keeps the keys it handles; the rest (Tab, typing in fields) goes on as usual.

import './synccompare-ui.css';
import { modal, esc } from '../core/dialogs.js';
import { comboFromEvent } from '../core/keys.js';

const topModal = () => { const all = document.querySelectorAll('.mc-modal'); return all[all.length - 1] || null; };

// openWindow shows a window. build(box, done) fills it; onKey(ev, combo) returns true for keys it handled. The promise
// resolves with what done() got (cancelValue for Esc, unless onKey keeps Esc).
export function openWindow({ kind, cls = '', build, onKey, cancelValue = null }) {
  let box = null;
  const key = (ev) => {
    if (!box || topModal() !== box.parentElement) return;
    const combo = comboFromEvent(ev);
    if (!combo) return;
    if (onKey?.(ev, combo)) { ev.preventDefault(); ev.stopImmediatePropagation(); }
  };
  window.addEventListener('keydown', key, true);
  return modal((b, done) => {
    box = b;
    box.classList.add('mc-sc-win');
    if (cls) box.classList.add(cls);
    build(box, done);
  }, cancelValue, { kind }).finally(() => window.removeEventListener('keydown', key, true));
}

// contextMenu shows a small menu at a point ({x, y}) and resolves with the chosen item's value, or null. Items:
// {label, value, disabled} or '-' for a line. Up/Down move, Enter chooses, Esc closes.
export function contextMenu(items, at, title = '') {
  let box = null;
  const buttons = () => [...box.querySelectorAll('button:not(:disabled)')];
  const key = (ev) => {
    if (!box || topModal() !== box.parentElement) return;
    const combo = comboFromEvent(ev);
    if (combo !== 'Up' && combo !== 'Down' && combo !== 'Home' && combo !== 'End') return;
    ev.preventDefault();
    ev.stopImmediatePropagation();
    const bs = buttons();
    if (!bs.length) return;
    let i = bs.indexOf(document.activeElement);
    if (combo === 'Up') i = i <= 0 ? bs.length - 1 : i - 1;
    else if (combo === 'Down') i = i >= bs.length - 1 ? 0 : i + 1;
    else i = combo === 'Home' ? 0 : bs.length - 1;
    bs[i].focus();
  };
  window.addEventListener('keydown', key, true);
  return modal((b, done) => {
    box = b;
    box.classList.add('mc-sc-menu');
    box.innerHTML = (title ? `<h2>${esc(title)}</h2>` : '') + items.map((it, i) => it === '-' ? '<hr>'
      : `<button type="button" data-i="${i}"${it.disabled ? ' disabled' : ''}>${esc(it.label)}</button>`).join('');
    box.onclick = (ev) => { const btn = ev.target.closest('button[data-i]'); if (btn) done(items[+btn.dataset.i].value); };
    box.onsubmit = (ev) => { ev.preventDefault(); const f = document.activeElement?.closest?.('button[data-i]'); if (f) done(items[+f.dataset.i].value); };
    const back = box.parentElement;
    back.classList.add('mc-sc-menuback');
    back.addEventListener('mousedown', (ev) => { if (ev.target === ev.currentTarget) done(null); });
    requestAnimationFrame(() => {
      const w = box.offsetWidth, h = box.offsetHeight;
      box.style.left = `${Math.max(6, Math.min(innerWidth - w - 6, at?.x ?? innerWidth / 2 - w / 2))}px`;
      box.style.top = `${Math.max(6, Math.min(innerHeight - h - 6, at?.y ?? innerHeight / 3))}px`;
    });
  }, null, { kind: 'menu' }).finally(() => window.removeEventListener('keydown', key, true));
}

// joinPath puts a slash-separated relative path under a folder with the folder's separator.
export function joinPath(dir, rel, sep = '/') {
  const r = sep === '/' ? rel : rel.split('/').join(sep);
  if (!r) return dir;
  return dir.endsWith(sep) || dir.endsWith('/') ? dir + r : dir + sep + r;
}
