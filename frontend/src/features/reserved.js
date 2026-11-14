// Reserved Total Commander commands that had a key or a menu item but nothing behind them (board issue #38), first
// slice: the command line history popup (Alt+F8, Ctrl+Down), the command browser (Commands > All commands…), Show >
// Programs (Ctrl+F11), Show > Custom filter (Ctrl+F12) with Define custom filter…, and the current folder bars of the
// left and right panel (Alt+F11, Alt+F12). Each one only adds what used to say "… is not ready yet"; nothing that
// worked before changes. The pure parts (mask, command search, history items) are in reserved-lib.js.

import { command, commandList, keysFor } from '../core/registry.js';
import { showList } from '../core/popup.js';
import { inputDialog } from '../core/dialogs.js';
import { cmdline } from '../core/cmdline.js';
import { label as keyLabel } from '../core/keys.js';
import { splitPath } from '../core/panel.js';
import { maskFilter, execFilter, filterCommands, historyItems } from './reserved-lib.js';

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
command({ name: 'cm_ShowCmdLineHistory', label: 'Command line history', short: 'History', run: (app) => showCmdLineHistory(app) });

// ---- command browser (Commands > All commands…) ----

// commandBrowser asks for words to look for (empty: every command), then lists the matching commands with their keys;
// the chosen one runs. args.filter skips the question (control mode, the command line).
async function commandBrowser(app, args = {}) {
  let text = args.filter;
  if (typeof text !== 'string') {
    text = await inputDialog({ title: 'All commands', label: 'Look for:', value: '', note: 'Words of the name or the menu text; leave it empty to list every command.', okLabel: 'List', kind: 'commandbrowser' });
    if (text === null) return false;
  }
  const found = filterCommands(commandList().filter((c) => c.name !== 'cm_CommandBrowser'), text);
  if (!found.length) { app.flash(`No command matches "${text}".`); return false; }
  const items = found.map((c) => {
    const keys = keysFor(c.name).slice(0, 2).map((k) => keyLabel(k, app.platform)).join(', ');
    return { label: c.label || c.name, detail: keys ? `${c.name}  ${keys}` : c.name, value: c.name };
  });
  const r = await showList({ title: `Commands (${items.length})`, items, kind: 'commandbrowser' });
  if (!r) return false;
  await app.run(r.item.value);
  return true;
}
command({ name: 'cm_CommandBrowser', label: 'All commands…', run: (app, args) => commandBrowser(app, args) });

// ---- Show > Programs, Custom filter ----

const srcKind = (app) => app.cur.filter?.mcSrc || '';

command({
  // Ctrl+F11: only folders and the files that can be run. Show > All files (Ctrl+F10) shows everything again.
  name: 'cm_SrcExecs', label: 'Programs', checked: (app) => srcKind(app) === 'execs',
  run: (app) => { app.cur.setFilter(execFilter(), 'Programs'); return true; },
});

// applyUserMask puts the custom mask on the active panel.
function applyUserMask(app, mask) {
  app.cur.setFilter(maskFilter(mask), mask);
  return true;
}

// defineUserMask asks for the custom filter's mask (args.mask skips the question), keeps it and applies it.
async function defineUserMask(app, args = {}) {
  let mask = typeof args.mask === 'string' ? args.mask : null;
  if (mask === null) {
    mask = await inputDialog({
      title: 'Custom filter', label: 'Show files matching:', value: app.setting(USER_MASK, '') || '*.*',
      note: 'For example *.txt *.md, or *.* | *.bak *.tmp to leave files out. Folders always stay.',
      validate: (v) => (v.trim() ? '' : 'Type a file mask, for example *.txt.'), kind: 'srcuserdef',
    });
    if (mask === null) return false;
  }
  mask = mask.trim();
  if (!mask) return false;
  app.setSetting(USER_MASK, mask);
  return applyUserMask(app, mask);
}

command({
  // Ctrl+F12: the custom filter defined last; the first time it asks for one.
  name: 'cm_SrcUserSpec', label: 'Custom filter', checked: (app) => srcKind(app) === 'user',
  run: (app, args) => {
    const mask = app.setting(USER_MASK, '');
    return mask ? applyUserMask(app, mask) : defineUserMask(app, args);
  },
});
command({ name: 'cm_SrcUserDef', label: 'Define custom filter…', run: (app, args) => defineUserMask(app, args) });

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
