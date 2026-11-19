// Synchronize folders (cm_SyncDirs, FEATURES.md F-198, F-360, F-485 and section 25, behaviour from docs/spec area 5,
// section 9): a window with the two folders, the file types, the compare options and the Show buttons, a result list
// with the action of every row (which can be changed per row or for a selection, from the arrow column, the keys or the
// right-click menu), and Synchronize… with a summary dialog, progress, the overwrite question and an automatic compare
// afterwards. Settings can be saved under a name. A pair opens in Compare by content (F-553), which re-reads the pair
// when a side was saved. The comparing, the default actions and the safe copying are internal/syncdirs'
// (feature_syncdirs.go); this file draws and takes the keys.
//
// Keys in the list: Up/Down/PgUp/PgDn/Home/End move; Space selects, Ins selects and moves down, Shift+arrows extend,
// Ctrl+A / Num+ / Num- select all / by pattern; Ctrl+Right, Ctrl+Left copy to the right / left, Ctrl+R reverses,
// Delete removes the action, Shift+Delete deletes on the right; Enter compares a pair by content; F3 / Shift+F3 view
// the left / right file and F4 / Shift+F4 edit it (a name on one side only: that file); Shift+F10 or the
// context-menu key opens the menu; Ctrl+1…Ctrl+6 toggle the Show buttons; F2 or Enter in a field compares; Esc stops a
// compare, or closes the window.
//
// The window is a dialog of kind "syncdirs" (fields sync-left, sync-right, sync-filter, the option checkboxes and the
// sync-show-* buttons), so control mode reads and answers it; the control command "syncdirs" tells the rows.
// A side can also be an archive (/backup/2024.zip/docs) or a server folder (sftp://me@host/www), typed or taken from a
// panel (F-555): the Go side tells what kind of place each side is, and the window says when one can't be changed, that
// deletes there can't go to the trash, and when a server can't keep file dates (syncdirs-lib.js).
//
// Setting "syncdirs": {options, show} — the last options and Show buttons; also confirmOverwrite, trash and trashAll from
// the Synchronize dialog (trashAll, off by default: emptied folders and replaced links go to the trash too, issue #99;
// trashReplaced, off by default: files a copy overwrites go to the trash first, local folders only).

import './syncdirs.css';
import { command } from '../core/registry.js';
import { go, on, errText, setClipboardText } from '../core/go.js';
import { esc, modal, messageDialog, inputDialog, formDialog } from '../core/dialogs.js';
import { t as tt, translateTree } from '../core/i18n.js';
import { addControlCommand } from '../core/control.js';
import { size, bytes, date, plural, dateWidth } from '../core/format.js';
import { openWindow, contextMenu, joinPath } from './synccompare-ui.js';
import { compareFiles } from './comparefiles.js';
import { isVirtual, readOnlyNote, dialogNotes, contentTip, fileSide, pathsWithAction } from './syncdirs-lib.js';

const ROW_H = 20;
const SHOW_KEYS = ['leftToRight', 'equal', 'unequal', 'rightToLeft', 'duplicates', 'singles'];
const SHOW_LABELS = { leftToRight: '→', equal: '=', unequal: '≠', rightToLeft: '←', duplicates: 'Duplicates', singles: 'Singles' };
const SHOW_TIPS = { leftToRight: 'Show files to copy to the right (Ctrl+1)', equal: 'Show equal files (Ctrl+2)', unequal: 'Show files that differ (Ctrl+3)',
  rightToLeft: 'Show files to copy to the left (Ctrl+4)', duplicates: 'Show names that are on both sides (Ctrl+5)', singles: 'Show names that are on one side only (Ctrl+6)' };
const DEFAULT_SHOW = { leftToRight: true, equal: false, unequal: true, rightToLeft: true, duplicates: true, singles: true };
const DEFAULT_OPTS = { subdirs: true, byContent: false, ignoreDate: false, asymmetric: false, ignoreEmptyDirs: false, toleranceSeconds: 2, ignoreHours: 0, filter: '*.*' };
const OVERWRITE = { Overwrite: 'overwrite', Skip: 'skip', 'Overwrite all': 'overwrite_all', 'Skip all': 'skip_all', Cancel: 'cancel' };

let current = null; // the open window's state, for control mode

const ms = (t) => { const v = Date.parse(t); return Number.isFinite(v) && v > 0 ? v : 0; };
const settings = (app) => app.setting('syncdirs', {}) || {};

// wildcard turns a simple pattern ("*.txt *.md", "a?c") into a test of a name, for Num+ / Num-.
function wildcard(pattern, ci) {
  const parts = pattern.split(/[\s;]+/).filter(Boolean).map((p) => new RegExp('^' + p.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.') + '$', ci ? 'i' : ''));
  return (name) => parts.some((r) => r.test(name));
}

async function clip(text) {
  try {
    if (await setClipboardText(text)) return true;
    await navigator.clipboard.writeText(text);
    return true;
  } catch { return false; }
}

function syncWindow(app, id, args = {}) {
  const saved = settings(app);
  const st = {
    id, rows: [], summary: null, errors: [], compared: false, running: false, syncing: false,
    stale: false, cmpLeft: '', cmpRight: '', // stale: the folder fields changed since the last compare
    cursor: 0, sel: new Set(), anchor: -1,
    show: { ...DEFAULT_SHOW, ...(saved.show || {}), ...(args.show || {}) },
    left: args.left || app.left.path, right: args.right || app.right.path,
    progress: null, sides: null, // sides: {left, right} kinds of place of the last compare (local, archive, ftp…)
  };
  current = st;
  const opts0 = { ...DEFAULT_OPTS, ...(saved.options || {}), ...(args.options || {}) };
  let box, body, spacer, rowsEl, status, offs = [];
  let done = () => {};

  const q = (s) => box.querySelector(s);
  const readOpts = () => ({
    subdirs: q('#sync-subdirs').checked, byContent: q('#sync-content').checked, ignoreDate: q('#sync-ignoredate').checked,
    asymmetric: q('#sync-asym').checked, ignoreEmptyDirs: q('#sync-emptydirs').checked, toleranceSeconds: q('#sync-fat').checked ? 2 : 0,
    ignoreHours: q('#sync-hour').checked ? 1 : 0, filter: q('#sync-filter').value.trim() || '*.*', onlySelected: q('#sync-onlysel').checked,
  });
  const writeOpts = (o) => {
    q('#sync-subdirs').checked = !!o.subdirs; q('#sync-content').checked = !!o.byContent; q('#sync-ignoredate').checked = !!o.ignoreDate;
    q('#sync-asym').checked = !!o.asymmetric; q('#sync-emptydirs').checked = !!o.ignoreEmptyDirs; q('#sync-fat').checked = (o.toleranceSeconds ?? 2) > 0;
    q('#sync-hour').checked = (o.ignoreHours || 0) > 0; q('#sync-filter').value = o.filter || '*.*';
  };
  const writeShow = () => { for (const k of SHOW_KEYS) q(`#sync-show-${k}`).checked = !!st.show[k]; };
  const remember = () => { const { onlySelected, ...o } = readOpts(); app.setSetting('syncdirs', { options: o, show: st.show }); };
  const filterArg = (o) => (o.filter === '*.*' || o.filter === '*' ? '' : o.filter);

  // ---- data ----
  const refresh = async (keepPath) => {
    const want = keepPath ?? st.rows[st.cursor]?.path;
    try {
      const v = await go.SyncDirs.Rows(st.id, st.show);
      st.rows = v.rows || [];
      st.summary = v.summary;
      st.errors = v.errors || [];
      st.compared = v.compared && !st.stale;
      st.cmpLeft = v.left || ''; st.cmpRight = v.right || '';
      st.sides = v.leftSide && v.rightSide ? { left: v.leftSide, right: v.rightSide } : null;
      if (st.stale) { st.rows = []; st.summary = null; st.errors = []; }
    } catch (e) { app.flash(errText(e), 'err'); }
    const paths = new Set(st.rows.map((r) => r.path));
    for (const p of [...st.sel]) if (!paths.has(p)) st.sel.delete(p);
    const i = want ? st.rows.findIndex((r) => r.path === want) : -1;
    st.cursor = i >= 0 ? i : Math.min(st.cursor, Math.max(0, st.rows.length - 1));
    paint();
  };

  const compare = async () => {
    if (st.running || st.syncing) return;
    const o = readOpts();
    remember();
    const selected = o.onlySelected ? [...new Set([...app.left.selectedEntries(), ...app.right.selectedEntries()].map((e) => e.name))] : [];
    const req = {
      left: q('#sync-left').value.trim(), right: q('#sync-right').value.trim(), selected,
      options: { ...o, filter: filterArg(o), onlySelected: o.onlySelected && selected.length > 0, followSymlinks: false, case: 'auto' },
    };
    st.running = true;
    st.progress = { dirs: 0, files: 0, current: '' };
    st.rows = []; st.sel.clear(); st.cursor = 0; st.compared = false; st.stale = false;
    st.fieldL = req.left; st.fieldR = req.right;
    paint();
    try {
      await go.SyncDirs.Compare(st.id, req);
    } catch (e) {
      st.running = false;
      st.stale = true; // the Go side may hold no result now; ask for a new compare
      paint();
      await messageDialog({ Title: 'Synchronize folders', Message: errText(e), Kind: 'syncdirs-error' });
    }
  };
  // The folder fields changed (typing, swap, loaded settings): the rows and their copy directions belong to the old
  // pair, so they go away and Synchronize… waits for a new Compare.
  const invalidate = () => {
    if (st.syncing) return;
    if (!st.stale && q('#sync-left').value.trim() === st.fieldL && q('#sync-right').value.trim() === st.fieldR) return; // same text
    st.stale = true;
    if (st.running) return; // refresh after the running compare drops its rows
    if (!st.compared && !st.rows.length) return;
    st.rows = []; st.sel.clear(); st.cursor = 0; st.compared = false; st.summary = null; st.errors = [];
    paint();
  };
  const compareDone = async (d) => {
    st.running = false;
    st.progress = null;
    await refresh(null);
    if (d.error) await messageDialog({ Title: 'Synchronize folders', Message: d.error, Kind: 'syncdirs-error' });
    else if (d.cancelled) app.flash('The compare was stopped; the list shows what was compared so far.');
    else if (st.errors.length) app.flash(`${plural(st.errors.length, 'folder')} could not be read: ${st.errors[0]}`, 'err');
    else if (st.summary && allEqual(st.summary)) app.flash('All files are equal.');
    else if (readOnlyNote(st.sides)) app.flash(readOnlyNote(st.sides));
  };
  const allEqual = (s) => s.leftNewer + s.rightNewer + s.leftOnly + s.rightOnly + s.unequal === 0;

  // ---- drawing ----
  const symbol = (r) => {
    switch (r.action) {
      case 'copyLeftToRight': return ['→', 'lr'];
      case 'copyRightToLeft': return ['←', 'rl'];
      case 'deleteLeft': return ['✕ ·', 'del'];
      case 'deleteRight': return ['· ✕', 'del'];
      case 'deleteBoth': return ['✕ ✕', 'del'];
    }
    if (r.category === 'equal') return ['=', 'eq'];
    if (r.category === 'unequal') return ['≠', 'ne'];
    return ['', ''];
  };
  const side = (r, e, s) => {
    if (!e) return `<span class="mc-sync-name"></span><span class="mc-sync-size"></span><span class="mc-sync-date"></span>`;
    const dir = e.kind === 'dir';
    const pad = `padding-left:${6 + r.depth * 14}px`;
    const name = `<span class="mc-sync-name${dir ? ' dir' : ''}${e.isLink ? ' link' : ''}" style="${pad}" title="${esc(r[s + 'Path'])}">${esc(e.name)}${dir ? '/' : ''}</span>`;
    const newer = (s === 'left' && r.category === 'leftNewer') || (s === 'right' && r.category === 'rightNewer');
    const sz = `<span class="mc-sync-size">${dir ? '&lt;DIR&gt;' : e.kind === 'link' ? 'link' : bytes(e.size)}</span>`;
    const dt = `<span class="mc-sync-date${newer ? ' newer' : ''}">${date(ms(e.modTime))}</span>`;
    return s === 'left' ? name + sz + dt : dt + sz + name;
  };
  const render = () => {
    const n = st.rows.length;
    spacer.style.height = `${n * ROW_H}px`;
    const top = body.scrollTop, h = body.clientHeight || 400;
    const from = Math.max(0, Math.floor(top / ROW_H) - 5), to = Math.min(n, Math.ceil((top + h) / ROW_H) + 5);
    let html = '';
    for (let i = from; i < to; i++) {
      const r = st.rows[i];
      const [sym, sc] = symbol(r);
      const cls = ['mc-sync-row', `c-${r.category}`];
      if (i === st.cursor) cls.push('cur');
      if (st.sel.has(r.path)) cls.push('sel');
      if (r.isDir) cls.push('dirrow');
      const tip = r.note ? ` title="${esc(r.note)}"` : '';
      html += `<div class="${cls.join(' ')}" data-i="${i}" data-path="${esc(r.path)}" style="top:${i * ROW_H}px"${tip}>`
        + `<span class="mc-sync-l">${side(r, r.left, 'left')}</span>`
        + `<span class="mc-sync-act ${sc}${r.unselected && r.action !== 'none' ? ' off' : ''}" title="${esc(actionTip(r))}">${sym}</span>`
        + `<span class="mc-sync-r">${side(r, r.right, 'right')}</span></div>`;
    }
    rowsEl.innerHTML = html;
  };
  const actionTip = (r) => {
    const t = { none: 'No action', copyLeftToRight: 'Copy to the right', copyRightToLeft: 'Copy to the left', deleteLeft: 'Delete on the left',
      deleteRight: 'Delete on the right', deleteBoth: 'Delete on both sides' }[r.action] || r.action;
    return (r.unselected && r.action !== 'none' ? `${t} (left out: click to include)` : t) + ' — click to change';
  };
  const paintStatus = () => {
    if (st.running) {
      const p = st.progress || {};
      status.textContent = `Comparing… ${plural(p.dirs || 0, 'folder')}, ${plural(p.files || 0, 'file')}${p.current ? ` · ${p.current}` : ''}  (Esc stops)`;
      return;
    }
    if (!st.compared) { status.textContent = tt(st.stale ? 'The folders changed: press Compare (Enter).' : 'Choose the folders and press Compare (Enter).'); return; }
    const s = st.summary || {};
    const files = st.rows.filter((r) => !r.isDir);
    const total = files.reduce((a, r) => a + Math.max(r.left?.size || 0, r.right?.size || 0), 0);
    let t = `Shown: ${plural(files.length, 'file')} (${size(total)}) · Equal ${s.equal ?? 0} · Different ${s.unequal ?? 0} · Newer left ${s.leftNewer ?? 0} · Newer right ${s.rightNewer ?? 0} · Only left ${s.leftOnly ?? 0} · Only right ${s.rightOnly ?? 0}`;
    if (st.errors.length) t += ` · ${plural(st.errors.length, 'error')}`;
    if (st.sel.size) t += ` · ${st.sel.size} selected`;
    status.textContent = t;
  };
  const paintTools = () => {
    const cb = q('.b-compare');
    cb.textContent = tt(st.running ? 'Stop' : 'Compare');
    q('.b-sync').disabled = !st.compared || st.running || st.syncing;
    q('#sync-onlysel').disabled = !app.left.selectedEntries().length && !app.right.selectedEntries().length;
    box.classList.toggle('running', st.running);
  };
  const paint = () => { paintTools(); paintStatus(); render(); };
  const showRow = (i) => {
    const top = i * ROW_H, h = body.clientHeight || 400;
    if (top < body.scrollTop) body.scrollTop = top;
    else if (top + ROW_H > body.scrollTop + h) body.scrollTop = top + ROW_H - h;
  };
  const move = (to, extend = false) => {
    if (!st.rows.length) return;
    to = Math.max(0, Math.min(st.rows.length - 1, to));
    if (extend) {
      if (st.anchor < 0) st.anchor = st.cursor;
      const [a, b] = st.anchor < to ? [st.anchor, to] : [to, st.anchor];
      st.sel.clear();
      for (let k = a; k <= b; k++) st.sel.add(st.rows[k].path);
    } else st.anchor = -1;
    st.cursor = to;
    showRow(to);
    paintStatus();
    render();
  };
  const targets = () => {
    if (st.sel.size) return st.rows.filter((r) => st.sel.has(r.path));
    return st.rows[st.cursor] ? [st.rows[st.cursor]] : [];
  };

  // ---- actions ----
  const busy = () => { if (st.running || st.syncing) { app.flash('Wait until the compare has finished.'); return true; } if (!st.compared) { app.flash('Compare the folders first.'); return true; } return false; };
  const act = async (fn) => {
    if (busy()) return;
    try { await fn(); } catch (e) { app.flash(errText(e), 'err'); }
    await refresh();
  };
  const setAction = (action, rows = targets()) => act(async () => {
    const dirs = rows.filter((r) => r.isDir).map((r) => r.path), files = rows.filter((r) => !r.isDir).map((r) => r.path);
    let n = 0, err = null;
    for (const [paths, tree] of [[files, false], [dirs, true]]) {
      if (!paths.length) continue;
      try { n += await go.SyncDirs.SetAction(st.id, paths, action, tree); } catch (e) { err = e; }
    }
    if (!n && err) throw err;
  });
  const reverse = (rows = targets()) => act(() => go.SyncDirs.Reverse(st.id, rows.map((r) => r.path)));
  const include = (on, rows = targets()) => act(() => go.SyncDirs.Select(st.id, rows.map((r) => r.path), on, true));
  // a click on the arrow: a left-out row is included first; otherwise the action cycles → ← none (as in TC)
  const cycle = async (r) => {
    if (busy()) return;
    if (r.unselected && r.action !== 'none') { await include(true, [r]); return; }
    const order = ['copyLeftToRight', 'copyRightToLeft', 'none'];
    let i = order.indexOf(r.action);
    for (let k = 1; k <= order.length; k++) {
      const a = order[(i + k) % order.length];
      try {
        const n = await go.SyncDirs.SetAction(st.id, [r.path], a, r.isDir);
        if (n > 0 || a === 'none') break;
      } catch { /* not possible for this row: try the next one */ }
    }
    await refresh(r.path);
  };
  const pathsOf = (r) => ({ l: joinPath(st.cmpLeft || q('#sync-left').value.trim(), r.leftPath, app.sepFor(st.left)), r: joinPath(st.cmpRight || q('#sync-right').value.trim(), r.rightPath, app.sepFor(st.right)) });
  const compareRow = async (r = st.rows[st.cursor]) => {
    if (!r) return;
    if (r.isDir || !r.left || !r.right || r.left.kind !== 'file' || r.right.kind !== 'file') { app.flash('Compare by content needs a file on both sides.'); return; }
    let { l, r: rp } = pathsOf(r);
    try {
      // The compare tool reads files on this computer: a file in an archive or on a server is compared as a temporary
      // copy, so edits made there are not written back (the row is not read again either).
      const copied = isVirtual(st.sides?.left) || isVirtual(st.sides?.right);
      if (isVirtual(st.sides?.left)) l = await go.FS.LocalCopy(l);
      if (isVirtual(st.sides?.right)) rp = await go.FS.LocalCopy(rp);
      if (copied) app.flash('A file in an archive or on a server is compared as a copy: changes are not saved back.');
      const res = await compareFiles(app, l, rp);
      if (res?.saved?.some(Boolean) && !copied) {
        await go.SyncDirs.Recompare(st.id, r.path).catch((e) => app.flash(errText(e), 'err'));
        await refresh(r.path);
      }
    } catch (e) { app.flash(errText(e), 'err'); }
  };
  const showInPanel = async (r, which) => {
    const e = which === 'left' ? r.left : r.right;
    if (!e) return;
    const full = pathsOf(r)[which === 'left' ? 'l' : 'r'];
    const sep = app.sepFor(full);
    const i = full.lastIndexOf(sep);
    const dir = i > 0 ? full.slice(0, i) : full.slice(0, i + 1);
    const panel = which === 'left' ? app.left : app.right;
    done(null);
    if (await panel.go(dir)) { panel.focusName(full.slice(i + 1)); app.setActive(panel); }
  };
  const openFile = (r, which) => {
    const full = pathsOf(r)[which === 'left' ? 'l' : 'r'];
    go.FS.OpenFile(full).catch((e) => app.flash(errText(e), 'err'));
  };
  // F3 / F4: the Lister or the editor for one side's file. The Lister reads a file in an archive or on a server from a
  // temporary copy (cm_List does that); the editor only edits files on this computer from here. The list gets the keys
  // back afterwards.
  const viewEdit = async (cmd, want, r = st.rows[st.cursor]) => {
    const which = fileSide(r, want);
    if (!which) { app.flash('Choose a file: folders and links can\'t be viewed or edited here.', 'err'); return; }
    if (cmd === 'cm_Edit' && isVirtual(st.sides?.[which])) { app.flash('A file in an archive or on a server can\'t be edited from here. View it (F3), or show it in a panel.', 'err'); return; }
    const full = pathsOf(r)[which === 'left' ? 'l' : 'r'];
    try { await app.run(cmd, { path: full, paths: [full] }); } catch (e) { app.flash(errText(e), 'err'); }
    if (box?.isConnected) body.focus({ preventScroll: true });
  };
  const selectWithAction = (dir) => {
    const ps = pathsWithAction(st.rows, dir);
    st.sel = new Set(ps);
    if (!ps.length) app.flash('No row copies in that direction.');
    paint();
  };
  const selectByPattern = async (on) => {
    const t = await inputDialog({ title: on ? 'Select rows' : 'Unselect rows', label: 'Names (for example *.txt *.md)', value: '*.*', kind: 'syncdirs-select' });
    if (t === null) return;
    const m = t.trim() === '*.*' ? () => true : wildcard(t.trim(), app.platform !== 'linux');
    for (const r of st.rows) {
      const name = (r.left || r.right)?.name || r.path.split('/').pop();
      if (m(name)) { if (on) st.sel.add(r.path); else st.sel.delete(r.path); }
    }
    paintStatus(); render();
  };
  const menu = async (at) => {
    if (busy()) return;
    const ts = targets();
    if (!ts.length) return;
    const one = ts.length === 1 ? ts[0] : null;
    const pair = one && !one.isDir && one.left?.kind === 'file' && one.right?.kind === 'file';
    const items = [
      { label: 'Compare by content…', value: 'cmp', disabled: !pair }, '-',
      { label: 'Copy from left to right  →', value: 'lr' }, { label: 'Copy from right to left  ←', value: 'rl' },
      { label: 'No action', value: 'none' }, { label: 'Reverse copy direction', value: 'rev' }, '-',
      { label: 'Delete on both sides', value: 'delboth' }, { label: 'Delete on the left', value: 'delleft' }, { label: 'Delete on the right', value: 'delright' }, '-',
      { label: 'Include in the synchronization', value: 'inc' }, { label: 'Leave out of the synchronization', value: 'exc' },
      { label: 'Reset all actions', value: 'reset' }, '-',
      { label: 'Select all rows', value: 'selall' }, { label: 'Select rows that copy to the right', value: 'selLR' },
      { label: 'Select rows that copy to the left', value: 'selRL' }, { label: 'Unselect all rows', value: 'selnone' }, '-',
      { label: 'View the left file (F3)', value: 'viewl', disabled: fileSide(one, 'left') !== 'left' },
      { label: 'View the right file (Shift+F3)', value: 'viewr', disabled: fileSide(one, 'right') !== 'right' },
      { label: 'Edit the left file (F4)', value: 'editl', disabled: fileSide(one, 'left') !== 'left' },
      { label: 'Edit the right file (Shift+F4)', value: 'editr', disabled: fileSide(one, 'right') !== 'right' },
      { label: 'Open the left file', value: 'openl', disabled: !one?.left || one.isDir }, { label: 'Open the right file', value: 'openr', disabled: !one?.right || one.isDir },
      { label: 'Show in the left panel', value: 'showl', disabled: !one?.left }, { label: 'Show in the right panel', value: 'showr', disabled: !one?.right }, '-',
      { label: 'Copy names to the clipboard', value: 'clip' },
    ];
    const v = await contextMenu(items, at, ts.length > 1 ? `${ts.length} rows` : (one.left || one.right)?.name || one.path);
    switch (v) {
      case 'cmp': compareRow(one); break;
      case 'lr': setAction('copyLeftToRight', ts); break;
      case 'rl': setAction('copyRightToLeft', ts); break;
      case 'none': setAction('none', ts); break;
      case 'rev': reverse(ts); break;
      case 'delboth': setAction('deleteBoth', ts); break;
      case 'delleft': setAction('deleteLeft', ts); break;
      case 'delright': setAction('deleteRight', ts); break;
      case 'inc': include(true, ts); break;
      case 'exc': include(false, ts); break;
      case 'reset': act(() => go.SyncDirs.Reset(st.id)); break;
      case 'selall': st.rows.forEach((r) => st.sel.add(r.path)); paint(); break;
      case 'selnone': st.sel.clear(); paint(); break;
      case 'selLR': selectWithAction('lr'); break;
      case 'selRL': selectWithAction('rl'); break;
      case 'viewl': viewEdit('cm_List', 'left', one); break;
      case 'viewr': viewEdit('cm_List', 'right', one); break;
      case 'editl': viewEdit('cm_Edit', 'left', one); break;
      case 'editr': viewEdit('cm_Edit', 'right', one); break;
      case 'openl': openFile(one, 'left'); break;
      case 'openr': openFile(one, 'right'); break;
      case 'showl': showInPanel(one, 'left'); break;
      case 'showr': showInPanel(one, 'right'); break;
      case 'clip': if (await clip(ts.map((r) => r.path).join('\n') + '\n')) app.flash(`Copied ${plural(ts.length, 'name')}.`); else app.flash('The clipboard could not be written.', 'err'); break;
    }
  };

  // ---- Synchronize… ----
  const synchronize = async () => {
    if (busy()) return;
    let plan;
    try { plan = await go.SyncDirs.Plan(st.id, { leftToRight: true, rightToLeft: true, delete: true, show: st.show }); } catch (e) { app.flash(errText(e), 'err'); return; }
    const dels = plan.deletesLeft + plan.deletesRight;
    if (!plan.filesLeftToRight && !plan.filesRightToLeft && !dels && !plan.needsWriteLeft && !plan.needsWriteRight) {
      await messageDialog({ Title: 'Synchronize', Message: plan.unselected ? `There is nothing to do: ${plural(plan.unselected, 'row is', 'rows are')} left out of the synchronization.` : 'There is nothing to synchronize.', Kind: 'syncdirs-nothing' });
      return;
    }
    const s = settings(app);
    const html = `
      <label class="mc-check"><input type="checkbox" id="sync-do-lr" ${plan.filesLeftToRight || plan.needsWriteRight ? 'checked' : 'disabled'}>Left to right: copy ${plural(plan.filesLeftToRight, 'file')}, ${size(plan.bytesLeftToRight)}</label>
      <label class="mc-check"><input type="checkbox" id="sync-do-rl" ${plan.filesRightToLeft || plan.needsWriteLeft ? 'checked' : 'disabled'}>Right to left: copy ${plural(plan.filesRightToLeft, 'file')}, ${size(plan.bytesRightToLeft)}</label>
      <label class="mc-check"><input type="checkbox" id="sync-do-del" ${dels ? 'checked' : 'disabled'}>Delete: ${plan.deletesLeft} on the left, ${plan.deletesRight} on the right</label>
      <hr class="mc-sync-hr">
      <label class="mc-check"><input type="checkbox" id="sync-confirm" ${s.confirmOverwrite === false ? '' : 'checked'}>Ask before replacing a file</label>
      <label class="mc-check"><input type="checkbox" id="sync-trash" ${s.trash === false ? '' : 'checked'}>Delete to the trash</label>
      <label class="mc-check" style="margin-left:1.5em" title="With the trash on, emptied folders and links replaced by a link copy go to the trash too, instead of being removed directly"><input type="checkbox" id="sync-trash-all" ${s.trashAll ? 'checked' : ''}>Also trash emptied folders and replaced links</label>
      <label class="mc-check" style="margin-left:1.5em" title="With the trash on, a file that a copy overwrites goes to the trash first instead of being replaced in place (local folders only)"><input type="checkbox" id="sync-trash-replaced" ${s.trashReplaced ? 'checked' : ''}>Also trash files that are overwritten</label>
      ${plan.unselected ? `<p class="mc-note">${plural(plan.unselected, 'row is', 'rows are')} left out and will not be synchronized.</p>` : ''}
      ${dialogNotes(plan, st.sides, s.trash !== false).map((n) => `<p class="mc-note">${esc(n)}</p>`).join('')}
      <p class="mc-note">Only the rows the Show buttons show are synchronized.</p>`;
    const r = await formDialog('Synchronize', html, (b) => {
      const v = { lr: b.querySelector('#sync-do-lr').checked, rl: b.querySelector('#sync-do-rl').checked, del: b.querySelector('#sync-do-del').checked,
        confirm: b.querySelector('#sync-confirm').checked, trash: b.querySelector('#sync-trash').checked,
        trashAll: b.querySelector('#sync-trash-all').checked, trashReplaced: b.querySelector('#sync-trash-replaced').checked };
      return v.lr || v.rl || v.del ? v : 'Choose at least one thing to do.';
    }, null, { okLabel: 'Synchronize', kind: 'syncdirs-confirm' });
    if (!r) return;
    app.setSetting('syncdirs', { ...settings(app), confirmOverwrite: r.confirm, trash: r.trash, trashAll: r.trashAll, trashReplaced: r.trashReplaced });
    if (busy()) return; // the folders may have changed while the dialog was open
    await runSync({ filter: { leftToRight: r.lr, rightToLeft: r.rl, delete: r.del, show: st.show }, confirmOverwrite: r.confirm, trash: r.trash, trashAll: r.trash && r.trashAll, trashReplaced: r.trash && r.trashReplaced,
      left: q('#sync-left').value.trim(), right: q('#sync-right').value.trim() });
  };

  const runSync = (req) => new Promise((resolve) => {
    st.syncing = true;
    paintTools();
    let closeBox = () => {}, paintBox = () => {}, last = { opsDone: 0, opsTotal: 0, bytesDone: 0, bytesTotal: 0, current: '' };
    const offs2 = [];
    const finish = async (d) => {
      offs2.forEach((f) => f());
      closeBox();
      st.syncing = false;
      await Promise.all(app.panels.map((p) => p.reload().catch(() => {})));
      const errs = d.errors || [];
      if (d.error) await messageDialog({ Title: 'Synchronize', Message: d.error, Kind: 'syncdirs-error' });
      else if (errs.length) await messageDialog({ Title: 'Synchronize', Message: `${plural(errs.length, 'operation')} failed:\n\n${errs.slice(0, 12).join('\n')}${errs.length > 12 ? `\n…and ${errs.length - 12} more.` : ''}`, Kind: 'syncdirs-errors' });
      else if (d.cancelled) app.flash(`Synchronization stopped after ${plural(d.done, 'operation')}.`);
      else app.flash(`Synchronized: ${plural(d.done, 'operation')}${d.skipped ? `, ${d.skipped} skipped` : ''}.`);
      if (d.warnings?.length) await messageDialog({ Title: 'Synchronize', Message: d.warnings.join('\n\n'), Kind: 'syncdirs-warning' });
      await compare(); // TC compares again, so done rows show as equal
      resolve();
    };
    offs2.push(on('syncdirs-exec', (p) => { if (p.id === st.id) { last = p; paintBox(); } }));
    offs2.push(on('syncdirs-exec-done', (d) => { if (d.id === st.id) finish(d); }));
    offs2.push(on('syncdirs-ask', async (p) => {
      if (p.id !== st.id) return;
      const name = String(p.target).split('/').pop();
      const msg = `${name} is already on the ${p.side} side.\n\nNew: ${size(p.srcSize || 0)}, ${date(p.srcTime)}\nThere: ${size(p.dstSize || 0)}, ${date(p.dstTime)}`;
      const b = await messageDialog({ Title: 'Overwrite?', Message: msg, Buttons: Object.keys(OVERWRITE), DefaultButton: 'Overwrite', CancelButton: 'Cancel', Kind: 'overwrite' });
      go.SyncDirs.Answer(st.id, OVERWRITE[b] || 'cancel').catch(() => {});
    }));
    modal((b, close) => {
      closeBox = () => close();
      b.innerHTML = `<h2>Synchronizing</h2><p class="mc-msg"></p><div class="mc-progress"><i></i></div><p class="mc-note"></p><div class="mc-btns"><button type="button" class="cancel">Cancel</button></div>`;
      translateTree(b);
      const msg = b.querySelector('.mc-msg'), bar = b.querySelector('.mc-progress i'), note = b.querySelector('.mc-note');
      paintBox = () => {
        msg.textContent = last.current ? `${last.kind === 'delete' || last.kind === 'removeDir' ? 'Deleting' : 'Copying'} ${last.current} (${last.side || ''})` : '';
        const f = last.bytesTotal > 0 ? last.bytesDone / last.bytesTotal : last.opsTotal ? last.opsDone / last.opsTotal : 0;
        bar.style.width = `${Math.round(Math.min(1, f) * 100)}%`;
        note.textContent = `${last.opsDone} of ${plural(last.opsTotal, 'operation')}${last.bytesTotal ? ` · ${size(last.bytesDone)} of ${size(last.bytesTotal)}` : ''}`;
      };
      paintBox();
      b.querySelector('.cancel').onclick = () => go.SyncDirs.Cancel(st.id);
      b.onsubmit = (ev) => ev.preventDefault();
    }, undefined, { kind: 'syncdirs-progress' }).then(() => { if (st.syncing) go.SyncDirs.Cancel(st.id).catch(() => {}); });
    go.SyncDirs.Synchronize(st.id, req).catch((e) => finish({ error: errText(e), errors: [], done: 0 }));
  });

  // ---- saved settings (F-554) ----
  const profilesMenu = async (at) => {
    let ps = [];
    try { ps = await go.SyncDirs.Profiles(); } catch (e) { app.flash(errText(e), 'err'); }
    const items = [{ label: 'Save settings as…', value: { op: 'save' } }];
    if (ps.length) items.push('-', ...ps.map((p) => ({ label: `Load: ${p.name}`, value: { op: 'load', p } })), '-', ...ps.map((p) => ({ label: `Delete: ${p.name}`, value: { op: 'delete', p } })));
    const v = await contextMenu(items, at, 'Saved settings');
    if (!v) return;
    if (v.op === 'save') {
      const name = await inputDialog({ title: 'Save settings', label: 'Name', value: '', kind: 'syncdirs-save', validate: (t) => (t.trim() ? '' : 'Give the settings a name.') });
      if (name === null) return;
      const { onlySelected, ...o } = readOpts();
      try {
        await go.SyncDirs.SaveProfile({ name: name.trim(), left: q('#sync-left').value.trim(), right: q('#sync-right').value.trim(), options: { ...o, filter: o.filter, case: 'auto' }, show: st.show });
        app.flash(`Saved the settings as "${name.trim()}".`);
      } catch (e) { app.flash(errText(e), 'err'); }
    } else if (v.op === 'load') {
      loadProfile(v.p);
    } else if (v.op === 'delete') {
      const b = await messageDialog({ Title: 'Delete saved settings', Message: `Delete the saved settings "${v.p.name}"?`, Buttons: ['Delete', 'Cancel'], Kind: 'syncdirs-delete-profile' });
      if (b === 'Delete') go.SyncDirs.DeleteProfile(v.p.name).then(() => app.flash(`Deleted "${v.p.name}".`)).catch((e) => app.flash(errText(e), 'err'));
    }
  };
  const loadProfile = (p) => {
    q('#sync-left').value = p.left || '';
    q('#sync-right').value = p.right || '';
    invalidate();
    writeOpts({ ...DEFAULT_OPTS, ...(p.options || {}), filter: p.options?.filter || '*.*' });
    if (p.show) { st.show = { ...DEFAULT_SHOW, ...p.show }; writeShow(); }
    app.flash(`Loaded "${p.name}". Press Compare.`);
  };

  const toggleShow = (k) => { st.show[k] = !st.show[k]; writeShow(); remember(); if (st.compared && !st.running) refresh(); };

  return openWindow({
    kind: 'syncdirs',
    cls: 'mc-sync-win',
    build: (b, d) => {
      box = b;
      done = d;
      box.style.setProperty('--sync-date-w', dateWidth(12)); // the date columns fit the widest date (no cut-off)
      const check = (id, label, tip = '') => `<label class="mc-check"${tip ? ` title="${esc(tip)}"` : ''}><input type="checkbox" id="${id}">${esc(label)}</label>`;
      box.innerHTML = `<h2>Synchronize folders</h2>
        <div class="mc-sync-paths">
          <input type="text" id="sync-left" spellcheck="false" title="Left folder: a folder, an archive (backup.zip or backup.zip/docs) or a server address (sftp://me@host/folder)">
          <button type="button" class="b-swap" title="Swap the two folders">⇄</button>
          <input type="text" id="sync-right" spellcheck="false" title="Right folder: a folder, an archive (backup.zip or backup.zip/docs) or a server address (sftp://me@host/folder)">
        </div>
        <div class="mc-sync-opts">
          <label class="mc-check" for="sync-filter">File types</label><input type="text" id="sync-filter" spellcheck="false" title="Names to compare, as in Find files: *.txt *.md | *.bak, a folder with a trailing slash">
          <button type="button" class="b-profiles" title="Save and load these settings">Saved settings…</button>
        </div>
        <div class="mc-sync-opts">
          ${check('sync-subdirs', 'Subfolders')}${check('sync-content', 'By content', 'Compare files of the same size byte by byte')}
          ${check('sync-ignoredate', 'Ignore date')}${check('sync-asym', 'Asymmetric', 'Make the right side a copy of the left side')}
          ${check('sync-emptydirs', 'Ignore empty folders')}${check('sync-fat', 'Ignore 2 seconds', 'Times up to 2 seconds apart count as equal (FAT drives)')}
          ${check('sync-hour', 'Ignore 1 hour', 'Times exactly 1 hour apart count as equal (time zones, daylight saving)')}
          ${check('sync-onlysel', 'Only selected', 'Compare only the names selected in the panels')}
          <button type="button" class="b-compare primary">Compare</button>
        </div>
        <div class="mc-sync-show">Show:
          ${SHOW_KEYS.map((k) => `<label class="mc-sync-tog" title="${esc(SHOW_TIPS[k])}"><input type="checkbox" id="sync-show-${k}"><span>${esc(SHOW_LABELS[k])}</span></label>`).join('')}
        </div>
        <div class="mc-sync-head"><span class="mc-sync-l"><span>Name</span><span>Size</span><span>Date</span></span><span class="mc-sync-act">Action</span><span class="mc-sync-r"><span>Date</span><span>Size</span><span>Name</span></span></div>
        <div class="mc-sync-body" tabindex="0"><div class="mc-sync-spacer"></div><div class="mc-sync-rows"></div></div>
        <div class="mc-sync-foot"><span class="mc-sync-status"></span><span class="mc-btns"><button type="button" class="b-sync" disabled>Synchronize…</button><button type="button" class="b-close">Close</button></span></div>`;
      translateTree(box); // (#113: the window's own words, before the folders and options are filled in)
      body = q('.mc-sync-body'); spacer = q('.mc-sync-spacer'); rowsEl = q('.mc-sync-rows'); status = q('.mc-sync-status');
      q('#sync-left').value = st.left;
      q('#sync-right').value = st.right;
      writeOpts(opts0);
      writeShow();
      q('#sync-left').setAttribute('autofocus', '');
      // "By content" reads every file of the same size; on a server that means downloading it, which the tip says.
      const tip = () => { q('#sync-content').closest('label').title = contentTip(q('#sync-left').value, q('#sync-right').value); };
      tip();
      q('.b-swap').onclick = () => { const l = q('#sync-left').value; q('#sync-left').value = q('#sync-right').value; q('#sync-right').value = l; invalidate(); };
      for (const s of ['#sync-left', '#sync-right']) q(s).addEventListener('input', () => { invalidate(); tip(); });
      q('.b-compare').onclick = () => (st.running ? go.SyncDirs.Cancel(st.id) : compare());
      q('.b-sync').onclick = () => synchronize();
      q('.b-close').onclick = () => done(null);
      q('.b-profiles').onclick = (ev) => profilesMenu({ x: ev.clientX, y: ev.clientY });
      for (const k of SHOW_KEYS) q(`#sync-show-${k}`).onchange = (ev) => { st.show[k] = ev.target.checked; remember(); if (st.compared && !st.running) refresh(); };
      box.onsubmit = (ev) => { ev.preventDefault(); if (!st.running) compare(); };
      body.addEventListener('scroll', () => render());
      new ResizeObserver(() => render()).observe(body);
      rowsEl.addEventListener('mousedown', (ev) => {
        const rEl = ev.target.closest('.mc-sync-row');
        if (!rEl) return;
        const i = +rEl.dataset.i, r = st.rows[i];
        body.focus();
        if (ev.button === 2) { if (!st.sel.has(r.path)) { st.sel.clear(); } st.cursor = i; paint(); return; }
        if (ev.target.closest('.mc-sync-act')) { st.cursor = i; cycle(r); return; }
        if (ev.ctrlKey || ev.metaKey) { if (st.sel.has(r.path)) st.sel.delete(r.path); else st.sel.add(r.path); st.cursor = i; st.anchor = i; paint(); return; }
        if (ev.shiftKey) { move(i, true); return; }
        st.sel.clear(); st.anchor = i; st.cursor = i; paint();
      });
      rowsEl.addEventListener('contextmenu', (ev) => { ev.preventDefault(); menu({ x: ev.clientX, y: ev.clientY }); });
      rowsEl.addEventListener('dblclick', (ev) => { if (!ev.target.closest('.mc-sync-act') && ev.target.closest('.mc-sync-row')) compareRow(); });
      offs.push(on('syncdirs-progress', (p) => { if (p.id === st.id && st.running) { st.progress = p; paintStatus(); } }));
      offs.push(on('syncdirs-done', (d) => { if (d.id === st.id) compareDone(d); }));
      paint();
      if (args.compare) requestAnimationFrame(() => compare());
    },
    onKey: (ev, combo) => {
      if (combo === 'Esc') {
        if (st.running) { go.SyncDirs.Cancel(st.id); return true; }
        return false; // the dialog closes
      }
      const m = /^Ctrl\+([1-6])$/.exec(combo);
      if (m) { toggleShow(SHOW_KEYS[+m[1] - 1]); return true; }
      if (combo === 'F2') { compare(); return true; }
      if (ev.target !== body) return false; // fields and buttons keep their keys (Enter in a field compares)
      const page = Math.max(1, Math.floor((body.clientHeight || 400) / ROW_H) - 1);
      switch (combo) {
        case 'Up': move(st.cursor - 1); return true;
        case 'Down': move(st.cursor + 1); return true;
        case 'PgUp': move(st.cursor - page); return true;
        case 'PgDn': move(st.cursor + page); return true;
        case 'Home': move(0); return true;
        case 'End': move(st.rows.length - 1); return true;
        case 'Shift+Up': move(st.cursor - 1, true); return true;
        case 'Shift+Down': move(st.cursor + 1, true); return true;
        case 'Shift+PgUp': move(st.cursor - page, true); return true;
        case 'Shift+PgDn': move(st.cursor + page, true); return true;
        case 'Shift+Home': move(0, true); return true;
        case 'Shift+End': move(st.rows.length - 1, true); return true;
        case 'Space': case 'Ins': {
          const r = st.rows[st.cursor];
          if (!r) return true;
          if (st.sel.has(r.path)) st.sel.delete(r.path); else st.sel.add(r.path);
          if (combo === 'Ins') move(st.cursor + 1); else { paintStatus(); render(); }
          return true;
        }
        case 'Ctrl+A': case 'Cmd+A': st.rows.forEach((r) => st.sel.add(r.path)); paint(); return true;
        case 'Num+': case 'Shift+=': selectByPattern(true); return true;
        case 'Num-': case 'Shift+-': selectByPattern(false); return true;
        case 'Ctrl+Right': case 'Alt+Right': setAction('copyLeftToRight'); return true;
        case 'Ctrl+Left': case 'Alt+Left': setAction('copyRightToLeft'); return true;
        case 'Ctrl+R': reverse(); return true;
        case 'Delete': case 'Backspace': setAction('none'); return true;
        case 'Shift+Delete': setAction('deleteRight'); return true;
        case 'Enter': compareRow(); return true;
        case 'F3': viewEdit('cm_List', 'left'); return true;
        case 'Shift+F3': viewEdit('cm_List', 'right'); return true;
        case 'F4': viewEdit('cm_Edit', 'left'); return true;
        case 'Shift+F4': viewEdit('cm_Edit', 'right'); return true;
        case 'Shift+F10': case 'Apps': {
          const el = rowsEl.querySelector('.mc-sync-row.cur');
          const r = el?.getBoundingClientRect();
          menu(r ? { x: r.left + r.width / 2, y: r.bottom } : null);
          return true;
        }
      }
      return false;
    },
  }).finally(() => {
    offs.forEach((f) => f());
    go.SyncDirs.Close(st.id).catch(() => {});
    if (current === st) current = null;
  });
}

command({
  name: 'cm_SyncDirs', menuKey: true, label: 'Synchronize folders…', short: 'Sync',
  run: async (app, args = {}) => {
    const id = await go.SyncDirs.Open().catch((e) => { throw new Error(errText(e)); });
    let a = args;
    if (args.profile) {
      const ps = await go.SyncDirs.Profiles().catch(() => []);
      const p = ps.find((x) => x.name === args.profile);
      if (!p) throw new Error(`There are no saved settings named "${args.profile}".`);
      a = { ...args, left: p.left, right: p.right, options: { ...p.options }, show: p.show };
    }
    return syncWindow(app, id, a);
  },
});

addControlCommand('syncdirs', async (m) => {
  const st = current;
  if (!st) return { ok: true, open: false };
  if (m.op === 'cursor' && m.path != null) {
    const i = st.rows.findIndex((r) => r.path === m.path);
    if (i < 0) throw new Error(`no row ${m.path}`);
    st.cursor = i;
    st.sel.clear();
    document.querySelector('.mc-sync-body')?.focus();
  }
  return {
    ok: true, open: true, id: st.id, compared: st.compared, running: st.running, syncing: st.syncing, show: st.show, summary: st.summary, errors: st.errors,
    cursor: st.cursor, selected: [...st.sel], sides: st.sides,
    rows: st.rows.map((r) => ({ path: r.path, isDir: r.isDir, category: r.category, action: r.action, unselected: !!r.unselected, left: !!r.left, right: !!r.right, note: r.note || '' })),
  };
});
