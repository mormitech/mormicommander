// Reserved Total Commander commands that had a key or a menu item but nothing behind them (board issue #38), second
// slice: the folder tree popup (Alt+F10, cm_CDtree), the separate tree commands (Show > Separate tree,
// Ctrl+Shift+F8, Shift+Tab), the in-window menu (F9, F10, cm_ActivateMenu) and printing (Ctrl+F9 cm_PrintFile,
// Files > Print file list cm_PrintDir). The first slice is features/reserved.js. Each command only replaces what used
// to say "… is not ready yet"; no key changes meaning and nothing that worked before behaves differently. The pure
// parts (which panels show the tree, the menu and popup items, the printed text) are in reserved-more-lib.js.

import './reserved-more.css';
import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { showList } from '../core/popup.js';
import { formDialog, messageDialog, esc } from '../core/dialogs.js';
import { menuSpec } from '../core/menus.js';
import { F } from '../core/format.js';
import { inTreeView, setTreeView } from './views.js';
import { treeLevel, treeTarget, menuItems, topMenuItems, dirNames, folderItems, fileListText } from './reserved-more-lib.js';

const isRemote = (p) => /^[a-z][a-z0-9+.-]*:\/\//i.test(p || '');

// ---- folder tree popup (Alt+F10) ----

// cdTree shows the folders of the active panel's folder in a popup, one level at a time: Enter (or Right) on a folder
// shows its subfolders, ".." goes up, "Go to …" (the first item, so Enter Enter goes into a folder) makes the panel
// show that folder, and typing filters the list, like the type-to-search of Total Commander's tree dialog. Shift+Enter
// on a folder goes there at once. TC shows the whole drive as one expanded tree; a level-by-level list keeps this
// popup fast on big disks and on servers, and the panel's own tree view (Ctrl+F8) is there for the expanded tree.
async function cdTree(app) {
  const p = app.cur;
  if (!p?.path) return false;
  let path = p.path, focus = '';
  for (;;) {
    let l;
    try { l = await go.FS.List('cdtree', path); } catch (e) { app.flash(errText(e), 'err'); return false; }
    const here = l.path || path;
    const sep = app.sepFor(here);
    const names = dirNames(l.entries, { showHidden: app.showHidden, dirFlag: F.DIR, hiddenFlag: F.HIDDEN });
    const items = folderItems(here, l.parent || '', names, sep);
    const index = focus ? Math.max(0, items.findIndex((it) => it.label === focus)) : 0;
    const r = await showList({ title: 'Folder tree', items, anchor: p.$.pathbar, index, kind: 'cdtree', filter: true });
    if (!r) return false;
    const v = r.item.value;
    if (v.go || r.shift) {
      app.setActive(p);
      return p.go(v.go || v.open);
    }
    // going up keeps the folder we came from highlighted, as the panel does after Backspace
    focus = v.from ? v.from.replace(/[\\/]+$/, '').split(/[\\/]/).pop() : '';
    path = v.open;
  }
}
command({ name: 'cm_CDtree', label: 'Folder tree…', run: (app) => cdTree(app) });

// ---- separate tree (Show > Separate tree, Ctrl+Shift+F8, Shift+Tab) ----

// The mapping onto the panels' tree view (Ctrl+F8) is explained in reserved-more-lib.js: Off = no panel in the tree
// view, One = one panel, Two = both, the cycle goes through them. Leaving the tree view puts a panel back into the view
// it had before, at the folder it showed before, just as Ctrl+F8 does.
const treeState = (app) => ({ active: inTreeView(app.cur), other: inTreeView(app.other) });

async function separateTree(app, want) {
  const target = treeTarget(treeState(app), want);
  const cur = app.cur, other = app.other;
  // the other panel first, so the active panel is the last one drawn and keeps the keyboard
  await setTreeView(other, target.other);
  await setTreeView(cur, target.active);
  app.setActive(cur);
  return true;
}

command({ name: 'cm_SeparateTreeOff', label: 'Separate tree: off', checked: (app) => treeLevel(treeState(app)) === 'off', run: (app) => separateTree(app, 'off') });
command({ name: 'cm_SeparateTree1', label: 'Separate tree: one for both panels', checked: (app) => treeLevel(treeState(app)) === 'one', run: (app) => separateTree(app, 'one') });
command({ name: 'cm_SeparateTree2', label: 'Separate tree: one per panel', checked: (app) => treeLevel(treeState(app)) === 'two', run: (app) => separateTree(app, 'two') });
command({ name: 'cm_SeparateTreeCycle', label: 'Separate tree: next state', short: 'Tree', run: (app) => separateTree(app, 'cycle') });

// cm_FocusTreeToggle (Shift+Tab) moves the keyboard between the tree and the file list in TC. Here the tree is one of
// the two panels, so with one panel in the tree view this goes to the other panel, and with none or both it switches
// panels the way Tab does. In every case it is the other panel that gets the keyboard; the command exists so Shift+Tab
// does something useful instead of saying it is not ready.
command({ name: 'cm_FocusTreeToggle', label: 'Switch between the tree and the file list', run: (app) => { app.setActive(app.other); return true; } });

// ---- the in-window menu (F9, F10) ----

// activateMenu opens the menu bar as a popup over the active panel. The native menu bar (built by Go from
// core/menus.js) can't be opened from the page, least of all on macOS, where it belongs to the system; this popup has
// the very same items, so the menus can be used with the keyboard alone: Up/Down move, Enter or Right opens a menu, Left
// or Esc goes back a level, a letter jumps to the item starting with it, and choosing an item runs its command just as
// the native menu does.
async function activateMenu(app) {
  const spec = menuSpec();
  const anchor = app.cur?.$?.pathbar || null;
  const stack = [];
  let level = { items: null, label: '' }, at = 0;
  for (;;) {
    const top = !level.items;
    const items = top ? topMenuItems(spec) : menuItems(level.items, (cmd) => app.keyLabel(cmd));
    const title = top ? 'Menu' : [...stack.slice(1).map((s) => s.level.label), level.label].filter(Boolean).join(' › ');
    const r = await showList({ title, items, anchor, index: at, kind: 'app-menu', left: !top });
    if (!r) {
      if (!stack.length) return false;
      ({ level, at } = stack.pop());
      continue;
    }
    const v = r.item.value;
    if (v.items) { stack.push({ level, at: r.index }); level = v; at = 0; continue; }
    if (v.cmd) return app.run(v.cmd);
    return false;
  }
}
command({ name: 'cm_ActivateMenu', label: 'Menu', run: (app) => activateMenu(app) });

// ---- printing (Ctrl+F9, Files > Print file list) ----

const printerNote = (app) => (app.platform === 'windows'
  ? 'Windows prints it with the app that belongs to the file type.'
  : 'It goes to the default printer (lp); choose another default printer in the system settings.');

// printFile prints the file under the cursor through the system, after asking: the system's print path (lp) prints
// straight away without a dialog of its own.
async function printFile(app) {
  const p = app.cur, e = p?.cur;
  if (!e || e.up || e.dir) { app.flash('Put the cursor on a file to print it.'); return false; }
  const path = p.fullPath(e.name);
  if (isRemote(path)) { app.flash('Only files on this computer can be printed.', 'err'); return false; }
  const b = await messageDialog({ Title: 'Print', Message: `Print ${e.name}?\n\n${printerNote(app)}`, Buttons: ['Print', 'Cancel'], Kind: 'print-file' });
  if (b !== 'Print') return false;
  try { await go.SystemLaunch.Print(path); } catch (err) { app.flash(errText(err), 'err'); return false; }
  app.flash(`${e.name} was sent to the printer.`);
  return true;
}
command({ name: 'cm_PrintFile', label: 'Print file', run: (app) => printFile(app) });

// printDir shows the file list of the active panel (the files it shows, in its order, with the filter that is on) as
// plain text, and prints it when the user presses Print. Plain text is what every system's print path accepts.
async function printDir(app) {
  const p = app.cur;
  if (!p?.path) return false;
  const text = fileListText({ path: p.path, entries: p.entries, filter: p.filterLabel || '' });
  const ok = await formDialog('Print file list', `<p class="mc-note">${esc(printerNote(app))}</p><pre class="mc-print-preview" tabindex="0">${esc(text)}</pre>`,
    () => true, null, { okLabel: 'Print', kind: 'print-dir' });
  if (!ok) return false;
  try { await go.SystemLaunch.PrintText(`File list ${p.path}`, text); } catch (err) { app.flash(errText(err), 'err'); return false; }
  app.flash('The file list was sent to the printer.');
  return true;
}
command({ name: 'cm_PrintDir', label: 'Print file list…', run: (app) => printDir(app) });
