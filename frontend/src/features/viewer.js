// Viewing and editing files (FEATURES.md sections 27 and 28; behaviour from docs/spec/area-4.md, sections 1 to 3):
//
//   F3 cm_List              the Lister on the file under the cursor (the first selected file when files are selected;
//                           N/P then walk the selection, otherwise all files of the folder). On a folder: its size.
//   Shift+F3 cm_ListCurrent the Lister on the file under the cursor only
//   Alt+F3 cm_ListExternal  the other viewer: the external one when F3 is the Lister, and the other way round
//   Alt+Shift+F3 cm_ListInternalOnly   the Lister with text, binary and hex only
//   Ctrl+Q cm_SrcQuickView  quick view: the other panel shows the file under the cursor (cm_SrcQuickInternalOnly: text only)
//   F4 cm_Edit, Shift+F4 cm_EditNewFile   the editor (configured, or the system's text editor)
//
// The Lister opens in a real window of its own (setting "viewerInWindow", on unless false): Go makes the window
// (feature_viewer_window.go) and its page, viewer.html with src/viewer-window.js, shows the file. With the setting off
// the Lister is a window over the panels (a page dialog, so control mode sees it as the dialog of kind "lister" and
// closes it with dialog_answer "Esc"). viewer/view.js is the view inside either, shared with quick view. Settings:
// "lister" (see LISTER_DEFAULTS in viewer/view.js), "editor" (the F4 line; "" = the system's text editor) and
// "viewerInWindow".

import './viewer.css';
import { command } from '../core/registry.js';
import { go, on, errText } from '../core/go.js';
import { modal, esc, messageDialog, inputDialog, formDialog } from '../core/dialogs.js';
import { popupOpen } from '../core/popup.js';
import { addKeyHandler } from '../core/keyboard.js';
import { addControlCommand } from '../core/control.js';
import { comboFromEvent } from '../core/keys.js';
import { size as fmtSize, bytes as fmtBytes } from '../core/format.js';
import { app as theApp } from '../core/app.js';
import { ListerView, listerSettings, saveListerSettings, applyListerOptions, LISTER_DEFAULTS } from './viewer/view.js';

let lister = null; // the open Lister window: {view, close, list, dir, panel, title}
let qv = null; // quick view: {panel, view, el, timer, internalOnly, off: [unsubscribe...]}

const remote = (p) => /^[a-z][a-z0-9+.-]*:\/\//i.test(p || '');

// panelCtx describes a panel for the placeholders of an external program (%P, %N, %S...).
const panelCtx = (p) => ({ Dir: p.path, Current: p.cur && !p.cur.up ? p.cur.name : '', Selected: p.selectedEntries().map((e) => e.name) });
// (a Lister window has no panels: its placeholders are empty)
const noPanel = { Dir: '', Current: '', Selected: [] };
const paramCtx = (app) => ({ Source: app.cur ? panelCtx(app.cur) : noPanel, Target: app.other ? panelCtx(app.other) : noPanel, File: '' });

// ---- the Lister in a window of its own ----

// The setting "viewerInWindow" (on unless it is false): F3 opens a real window (feature_viewer_window.go,
// src/viewer-window.js) instead of the dialog over the panels below. The dialog stays as the fallback.
const inWindowSetting = (app) => app.setting('viewerInWindow', true) !== false;
const inListerWindow = () => !!globalThis.document?.body?.classList.contains('mc-viewer-window');

// openViewer opens the Lister on a file: in a window when the setting says so and Go has Lister windows, else over the
// panels.
async function openViewer(app, o) {
  if (inWindowSetting(app) && !inListerWindow() && !lister) {
    try {
      await go.ListerWindows.Open({ path: o.path, names: o.names || [], dir: o.dir || '', side: o.panel?.side || '', internalOnly: !!o.internalOnly, mode: o.mode || 'auto' });
      return;
    } catch (e) {
      if (!/is not available/.test(errText(e))) { app.flash(errText(e), 'err'); return; }
    }
  }
  return openLister(app, o);
}

// ---- the Lister window ----

const fileOf = (path, sep) => path.slice(path.lastIndexOf(sep) + 1);

async function openLister(app, { path, names = [], dir = '', panel = null, internalOnly = false, mode = 'auto' }) {
  if (lister) { // (only reached from control mode or a menu while one is open) the open window shows the file
    await lister.show(path);
    return;
  }
  // (focus goes back where it was when the Lister closes: the panel, or the dialog it was opened from, like Find files)
  const before = document.activeElement;
  const view = new ListerView(app, { internalOnly });
  try {
    await view.open(path, mode);
  } catch (e) {
    view.destroy();
    await messageDialog({ Title: 'Lister', Message: errText(e), Kind: 'lister-error' });
    return;
  }
  let done = null;
  const sep = app.sepFor(path);
  const state = {
    view, names, dir, panel, path,
    close: () => done?.(),
    show: async (p) => {
      try {
        await view.open(p);
        state.path = p;
        setTitle();
        return true;
      } catch (e) {
        view.note(errText(e), true);
        return false;
      }
    },
  };
  const setTitle = () => {
    const h = state.box?.querySelector('h2');
    if (h) { h.textContent = `Lister – ${state.path}`; h.title = state.path; }
  };
  // N / P: the next or previous file of the list, and the panel's cursor follows (spec area 4, 1.6)
  view.onNeighbor = async (d) => {
    const cur = fileOf(state.path, sep);
    const i = names.indexOf(cur);
    const j = i + d;
    if (i < 0 || j < 0 || j >= names.length) { view.note(d > 0 ? 'Last file' : 'First file'); return; }
    const name = names[j];
    const p = panel && panel.path === dir ? panel.fullPath(name) : app.resolvePath(name, dir);
    if (await state.show(p)) panel?.focusName(name);
  };
  view.onClose = () => state.close();
  lister = state;

  // The window's own keys come before the dialog's (which would swallow them): registered first, so it runs first.
  const onKey = (ev) => {
    const all = document.querySelectorAll('.mc-modal');
    if (all[all.length - 1] !== state.back || popupOpen()) return; // a dialog or a menu over the Lister has the keys
    if (ev.target?.matches?.('input, select, textarea') && ev.key !== 'Escape') return;
    const combo = comboFromEvent(ev);
    if (!combo) return;
    if (combo === 'Esc') {
      if (view.cancelWork()) { ev.preventDefault(); ev.stopImmediatePropagation(); }
      return; // otherwise the dialog closes itself
    }
    const closeKeys = ['Q', 'Alt+F4', ...(app.platform === 'darwin' ? ['Cmd+W'] : [])];
    if (closeKeys.includes(combo)) { ev.preventDefault(); ev.stopImmediatePropagation(); state.close(); return; }
    if (view.key(combo)) { ev.preventDefault(); ev.stopImmediatePropagation(); }
  };
  window.addEventListener('keydown', onKey, true);
  try {
    await modal((box, close) => {
      done = close;
      state.box = box;
      state.back = box.parentElement;
      box.classList.add('mc-viewer-box');
      box.innerHTML = `<div class="mc-viewer-head"><h2></h2><div class="mc-viewer-menus">${['File', 'Edit', 'Options', 'Image', 'Encoding']
        .map((m) => `<button type="button" class="mc-viewer-menu" data-menu="${m}">${m}</button>`).join('')}</div></div>`;
      box.querySelectorAll('.mc-viewer-menu').forEach((b) => { b.onclick = () => view.menu(b.dataset.menu, b); });
      box.append(view.el);
      view.el.setAttribute('autofocus', '');
      box.onsubmit = (ev) => ev.preventDefault();
      // (the dialog appends the box to the page after this; the parent element is known then)
      queueMicrotask(() => { state.back = box.parentElement; setTitle(); });
    }, undefined, { kind: 'lister' });
  } finally {
    window.removeEventListener('keydown', onKey, true);
    view.destroy();
    lister = null;
    if (before && before !== document.body && before.isConnected && !state.box?.contains(before)) before.focus({ preventScroll: true });
    else (state.panel || app.cur)?.$.list.focus({ preventScroll: true });
  }
}

// viewTarget decides what F3 shows: the file under the cursor, or the first selected file with the selection as the N/P
// list. A folder gets its size counted instead (TC does that, and shows it in place of <DIR>).
async function view(app, args = {}) {
  const p = app.cur;
  if (args.path) { // control mode, Find files results, or "Open with another app" in the Lister
    // a file inside an archive or on a server (a Find files result in an archive) is viewed from a temporary copy
    let path = args.path;
    try { path = await go.FS.LocalCopy(args.path) || args.path; } catch (err) {
      if (!/is not available/.test(errText(err))) { app.flash(errText(err), 'err'); return; } // (no vfs feature: as it is)
    }
    if (args.external) return external(app, path);
    const sep = app.sepFor(path);
    return openViewer(app, { path, names: [fileOf(path, sep)], dir: path.slice(0, path.lastIndexOf(sep)) || sep, internalOnly: !!args.internalOnly, mode: args.mode || 'auto' });
  }
  const e = p.cur;
  if (!e || e.up) return;
  if (e.dir) { await folderSize(app, p, e); return; }
  const sel = args.current ? [] : p.selectedEntries().filter((x) => !x.dir);
  const s = listerSettings(app);
  const first = sel.length ? sel[0] : e;
  if (s.viewer && !args.internal) return external(app, p.fullPath(first.name), s.viewer);
  if (remote(p.path)) { app.flash('Viewing files inside archives or on servers is not ready yet.', 'err'); return; }
  const names = sel.length ? sel.map((x) => x.name) : p.entries.filter((x) => !x.dir && !x.up).map((x) => x.name);
  if (sel.length && first !== e) p.focusName(first.name);
  await openViewer(app, { path: p.fullPath(first.name), names, dir: p.path, panel: p, internalOnly: !!args.internalOnly, mode: args.mode || 'auto' });
}

async function folderSize(app, p, e) {
  app.flash(`Counting ${e.name}…`);
  try {
    const r = await go.Lister.FolderSize('list', p.fullPath(e.name));
    p.dirSizes.set(e.name, r.bytes);
    p.rebuild();
    app.flash(`${e.name}: ${fmtSize(r.bytes)} (${fmtBytes(r.bytes)} bytes) in ${fmtBytes(r.files)} ${r.files === 1 ? 'file' : 'files'} and ${fmtBytes(r.dirs)} ${r.dirs === 1 ? 'folder' : 'folders'}`);
  } catch (err) {
    app.flash(errText(err), 'err');
  }
}

// external opens a file in the external viewer line, or with its default app.
async function external(app, path, tool = listerSettings(app).viewer) {
  await go.ListerEdit.View(path, tool || '', paramCtx(app));
}

command({ name: 'cm_List', label: 'View', short: 'View', run: (app, args) => view(app, args) });
command({ name: 'cm_ListCurrent', label: 'View the file under the cursor', run: (app, args) => view(app, { ...args, current: true }) });
command({ name: 'cm_ListInternalOnly', label: 'View with the built-in viewer only', run: (app, args) => view(app, { ...args, current: true, internal: true, internalOnly: true }) });
command({
  // Alt+F3: the other viewer. With an external viewer set for F3 that is the Lister; without one, the file's default app.
  name: 'cm_ListExternal', label: 'View with the other viewer',
  run: async (app, args) => {
    if (args.path && args.external) return external(app, args.path, listerSettings(app).viewer || '');
    const p = app.cur;
    const e = p.cur;
    if (!e || e.up || e.dir) return;
    if (listerSettings(app).viewer) return view(app, { ...args, current: true, internal: true });
    return external(app, p.fullPath(e.name), '');
  },
});

// ---- quick view ----

function qvUpdate(app, now = false) {
  if (!qv) return;
  clearTimeout(qv.timer);
  qv.timer = setTimeout(async () => {
    if (!qv) return;
    const src = app.panels.find((p) => p !== qv.panel);
    const e = src?.cur;
    const v = qv.view;
    const key = e ? `${src.path}\u0000${e.name}` : '';
    if (key === qv.shown) return;
    qv.shown = key;
    if (!e || e.up) { go.Lister.CancelFolderSize('quickview').catch(() => {}); v.showNote(e ? 'Move the cursor to a file to see it here.' : 'The folder is empty.'); return; }
    const path = src.fullPath(e.name);
    if (e.dir) { await v.showFolder(e.name, path); return; }
    go.Lister.CancelFolderSize('quickview').catch(() => {});
    if (remote(src.path)) { v.showNote('Viewing files inside archives or on servers is not ready yet.'); return; }
    try {
      await v.open(path);
    } catch (err) {
      if (qv?.shown === key) v.showNote(errText(err));
    }
  }, now ? 0 : 150); // a short pause, so fast scrolling does not load every file (spec area 4, section 2)
}

function quickView(app, internalOnly) {
  if (qv) {
    const same = qv.internalOnly === internalOnly;
    endQuickView(app);
    if (same) return;
  }
  const panel = app.other;
  const el = document.createElement('div');
  el.className = 'mc-viewer-qv';
  panel.$.header.after(el);
  panel.el.classList.add('mc-viewer-qv-on');
  const v = new ListerView(app, { quick: true, internalOnly });
  el.append(v.el);
  qv = { panel, view: v, el, timer: 0, internalOnly, shown: null, off: [] };
  const follow = (p) => { if (qv && p !== qv.panel) qvUpdate(app); };
  qv.off.push(app.on('cursor', follow), app.on('panel', follow));
  // the quick view panel is not a file panel while it shows a file: the other panel stays the active one
  qv.off.push(app.on('active', (p) => {
    if (qv && p === qv.panel) setTimeout(() => { if (qv && app.cur === qv.panel) app.setActive(app.panels.find((x) => x !== qv.panel)); }, 0);
  }));
  app.emit('quickview', true);
  qvUpdate(app, true);
}

function endQuickView(app) {
  if (!qv) return;
  const { panel, view: v, el, off } = qv;
  clearTimeout(qv.timer);
  off.forEach((f) => f());
  qv = null;
  v.destroy();
  el.remove();
  panel.el.classList.remove('mc-viewer-qv-on');
  panel.renderRows();
  app.emit('quickview', false);
  if (document.activeElement === document.body || !document.activeElement) app.cur.$.list.focus({ preventScroll: true });
}

command({ name: 'cm_SrcQuickView', label: 'Quick view', run: (app) => quickView(app, false), checked: () => !!qv && !qv.internalOnly });
command({ name: 'cm_SrcQuickInternalOnly', label: 'Quick view, built-in viewer only', run: (app) => quickView(app, true), checked: () => !!qv?.internalOnly, menu: 'Show', menuAfter: 'cm_SrcQuickView' });

// ---- the editor ----

command({
  // F4: the file under the cursor (F4 ignores the selection, unlike F3); nothing on a folder or "..".
  name: 'cm_Edit', label: 'Edit', short: 'Edit',
  run: async (app, args) => {
    const p = app.cur;
    const path = args.path || (p.cur && !p.cur.up && !p.cur.dir ? p.fullPath(p.cur.name) : '');
    if (!path) return;
    await go.ListerEdit.Edit(path, app.setting('editor', '') || '', paramCtx(app));
  },
});

command({
  // Shift+F4: asks for a name (the file under the cursor, selected), creates the file empty when it is not there, opens
  // it in the editor; the panel then shows the new file under the cursor.
  name: 'cm_EditNewFile', label: 'Edit new file…', short: 'New file',
  run: async (app, args) => {
    const p = app.cur;
    if (remote(p.path)) { app.flash('Editing files inside archives or on servers is not ready yet.', 'err'); return; }
    const cur = p.cur && !p.cur.up && !p.cur.dir ? p.cur.name : '';
    const dot = cur.lastIndexOf('.');
    const name = args.name ?? await inputDialog({ title: 'New file', label: 'Edit new file:', value: cur, select: [0, dot > 0 ? dot : cur.length], okLabel: 'Edit',
      kind: 'lister-newfile', validate: (v) => (v.trim() ? '' : 'Type a file name.') });
    if (!name) return;
    const r = await go.ListerEdit.EditNew(p.path, name.trim(), app.setting('editor', '') || '', paramCtx(app));
    if (r.created || r.dir === p.path) {
      if (r.dir === p.path || app.resolvePath(r.dir, p.path) === p.path) {
        await p.reload();
        p.focusName(r.name);
      }
    }
  },
});

// ---- Lister options (Options > Lister options… in the Lister; Configuration menu) ----

command({
  name: 'cm_ListerConfig', label: 'Viewer and editor options…', menu: 'Configuration', menuAfter: 'cm_Config',
  run: async (app) => {
    const s = listerSettings(app);
    const sel = (id, value, opts) => `<select id="${id}">${opts.map(([v, l]) => `<option value="${v}"${String(v) === String(value) ? ' selected' : ''}>${l}</option>`).join('')}</select>`;
    const chk = (id, label, on) => `<label class="mc-check"><input type="checkbox" id="${id}"${on ? ' checked' : ''}> ${label}</label>`;
    const html = `<div class="mc-viewer-opts">
      <div class="mc-row"><label for="lo-mode">Start in</label>${sel('lo-mode', s.defaultMode, [['auto', 'Auto-detect'], ['text', 'Text'], ['binary', 'Binary'], ['hex', 'Hex']])}</div>
      <div class="mc-row"><label for="lo-charset">Encoding</label>${sel('lo-charset', s.defaultCharset, [['auto', 'Auto-detect'], ['ansi', 'ANSI (Windows charset)'], ['oem', 'ASCII (DOS charset)'], ['utf-8', 'UTF-8'], ['unicode', 'Unicode (UTF-16)']])}</div>
      ${chk('lo-hex', 'Open binary files in hex mode', s.autoHex)}
      ${chk('lo-media', 'Show images, audio and video', s.showMedia)}
      ${chk('lo-html', 'Show HTML and Markdown rendered', s.renderHTML)}
      ${chk('lo-wrap', 'Wrap text', s.wrap)}
      ${chk('lo-fit', 'Fit images to the window', s.fitImage)}
      ${!inListerWindow() ? chk('lo-window', 'Open the Lister in a window of its own', inWindowSetting(app)) : ''}
      <div class="mc-viewer-optrow">
        <div class="mc-row"><label for="lo-tab">Tab width</label><input id="lo-tab" type="text" value="${esc(s.tabWidth)}"></div>
        <div class="mc-row"><label for="lo-bin">Binary width</label><input id="lo-bin" type="text" value="${esc(s.binaryWidth)}"></div>
        <div class="mc-row"><label for="lo-size">Font size</label><input id="lo-size" type="text" value="${esc(s.fontSize)}"></div>
        <div class="mc-row"><label for="lo-font">Font</label>${sel('lo-font', s.font, [['fixed', 'Fixed width'], ['variable', 'Variable width']])}</div>
      </div>
      <div class="mc-row"><label for="lo-viewer">Viewer for F3 (empty: the built-in Lister; Alt+F3 opens the other one)</label><input id="lo-viewer" type="text" spellcheck="false" value="${esc(s.viewer)}" placeholder="Built-in Lister"></div>
      <div class="mc-row"><label for="lo-editor">Editor for F4 (empty: the system's text editor; %1 stands for the file)</label><input id="lo-editor" type="text" spellcheck="false" value="${esc(app.setting('editor', ''))}" placeholder="System text editor"></div>
    </div>`;
    const r = await formDialog('Viewer and editor options', html, (box) => {
      const v = (id) => box.querySelector(`#${id}`);
      const num = (id, lo, hi) => { const n = parseInt(v(id).value, 10); return n >= lo && n <= hi ? n : null; };
      const tab = num('lo-tab', 1, 32), bin = num('lo-bin', 8, 1024), fs = num('lo-size', 8, 32);
      if (tab == null) return 'The tab width must be a number from 1 to 32.';
      if (bin == null) return 'The binary width must be a number from 8 to 1024.';
      if (fs == null) return 'The font size must be a number from 8 to 32.';
      return { defaultMode: v('lo-mode').value, defaultCharset: v('lo-charset').value, autoHex: v('lo-hex').checked, showMedia: v('lo-media').checked,
        renderHTML: v('lo-html').checked, wrap: v('lo-wrap').checked, fitImage: v('lo-fit').checked, tabWidth: tab, binaryWidth: bin, fontSize: fs,
        font: v('lo-font').value, viewer: v('lo-viewer').value.trim(), editor: v('lo-editor').value.trim(), inWindow: v('lo-window') ? v('lo-window').checked : null };
    }, null, { okLabel: 'Save', kind: 'lister-options' });
    if (!r) return;
    const { editor, inWindow, ...lo } = r;
    await saveListerSettings(app, lo);
    await app.setSetting('editor', editor);
    if (inWindow !== null) await app.setSetting('viewerInWindow', inWindow);
    // the open views take the new options at once (those of other windows hear of it through the "setting" event)
    for (const v of [lister?.view, qv?.view].filter(Boolean)) applyListerOptions(v, lo);
    app.emit('lister-options', lo);
  },
});

// ---- control mode ----

addControlCommand('lister', async () => ({
  ok: true, open: !!lister, lister: lister ? { ...lister.view.state(), list: lister.names } : null,
  quickView: qv ? { side: qv.panel.side, internalOnly: qv.internalOnly, ...qv.view.state() } : null,
  sessions: await go.Lister.Sessions().catch(() => -1), settings: listerSettings(theApp), defaults: LISTER_DEFAULTS,
}));
// lister_select {start, end, quick}: sets the selection by bytes, for checks of copy and save
addControlCommand('lister_select', async (m) => {
  const v = m.quick ? qv?.view : lister?.view;
  if (!v) throw new Error('no viewer is open');
  v.select(m.start ?? 0, m.end ?? 0);
  return { ok: true, selection: v.state().selection };
});
// lister_menu {menu, item}: chooses an item of a Lister menu by its label (without the check mark)
addControlCommand('lister_menu', async (m) => {
  const v = lister?.view;
  if (!v) throw new Error('the Lister is not open');
  const it = v.menuItems(m.menu).find((x) => !x.sep && x.label.replace(/^✓ /, '').toLowerCase() === String(m.item).toLowerCase());
  if (!it) throw new Error(`the ${m.menu} menu has no item "${m.item}"`);
  if (it.disabled) throw new Error(`"${m.item}" is not available now`);
  it.run();
  return { ok: true };
});

export function init(app) {
  // Quick view's keys: Tab moves into the view (so 1-7, W, the cursor keys... work there) and back out.
  addKeyHandler((ev, combo) => {
    if (!qv) return false;
    const inView = qv.view.el.contains(document.activeElement);
    if (inView) {
      if (combo === 'Tab' || combo === 'Esc') { app.cur.$.list.focus({ preventScroll: true }); return true; }
      return qv.view.key(combo);
    }
    if (combo === 'Tab' && document.activeElement?.closest?.('.panel') === app.cur.el) { qv.view.focus(); return true; }
    return false;
  });
  // A folder change in the quick view panel (Ctrl+U, the tree...) leaves it a file panel again.
  app.on('exchange', () => { if (qv) qvUpdate(app, true); });
  // Lister windows: N / P in one moves the cursor of the panel its list came from (when it still shows that folder),
  // and once the panels are up Go makes the first window in advance, so the first F3 is quick.
  on('lister-cursor', (d) => {
    const p = d?.side && app.panel(d.side);
    if (p && p.path === d.dir) p.focusName(d.name);
  });
  app.on('ready', () => { if (inWindowSetting(app)) go.ListerWindows.Prepare().catch(() => {}); });
}
