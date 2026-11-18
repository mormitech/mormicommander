// Reserved Total Commander commands that had a key or a menu item but nothing behind them (board issue #38), first
// slice: the command line history popup (Alt+F8, Ctrl+Down), Show >
// Programs (Ctrl+F11), Show > Custom filter (Ctrl+F12) with Define custom filter…, and the current folder bars of the
// left and right panel (Alt+F11, Alt+F12). Each one only adds what used to say "… is not ready yet"; nothing that
// worked before changes. The pure parts (mask, command search, history items) are in reserved-lib.js.

import { command } from '../core/registry.js';
import { showList } from '../core/popup.js';
import { inputDialog } from '../core/dialogs.js';
import { cmdline } from '../core/cmdline.js';
import { splitPath } from '../core/panel.js';
import { maskFilter, execFilter, historyItems, selectionFilter } from './reserved-lib.js';

const USER_MASK = 'srcUserMask'; // the setting that keeps the custom filter's mask between runs

// ---- command line history (Alt+F8, Ctrl+Down) ----

// showCmdLineHistory lists the lines run before (newest first); the chosen one goes into the command line, ready to
// edit or run with Enter, as in Total Commander.
async function showCmdLineHistory(app) {
  const items = historyItems(cmdline.history);
  if (!items.length) { app.flash('The command line history is empty.'); return false; }
  const r = await showList({ title: 'Command line history', items, anchor: document.getElementById('cmdline'), kind: 'cmdhistory' });
  if (!r) return false;
  cmdline.set(r.item.value);
  return true;
}
command({ name: 'cm_ShowCmdLineHistory', menuKey: true, label: 'Command line history', short: 'History', run: (app) => showCmdLineHistory(app) });

// ---- Show > Programs, Custom filter ----

const srcKind = (app) => app.cur.filter?.mcSrc || '';

command({
  // Ctrl+F11: only folders and the files that can be run. Show > All files (Ctrl+F10) shows everything again.
  name: 'cm_SrcExecs', menuKey: true, label: 'Programs', checked: (app) => srcKind(app) === 'execs',
  run: (app) => { app.cur.setFilter(execFilter(), 'Programs'); return true; },
});

// applyUserMask puts the custom mask on the active panel.
function applyUserMask(app, mask) {
  app.cur.setFilter(maskFilter(mask), mask);
  return true;
}

// defineUserMask asks for the custom filter's mask (args.mask skips the question), keeps it and applies it.
async function defineUserMask(app, args = {}) {
  const mask = await askUserMask(app, args);
  return mask === null ? false : applyUserMask(app, mask);
}

// askUserMask asks for the custom filter's mask (args.mask skips the question) and keeps it; null when cancelled.
async function askUserMask(app, args = {}) {
  let mask = typeof args.mask === 'string' ? args.mask : null;
  if (mask === null) {
    mask = await inputDialog({
      title: 'Custom filter', label: 'Show files matching:', value: app.setting(USER_MASK, '') || '*.*',
      note: 'For example *.txt *.md, or *.* | *.bak *.tmp to leave files out. Folders always stay.',
      validate: (v) => (v.trim() ? '' : 'Type a file mask, for example *.txt.'), kind: 'srcuserdef',
    });
    if (mask === null) return null;
  }
  mask = mask.trim();
  if (!mask) return null;
  app.setSetting(USER_MASK, mask);
  return mask;
}

command({
  // Ctrl+F12: the custom filter defined last; the first time it asks for one.
  name: 'cm_SrcUserSpec', menuKey: true, label: 'Custom filter', checked: (app) => srcKind(app) === 'user',
  run: (app, args) => {
    const mask = app.setting(USER_MASK, '');
    return mask ? applyUserMask(app, mask) : defineUserMask(app, args);
  },
});
command({ name: 'cm_SrcUserDef', label: 'Define custom filter…', run: (app, args) => defineUserMask(app, args) });

// Show > Show only selected files / Hide selected files (F-408, ours names): the active panel shows only the selected
// entries, or all but them, until the folder is read again (Ctrl+R) or left; Show > All files (Ctrl+F10) ends it too.
function showSelected(app, keep) {
  const p = app.cur;
  const names = [...p.selected];
  if (!names.length) { app.flash(keep ? 'Select the files to show first.' : 'Select the files to hide first.'); return false; }
  p.setFilter(selectionFilter(names, keep, p.filter, p.filterLabel), keep ? 'Only selected' : 'Selected hidden');
  return true;
}
command({
  name: 'cm_ShowOnlySelected', label: 'Show only selected files', menu: 'Show', menuAfter: 'cm_SrcUserDef',
  checked: (app) => srcKind(app) === 'onlysel', run: (app) => showSelected(app, true),
});
command({
  name: 'cm_HideSelected', label: 'Hide selected files', menu: 'Show', menuAfter: 'cm_ShowOnlySelected',
  checked: (app) => srcKind(app) === 'hidesel', run: (app) => showSelected(app, false),
});

// The Show menu filters for a named panel (F-786): Left, Right and Trg (the other panel) variants of All files,
// Programs, Custom filter and Define custom filter. Each acts on that panel only and leaves the active panel as it is.
for (const [pre, pick, w] of [['Left', (app) => app.left, ' (left panel)'], ['Right', (app) => app.right, ' (right panel)'], ['Trg', (app) => app.other, ' (other panel)']]) {
  const kind = (app) => pick(app).filter?.mcSrc || '';
  command({ name: `cm_${pre}AllFiles`, label: `All files${w}`, checked: (app) => !pick(app).filter, run: (app) => { const p = pick(app); if (p.filter) p.setFilter(null); return true; } });
  command({ name: `cm_${pre}Execs`, label: `Programs${w}`, checked: (app) => kind(app) === 'execs', run: (app) => { pick(app).setFilter(execFilter(), 'Programs'); return true; } });
  command({
    name: `cm_${pre}UserSpec`, label: `Custom filter${w}`, checked: (app) => kind(app) === 'user',
    run: async (app, args) => {
      let mask = app.setting(USER_MASK, '');
      if (!mask) { mask = await askUserMask(app, args); if (mask === null) return false; }
      pick(app).setFilter(maskFilter(mask), mask);
      return true;
    },
  });
  command({
    name: `cm_${pre}UserDef`, label: `Define custom filter…${w}`,
    run: async (app, args) => {
      const mask = await askUserMask(app, args);
      if (mask === null) return false;
      pick(app).setFilter(maskFilter(mask), mask);
      return true;
    },
  });
}

// ---- current folder bars (Alt+F11, Alt+F12) ----

// curDirBar opens the path bar of one panel as a list of its folders, from the root down to the current one; choosing
// one makes that panel active and opens the folder.
async function curDirBar(app, side) {
  const p = app.panel(side);
  if (!p?.path) return false;
  const parts = splitPath(p.path);
  const items = parts.map((x) => ({ label: x.path, value: x.path }));
  const r = await showList({ title: side === 'left' ? 'Left folder bar' : 'Right folder bar', items, anchor: p.$.pathbar, index: items.length - 1, kind: 'curdirbar' });
  if (!r) return false;
  app.setActive(p);
  if (r.item.value !== p.path) await p.go(r.item.value);
  return true;
}
command({ name: 'cm_LeftCurDirBar', label: 'Left folder bar', run: (app) => curDirBar(app, 'left') });
command({ name: 'cm_RightCurDirBar', label: 'Right folder bar', run: (app) => curDirBar(app, 'right') });
