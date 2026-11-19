// Selecting (FEATURES.md sections 6 and 13, behaviour from docs/spec area 3, sections 1 and 4): Ins and Space, Shift
// with the cursor keys, select all, unselect all, invert, save and restore; the Select files / Unselect files dialogs
// (Num + / Num -) with their mask history and saved filters; same extension / same name; selection files and the
// clipboard; copying names to the clipboard; folder compare (Shift+F2). Which rows a mask, a name list or a compare
// picks is decided in Go by internal/masks (feature_select.go), so the page never has mask rules of its own.
//
// The folder rule (setting "markDirectories", TC's MarkDirectories, default off): select all, invert and the group
// dialogs touch files only; the "Both" variants (Shift) touch folders too, and the other way round when it is on.
//
// Settings: "selectHistory" (the masks used in the group dialogs, newest first, shared by select and unselect),
// "selectFilters" ({name: mask}, used as ">name"), "compareIgnoreHourShift" (TC's 1hourdif).

import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { modal, messageDialog, formDialog, inputDialog, esc } from '../core/dialogs.js';
import { addControlCommand } from '../core/control.js';
import { attr } from '../core/format.js';
import { app as theApp } from '../core/app.js';
import { maskKey } from './select-mask-keys.js';
import { templateName, kindIndexes } from './find-select-lib.js';
import './selection.css';
import { tm } from '../core/i18n.js';

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
// Shift+Up / Shift+Down move as Up / Down do: one entry, or a row of tiles in Thumbnails (the panel's vertStep, F-850).
export const vertStep = (p) => Math.max(1, typeof p.vertStep === 'function' ? p.vertStep() : 1);
command({ name: 'cm_SelUp', label: 'Select up', run: (app) => selMove(app, app.cur.cursor - vertStep(app.cur)) });
command({ name: 'cm_SelDown', label: 'Select down', run: (app) => selMove(app, app.cur.cursor + vertStep(app.cur), app.cur.cursor === app.cur.entries.length - 1) });
command({ name: 'cm_SelPageUp', label: 'Select page up', run: (app) => selMove(app, app.cur.cursor - app.cur.pageSize()) });
command({ name: 'cm_SelPageDown', label: 'Select page down', run: (app) => selMove(app, app.cur.cursor + app.cur.pageSize()) });
// Shift+Left / Shift+Right (ours: TC has no cm_* names for them; spec area 3, 1.2): one side step with range marking.
// The views feature gives each panel sideStep(): a column in Brief, a tile in Thumbnails; without it (the Full view)
// the step is a page, so they act as Shift+PgUp / Shift+PgDn.
export const sideStep = (p) => Math.max(1, typeof p.sideStep === 'function' ? p.sideStep() : p.pageSize());
command({ name: 'cm_SelLeft', label: 'Select left', run: (app) => selMove(app, app.cur.cursor - sideStep(app.cur)) });
command({ name: 'cm_SelRight', label: 'Select right', run: (app) => selMove(app, app.cur.cursor + sideStep(app.cur)) });
command({ name: 'cm_SelHome', label: 'Select to the top', run: (app) => selMove(app, 0, true) });
command({ name: 'cm_SelEnd', label: 'Select to the end', run: (app) => selMove(app, app.cur.entries.length - 1, true) });

command({ name: 'cm_SelectAll', menuKey: true, label: 'Select all', run: (app) => app.cur.selectWhere((e) => !e.dir || markDirs(app)) });
command({ name: 'cm_SelectAllBoth', label: 'Select all, folders too', run: (app) => app.cur.selectWhere(() => true) });
command({ name: 'cm_SelectAllFiles', label: 'Select all files', run: (app) => app.cur.selectWhere((e) => !e.dir) });
command({ name: 'cm_SelectAllFolders', label: 'Select all folders', run: (app) => app.cur.selectWhere((e) => e.dir) });
command({ name: 'cm_ClearAll', menuKey: true, label: 'Unselect all', run: (app) => app.cur.clearSelection() });
command({ name: 'cm_ClearFiles', label: 'Unselect files', run: (app) => app.cur.selectWhere((e) => !e.dir, false) });
command({ name: 'cm_ClearFolders', label: 'Unselect folders', run: (app) => app.cur.selectWhere((e) => e.dir, false) });

function invert(app, which) {
  const p = app.cur;
  for (const e of p.entries) if (!e.up && which(e)) p.toggle(e);
  p.selectionChanged();
}
command({ name: 'cm_ExchangeSelection', menuKey: true, label: 'Invert selection', run: (app) => invert(app, (e) => !e.dir || markDirs(app)) });
command({ name: 'cm_ExchangeSelBoth', label: 'Invert selection, folders too', run: (app) => invert(app, () => true) });
command({ name: 'cm_ExchangeSelFiles', label: 'Invert selection of files', run: (app) => invert(app, (e) => !e.dir) });
command({ name: 'cm_ExchangeSelFolders', label: 'Invert selection of folders', run: (app) => invert(app, (e) => e.dir) });

command({ name: 'cm_SaveSelection', label: 'Save selection', run: (app) => app.cur.saveSelection() });
command({
  // Num /: selects the saved names in whatever folder is shown (adds to the selection); nothing saved, nothing happens.
  name: 'cm_RestoreSelection', menuKey: true, label: 'Restore selection',
  run: (app) => {
    const names = new Set(app.cur.saved);
    if (names.size) app.cur.selectWhere((e) => names.has(e.name));
  },
});
command({
  // TC's "load selection": the saved names replace the selection (Num / adds them instead).
  name: 'cm_LoadSelection', label: 'Load selection',
  run: (app) => {
    const p = app.cur;
    const names = new Set(p.saved);
    for (const e of p.entries) if (!e.up) p.select(e, names.has(e.name));
    p.selectionChanged();
  },
});

// ---- the rows as Go sees them ----

// rows turns the shown entries of a panel into what feature_select.go gets; the indexes it answers with are indexes
// into p.entries. withAttr adds the Attr column (only the details copy needs it). The AI tool select_files
// (aitools-basic.js) uses it too, so a model's mask selects exactly what the same mask selects in Num +.
export function rows(app, p, list = p.entries, withAttr = false) {
  return list.map((e) => ({
    name: e.name,
    dir: !!e.dir,
    size: e.dir ? (p.dirSizes?.get(e.name) ?? e.dirSize ?? -1) : e.size,
    mtime: e.mtime || 0,
    attr: withAttr && !e.up ? attr(e.mode, e.flags, app.platform) : '',
  }));
}

// mark selects (on) or unselects the rows at the given indexes of p.entries.
function mark(p, idx, on = true) {
  for (const i of idx || []) { const e = p.entries[i]; if (e && !e.up) p.select(e, on); }
  p.selectionChanged();
  return (idx || []).length;
}

const groupKind = (app, shift) => (markDirs(app) !== shift ? 'both' : 'files');
const filters = (app) => app.setting('selectFilters', {}) || {};
const noFiles = () => messageDialog({ Title: 'mormiCommander', Message: 'No files selected.', Kind: 'select-none' });
const join = (app, dir, name) => { const sep = app.sepFor(dir); return dir.endsWith(sep) || dir.endsWith('/') ? dir + name : dir + sep + name; };
const isLocal = (path) => !/^[a-z][a-z0-9+.-]*:\/\//i.test(path || '');

// ---- Select files / Unselect files (Num + / Num -) ----

const HISTORY_MAX = 20;
function remember(app, mask) {
  const h = (app.setting('selectHistory', []) || []).filter((m) => m !== mask);
  h.unshift(mask);
  app.setSetting('selectHistory', h.slice(0, HISTORY_MAX));
}

const TITLES = {
  true: { files: 'Select files', folders: 'Select folders', both: 'Select files and folders' },
  false: { files: 'Unselect files', folders: 'Unselect folders', both: 'Unselect files and folders' },
};

// groupDialog asks for a mask (the history in a list under the box, saved filters after it) and answers the mask, or
// null when cancelled. The mask is checked by Go before the dialog closes: a bad regular expression keeps it open.
function groupDialog(app, select, kind) {
  const history = app.setting('selectHistory', []) || [];
  const value = history[0] || '*.*';
  let onKey = () => false; // set once the box is built
  let searches = []; // the saved Find files searches, also usable as ">name" (F-537)
  go.Find.TemplateNames().then((n) => { searches = Array.isArray(n) ? n : []; }).catch(() => {});
  return modal((box, done) => {
    box.innerHTML = `<h2>${esc(TITLES[select][kind])}</h2>
      <div class="mc-row"><label for="mask">${select ? 'Select' : 'Unselect'} the ${kind === 'both' ? 'files and folders' : kind} that match:</label>
        <div class="mc-select-combo"><input id="mask" type="text" spellcheck="false" autocomplete="off" value="${esc(value)}" autofocus data-select="0,${value.length}">
        <button type="button" class="mc-select-drop" title="Masks used before (Alt+Down)" aria-label="Masks used before">▾</button></div></div>
      <ul class="mc-select-list" role="listbox" hidden></ul>
      <p class="mc-note">* and ? are wildcards; separate masks with spaces or ";"; after "|" come the masks to leave out (|*.bak); "dir\\" or "dir/" is a folder; "&lt;" starts a regular expression; "&gt;name" is a saved filter or a saved Find files search.</p>
      <div class="mc-select-tools"><button type="button" class="mc-select-define" title="Save a mask under a name">Define…</button></div>
      <p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">OK</button></div>`;
    const input = box.querySelector('#mask');
    const list = box.querySelector('.mc-select-list');
    const err = box.querySelector('.mc-err');
    let at = -1; // history step with Up/Down while the list is closed
    let items = [];
    let on = -1;
    const fill = () => {
      const fl = Object.keys(filters(app)).sort((a, b) => a.localeCompare(b));
      const sl = searches.filter((n) => templateName(`>${n}`, fl, searches));
      items = [...(app.setting('selectHistory', []) || []).map((m) => ({ text: m })), ...fl.map((n) => ({ text: `>${n}`, filter: true })),
        ...sl.map((n) => ({ text: `>${n}`, filter: true }))];
      list.innerHTML = items.length
        ? items.map((it, i) => `<li role="option" data-i="${i}" class="${it.filter ? 'mc-select-filter' : ''}">${esc(it.text)}</li>`).join('')
        : '<li class="mc-select-empty">No masks used yet</li>';
    };
    const highlight = (i) => {
      on = Math.max(0, Math.min(items.length - 1, i));
      list.querySelectorAll('li').forEach((li, k) => li.classList.toggle('on', k === on));
      list.querySelector('li.on')?.scrollIntoView({ block: 'nearest' });
    };
    const openList = (show) => { if (show) { fill(); on = -1; } list.hidden = !show; };
    const pick = (i) => { if (items[i]) { input.value = items[i].text; input.select(); } openList(false); input.focus(); };
    box.querySelector('.mc-select-drop').onclick = () => { openList(list.hidden); input.focus(); };
    list.onmousedown = (ev) => { const li = ev.target.closest('li[data-i]'); if (li) { ev.preventDefault(); pick(+li.dataset.i); } };
    // The mask box's keys (Alt+Down, Up/Down, Enter in the list) come through meta.onKey below: modal() stops key
    // events before they reach the input, so a listener on the input itself would never run.
    onKey = (ev) => {
      if (ev.target !== input) return false;
      const h = app.setting('selectHistory', []) || [];
      const a = maskKey(ev, { listOpen: !list.hidden, count: items.length, on, at, historyLength: h.length });
      if (!a) return false;
      if (a.do === 'toggle') openList(list.hidden);
      else if (a.do === 'highlight') highlight(a.index);
      else if (a.do === 'pick') pick(a.index);
      else if (a.do === 'step') { at = a.at; input.value = h[at]; input.select(); err.hidden = true; }
      return true;
    };
    input.addEventListener('input', () => { err.hidden = true; });
    box.querySelector('.mc-select-define').onclick = async () => {
      const r = await defineFilter(app, input.value.trim().startsWith('>') ? '' : input.value.trim());
      if (r) { input.value = `>${r}`; input.select(); }
      if (!list.hidden) fill();
      input.focus();
    };
    box.querySelector('.cancel').onclick = () => done(null);
    box.onsubmit = async (ev) => {
      ev.preventDefault();
      const mask = input.value.trim();
      if (!mask) { done(''); return; }
      if (templateName(mask, Object.keys(filters(app)), searches)) { done(mask); return; }
      const problem = await go.Select.Check(mask, filters(app)).catch((e) => errText(e));
      if (problem) { err.hidden = false; err.textContent = problem; input.focus(); return; }
      done(mask);
    };
  }, null, { kind: select ? 'select-group' : 'unselect-group', onKey: (ev) => onKey(ev) });
}

// defineFilter saves a mask under a name (used as ">name" in the dialogs); an empty mask deletes the name. It answers
// the name, or null.
function defineFilter(app, mask) {
  return formDialog('Saved filter',
    `<div class="mc-row"><label for="filter-name">Name:</label><input id="filter-name" type="text" spellcheck="false" autofocus></div>
     <div class="mc-row"><label for="filter-mask">Mask:</label><input id="filter-mask" type="text" spellcheck="false" value="${esc(mask)}"></div>
     <p class="mc-note">Use it as "&gt;name" in Select files. An empty mask deletes the saved filter of that name.</p>`,
    (box) => {
      const name = box.querySelector('#filter-name').value.trim();
      const m = box.querySelector('#filter-mask').value.trim();
      if (!name) return 'Give the filter a name.';
      if (/[\s;|"<>]/.test(name)) return 'A filter name cannot contain spaces or ; | " < >.';
      return { name, mask: m };
    },
    null, { okLabel: 'Save', kind: 'select-filter' },
  ).then(async (r) => {
    if (!r) return null;
    const all = { ...filters(app) };
    for (const k of Object.keys(all)) if (k.toLowerCase() === r.name.toLowerCase()) delete all[k];
    if (r.mask) {
      const problem = await go.Select.Check(r.mask, all).catch((e) => errText(e));
      if (problem) { app.flash(problem, 'err'); return null; }
      all[r.name] = r.mask;
    }
    app.setSetting('selectFilters', all);
    return r.mask ? r.name : null;
  });
}

// group runs a select (select = true) or unselect group command; args.mask skips the dialog (control mode, buttons).
async function group(app, args, select, kind) {
  const p = app.cur;
  let mask = typeof args?.mask === 'string' ? args.mask.trim() : null;
  if (mask === null) mask = await groupDialog(app, select, kind);
  if (!mask) return false; // cancelled, or empty: nothing happens, nothing is kept
  let idx;
  try {
    // ">name" of a saved Find files search (F-537): Go judges the rows by the search's filters
    const searches = mask.startsWith('>') ? await go.Find.TemplateNames().catch(() => []) : [];
    const tpl = templateName(mask, Object.keys(filters(app)), searches);
    if (tpl) idx = kindIndexes(p.entries, await go.Find.TemplateMatch(tpl, p.path, p.entries.map((e) => (e.up ? '' : e.name))), kind);
    else idx = await go.Select.Group(rows(app, p), mask, kind, filters(app));
  } catch (e) { app.flash(errText(e), 'err'); return false; }
  remember(app, mask);
  mark(p, idx, select);
  return true;
}

command({ name: 'cm_SpreadSelection', menuKey: true, label: 'Select group…', run: (app, a) => group(app, a, true, groupKind(app, false)) });
command({ name: 'cm_SelectBoth', label: 'Select group, folders too…', run: (app, a) => group(app, a, true, groupKind(app, true)) });
command({ name: 'cm_SelectFiles', label: 'Select a group of files…', run: (app, a) => group(app, a, true, 'files') });
command({ name: 'cm_SelectFolders', label: 'Select a group of folders…', run: (app, a) => group(app, a, true, 'folders') });
command({ name: 'cm_ShrinkSelection', menuKey: true, label: 'Unselect group…', run: (app, a) => group(app, a, false, groupKind(app, false)) });
command({ name: 'cm_UnselectFiles', label: 'Unselect a group of files…', run: (app, a) => group(app, a, false, 'files') });
command({ name: 'cm_UnselectFolders', label: 'Unselect a group of folders…', run: (app, a) => group(app, a, false, 'folders') });

// ---- same extension, same name (Alt+Num + / Alt+Num -) ----

async function same(app, mode, on) {
  const p = app.cur;
  if (!p.cur || p.cur.up) return false;
  const idx = await go.Select.Same(rows(app, p), p.cursor, mode, groupKind(app, false));
  mark(p, idx, on);
  return true;
}
command({ name: 'cm_SelectCurrentExtension', menuKey: true, label: 'Select all with the same extension', run: (app) => same(app, 'ext', true) });
command({ name: 'cm_UnselectCurrentExtension', menuKey: true, label: 'Unselect all with the same extension', run: (app) => same(app, 'ext', false) });
command({ name: 'cm_SelectCurrentName', label: 'Select all with the same name', run: (app) => same(app, 'name', true) });
command({ name: 'cm_UnselectCurrentName', label: 'Unselect all with the same name', run: (app) => same(app, 'name', false), menu: 'Mark', menuAfter: 'cm_SelectCurrentName' });
command({ name: 'cm_SelectCurrentNameExt', label: 'Select all with the same name and extension', run: (app) => same(app, 'nameext', true), menu: 'Mark', menuAfter: 'cm_UnselectCurrentName' });
command({ name: 'cm_UnselectCurrentNameExt', label: 'Unselect all with the same name and extension', run: (app) => same(app, 'nameext', false), menu: 'Mark', menuAfter: 'cm_SelectCurrentNameExt' });
command({
  // ours (F-143): the names selected in the other panel get selected here
  name: 'cm_SelectFromOtherPanel', label: 'Select the names selected in the other panel', menu: 'Mark', menuAfter: 'cm_UnselectCurrentNameExt',
  enabled: (app) => app.other.selectedEntries().length > 0,
  run: async (app) => {
    const names = app.other.selectedEntries().map((e) => e.name);
    if (!names.length) return false;
    mark(app.cur, await go.Select.Names(rows(app, app.cur), names));
    return true;
  },
});

// ---- selection files and the clipboard ----

const defaultFile = (app, p) => join(app, isLocal(p.path) ? p.path : app.home, 'selection.txt');

command({
  name: 'cm_SaveSelectionToFile', label: 'Save selection to file…',
  run: async (app, args = {}) => {
    const p = app.cur;
    const names = p.selectedEntries().map((e) => e.name);
    if (!names.length) { await noFiles(); return false; }
    let r = args.path ? { path: args.path, full: !!args.full } : null;
    if (!r) {
      r = await formDialog('Save selection to file',
        `<div class="mc-row"><label for="sel-file">Save the ${names.length === 1 ? 'selected name' : `${names.length} selected names`} to the file:</label>
           <input id="sel-file" type="text" spellcheck="false" autofocus value="${esc(defaultFile(app, p))}"></div>
         <label class="mc-check"><input id="sel-full" type="checkbox"> Full paths instead of names</label>
         <p class="mc-note">One name per line. Load selection from file reads it back.</p>`,
        (box) => {
          const path = box.querySelector('#sel-file').value.trim();
          return path ? { path, full: box.querySelector('#sel-full').checked } : 'Type the name of the file.';
        }, null, { okLabel: 'Save', kind: 'select-save-file' });
      if (!r) return false;
    }
    const path = app.resolvePath(r.path, isLocal(p.path) ? p.path : app.home);
    if (!args.overwrite && await go.Select.Exists(path)) {
      const a = await messageDialog({ Title: 'Save selection to file', Message: `The file ${path} is already there. Replace it?`, Buttons: ['Replace', 'Cancel'], Kind: 'select-overwrite' });
      if (a !== 'Replace') return false;
    }
    try { await go.Select.SaveFile(path, p.path, names, r.full); }
    catch (e) { await messageDialog({ Title: 'Save selection to file', Message: errText(e), Kind: 'select-error' }); return false; }
    app.flash(tm('Saved {what} to {path}.', { what: names.length === 1 ? tm('1 name') : tm('{n} names', { n: names.length }), path }));
    const dir = path.slice(0, Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')));
    for (const q of app.panels) if (q.path === dir) q.reload();
    return true;
  },
});

command({
  name: 'cm_LoadSelectionFromFile', label: 'Load selection from file…',
  run: async (app, args = {}) => {
    const p = app.cur;
    let file = args.path || null;
    if (!file) {
      file = await inputDialog({ title: 'Load selection from file', label: 'Select the names listed in the file:', value: defaultFile(app, p),
        okLabel: 'Load', note: 'A line can be a name, a full path in this folder, or a mask such as *.jpg.', kind: 'select-load-file',
        validate: (v) => (v.trim() ? '' : 'Type the name of the file.') });
      if (!file) return false;
    }
    const path = app.resolvePath(file, isLocal(p.path) ? p.path : app.home);
    try { mark(p, await go.Select.LoadFile(path, p.path, rows(app, p))); }
    catch (e) { await messageDialog({ Title: 'Load selection from file', Message: errText(e), Kind: 'select-error' }); return false; }
    return true;
  },
});

command({
  name: 'cm_LoadSelectionFromClip', label: 'Load selection from clipboard',
  run: async (app) => {
    const p = app.cur;
    try { mark(p, await go.Select.LoadClip(p.path, rows(app, p))); }
    catch (e) { app.flash(errText(e), 'err'); return false; }
    return true;
  },
});

// ---- copy names to the clipboard ----

function copyNames(name, label, extra = {}) {
  command({
    name, label, ...extra,
    run: async (app) => {
      const p = app.cur;
      const list = p.targets();
      if (!list.length) { await noFiles(); return false; }
      try { await go.Select.CopyNames(name, p.path, rows(app, p, list, /Details/.test(name))); }
      catch (e) { app.flash(errText(e), 'err'); return false; }
      app.flash(list.length === 1 ? `Copied ${list[0].name} to the clipboard.` : `Copied ${list.length} names to the clipboard.`);
      return true;
    },
  });
}
copyNames('cm_CopyNamesToClip', 'Copy names to clipboard');
copyNames('cm_CopyFullNamesToClip', 'Copy names with full path');
copyNames('cm_CopyDetailsToClip', 'Copy names with details');
copyNames('cm_CopyFullDetailsToClip', 'Copy full names with details', { menu: 'Mark', menuAfter: 'cm_CopyDetailsToClip' });
copyNames('cm_CopyNetNamesToClip', 'Copy names with network path', { menu: 'Mark', menuAfter: 'cm_CopyFullDetailsToClip' });
copyNames('cm_CopyUnixNamesToClip', 'Copy names as Unix paths', { menu: 'Mark', menuAfter: 'cm_CopyNetNamesToClip' });
copyNames('cm_CopyURLNamesToClip', 'Copy names as file URLs', { menu: 'Mark', menuAfter: 'cm_CopyUnixNamesToClip' });
// ours: full paths in double quotes, one per line, for pasting into a shell (no default key; assignable)
copyNames('cm_CopyFullNamesQuotedToClip', 'Copy names with full path, quoted', { menu: 'Mark', menuAfter: 'cm_CopyURLNamesToClip' });

async function copyPath(app, p) {
  try { const t = await go.Select.CopyPath(p.path); app.flash(tm('Copied {what} to the clipboard.', { what: t })); return true; }
  catch (e) { app.flash(errText(e), 'err'); return false; }
}
command({ name: 'cm_CopySrcPathToClip', label: 'Copy folder path', run: (app) => copyPath(app, app.cur) });
command({ name: 'cm_CopyTrgPathToClip', label: 'Copy target folder path', run: (app) => copyPath(app, app.other), menu: 'Mark', menuAfter: 'cm_CopySrcPathToClip' });

// (Shift+F2 compare folders: features/comparedirs.js)

// control mode: the selected names of both panels and the mask history, for tests
addControlCommand('selection', async () => {
  const a = theApp;
  return {
    ok: true,
    left: a.left.selectedEntries().map((e) => e.name),
    right: a.right.selectedEntries().map((e) => e.name),
    saved: { left: [...a.left.saved], right: [...a.right.saved] },
    history: a.setting('selectHistory', []),
    filters: a.setting('selectFilters', {}),
  };
});

export function init(app) {
  // a key that is not a Shift move ends a Shift run
  app.on('cursor', (p) => { if (paint && paint.panel === p && paint.at !== p.cursor) paint = null; });
}
