// Moving around: the cursor, opening folders and files, parent, root, home, history, switching and swapping panels,
// rereading, hidden files (FEATURES.md sections 1 and 3).

import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { cmdline } from '../core/cmdline.js';

const base = (p) => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || p;

// ---- cursor (ours: TC has no cm_* names for the arrow keys) ----
command({ name: 'cm_CursorUp', label: 'Cursor up', run: (app) => app.cur.moveCursor(-1) });
command({ name: 'cm_CursorDown', label: 'Cursor down', run: (app) => app.cur.moveCursor(1) });
command({ name: 'cm_PageUp', label: 'Page up', run: (app) => app.cur.moveCursor(-app.cur.pageSize()) });
command({ name: 'cm_PageDown', label: 'Page down', run: (app) => app.cur.moveCursor(app.cur.pageSize()) });
command({ name: 'cm_GoToFirstEntry', label: 'First entry', run: (app) => app.cur.setCursor(0) });
command({ name: 'cm_GoToLastEntry', label: 'Last entry', run: (app) => app.cur.setCursor(app.cur.entries.length - 1) });
// Left/Right move by a column in the Brief view (the views feature replaces these); in the Full view they do nothing,
// like in Total Commander.
command({ name: 'cm_CursorLeft', label: 'Cursor left', run: () => {} });
command({ name: 'cm_CursorRight', label: 'Cursor right', run: () => {} });
command({
  name: 'cm_GoToFirstFile', label: 'First file',
  run: (app) => { const i = app.cur.entries.findIndex((e) => !e.dir); if (i >= 0) app.cur.setCursor(i); },
});
command({
  name: 'cm_GoToNextSelected', label: 'Next selected',
  run: (app) => { const p = app.cur; const i = p.entries.findIndex((e, k) => k > p.cursor && p.isSelected(e)); if (i >= 0) p.setCursor(i); },
});
command({
  name: 'cm_GoToPrevSelected', label: 'Previous selected',
  run: (app) => { const p = app.cur; for (let k = p.cursor - 1; k >= 0; k--) if (p.isSelected(p.entries[k])) { p.setCursor(k); return; } },
});

// ---- opening ----

// enter opens what is under the cursor: ".." goes up, a folder (or a link to one) opens, a file goes to the open
// handlers (archives, URL files…) and then to the system's app for it.
async function enter(app) {
  const p = app.cur, e = p.cur;
  if (!e) return false;
  if (e.up) return goParent(p);
  const path = p.fullPath(e.name);
  if (e.dir) return p.go(path);
  for (const h of app.openHandlers) if (await h(p, e, path)) return true;
  if (e.broken) { app.flash(`${e.name} is a link to something that is not there.`, 'err'); return false; }
  await go.FS.OpenFile(path);
  app.flash(`Opened ${e.name}.`);
  return true;
}

function goParent(p) {
  if (!p.parent) return Promise.resolve(false);
  return p.go(p.parent, { focus: base(p.path) });
}

command({ name: 'cm_Enter', label: 'Open', run: enter });
command({ name: 'cm_GoToParent', label: 'Parent folder', run: (app) => goParent(app.cur) });
command({
  // Ctrl+PgDn (ours: TC's Ctrl+PgDn): open the folder under the cursor; on a link, jump to its target with the cursor
  // on it (F-076, F-078).
  name: 'cm_EnterDir', label: 'Open folder under cursor',
  run: async (app) => {
    const p = app.cur, e = p.cur;
    if (!e || e.up) return goParent(p);
    if (e.link) {
      const st = await go.FS.Stat(p.fullPath(e.name));
      if (st.target) {
        if (e.dir) return p.go(st.target);
        const sep = app.sepFor(st.target);
        const i = st.target.lastIndexOf(sep);
        return p.go(i > 0 ? st.target.slice(0, i) : sep, { focus: st.target.slice(i + 1) });
      }
    }
    return e.dir ? p.go(p.fullPath(e.name)) : enter(app);
  },
});
command({
  name: 'cm_GoToRoot', label: 'Root folder',
  run: async (app) => { const p = app.cur; const root = await go.FS.Root(p.path); return root !== p.path && p.go(root); },
});
command({ name: 'cm_GoToHome', label: 'Home folder', run: (app) => app.cur.go(app.home) });
command({ name: 'cm_GoToPreviousDir', label: 'Back', run: (app) => app.cur.goBack() });
command({ name: 'cm_GoToNextDir', label: 'Forward', run: (app) => app.cur.goForward() });
command({ name: 'cm_EditPath', label: 'Edit path', run: (app) => app.cur.editPath() });
// (cm_DirectoryHistory, Alt+Down, is the hotlist feature's: features/hotlist.js)

// ---- panels ----
command({ name: 'cm_FocusTrg', label: 'Switch panel', run: (app) => app.setActive(app.other) });
command({ name: 'cm_FocusLeft', label: 'Focus left panel', run: (app) => app.setActive(app.left) });
command({ name: 'cm_FocusRight', label: 'Focus right panel', run: (app) => app.setActive(app.right) });
command({ name: 'cm_FocusSrc', label: 'Focus source panel', run: (app) => app.setActive(app.cur) });
command({
  name: 'cm_MatchSrc', label: 'Target = source',
  run: (app) => app.other.go(app.cur.path, { focus: app.cur.cur?.up ? null : app.cur.cur?.name }),
});

// transfer opens, in the panel on `side`, the folder under the cursor of the active panel; on a file or "..", the
// active panel's own folder, with the cursor on that file (F-007, docs/spec area 2, 1.4). When `side` is the active
// panel itself, this just opens the folder under the cursor.
function transfer(app, side) {
  const to = app.panel(side), from = app.cur, e = from.cur;
  if (!e) return false;
  if (e.dir && !e.up) return to.go(from.fullPath(e.name));
  if (to === from) return false;
  return to.go(from.path, { focus: e.up ? null : e.name });
}
command({ name: 'cm_TransferLeft', label: 'Show folder under cursor in left panel', run: (app) => transfer(app, 'left') });
command({ name: 'cm_TransferRight', label: 'Show folder under cursor in right panel', run: (app) => transfer(app, 'right') });
command({
  name: 'cm_Exchange', label: 'Swap panels',
  run: (app) => {
    app.left.swapWith(app.right);
    app.emit('exchange');
    for (const p of app.panels) app.panelChanged(p, { moved: true });
  },
});
command({
  name: 'cm_RereadSource', label: 'Reread',
  run: async (app) => { await Promise.all([app.cur.reload(), app.loadVolumes()]); },
});
command({
  name: 'cm_SwitchHidSys', label: 'Hidden files', checked: (app) => app.showHidden, menuKey: true,
  run: (app) => {
    app.showHidden = !app.showHidden;
    for (const p of app.panels) { p.rebuild(); app.panelChanged(p, { moved: false }); }
    app.flash(app.showHidden ? 'Hidden files are shown.' : 'Hidden files are hidden.');
  },
});
command({
  // Esc: clears the command line, and stops reading a slow folder (F-741).
  name: 'cm_ClearCmdLine', label: 'Clear command line',
  run: (app) => {
    for (const p of app.panels) if (p.loading) go.FS.CancelList(p.side).catch(() => {});
    cmdline.clear();
  },
});

export function init(app) {
  // a folder that can't be read when the app starts leaves its reason in the panel; that is enough feedback
  app.on('ready', () => {
    for (const p of app.panels) if (!p.path) p.error ||= errText('No folder could be opened.');
  });
}
