// The separate tree as a pane of its own (F-025, F-279, F-400, F-807): with Options > Layout > "Separate tree as a
// pane of its own beside the file list" on (Layout.SeparateTreePane, off by default), Show > Separate tree (and
// Ctrl+Shift+F8, Layout > Separate folder tree) shows folder tree panes instead of putting the panels into their tree
// view: "one for both panels" is one pane at the left that follows the active panel, "one per panel" is a pane before
// each panel that follows that panel. A click on a folder (or moving to it with the keyboard) shows it in the panel,
// Enter shows it and gives the keyboard to the file list, Shift+Tab moves the keyboard between the pane and the list.
// Folders are read when they are opened. With the option off nothing here runs and the commands work as before.

import { FolderTree, folderReader, rootOf } from './foldertree.js';

const panes = []; // {el, tree, bind: 'left' | 'right' | 'active', seq, timer}
let level = 'off';

let theApp = null; // (the app, from the first call; this module does not import it, so it stays light to load)
const optOn = (k, app = theApp) => app?.config?.values?.[k] === '1';
export const treePaneOn = (app) => optOn('Layout.SeparateTreePane', app || theApp);
export const treePaneLevel = () => level;

const isRemote = (p) => /^[a-z][a-z0-9+.-]*:\/\//i.test(p || '');
const panelOf = (app, pane) => (pane.bind === 'active' ? app.cur : app.panels.find((p) => p.side === pane.bind));

// paneBinds is which panes a separate tree level shows ('one': one following the active panel, 'two': one per panel).
export function paneBinds(lvl) {
  return lvl === 'one' ? ['active'] : lvl === 'two' ? ['left', 'right'] : [];
}

function removePanes() {
  for (const pane of panes.splice(0)) { clearTimeout(pane.timer); pane.el.remove(); }
}

function makePane(app, bind) {
  const el = document.createElement('aside');
  el.className = 'mc-treepane';
  el.dataset.for = bind;
  el.tabIndex = 0;
  el.setAttribute('aria-label', 'Folder tree');
  const rows = document.createElement('div');
  el.append(rows);
  const pane = { el, bind, seq: 0, timer: 0, tree: null };
  const show = (n, { focusList = false } = {}) => {
    const p = panelOf(app, pane);
    if (!p || !n) return;
    clearTimeout(pane.timer);
    app.setActive(p, { focus: false });
    if (n.path !== p.path) p.go(n.path);
    if (focusList) p.$.list.focus({ preventScroll: true });
  };
  pane.tree = new FolderTree({
    el: rows,
    read: folderReader(app, { hidden: () => optOn('Tree.ShowHidden', app) }),
    ignoreCase: app.platform === 'windows',
    filter: false, // (letters jump to a folder, as in the panel's tree view)
    onGo: (n) => show(n, { focusList: true }),
    onPick: (n) => show(n),
    onMove: (n) => { clearTimeout(pane.timer); pane.timer = setTimeout(() => show(n), 200); },
    onError: (t) => app.flash(t, 'err'),
  });
  el.addEventListener('keydown', (ev) => {
    if (ev.defaultPrevented || ev.target !== el) return;
    let used = pane.tree.key(ev);
    // a letter jumps to the next folder whose name starts with it
    if (!used && ev.key.length === 1 && ev.key !== ' ' && !ev.ctrlKey && !ev.metaKey && !ev.altKey) {
      used = jumpTo(pane.tree, ev.key);
    }
    if (!used && ev.key === 'Escape') { panelOf(app, pane)?.$.list.focus({ preventScroll: true }); used = true; }
    if (used) { ev.preventDefault(); ev.stopPropagation(); }
  });
  el.addEventListener('mousedown', () => { const p = panelOf(app, pane); if (p && p !== app.cur) app.setActive(p, { focus: false }); });
  return pane;
}

// jumpTo moves a tree's highlight to the next row whose name starts with the letter c; false when none does.
export function jumpTo(tree, c) {
  const len = tree.nodes.length, low = String(c).toLowerCase();
  for (let j = 1; j <= len; j++) {
    const k = (tree.at + j) % len;
    if (String(tree.nodes[k].name).toLowerCase().startsWith(low)) { tree.move(k); return true; }
  }
  return false;
}

// follow brings a pane to the folder its panel shows: the way down opened, the highlight on it; a folder on another
// drive starts the tree again at that drive. Server folders are only read with Options > Tree > "Read network folders
// in the tree", as for the panel's own tree.
async function follow(app, pane) {
  const p = panelOf(app, pane);
  if (!p?.path) return;
  const seq = ++pane.seq;
  const tree = pane.tree;
  if (isRemote(p.path) && !optOn('Tree.ShowNetwork', app)) {
    tree.root = null; tree.nodes = [];
    tree.el.innerHTML = '<div class="mc-treepane-note">Server folders are not read in the tree (Options &gt; Tree &gt; Read network folders in the tree).</div>';
    return;
  }
  const root = await rootOf(p.path);
  if (seq !== pane.seq) return;
  if (!tree.root || tree.root.path !== root) await tree.openTo(root, p.path);
  else await tree.reveal(p.path);
}

// setTreePane shows the panes of a level ('off', 'one', 'two').
export async function setTreePane(app, lvl) {
  if (!['off', 'one', 'two'].includes(lvl)) return false;
  listen(app);
  removePanes();
  level = lvl;
  const box = document.getElementById('panels');
  for (const bind of paneBinds(lvl)) {
    const pane = makePane(app, bind);
    const before = box?.querySelector(`.panel[data-side="${bind === 'active' ? 'left' : bind}"]`);
    if (!before) continue;
    box.insertBefore(pane.el, before);
    panes.push(pane);
  }
  for (const p of app.panels) { try { p.measure(); p.render(); } catch { /* not drawn yet */ } }
  await Promise.all(panes.map((pane) => follow(app, pane)));
  return true;
}

// focusTreePane moves the keyboard between the active panel's pane and its file list (Shift+Tab); false when the
// active panel has no pane.
export function focusTreePane(app) {
  const pane = panes.find((x) => panelOf(app, x) === app.cur);
  if (!pane) return false;
  if (pane.el.contains(document.activeElement)) app.cur.$.list.focus({ preventScroll: true });
  else pane.el.focus({ preventScroll: true });
  return true;
}

// syncTreePane takes the panes away when the option is turned off (features/config/layout.js calls it).
export function syncTreePane(app) {
  if (!treePaneOn(app) && panes.length) { removePanes(); level = 'off'; }
}

// listen makes the panes follow their panels (once, from the first pane shown).
function listen(app) {
  if (theApp) return;
  theApp = app;
  app.on?.('panel', (p, info) => {
    if (!panes.length || info?.moved === false) return;
    for (const pane of panes) if (panelOf(app, pane) === p) follow(app, pane);
  });
  app.on?.('active', () => {
    for (const pane of panes) if (pane.bind === 'active') follow(app, pane);
  });
}
