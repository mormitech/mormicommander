// Selecting with the keyboard: Ins and Space, Shift with the cursor keys, select all, unselect all, invert, save and
// restore (FEATURES.md section 6, behaviour from docs/spec area 3, section 1). Group selection by pattern (Num+ / Num-)
// and selecting by extension are features of their own.
//
// The folder rule (setting "markDirectories", TC's MarkDirectories, default off): select all and invert touch files
// only; the "Both" variants touch folders too.

import { command } from '../core/registry.js';

const markDirs = (app) => !!app.setting('markDirectories', false);

command({
  // Ins: toggle and go down; on ".." it only goes down; on the last row it toggles and stays.
  name: 'cm_SelectToggleDown', label: 'Select and go down',
  run: (app) => { const p = app.cur; p.toggle(p.cur); p.moveCursor(1); p.selectionChanged(); },
});
command({
  // Space: toggle without moving (setting "spaceMovesDown" moves like Ins). The folder size count is the folder-size
  // feature's: it listens to 'selection'.
  name: 'cm_SelectToggle', label: 'Select',
  run: (app) => {
    const p = app.cur;
    if (!p.cur || p.cur.up) return;
    p.toggle(p.cur);
    if (app.setting('spaceMovesDown', false)) p.moveCursor(1);
    p.selectionChanged();
    app.emit('space-select', p, p.cur);
  },
});

// Shift with a cursor key paints a state over the rows the cursor leaves (spec area 3, 1.4): the state is decided by
// the row under the cursor at the first Shift move of a run, and kept while the run goes on. Turning back within a run
// takes back: the rows the cursor comes back onto get the opposite state, so Shift+Down, Shift+Down, Shift+Up leaves
// one entry selected, as in every other file manager.
let paint = null; // {panel, state, at, dir, back}: the run in progress
function selMove(app, to, inclusive = false) {
  const p = app.cur;
  const from = p.cursor;
  to = Math.max(0, Math.min(p.entries.length - 1, to));
  const step = to > from ? 1 : -1;
  let run = paint && paint.panel === p && paint.at === from ? paint : null;
  if (!run) run = { panel: p, state: !p.isSelected(p.cur), dir: step, back: false };
  else if (to !== from && step !== run.dir) { run.back = !run.back; run.dir = step; }
  if (to === from) p.select(p.cur, run.state);
  else if (run.back) for (let i = from + step; i !== to + step; i += step) p.select(p.entries[i], !run.state);
  else for (let i = from; i !== to; i += step) p.select(p.entries[i], run.state);
  if (inclusive && !run.back) p.select(p.entries[to], run.state);
  p.setCursor(to); // (its 'cursor' event ends any run; this one goes on below)
  run.at = p.cursor;
  paint = run;
  p.selectionChanged();
}
command({ name: 'cm_SelUp', label: 'Select up', run: (app) => selMove(app, app.cur.cursor - 1) });
command({ name: 'cm_SelDown', label: 'Select down', run: (app) => selMove(app, app.cur.cursor + 1, app.cur.cursor === app.cur.entries.length - 1) });
command({ name: 'cm_SelPageUp', label: 'Select page up', run: (app) => selMove(app, app.cur.cursor - app.cur.pageSize()) });
command({ name: 'cm_SelPageDown', label: 'Select page down', run: (app) => selMove(app, app.cur.cursor + app.cur.pageSize()) });
command({ name: 'cm_SelHome', label: 'Select to the top', run: (app) => selMove(app, 0, true) });
command({ name: 'cm_SelEnd', label: 'Select to the end', run: (app) => selMove(app, app.cur.entries.length - 1, true) });

command({ name: 'cm_SelectAll', label: 'Select all', run: (app) => app.cur.selectWhere((e) => !e.dir || markDirs(app)) });
command({ name: 'cm_SelectAllBoth', label: 'Select all, folders too', run: (app) => app.cur.selectWhere(() => true) });
command({ name: 'cm_SelectAllFiles', label: 'Select all files', run: (app) => app.cur.selectWhere((e) => !e.dir) });
command({ name: 'cm_SelectAllFolders', label: 'Select all folders', run: (app) => app.cur.selectWhere((e) => e.dir) });
command({ name: 'cm_ClearAll', label: 'Unselect all', run: (app) => app.cur.clearSelection() });
command({ name: 'cm_ClearFiles', label: 'Unselect files', run: (app) => app.cur.selectWhere((e) => !e.dir, false) });
command({ name: 'cm_ClearFolders', label: 'Unselect folders', run: (app) => app.cur.selectWhere((e) => e.dir, false) });

function invert(app, which) {
  const p = app.cur;
  for (const e of p.entries) if (!e.up && which(e)) p.toggle(e);
  p.selectionChanged();
}
command({ name: 'cm_ExchangeSelection', label: 'Invert selection', run: (app) => invert(app, (e) => !e.dir || markDirs(app)) });
command({ name: 'cm_ExchangeSelBoth', label: 'Invert selection, folders too', run: (app) => invert(app, () => true) });
command({ name: 'cm_ExchangeSelFiles', label: 'Invert selection of files', run: (app) => invert(app, (e) => !e.dir) });
command({ name: 'cm_ExchangeSelFolders', label: 'Invert selection of folders', run: (app) => invert(app, (e) => e.dir) });

command({ name: 'cm_SaveSelection', label: 'Save selection', run: (app) => app.cur.saveSelection() });
command({
  // Num /: selects the saved names in whatever folder is shown (adds to the selection); nothing saved, nothing happens.
  name: 'cm_RestoreSelection', label: 'Restore selection',
  run: (app) => {
    const names = new Set(app.cur.saved);
    if (names.size) app.cur.selectWhere((e) => names.has(e.name));
  },
});

export function init(app) {
  // a key that is not a Shift move ends a Shift run
  app.on('cursor', (p) => { if (paint && paint.panel === p && paint.at !== p.cursor) paint = null; });
}
