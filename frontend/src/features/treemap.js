// The folder size treemap (board #24, F-872): "Folder size treemap" (cm_ShowTreemap, ours, in the Show menu, also on
// the command line and in the command palette) replaces the file list of the active panel with a picture of its
// folder: one block per file and folder, its area proportional to its size. Folder sizes are counted in the
// background with the same engine as Space and Alt+Shift+Enter (tools/space.js over internal/dirsize), and the blocks
// grow while the count runs. Folders that were already counted (Space, Alt+Shift+Enter, an earlier treemap) are not
// counted again, and every size the treemap counts goes into the panel's size column, as Alt+Shift+Enter's would.
//
// Clicking a folder block (or Enter on the highlighted one) shows that folder's treemap; Backspace or the "Up" button
// goes to the parent; clicking a file block puts the cursor on that file and returns to the list. Esc returns to the
// list (stopping a count still running), with the cursor on the last highlighted block. The arrow keys, Home and End
// walk the blocks from the biggest to the smallest. While the treemap is open the sizes of folders it has counted are
// remembered by path, so going into a folder and back does not count again; they are forgotten when it closes.
//
// Additive only: nothing changes until the command is run, and closing it leaves the panel as it was. Folders that
// are not on this computer (archives, servers) show their files; their folders cannot be counted. Control mode:
// "treemap" ({side}) returns whether the treemap is open in a panel, its folder, what is still counted, and the blocks
// with their sizes and places. The layout and the choice of blocks are pure (treemap-lib.js).

import './treemap.css';
import { command } from '../core/registry.js';
import { addKeyHandler } from '../core/keyboard.js';
import { dialogOpen, esc } from '../core/dialogs.js';
import { cmdline } from '../core/cmdline.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { size, plural } from '../core/format.js';
import { countSizes, stopCounting, counting } from './tools/space.js';
import { isLocal } from './tools/common.js';
import { treemapItems, squarify, blockHue, labelFits, share, stepIndex } from './treemap-lib.js';

const OWNER = 'treemap';
const open = new Map(); // panel → view state {el, area, head, blocks, rects, hot, path, cache, ro}

const isOpen = (p) => open.has(p);

// sizeOf / doneOf: what the treemap knows about an entry's size, from the panel (the size column) and the remembered
// counts of this treemap.
const sizeOf = (p) => (e) => (e.dir ? (e.dirSize ?? p.dirSizes.get(e.name) ?? null) : e.size);
const doneOf = (p) => (e) => !e.dir || p.dirSizes.has(e.name);

// remember copies the folder sizes the panel knows now into the treemap's memory, by full path.
function remember(p, v) {
  for (const [name, n] of p.dirSizes) v.cache.set(p.fullPath(name), n);
}

// prefill gives the panel the sizes the treemap remembers for its folders, so they are neither counted again nor shown
// as <DIR>.
function prefill(p, v) {
  let changed = false;
  for (const e of p.entries) {
    if (!e.dir || e.up || p.dirSizes.has(e.name)) continue;
    const n = v.cache.get(p.fullPath(e.name));
    if (n == null) continue;
    p.dirSizes.set(e.name, n);
    e.dirSize = n;
    changed = true;
  }
  if (changed) { p.renderRows(); p.renderStatus(); }
}

// startCount counts the folders of the panel whose size is not known yet, unless a count of this folder runs already.
function startCount(app, p, v) {
  if (!isLocal(app, p.path) || counting(OWNER, p)) return;
  const missing = p.entries.filter((e) => e.dir && !e.up && !p.dirSizes.has(e.name));
  if (!missing.length) return;
  const path = p.path;
  countSizes(app, p, missing, { owner: OWNER, hint: 'Esc returns to the list', onUpdate: () => { if (open.get(p) === v && p.path === path) draw(app, p, v); } })
    .then(() => {
      if (open.get(p) !== v) return;
      if (p.path === path) { remember(p, v); draw(app, p, v); }
      else startCount(app, p, v); // the panel went to another folder while this count was being stopped: count that one
    });
}

// draw lays the blocks out for the panel's current entries and size of the area, and writes the header line.
function draw(app, p, v) {
  const { items, pending, empty, total, others } = treemapItems(p.entries, { sizeOf: sizeOf(p), doneOf: doneOf(p) });
  const w = v.area.clientWidth, h = v.area.clientHeight;
  const rects = squarify(items, { x: 0, y: 0, w, h });
  const byKey = new Map(items.map((it) => [it.key, it]));
  v.items = items;
  v.rects = rects;
  const hotKey = v.hot >= 0 ? v.blocks?.[v.hot]?.key : null;
  v.blocks = rects.map((r) => ({ ...r, ...byKey.get(r.key) }));
  v.hot = hotKey != null ? v.blocks.findIndex((b) => b.key === hotKey) : -1;
  v.path = p.path;
  v.area.innerHTML = v.blocks.map((b, i) => {
    const hue = blockHue(b);
    const cls = ['mc-tm-b', b.dir ? 'dir' : 'file', b.counting ? 'counting' : '', b.others ? 'others' : '', i === v.hot ? 'hot' : '', hue == null ? 'grey' : ''].filter(Boolean).join(' ');
    const style = `left:${b.x.toFixed(1)}px;top:${b.y.toFixed(1)}px;width:${Math.max(0, b.w).toFixed(1)}px;height:${Math.max(0, b.h).toFixed(1)}px${hue == null ? '' : `;--h:${hue}`}`;
    const tip = `${b.name}${b.dir ? '/' : ''} — ${size(b.value)}${b.counting ? ' so far' : ''} (${share(b.value, total)})`;
    const label = labelFits(b) ? `<span class="n">${esc(b.name)}${b.dir ? '/' : ''}</span>${b.h >= 34 ? `<span class="s">${esc(size(b.value))}</span>` : ''}` : '';
    return `<div class="${cls}" data-i="${i}" style="${style}" title="${esc(tip)}">${label}</div>`;
  }).join('');
  if (!items.length) {
    v.area.innerHTML = `<div class="mc-tm-empty">${pending.length ? 'Counting folder sizes…' : 'Nothing here takes any space.'}</div>`;
  }
  const notes = [];
  if (pending.length) notes.push(isLocal(app, p.path) ? `counting ${plural(pending.length, 'folder')}…` : `${plural(pending.length, 'folder')} not counted (not on this computer)`);
  if (empty.length) notes.push(`${plural(empty.length, 'empty item')} not shown`);
  if (others) notes.push(`${plural(others, 'small item')} in one block`);
  v.head.querySelector('.p').textContent = p.path;
  v.head.querySelector('.t').textContent = `${size(total)}${notes.length ? ' · ' + notes.join(', ') : ''}`;
  v.head.querySelector('.up').disabled = !p.parent;
}

// highlight marks block i and puts the panel's cursor on its entry, so Esc lands there.
function highlight(p, v, i) {
  v.hot = i;
  v.area.querySelectorAll('.mc-tm-b.hot').forEach((el) => el.classList.remove('hot'));
  const b = v.blocks[i];
  if (!b) return;
  v.area.querySelector(`[data-i="${i}"]`)?.classList.add('hot');
  if (!b.others) p.focusName?.(b.name);
}

// enter opens block i: a folder shows its treemap, a file goes back to the list with the cursor on it.
async function enter(app, p, v, i) {
  const b = v.blocks[i];
  if (!b || b.others) return;
  app.setActive(p);
  if (b.dir) {
    remember(p, v);
    stopCounting(OWNER, p);
    v.hot = -1;
    await p.go(p.fullPath(b.name));
  } else {
    closeTreemap(app, p);
    p.focusName?.(b.name);
  }
}

// goUp shows the treemap of the parent folder.
async function goUp(app, p, v) {
  if (!p.parent) return;
  remember(p, v);
  stopCounting(OWNER, p);
  v.hot = -1;
  await p.go(p.parent);
}

// openTreemap shows the treemap in panel p (again: it is drawn anew).
export function openTreemap(app, p = app.cur) {
  let v = open.get(p);
  if (!v) {
    const el = document.createElement('div');
    el.className = 'mc-treemap';
    el.innerHTML = `<div class="mc-tm-head"><button type="button" class="up" title="Parent folder (Backspace)">Up</button>
      <span class="p"></span><span class="t"></span><button type="button" class="close" title="Back to the list (Esc)">List</button></div>
      <div class="mc-tm-area" role="img" aria-label="Folder size treemap"></div>`;
    v = { el, head: el.querySelector('.mc-tm-head'), area: el.querySelector('.mc-tm-area'), blocks: [], rects: [], items: [], hot: -1, path: '', cache: new Map() };
    p.el.insertBefore(el, p.$.status);
    p.el.classList.add('mc-treemap-on');
    open.set(p, v);
    const vv = v;
    v.head.querySelector('.up').onclick = () => goUp(app, p, vv);
    v.head.querySelector('.close').onclick = () => closeTreemap(app, p);
    v.area.addEventListener('mousedown', (ev) => { app.setActive(p); const i = Number(ev.target.closest('.mc-tm-b')?.dataset.i); if (i >= 0) highlight(p, vv, i); });
    v.area.addEventListener('click', (ev) => { const i = Number(ev.target.closest('.mc-tm-b')?.dataset.i); if (i >= 0) enter(app, p, vv, i); });
    let raf = 0;
    v.ro = new ResizeObserver(() => { if (!raf) raf = requestAnimationFrame(() => { raf = 0; if (open.get(p) === vv) draw(app, p, vv); }); });
    v.ro.observe(v.area);
  }
  remember(p, v);
  prefill(p, v);
  draw(app, p, v);
  startCount(app, p, v);
  return v;
}

// closeTreemap returns panel p to its file list and stops what the treemap was counting there.
export function closeTreemap(app, p = app.cur) {
  const v = open.get(p);
  if (!v) return false;
  stopCounting(OWNER, p);
  v.ro?.disconnect();
  v.el.remove();
  open.delete(p);
  p.el.classList.remove('mc-treemap-on');
  p.renderRows();
  p.renderStatus();
  if (p === app.cur) p.$.list.focus?.();
  return true;
}

command({
  name: 'cm_ShowTreemap', label: 'Folder size treemap', menu: 'Show', menuAfter: 'cm_SwitchHidSys',
  checked: (app) => isOpen(app.cur),
  run: (app) => { if (isOpen(app.cur)) closeTreemap(app, app.cur); else openTreemap(app, app.cur); },
});

export function init(app) {
  // a new folder in a panel with the treemap open (a block clicked, Backspace, any other way of changing folder) shows
  // that folder's treemap; a reread or re-sort of the same folder draws it again
  app.on('panel', (p, info) => {
    const v = open.get(p);
    if (!v) return;
    if (info?.moved || p.path !== v.path) { v.hot = -1; prefill(p, v); }
    draw(app, p, v);
    startCount(app, p, v);
  });
  // the keys of the treemap, in the active panel only; every other key works as usual
  addKeyHandler((ev, combo) => {
    const p = app.cur;
    const v = p && open.get(p);
    if (!v || dialogOpen() || ev.target?.id === 'cmd') return false;
    switch (combo) {
      case 'Esc': closeTreemap(app, p); return true;
      case 'Backspace': if (cmdline.text.length) return false; goUp(app, p, v); return true;
      case 'Enter': if (cmdline.text.length || v.hot < 0) return false; enter(app, p, v, v.hot); return true;
      case 'Up': case 'Down': case 'Left': case 'Right': case 'Home': case 'End': {
        if (cmdline.text.length && (combo === 'Left' || combo === 'Right' || combo === 'Home' || combo === 'End')) return false;
        const i = stepIndex(v.hot, v.blocks.length, combo);
        if (i >= 0) highlight(p, v, i);
        return true;
      }
      default: return false;
    }
  });
}

addControlCommand('treemap', async (m) => {
  const p = theApp.panel(m?.side || 'active');
  const v = p && open.get(p);
  if (!v) return { ok: true, open: false, side: p?.side };
  return {
    ok: true, open: true, side: p.side, path: p.path, counting: counting(OWNER, p), hot: v.hot,
    blocks: v.blocks.map((b) => ({ name: b.name, bytes: b.value, dir: b.dir, counting: !!b.counting, others: !!b.others, x: Math.round(b.x), y: Math.round(b.y), w: Math.round(b.w), h: Math.round(b.h) })),
  };
});
