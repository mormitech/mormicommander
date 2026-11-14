// Folder tabs (FEATURES.md section 2, behaviour from docs/spec area-2 section 6 and area-7 section 3.8): a tab bar
// above each panel's path bar; new, close, close all, next/previous, activate by number, lock (two kinds), rename,
// duplicate and copy/move to the other side, reorder by dragging, reopen a closed tab, save and load .tab files,
// favourite tab sets, and the tabs of both panels coming back after a restart (saved by feature_tabshotlist.go through
// internal/config, in TC's [lefttabs]/[righttabs] form; tabs-saved.js maps a tab to what is saved and back).
//
// How a tab keeps its state: the active tab of a side *is* the panel (its folder, sort, history, cursor, selection all
// live in the Panel object). When another tab becomes active, the panel's fields (Panel.STATE_KEYS, so features that add
// per-panel state are included) are copied into the tab being left and the new tab's copy is put back, then the folder
// is read again. Back/forward (Alt+Left/Right) is therefore per tab (F-047), and so is the sort (F-117). Features
// whose per-panel state is an object they change in place (views: the view mode, issue #128) register it with
// addTabState, so the tab being left keeps its object and the panel goes on with a copy, and it is saved with the tab.
//
// Locked tabs: each panel's go() is wrapped once at start; in a locked tab a change to another folder opens it in a new
// tab instead (or reuses a tab, per the "lockedChange" option), whatever asked for it: Enter, Backspace, the hotlist,
// the history, the drive bar. "Locked, but folder changes allowed" tabs change folder in place and can go back to their
// locked folder when they are activated again.
//
// Options live in the settings key "folderTabs" (the Folder tabs settings dialog, cm_DirTabsConfig).

import './tabs-hotlist.css';
import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { showList } from '../core/popup.js';
import { messageDialog, inputDialog, formDialog, modal, esc } from '../core/dialogs.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { Panel } from '../core/panel.js';
import { TAB_DEFAULTS } from './tabs-defaults.js';
import { tabFields, lockField } from './tabs-saved.js';

export const LOCK_NONE = 0, LOCK_FULL = 1, LOCK_DIRS = 2;

// ---- options ----

// The defaults are in tabs-defaults.js (shared with the Options dialog's Folder tabs page, board issue #94).
const DEFAULTS = TAB_DEFAULTS;
export const tabOpt = (k) => { const o = theApp.setting('folderTabs', {}) || {}; return o[k] === undefined ? DEFAULTS[k] : o[k]; };
const setOpts = (patch) => { theApp.setSetting('folderTabs', { ...(theApp.setting('folderTabs', {}) || {}), ...patch }); renderAll(); theApp.emit('settings'); };

// ---- the model ----

const sides = { left: null, right: null }; // {tabs: [tab], active: index}; null until the tabs were restored
const closedTabs = [];                      // for cm_ReopenClosedTab, newest last
const origGo = new WeakMap();                // panel → its own go(), without the lock rule
const lockedHistory = new WeakMap();         // panel → {back, fwd} of a locked tab, put back when a change is redirected
let nextId = 1;
let favIndex = -1;                           // for cm_NextFavoriteTab / cm_PrevFavoriteTab

// tab: {id, path, caption, lock, lockPath, sort, state, saved}; state is the panel snapshot of a tab that is not
// active; saved is what the tab was read back from (a restart, a .tab file, favourite tabs, a closed tab), whose fields
// the features of addTabState read when the tab is first shown.
function makeTab(o = {}) {
  return { id: nextId++, path: o.path || '', caption: o.caption || '', lock: o.lock || LOCK_NONE, lockPath: o.lockPath || (o.lock ? o.path : '') || '',
    sort: o.sort ? { ...o.sort } : null, state: o.state || null, saved: o.saved || null };
}

// ---- per-tab state of other features ----

// tabStates: Panel field → {copy(v): the tab's own copy, save(v): the fields saved with the tab, load(saved): the value
// for a tab read back from those fields, or null to keep what the panel has}.
const tabStates = new Map();
export function addTabState(key, hooks) { tabStates.set(key, hooks); }

// forkState gives the panel its own copy of each registered field, so the tab just left keeps the object it had.
function forkState(p) { for (const [k, h] of tabStates) p[k] = h.copy(p[k]); }

// tabExtra is the fields the registered features save with a tab.
function tabExtra(p, t) {
  const out = {};
  for (const [k, h] of tabStates) {
    let v = t === activeTab(p) ? p[k] : t.state && k in t.state ? t.state[k] : undefined;
    if (v === undefined && t.saved) v = h.load(t.saved) ?? undefined;
    if (v !== undefined) Object.assign(out, h.save(v));
  }
  return out;
}

const panelOf = (side) => (side === 'left' ? theApp.left : theApp.right);
const sideOf = (p) => p.side;
const setOf = (p) => sides[sideOf(p)];
export const activeTab = (p) => { const s = setOf(p); return s ? s.tabs[s.active] : null; };
export const isLocked = (p) => (activeTab(p)?.lock || LOCK_NONE) === LOCK_FULL;
const tabPath = (p, t) => (t === activeTab(p) ? p.path : t.state?.path || t.path);
const tabSort = (p, t) => (t === activeTab(p) ? p.sort : t.state?.sort || t.sort || { key: 'name', desc: false });

const trimSep = (x) => (x.length > 1 ? x.replace(/[\\/]+$/, '') : x) || x;
const samePath = (a, b) => {
  if (!a || !b) return false;
  const [x, y] = [trimSep(a), trimSep(b)];
  return theApp.platform === 'linux' ? x === y : x.toLowerCase() === y.toLowerCase();
};

// snapshot copies a panel's state (for the tab being left); clone makes it safe to share with a second tab.
function snapshot(p) {
  const s = {};
  for (const k of Panel.STATE_KEYS) s[k] = p[k];
  s.scrollTop = p.$.list.scrollTop;
  return s;
}
function cloneState(s) {
  const c = { ...s };
  for (const [k, v] of Object.entries(s)) {
    if (v instanceof Map) c[k] = new Map(v);
    else if (v instanceof Set) c[k] = new Set();
    else if (Array.isArray(v) && (k === 'back' || k === 'fwd')) c[k] = [...v];
  }
  if (s.sort) c.sort = { ...s.sort };
  for (const [k, h] of tabStates) if (k in s) c[k] = h.copy(s[k]);
  return c;
}

// caption is what a tab shows: its own name, or the folder name ("/" and "c:" for roots).
function folderName(path) {
  if (!path) return '…';
  if (/^[a-z]:\\?$/i.test(path)) return path.slice(0, 2).toLowerCase();
  const m = /^([a-z][a-z0-9+.-]*:\/\/[^/]+)\/?$/i.exec(path);
  if (m) return m[1];
  const t = trimSep(path);
  return t.split(/[\\/]/).pop() || t;
}
export function tabCaption(p, t, { plain = false } = {}) {
  let c = t.caption || folderName(t.lock && t.lockPath && t.lock === LOCK_DIRS && !t.caption ? t.lockPath : tabPath(p, t));
  if (!plain && tabOpt('limitWidth')) {
    const n = Math.max(3, +tabOpt('widthLimit') || 32);
    if (c.length > n) c = c.slice(0, n - 1) + '…';
  }
  if (!plain && t.lock && tabOpt('starLocked')) c = '*' + c;
  return c;
}

// ---- switching ----

// restore puts a tab's state into its panel and reads the folder again; a tab that has only a path (restored at
// start, from a file, a new tab of another folder) reads it fresh, falling back to the nearest parent that can be read
// and then to the home folder.
async function restore(p, t, opts = {}) {
  let target = t.path;
  if (t.lock === LOCK_DIRS && tabOpt('resetLocked') && t.lockPath) target = t.lockPath;
  const st = t.state;
  t.state = null;
  if (st && samePath(st.path, target) && st.path) {
    for (const k of Panel.STATE_KEYS) if (k in st) p[k] = st[k];
    p.render();
    if (st.scrollTop != null) p.$.list.scrollTop = st.scrollTop;
    p.ensureVisible(false);
    theApp.panelChanged(p, { moved: true });
    if (opts.focus) p.focusName(opts.focus);
    return reread(p);
  }
  if (st) {
    // (a tab going back to its locked folder keeps its history and sort, not what it showed)
    const skip = new Set(['path', 'parent', 'all', 'entries', 'index', 'cursor', 'selected', 'dirSizes', 'filter', 'filterLabel', 'error']);
    for (const k of Panel.STATE_KEYS) if (k in st && !skip.has(k)) p[k] = st[k];
  } else {
    p.back = []; p.fwd = []; p.lastCursor = new Map();
    if (t.saved) for (const [k, h] of tabStates) { const v = h.load(t.saved); if (v != null) p[k] = v; }
  }
  p.selected = new Set();
  p.filter = null; p.filterLabel = '';
  p.dirSizes = new Map();
  if (t.sort) p.sort = { ...t.sort };
  return openFresh(p, target, opts);
}

async function openFresh(p, target, opts = {}) {
  const goNow = origGo.get(p) || p.go.bind(p);
  let at = target;
  for (let n = 0; at && n < 40; n++) {
    if (await goNow(at, { history: false, focus: opts.focus ?? null })) return true;
    let up = '';
    try { up = await go.FS.Parent(at); } catch { up = ''; }
    if (!up || samePath(up, at)) break;
    at = up;
  }
  return goNow(theApp.home, { history: false });
}

// reread refreshes a restored tab without losing the cursor, like TC rereads a folder when its tab comes back.
function reread(p) { return p.reload(); }

// activate makes tab i of a side the active one.
export async function activate(side, i, opts = {}) {
  const s = sides[side];
  if (!s || i < 0 || i >= s.tabs.length) return false;
  const p = panelOf(side);
  if (i === s.active) { if (opts.focus) p.focusName(opts.focus); render(side); return true; }
  const cur = s.tabs[s.active];
  if (cur) { cur.state = snapshot(p); cur.path = p.path; cur.sort = { ...p.sort }; forkState(p); }
  s.active = i;
  render(side);
  const ok = await restore(p, s.tabs[i], opts);
  render(side);
  saveSoon();
  return ok;
}

// newTab adds a tab to a side: with the panel's own state (o.same), or for a folder (o.path). It becomes the active
// tab unless o.background.
export async function newTab(side, o = {}) {
  const s = sides[side];
  if (!s) return false;
  const p = panelOf(side);
  const t = makeTab({ path: o.path || p.path, caption: o.caption, lock: o.lock, lockPath: o.lockPath, sort: o.sort || { ...p.sort }, saved: o.saved });
  if (o.same) { t.state = cloneState(snapshot(p)); t.state.path = p.path; t.path = p.path; }
  else if (o.state) t.state = o.state;
  const at = tabOpt('nextToCurrent') ? s.active + 1 : s.tabs.length;
  s.tabs.splice(at, 0, t);
  if (o.background) { render(side); saveSoon(); return true; }
  return activate(side, at, { focus: o.focus });
}

// openInNewTab opens a folder in a new tab of a panel (the hotlist and history use it for Shift+Enter, F-777).
export function openInNewTab(p, path, o = {}) { return newTab(p.side, { path, ...o }); }

// closeTab closes tab i; the last tab of a side never closes; a locked tab asks first.
export async function closeTab(side, i, { force = false } = {}) {
  const s = sides[side];
  if (!s || s.tabs.length <= 1 || i < 0 || i >= s.tabs.length) return false;
  const p = panelOf(side);
  const t = s.tabs[i];
  if (t.lock && !force) {
    const a = await messageDialog({ Title: 'Close tab', Message: `The tab ${tabCaption(p, t, { plain: true })} is locked. Close it anyway?`, Buttons: ['Close', 'Cancel'], Kind: 'tab-close-locked' });
    if (a !== 'Close') return false;
  }
  closedTabs.push({ side, path: tabPath(p, t), caption: t.caption, lock: t.lock, lockPath: t.lockPath, sort: { ...tabSort(p, t) }, saved: tabExtra(p, t) });
  if (closedTabs.length > 30) closedTabs.shift();
  const wasActive = i === s.active;
  s.tabs.splice(i, 1);
  if (wasActive) {
    // the tab to its right comes next, or the one to its left when it was the last
    s.active = -1;
    const next = Math.min(i, s.tabs.length - 1);
    s.active = next;
    render(side);
    await restore(p, s.tabs[next]);
  } else if (i < s.active) s.active--;
  render(side);
  saveSoon();
  return true;
}

// closeOthers closes every tab of a side but one (locked tabs stay unless force).
async function closeOthers(side, keep, { ask = true } = {}) {
  const s = sides[side];
  if (!s || s.tabs.length <= 1) return false;
  if (ask && tabOpt('confirmCloseAll')) {
    const a = await messageDialog({ Title: 'Close all tabs', Message: 'Close all tabs except the current one? Locked tabs stay open.', Buttons: ['Close tabs', 'Cancel'], Kind: 'tab-close-all' });
    if (a !== 'Close tabs') return false;
  }
  if (keep !== s.active) await activate(side, keep);
  const p = panelOf(side);
  const keepTab = s.tabs[s.active];
  for (const t of s.tabs) if (t !== keepTab && !t.lock) closedTabs.push({ side, path: tabPath(p, t), caption: t.caption, lock: t.lock, lockPath: t.lockPath, sort: { ...tabSort(p, t) }, saved: tabExtra(p, t) });
  s.tabs = s.tabs.filter((t) => t === keepTab || t.lock);
  s.active = s.tabs.indexOf(keepTab);
  render(side);
  saveSoon();
  return true;
}

// copyToOther puts a copy of tab i into the other panel (after its active tab, which it becomes); move closes it here.
async function copyToOther(side, i, { move = false } = {}) {
  const s = sides[side];
  const p = panelOf(side);
  const t = s.tabs[i];
  if (!t) return false;
  const other = side === 'left' ? 'right' : 'left';
  const st = t === activeTab(p) ? cloneState(snapshot(p)) : t.state ? cloneState(t.state) : null;
  await newTab(other, { path: tabPath(p, t), caption: t.caption, lock: t.lock, lockPath: t.lockPath, sort: { ...tabSort(p, t) }, state: st, saved: tabExtra(p, t) });
  if (move && s.tabs.length > 1) await closeTab(side, s.tabs.indexOf(t), { force: true });
  return true;
}

// replaceTabs makes a side show a saved set of tabs (a .tab file, favourite tabs).
async function replaceTabs(side, set) {
  const s = sides[side];
  if (!s || !set?.tabs?.length) return false;
  s.tabs = set.tabs.map((x) => makeTab(tabFields(x)));
  s.active = Math.max(0, Math.min(set.active || 0, s.tabs.length - 1));
  render(side);
  await restore(panelOf(side), s.tabs[s.active]);
  render(side);
  saveSoon();
  return true;
}

// tabSet is a side's tabs as Go stores them.
function tabSet(side) {
  const s = sides[side], p = panelOf(side);
  if (!s) return { tabs: [], active: 0 };
  return { tabs: s.tabs.map((t) => { const so = tabSort(p, t); const path = tabPath(p, t); return { path, caption: t.caption, lock: t.lock, ...lockField(t.lock, t.lockPath, path), sort: so.key, desc: !!so.desc, ...tabExtra(p, t) }; }), active: s.active };
}

// ---- the lock rule ----

function guardedGo(p, path, opts = {}) {
  const own = origGo.get(p);
  const t = activeTab(p);
  if (!t || t.lock !== LOCK_FULL || !p.path || samePath(theApp.resolvePath(String(path), p.path), p.path)) return own(path, opts);
  // the locked tab keeps its folder and its history (Alt+Left took an entry out of it before asking for the folder)
  const h = lockedHistory.get(p);
  if (h) { p.back = [...h.back]; p.fwd = [...h.fwd]; }
  return redirectLocked(p, path, opts);
}

async function redirectLocked(p, path, opts) {
  const s = setOf(p), side = sideOf(p);
  const mode = +tabOpt('lockedChange') || 0;
  let i = -1;
  if (mode === 1) i = s.tabs.findIndex((t, k) => k !== s.active && samePath(tabPath(p, t), path));
  else if (mode === 2) i = s.tabs.findIndex((t) => !t.lock);
  else if (mode === 3) i = s.tabs.map((t) => !t.lock).lastIndexOf(true);
  if (i >= 0) {
    await activate(side, i);
    if (mode === 1) { if (opts.focus) p.focusName(opts.focus); return true; }
    return origGo.get(p)(path, opts);
  }
  return newTab(side, { path, focus: opts.focus });
}

// ---- drawing ----

let dragging = null; // {side, i} of the tab being dragged

function render(side) {
  const p = panelOf(side), s = sides[side];
  if (!p || !s) return;
  const bar = p.$.tabbar;
  bar.hidden = !tabOpt('show') || (s.tabs.length < 2 && !tabOpt('showOne'));
  bar.classList.add('mc-tabs-bar');
  bar.classList.toggle('mc-tabs-multi', !!tabOpt('multiLine'));
  bar.classList.toggle('mc-tabs-x', !!tabOpt('closeButton'));
  bar.setAttribute('role', 'tablist');
  bar.innerHTML = s.tabs.map((t, i) => `<div class="mc-tabs-tab${i === s.active ? ' on' : ''}${t.lock ? ' locked' : ''}" role="tab" aria-selected="${i === s.active}" draggable="true" data-i="${i}" title="${esc(tabPath(p, t))}${t.lock === LOCK_FULL ? ' (locked)' : t.lock === LOCK_DIRS ? ' (locked, folder changes allowed)' : ''}"><span class="mc-tabs-cap">${esc(tabCaption(p, t))}</span>${s.tabs.length > 1 ? '<button type="button" class="mc-tabs-close" tabindex="-1" aria-label="Close tab" title="Close tab">×</button>' : ''}</div>`).join('')
    + '<button type="button" class="mc-tabs-fav" tabindex="-1" title="Favourite tabs" aria-label="Favourite tabs">☆</button>';
  bar.querySelector('.mc-tabs-tab.on')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
}

export function renderAll() { render('left'); render('right'); }

function wireBar(p) {
  const bar = p.$.tabbar;
  const tabAt = (ev) => ev.target.closest('.mc-tabs-tab');
  bar.addEventListener('mousedown', (ev) => {
    if (ev.button === 1) ev.preventDefault(); // (no autoscroll on middle click)
  });
  bar.addEventListener('click', (ev) => {
    const side = p.side;
    if (ev.target.closest('.mc-tabs-fav')) { ev.stopPropagation(); theApp.setActive(p); favoritesMenu({ anchor: ev.target }); return; }
    const el = tabAt(ev);
    if (!el) return;
    const i = +el.dataset.i;
    if (ev.target.closest('.mc-tabs-close')) { ev.stopPropagation(); closeTab(side, i); return; }
    theApp.setActive(p);
    activate(side, i);
  });
  bar.addEventListener('dblclick', (ev) => {
    if (ev.target.closest('.mc-tabs-close, .mc-tabs-fav')) return;
    const el = tabAt(ev);
    theApp.setActive(p);
    if (!el) newTab(p.side, { same: true });                       // F-041: empty area opens a new tab
    else if (tabOpt('dblClickClose')) closeTab(p.side, +el.dataset.i);
  });
  bar.addEventListener('auxclick', (ev) => {
    const el = tabAt(ev);
    if (ev.button === 1 && el) { ev.preventDefault(); closeTab(p.side, +el.dataset.i); } // F-042
  });
  bar.addEventListener('contextmenu', (ev) => {
    ev.preventDefault();
    const el = tabAt(ev);
    theApp.setActive(p);
    const s = sides[p.side];
    tabMenu(p.side, el ? +el.dataset.i : s.active, { x: ev.clientX, y: ev.clientY });
  });
  bar.addEventListener('wheel', (ev) => {
    const s = sides[p.side];
    if (!s || s.tabs.length < 2 || tabOpt('multiLine')) return;
    ev.preventDefault();
    const d = (ev.deltaY || ev.deltaX) > 0 ? 1 : -1;
    activate(p.side, (s.active + d + s.tabs.length) % s.tabs.length);
  }, { passive: false });
  // dragging a tab: reorder within the bar, or move it to the other panel's bar (Ctrl/Alt copies) (F-039)
  bar.addEventListener('dragstart', (ev) => {
    const el = tabAt(ev);
    if (!el) return;
    dragging = { side: p.side, i: +el.dataset.i };
    ev.dataTransfer.effectAllowed = 'copyMove';
    ev.dataTransfer.setData('application/x-mc-tab', JSON.stringify(dragging));
  });
  bar.addEventListener('dragend', () => { dragging = null; bar.querySelectorAll('.drop').forEach((x) => x.classList.remove('drop')); });
  bar.addEventListener('dragover', (ev) => {
    if (!dragging) return;
    ev.preventDefault();
    ev.dataTransfer.dropEffect = dragging.side !== p.side && (ev.ctrlKey || ev.altKey || ev.metaKey) ? 'copy' : 'move';
    bar.querySelectorAll('.drop').forEach((x) => x.classList.remove('drop'));
    tabAt(ev)?.classList.add('drop');
  });
  bar.addEventListener('drop', (ev) => {
    if (!dragging) return;
    ev.preventDefault();
    const from = dragging;
    dragging = null;
    const el = tabAt(ev);
    const to = el ? +el.dataset.i : sides[p.side].tabs.length;
    dropTab(from, p.side, to, ev.ctrlKey || ev.altKey || ev.metaKey);
  });
}

async function dropTab(from, side, to, copy) {
  const src = sides[from.side];
  if (from.side === side) {
    if (to === from.i) return;
    const activeT = src.tabs[src.active];
    const [t] = src.tabs.splice(from.i, 1);
    // dropped on a tab to the right: after it; to the left: before it; on the empty area: at the end
    src.tabs.splice(Math.min(src.tabs.length, to), 0, t);
    src.active = src.tabs.indexOf(activeT);
    render(side);
    saveSoon();
    return;
  }
  await copyToOther(from.side, from.i, { move: !copy });
}

// ---- menus and dialogs ----

const check = (on, label) => (on ? '✓ ' : ' ') + label;

async function tabMenu(side, i, anchor) {
  const s = sides[side];
  const p = panelOf(side);
  const t = s?.tabs[i];
  if (!t) return false;
  const many = s.tabs.length > 1;
  const items = [
    { label: 'Close tab', value: 'close', disabled: !many },
    { label: 'Close all other tabs', value: 'others', disabled: !many },
    { sep: true },
    { label: check(t.lock === LOCK_FULL, 'Lock tab'), value: 'lock' },
    { label: check(t.lock === LOCK_DIRS, 'Locked, but folder changes allowed'), value: 'lockdirs' },
    { label: 'Rename or lock tab…', value: 'rename' },
    { sep: true },
    { label: 'New tab', value: 'new' },
    { label: 'Duplicate tab', value: 'dup' },
    { label: 'Copy tab to other side', value: 'copy' },
    { label: 'Move tab to other side', value: 'move', disabled: !many },
    { label: 'Reopen closed tab', value: 'reopen', disabled: !closedTabs.length },
    { sep: true },
    { label: 'Save tabs to file…', value: 'save' },
    { label: 'Load tabs from file…', value: 'load' },
    { label: 'Favourite tabs ▸', value: 'fav' },
    { sep: true },
    { label: 'Folder tab settings…', value: 'config' },
  ];
  const r = await showList({ title: tabCaption(p, t, { plain: true }), items, anchor, kind: 'tab-menu' });
  if (!r) return false;
  switch (r.item.value) {
    case 'close': return closeTab(side, i);
    case 'others': return closeOthers(side, i);
    case 'lock': return setLock(side, i, t.lock === LOCK_FULL ? LOCK_NONE : LOCK_FULL);
    case 'lockdirs': return setLock(side, i, t.lock === LOCK_DIRS ? LOCK_NONE : LOCK_DIRS);
    case 'rename': return renameTab(side, i);
    case 'new': return newTab(side, { same: true });
    case 'dup': { if (i !== s.active) await activate(side, i); return newTab(side, { same: true, lock: t.lock, lockPath: t.lockPath, caption: t.caption }); }
    case 'copy': return copyToOther(side, i);
    case 'move': return copyToOther(side, i, { move: true });
    case 'reopen': return theApp.run('cm_ReopenClosedTab');
    case 'save': return theApp.run('cm_SaveTabs');
    case 'load': return theApp.run('cm_LoadTabs');
    case 'fav': return favoritesMenu({ anchor });
    case 'config': return theApp.run('cm_DirTabsConfig');
  }
  return false;
}

function setLock(side, i, lock) {
  const s = sides[side], p = panelOf(side), t = s?.tabs[i];
  if (!t) return false;
  t.lock = lock;
  t.lockPath = lock ? tabPath(p, t) : '';
  if (i === s.active) lockedHistory.set(p, { back: [...p.back], fwd: [...p.fwd] });
  render(side);
  saveSoon();
  return true;
}

// renameTab: TC's "Rename/lock tab" dialog (F-037): a caption (empty = the folder name) and the lock mode.
async function renameTab(side, i) {
  const s = sides[side], p = panelOf(side), t = s?.tabs[i];
  if (!t) return false;
  const name = t.caption || folderName(tabPath(p, t));
  const radio = (id, v, label) => `<label class="mc-check"><input type="radio" name="tab-lock" id="${id}" value="${v}"${t.lock === v ? ' checked' : ''}> ${label}</label>`;
  const r = await formDialog('Rename or lock tab',
    `<div class="mc-row"><label for="tab-name">New name (empty: the folder name)</label><input type="text" id="tab-name" spellcheck="false" value="${esc(name)}" autofocus data-select="0,${name.length}"></div>
     ${radio('lock-none', LOCK_NONE, 'Not locked')}${radio('lock-full', LOCK_FULL, 'Locked')}${radio('lock-dirs', LOCK_DIRS, 'Locked, but folder changes allowed')}`,
    (box) => {
      const v = box.querySelector('#tab-name').value.trim();
      const lock = +(box.querySelector('input[name=tab-lock]:checked')?.value ?? LOCK_NONE);
      return { caption: v === folderName(tabPath(p, t)) ? '' : v, lock };
    }, null, { kind: 'tab-rename' });
  if (!r) return false;
  t.caption = r.caption;
  if (r.lock !== t.lock) setLock(side, i, r.lock);
  render(side);
  saveSoon();
  return true;
}

// Folder tab settings (TC's Configuration > Folder tabs page, cm_DirTabsConfig, F-046, F-432).
async function configDialog() {
  const box = (id, label) => `<label class="mc-check"><input type="checkbox" id="tabs-${id}"${tabOpt(id) ? ' checked' : ''}> ${label}</label>`;
  const lc = +tabOpt('lockedChange') || 0;
  const radio = (v, label) => `<label class="mc-check"><input type="radio" name="tabs-locked" id="tabs-locked-${v}" value="${v}"${lc === v ? ' checked' : ''}> ${label}</label>`;
  const r = await formDialog('Folder tab settings',
    `<div class="mc-tabs-cfg">${box('show', 'Show folder tabs')}${box('showOne', 'Show the tab bar also when there is only one tab')}
     ${box('confirmCloseAll', 'Confirm closing of all tabs')}${box('dblClickClose', 'Close a tab with a double click')}
     ${box('ctrlUpForeground', 'Ctrl+Up opens the new tab in the foreground')}${box('nextToCurrent', 'Open new tabs next to the current tab')}
     ${box('activateTarget', 'Activate the other panel when opening a tab there (Ctrl+Shift+Up)')}
     ${box('multiLine', 'Tabs on several lines')}${box('starLocked', 'Show locked tabs with a star *')}${box('closeButton', 'Show a close button on tabs')}
     <div class="mc-tabs-inline">${box('limitWidth', 'Limit tab width to')}<input type="text" id="tabs-widthLimit" value="${esc(tabOpt('widthLimit'))}" size="4"> characters</div>
     <p class="mc-note">Locked tabs: when the folder changes in a locked tab</p>
     ${radio(0, 'Open a new tab')}${radio(1, 'Use a tab that already shows that folder')}${radio(2, 'Use the first tab that is not locked')}${radio(3, 'Use the last tab that is not locked')}
     ${box('resetLocked', 'Tabs locked with folder changes allowed go back to their folder when activated again')}</div>`,
    (b) => {
      const out = {};
      for (const k of ['show', 'showOne', 'confirmCloseAll', 'dblClickClose', 'ctrlUpForeground', 'nextToCurrent', 'activateTarget', 'multiLine', 'starLocked', 'closeButton', 'limitWidth', 'resetLocked']) out[k] = b.querySelector(`#tabs-${k}`).checked;
      const n = parseInt(b.querySelector('#tabs-widthLimit').value, 10);
      if (!(n >= 3 && n <= 200)) return 'The tab width must be a number from 3 to 200.';
      out.widthLimit = n;
      out.lockedChange = +(b.querySelector('input[name=tabs-locked]:checked')?.value ?? 0);
      return out;
    }, null, { kind: 'tabs-config' });
  if (!r) return false;
  setOpts(r);
  return true;
}

// ---- saving ----

let saveTimer = 0;
export function saveSoon() {
  if (theApp.demo) return;
  clearTimeout(saveTimer);
  saveTimer = setTimeout(saveNow, 500);
}
function saveNow() {
  if (!sides.left || !sides.right) return;
  const max = +theApp.setting('historyLength', 30) || 30;
  const hist = (p) => [...p.back].reverse();
  go.TabsHotlist.SaveTabs({ left: tabSet('left'), right: tabSet('right'), leftHistory: hist(theApp.left), rightHistory: hist(theApp.right), historyMax: max }).catch(() => {});
}

// ---- .tab files and favourite tabs ----

async function chooseTabFile(save) {
  const dir = await go.TabsHotlist.TabDir().catch(() => '');
  const start = dir ? (save ? `${dir}${theApp.sepFor(dir)}${folderName(theApp.cur.path).replace(/[\\/:*?"<>|]/g, '_') || 'tabs'}.tab` : dir) : '';
  return go.TabsHotlist.ChooseFile(save, save ? 'Save tabs' : 'Load tabs', start, 'tab');
}

export async function loadTabFile(path, p = theApp.cur, { append = false } = {}) {
  const f = await go.TabsHotlist.LoadTabFile(path);
  const side = p.side, other = side === 'left' ? 'right' : 'left';
  if (append) {
    for (const x of f.active.tabs) await newTab(side, { ...tabFields(x), background: true });
    return true;
  }
  await replaceTabs(side, f.active);
  if (f.inactive?.tabs?.length) await replaceTabs(other, f.inactive);
  return true;
}

// saveBothTabs writes a .tab file with both panels' tabs (the hotlist's "Add current tabs", F-045/F-664).
export async function saveBothTabs(path) {
  const side = theApp.cur.side, other = side === 'left' ? 'right' : 'left';
  await go.TabsHotlist.SaveTabFile(path, { active: tabSet(side), inactive: tabSet(other) });
}

async function favoritesMenu({ anchor = null } = {}) {
  const favs = await go.TabsHotlist.Favorites();
  const items = favs.map((f) => ({ label: f.name, detail: `${f.left.tabs.length} + ${f.right.tabs.length} tabs`, value: f }));
  if (items.length) items.push({ sep: true });
  items.push({ label: 'Add current tabs to favourites…', value: 'add' }, { label: 'Configure favourite tabs…', value: 'config' });
  const r = await showList({ title: 'Favourite tabs', items, anchor: anchor || theApp.cur.$.tabbar, kind: 'favorite-tabs' });
  if (!r) return false;
  if (r.item.value === 'add') return theApp.run('cm_SaveFavoriteTabs');
  if (r.item.value === 'config') return theApp.run('cm_ConfigFavoriteTabs');
  return applyFavorite(r.item.value);
}

async function applyFavorite(f) {
  if (f.left?.tabs?.length) await replaceTabs('left', f.left);
  if (f.right?.tabs?.length) await replaceTabs('right', f.right);
  theApp.flash(`Favourite tabs ${f.name} are open.`);
  return true;
}

async function configFavorites() {
  let favs = await go.TabsHotlist.Favorites();
  return modal((box, done) => {
    const draw = (sel = 0) => {
      box.innerHTML = `<h2>Favourite tabs</h2>
        <div class="mc-row"><label for="fav-list">Saved tab sets</label><select id="fav-list" size="8">${favs.map((f, i) => `<option value="${esc(f.name)}"${i === sel ? ' selected' : ''}>${esc(f.name)} (${f.left.tabs.length} + ${f.right.tabs.length} tabs)</option>`).join('')}</select></div>
        ${favs.length ? '' : '<p class="mc-note">No favourite tabs yet. Use "Add current tabs" to save the tabs of both panels under a name.</p>'}
        <p class="mc-err" hidden></p>
        <div class="mc-btns"><button type="button" data-a="add">Add current tabs…</button><button type="button" data-a="rename"${favs.length ? '' : ' disabled'}>Rename…</button><button type="button" data-a="delete"${favs.length ? '' : ' disabled'}>Delete</button><button type="button" data-a="open"${favs.length ? '' : ' disabled'}>Open</button><button type="button" data-a="close" class="primary">Close</button></div>`;
    };
    const err = (m) => { const e = box.querySelector('.mc-err'); e.hidden = !m; e.textContent = m || ''; };
    const selected = () => favs.find((f) => f.name === box.querySelector('#fav-list')?.value);
    const reload = async (name) => { favs = await go.TabsHotlist.Favorites(); draw(Math.max(0, favs.findIndex((f) => f.name === name))); };
    box.onsubmit = (ev) => { ev.preventDefault(); done(true); };
    box.addEventListener('click', async (ev) => {
      const a = ev.target.closest('button[data-a]')?.dataset.a;
      if (!a) return;
      const f = selected();
      try {
        if (a === 'close') done(true);
        else if (a === 'open' && f) { done(true); await applyFavorite(f); }
        else if (a === 'add') { const n = await saveFavorite(); if (n) await reload(n); }
        else if (a === 'rename' && f) {
          const n = await inputDialog({ title: 'Rename favourite tabs', label: 'New name', value: f.name, kind: 'favorite-rename' });
          if (n && n !== f.name) { await go.TabsHotlist.RenameFavorite(f.name, n); await reload(n.trim()); }
        } else if (a === 'delete' && f) {
          const y = await messageDialog({ Title: 'Delete favourite tabs', Message: `Delete the favourite tabs ${f.name}?`, Buttons: ['Delete', 'Cancel'], Kind: 'favorite-delete' });
          if (y === 'Delete') { await go.TabsHotlist.DeleteFavorite(f.name); await reload(''); }
        }
      } catch (e) { err(errText(e)); }
    });
    draw();
  }, false, { kind: 'favorite-tabs-config' });
}

async function saveFavorite(name) {
  if (!name) {
    const def = `${folderName(theApp.left.path)} – ${folderName(theApp.right.path)}`;
    name = await inputDialog({ title: 'Add current tabs to favourites', label: 'Name of the favourite tabs', value: def, kind: 'favorite-add',
      validate: (v) => (v.trim() ? '' : 'Please give the favourite tabs a name.') });
    if (!name) return '';
  }
  await go.TabsHotlist.SaveFavorite({ name, left: tabSet('left'), right: tabSet('right') });
  theApp.flash(`The tabs of both panels are saved as ${name.trim()}.`);
  return name.trim();
}

async function stepFavorite(d) {
  const favs = await go.TabsHotlist.Favorites();
  if (!favs.length) { theApp.flash('There are no favourite tabs yet.'); return false; }
  favIndex = ((favIndex + d) % favs.length + favs.length) % favs.length;
  return applyFavorite(favs[favIndex]);
}

// ---- commands ----

const curSide = (app) => app.cur.side;

command({ name: 'cm_OpenNewTab', label: 'New tab', run: (app) => newTab(curSide(app), { same: true }) });
command({ name: 'cm_OpenNewTabBg', label: 'New tab in the background', run: (app) => newTab(curSide(app), { same: true, background: true }) });

// target folder of Ctrl+Up: the folder under the cursor; ".." the parent; a file the current folder (cursor on it)
function dirUnderCursor(p) {
  const e = p.cur;
  if (!e) return { path: p.path };
  if (e.up) return { path: p.parent || p.path };
  if (e.dir) return { path: p.fullPath(e.name) };
  return { path: p.path, focus: e.name };
}
command({
  name: 'cm_OpenDirInNewTab', label: 'Open folder under cursor in a new tab',
  run: (app, args) => {
    const { path, focus } = args.path ? { path: args.path } : dirUnderCursor(app.cur);
    return newTab(curSide(app), { path, focus, background: !tabOpt('ctrlUpForeground') });
  },
});
command({
  name: 'cm_OpenDirInNewTabOther', label: 'Open folder under cursor in a new tab of the other panel',
  run: async (app, args) => {
    const { path, focus } = args.path ? { path: args.path } : dirUnderCursor(app.cur);
    const other = app.other;
    const ok = await newTab(other.side, { path, focus });
    if (tabOpt('activateTarget')) app.setActive(other);
    return ok;
  },
});
command({ name: 'cm_CloseCurrentTab', label: 'Close tab', run: (app) => { const s = sides[curSide(app)]; return s ? closeTab(curSide(app), s.active) : false; } });
command({ name: 'cm_CloseAllTabs', label: 'Close all tabs', run: (app) => { const s = sides[curSide(app)]; return s ? closeOthers(curSide(app), s.active) : false; } });
command({
  name: 'cm_SwitchToNextTab', label: 'Next tab',
  run: (app) => { const s = sides[curSide(app)]; return s && s.tabs.length > 1 ? activate(curSide(app), (s.active + 1) % s.tabs.length) : false; },
});
command({
  name: 'cm_SwitchToPreviousTab', label: 'Previous tab',
  run: (app) => { const s = sides[curSide(app)]; return s && s.tabs.length > 1 ? activate(curSide(app), (s.active - 1 + s.tabs.length) % s.tabs.length) : false; },
});
command({
  name: 'cm_ToggleLockCurrentTab', label: 'Lock tab', checked: (app) => isLocked(app.cur),
  run: (app) => { const s = sides[curSide(app)]; return s ? setLock(curSide(app), s.active, s.tabs[s.active].lock === LOCK_FULL ? LOCK_NONE : LOCK_FULL) : false; },
});
command({
  name: 'cm_ToggleLockDcaCurrentTab', label: 'Lock tab, folder changes allowed', checked: (app) => activeTab(app.cur)?.lock === LOCK_DIRS,
  run: (app) => { const s = sides[curSide(app)]; return s ? setLock(curSide(app), s.active, s.tabs[s.active].lock === LOCK_DIRS ? LOCK_NONE : LOCK_DIRS) : false; },
});
command({ name: 'cm_RenameTab', label: 'Rename or lock tab…', run: (app) => { const s = sides[curSide(app)]; return s ? renameTab(curSide(app), s.active) : false; } });
command({ name: 'cm_DuplicateTab', label: 'Duplicate tab', run: (app) => { const t = activeTab(app.cur); return newTab(curSide(app), { same: true, lock: t?.lock, lockPath: t?.lockPath, caption: t?.caption }); } });
command({ name: 'cm_CopyTabToOtherSide', label: 'Copy tab to other side', run: (app) => copyToOther(curSide(app), sides[curSide(app)]?.active ?? -1) });
command({ name: 'cm_MoveTabToOtherSide', label: 'Move tab to other side', run: (app) => copyToOther(curSide(app), sides[curSide(app)]?.active ?? -1, { move: true }) });
command({ name: 'cm_TabMenu', label: 'Tab menu', run: (app, args) => { const s = sides[curSide(app)]; return s ? tabMenu(curSide(app), args.index ?? s.active, args.x != null ? { x: args.x, y: args.y } : app.cur.$.tabbar.hidden ? app.cur.$.pathbar : app.cur.$.tabbar) : false; } });
command({
  name: 'cm_ReopenClosedTab', label: 'Reopen closed tab', enabled: () => closedTabs.length > 0,
  run: async () => {
    const c = closedTabs.pop();
    if (!c) return false;
    return newTab(c.side, { path: c.path, caption: c.caption, lock: c.lock, lockPath: c.lockPath, sort: c.sort, saved: c.saved });
  },
});
command({
  // Ctrl+Shift+U: the whole tab sets of both sides swap (F-005); Ctrl+U (the core) swaps only what the active tabs show.
  name: 'cm_ExchangeWithTabs', label: 'Swap panels with tabs',
  run: (app) => {
    [sides.left, sides.right] = [sides.right, sides.left];
    app.left.swapWith(app.right);
    app.emit('exchange');
    for (const p of app.panels) app.panelChanged(p, { moved: true });
    renderAll();
    saveSoon();
  },
});
command({
  name: 'cm_SaveTabs', label: 'Save tabs to file…',
  run: async (app, args) => {
    const path = args.file || await chooseTabFile(true);
    if (!path) return false;
    await go.TabsHotlist.SaveTabFile(path, { active: tabSet(curSide(app)), inactive: { tabs: [], active: 0 } });
    app.flash(`The tabs are saved to ${folderName(path)}.`);
    return true;
  },
});
command({
  name: 'cm_LoadTabs', label: 'Load tabs from file…',
  run: async (app, args) => {
    const path = args.file || await chooseTabFile(false);
    if (!path) return false;
    return loadTabFile(path, app.cur, { append: !!args.append });
  },
});
command({ name: 'cm_SaveFavoriteTabs', label: 'Add current tabs to favourites…', run: async (app, args) => !!(await saveFavorite(args.name)) });
command({
  name: 'cm_LoadFavoriteTabs', label: 'Favourite tabs…',
  run: async (app, args) => {
    if (!args.name) return favoritesMenu();
    const f = (await go.TabsHotlist.Favorites()).find((x) => x.name.toLowerCase() === String(args.name).toLowerCase());
    if (!f) { app.flash(`There are no favourite tabs named ${args.name}.`, 'err'); return false; }
    return applyFavorite(f);
  },
});
command({ name: 'cm_ConfigFavoriteTabs', label: 'Configure favourite tabs…', run: () => configFavorites() });
command({ name: 'cm_NextFavoriteTab', label: 'Next favourite tabs', run: () => stepFavorite(1) });
command({ name: 'cm_PrevFavoriteTab', label: 'Previous favourite tabs', run: () => stepFavorite(-1) });
command({ name: 'cm_DirTabsConfig', label: 'Folder tab settings…', run: () => configDialog() });
command({ name: 'cm_VisDirTabs', label: 'Folder tabs', checked: () => !!tabOpt('show'), run: () => setOpts({ show: !tabOpt('show') }) });

// cm_SrcActivateTab1..99, cm_LeftActivateTab…, cm_RightActivateTab…, cm_TrgActivateTab… (F-033, F-774)
const whichSide = { Src: (app) => app.cur.side, Trg: (app) => app.other.side, Left: () => 'left', Right: () => 'right' };
const sideWord = { Src: 'this panel', Trg: 'the other panel', Left: 'the left panel', Right: 'the right panel' };
for (const [pre, f] of Object.entries(whichSide)) {
  for (let n = 1; n <= 99; n++) {
    command({ name: `cm_${pre}ActivateTab${n}`, label: `Tab ${n} of ${sideWord[pre]}`, run: (app) => activate(f(app), n - 1) });
  }
}

// ---- control mode: the tabs of both sides (for QA) ----
addControlCommand('tabs', async () => {
  const side = (s) => ({ active: sides[s]?.active ?? 0, hidden: panelOf(s).$.tabbar.hidden,
    tabs: (sides[s]?.tabs || []).map((t) => ({ path: tabPath(panelOf(s), t), caption: tabCaption(panelOf(s), t, { plain: true }), shown: tabCaption(panelOf(s), t), lock: t.lock, lockPath: t.lockPath, sort: { ...tabSort(panelOf(s), t) }, ...tabExtra(panelOf(s), t) })) });
  return { ok: true, left: side('left'), right: side('right'), closed: closedTabs.length };
});
addControlCommand('tab_menu', async (m) => { const side = m.side || theApp.cur.side; tabMenu(side, m.index ?? sides[side]?.active ?? 0, theApp.cur.$.tabbar); return { ok: true }; });

// ---- start-up ----

export function init(app) {
  for (const p of app.panels) {
    origGo.set(p, p.go.bind(p));
    p.go = (path, opts) => guardedGo(p, path, opts);
    wireBar(p);
  }
  app.on('panel', (p, info = {}) => {
    const t = activeTab(p);
    if (!t) return;
    if (info.moved) { t.path = p.path; }
    if (t.lock === LOCK_FULL) lockedHistory.set(p, { back: [...p.back], fwd: [...p.fwd] });
    render(p.side);
    saveSoon();
  });
  app.on('exchange', () => renderAll());
  app.on('settings', () => renderAll()); // (the Options dialog's Folder tabs page changes "folderTabs" too)
  app.on('ready', async () => {
    let saved = null;
    if (!app.demo) { try { saved = await go.TabsHotlist.Tabs(); } catch { saved = null; } }
    for (const side of ['left', 'right']) {
      const p = panelOf(side);
      const set = saved?.[side];
      const hist = saved?.[`${side}History`] || [];
      if (set?.tabs?.length) {
        const tabs = set.tabs.map((x) => makeTab(tabFields(x)));
        const active = Math.max(0, Math.min(set.active || 0, tabs.length - 1));
        // the active tab is what the panel shows now (the start folder from the command line wins over the saved one)
        if (samePath(tabs[active].path, p.path) && !p.back.length) p.back = [...hist].reverse().filter((x) => !samePath(x, p.path));
        if (tabs[active].sort) { const so = tabs[active].sort; if (so.key !== p.sort.key || so.desc !== p.sort.desc) p.sortBy(so.key, so.desc); }
        tabs[active].path = p.path;
        // the active tab's saved view (and other registered state) is what the panel shows; the panel is told below
        let changed = false;
        for (const [k, h] of tabStates) { const v = h.load(tabs[active].saved); if (v != null) { p[k] = v; changed = true; } }
        sides[side] = { tabs, active };
        if (changed) app.panelChanged(p, { moved: false });
        if (tabs[active].lock === LOCK_FULL) lockedHistory.set(p, { back: [...p.back], fwd: [...p.fwd] });
      } else {
        sides[side] = { tabs: [makeTab({ path: p.path })], active: 0 };
      }
      render(side);
    }
  });
}
