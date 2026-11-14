// Popup lists: the drive list (Alt+F1/Alt+F2), the folder history (Alt+Down), the hotlist (Ctrl+D), right-click menus.
// Keys: Up/Down/PgUp/PgDn/Home/End move, Enter chooses (Shift+Enter too: the caller can open in a new tab), Esc closes,
// a letter jumps to the next item that starts with it (or whose `key` is that letter), and chooses it when it is the
// only one. Control mode reads and chooses
// with currentPopup() / choosePopup().
//
//   const r = await showList({title, items: [{label, detail, value, key, disabled, path, sub, only} | {sep: true}],
//                             anchor, index, filter, left});
//   r → {item, index, shift} or null
//
// Two additions for menus with submenus, both off unless the caller asks (board issue #111), so every other list keys
// exactly as before:
// - Right on an item marked sub: true chooses it (the caller opens the submenu), as Enter does; left: true makes Left
//   close the list the way Esc does (the caller goes back up a level). Without these the arrows do nothing, as before.
// - filter: true turns letters into a filter instead of jumps: what is typed shows under the title, only items whose
//   label or detail have every typed word stay, Backspace takes a letter back and Esc first clears the filter. Items
//   marked only: true (entries of submenus, say) show only while a filter is typed.
// - With filter: true, match: (item, query) => bool replaces the word test above for the items while something is
//   typed (the frecency jump matches folder names its own way), and query: '…' starts the list already filtered.
//
// An item with path: true is a folder path (the folder history, the frequently used folders): when the list is too
// narrow for it, its start is shortened with "…" and its last name stays whole, and the full path shows as a tooltip,
// instead of the row being cut at the right edge behind a horizontal scrollbar (issue #95).

import { esc } from './dialogs.js';
import { waitForUser } from './busy.js';
import { popupArea } from './overlay.js';
import { splitTail } from './fitpath.js';

let current = null;

export const popupOpen = () => !!current;

// popupMatch says whether an item stays in a filtered list: every word of the query is somewhere in its label or its
// detail, ignoring case. An empty query keeps everything but the "only while filtering" items.
export function popupMatch(it, query) {
  if (!it || it.sep) return false;
  const words = String(query || '').toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return !it.only;
  const text = `${it.label ?? ''} ${it.detail ?? ''}`.toLowerCase();
  return words.every((w) => text.includes(w));
}

export function showList({ title = '', items, anchor = null, index = 0, kind = 'list', filter = false, left = false, match = null, query: startQuery = '' }) {
  current?.close(null);
  return waitForUser(new Promise((resolve) => {
    const el = document.createElement('div');
    el.className = 'mc-popup';
    el.setAttribute('role', 'listbox');
    el.innerHTML = (title ? `<div class="t">${esc(title)}</div>` : '') + (filter ? '<div class="q" hidden></div>' : '') + items.map((it, i) => it.sep ? '<div class="sep"></div>'
      : `<div class="it${it.disabled ? ' off' : ''}${it.path ? ' p' : ''}" data-i="${i}" role="option"${it.path ? ` title="${esc(it.label)}"` : ''}>${labelHTML(it)}${it.detail ? `<span class="d">${esc(it.detail)}</span>` : ''}</div>`).join('');
    document.body.append(el);
    let query = filter ? String(startQuery || '') : '';
    const test = match ? (it, q) => (!it || it.sep ? false : String(q).trim() ? !!match(it, q) : !it.only) : popupMatch;
    const shown = (i) => (filter ? test(items[i], query) : true);
    const usable = (i) => items[i] && !items[i].sep && !items[i].disabled && shown(i);
    let at = usable(index) ? index : items.findIndex((_, i) => usable(i));
    // refilter hides the items the typed filter leaves out (and the separators while a filter is typed)
    const refilter = () => {
      const q = el.querySelector('.q');
      q.hidden = !query;
      q.textContent = query ? `Filter: ${query}` : '';
      el.querySelectorAll('.it').forEach((d) => { d.hidden = !shown(+d.dataset.i); });
      el.querySelectorAll('.sep').forEach((d) => { d.hidden = !!query; });
      if (!usable(at)) at = items.findIndex((_, i) => usable(i));
      paint();
    };
    const paint = () => {
      el.querySelectorAll('.it').forEach((d) => d.classList.toggle('on', +d.dataset.i === at));
      el.querySelector('.it.on')?.scrollIntoView({ block: 'nearest' });
    };
    // place: below the anchor element, or at a point, kept on screen (inside the dialog's window when the popup comes
    // from a dialog shown in a window of its own, see overlay.js)
    const A = popupArea(), aw = A.right - A.left, ah = A.bottom - A.top;
    const r = anchor instanceof Element ? anchor.getBoundingClientRect() : anchor ? { left: anchor.x, bottom: anchor.y, top: anchor.y } : { left: A.left + aw / 2 - 150, bottom: A.top + ah / 3, top: A.top + ah / 3 };
    const w = el.offsetWidth, h = el.offsetHeight;
    el.style.left = `${Math.max(A.left + 6, Math.min(A.right - w - 6, r.left))}px`;
    el.style.top = `${r.bottom + h + 6 < A.bottom ? r.bottom + 2 : Math.max(A.top + 6, r.top - h - 2)}px`;
    if (filter) refilter(); else paint();

    const close = (v) => {
      if (current !== api) return;
      current = null;
      el.remove();
      window.removeEventListener('keydown', key, true);
      window.removeEventListener('mousedown', outside, true);
      resolve(v);
    };
    const choose = (i, shift = false) => { if (usable(i)) close({ item: items[i], index: i, shift }); };
    const move = (d) => {
      let i = at;
      for (let n = 0; n < items.length; n++) {
        i = Math.max(0, Math.min(items.length - 1, i + d));
        if (usable(i)) { at = i; break; }
        if (i === 0 || i === items.length - 1) break;
      }
      paint();
    };
    const key = (ev) => {
      ev.stopImmediatePropagation();
      const k = ev.key;
      if (k === 'Escape' && filter && query) { ev.preventDefault(); query = ''; refilter(); return; }
      if (k === 'Escape' || (k === 'F4' && ev.altKey)) { ev.preventDefault(); close(null); return; }
      if (k === 'ArrowRight' && usable(at) && items[at].sub) { ev.preventDefault(); choose(at); return; }
      if (k === 'ArrowLeft' && left) { ev.preventDefault(); close(null); return; }
      if (filter && k === 'Backspace') { ev.preventDefault(); if (query) { query = query.slice(0, -1); refilter(); } return; }
      if (filter && k.length === 1 && !ev.ctrlKey && !ev.metaKey && !ev.altKey) { ev.preventDefault(); query += k; refilter(); return; }
      if (k === 'Enter') { ev.preventDefault(); choose(at, ev.shiftKey); return; }
      if (k === 'ArrowDown') { ev.preventDefault(); move(1); return; }
      if (k === 'ArrowUp') { ev.preventDefault(); move(-1); return; }
      if (k === 'PageDown') { ev.preventDefault(); move(8); return; }
      if (k === 'PageUp') { ev.preventDefault(); move(-8); return; }
      if (k === 'Home') { ev.preventDefault(); at = -1; move(1); return; }
      if (k === 'End') { ev.preventDefault(); at = items.length; move(-1); return; }
      if (k.length === 1 && !ev.ctrlKey && !ev.metaKey) {
        ev.preventDefault();
        const c = (ev.code?.startsWith('Key') ? ev.code.slice(3) : k).toLowerCase();
        const n = items.length;
        // the only item with that letter (or accelerator) is chosen at once, like a drive letter in Total Commander
        const hits = items.map((it, i) => i).filter((i) => usable(i) && ((items[i].key || '').toLowerCase() === c || String(items[i].label).toLowerCase().startsWith(c)));
        if (hits.length === 1) { choose(hits[0]); return; }
        // an accelerator letter chooses its item at once, before any label-prefix cycling: the highlighted item
        // if it is the one, otherwise the next item (from the highlight on) with that accelerator
        const isKey = (i) => usable(i) && (items[i].key || '').toLowerCase() === c;
        if (isKey(at)) { choose(at); return; }
        for (let j = 1; j <= n; j++) {
          const i = (at + j) % n;
          if (isKey(i)) { at = i; paint(); choose(i); return; }
        }
        // no accelerator: move the highlight to the next item whose label starts with the letter
        for (let j = 1; j <= n; j++) {
          const i = (at + j) % n;
          if (usable(i) && String(items[i].label).toLowerCase().startsWith(c)) { at = i; paint(); return; }
        }
      }
    };
    const outside = (ev) => { if (!el.contains(ev.target)) close(null); };
    el.addEventListener('mousemove', (ev) => { const d = ev.target.closest('.it'); if (d && usable(+d.dataset.i) && at !== +d.dataset.i) { at = +d.dataset.i; paint(); } });
    el.addEventListener('click', (ev) => { const d = ev.target.closest('.it'); if (d) choose(+d.dataset.i, ev.shiftKey); });
    window.addEventListener('keydown', key, true);
    setTimeout(() => window.addEventListener('mousedown', outside, true), 0);
    const api = { close, choose, kind, title, items, get at() { return at; }, get query() { return query; } };
    current = api;
  }));
}

// labelHTML draws an item's label: a path item as two parts, the start (which CSS may shorten with "…") and the last
// name (which it never shortens).
export function labelHTML(it) {
  if (!it.path) return `<span>${esc(it.label)}</span>`;
  const [head, tail] = splitTail(it.label);
  return `<span class="pl"><span class="ph">${esc(head)}</span><span class="pt">${esc(tail)}</span></span>`;
}

// currentPopup describes the open popup for control mode.
export function currentPopup() {
  if (!current) return null;
  return { kind: current.kind, title: current.title, at: current.at, filter: current.query, items: current.items.map((it) => (it.sep ? { sep: true } : { label: it.label, detail: it.detail || '', disabled: !!it.disabled })) };
}

// choosePopup picks an item by index or by label (exact, or the first that starts with it); null closes the popup.
export function choosePopup(which, shift = false) {
  if (!current) throw new Error('no popup is open');
  if (which === null || which === undefined) { current.close(null); return; }
  let i = typeof which === 'number' ? which : current.items.findIndex((it) => it.label === which);
  if (i < 0 && typeof which === 'string') i = current.items.findIndex((it) => String(it.label).toLowerCase().startsWith(which.toLowerCase()));
  if (i < 0 || !current.items[i] || current.items[i].sep) throw new Error(`the popup has no item ${JSON.stringify(which)}`);
  current.choose(i, shift);
}

export const closePopup = () => current?.close(null);
