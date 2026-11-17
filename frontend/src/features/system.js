// Desktop integration (FEATURES.md sections 3, 10, 36-39): Enter opens a file with its app or runs a program (F-051,
// smart Enter on shortcut files F-738), Shift+Enter runs it in a terminal (F-199, F-265), "Open with" (F-200), a
// terminal in the current folder (F-196, F-363, F-371), "Show in Finder / Explorer" (F-366), the desktop folder, the
// command line runs programs (F-293, F-294, F-302), the files clipboard shared with Finder / Explorer (F-237, F-739,
// F-740), automatic refresh (F-065), volumes appearing and disappearing (F-709), the error log window (F-718) and the
// safety confirmations (F-719). Drag and drop is in system-dnd.js, the context menus in system-menus.js; the Go side is
// feature_system*.go over internal/launch, internal/clipfiles, internal/fswatch and internal/links.
//
// Settings (all optional): "terminal" (program or command line with %D), "autoRefresh" (true), "refreshOnlyActive"
// (false), "refreshExclude" (folders, one per line or ";"-separated), "refreshNetwork" (true), "confirmDrop" (true),
// "enableLog" / "logFile" (the log file), "dimCutFiles" (true: files cut to the clipboard stay dimmed until pasted, and
// Esc cancels the cut; false: no dimming, Esc leaves the clipboard alone). The confirmations dialog
// (cm_SafetyConfirmations) writes where each feature reads.

import './system.css';
import { command, getCommand } from '../core/registry.js';
import { go, on, errText } from '../core/go.js';
import { showList } from '../core/popup.js';
import { inputDialog, modal, esc } from '../core/dialogs.js';
import { addCmdlineHandler } from '../core/cmdline.js';
import { addControlCommand } from '../core/control.js';
import { plural } from '../core/format.js';
import { app as theApp } from '../core/app.js';
import { applyOptions } from './config/util.js';
import { isLocal, baseName, dirName, logError, errorLog, onLog, runTransfer, focusIn } from './system-lib.js';
import { createCutState } from './system-cut.js';
import { Panel } from '../core/panel.js';
import { addKeyHandler } from '../core/keyboard.js';
import { cmdline } from '../core/cmdline.js';

const REVEAL = { darwin: 'Show in Finder', windows: 'Show in Explorer', linux: 'Open in the file manager' };
export const revealLabel = (app) => REVEAL[app.platform] || REVEAL.linux;

// anchorFor places a popup at the mouse (args.x/y) or under the cursor row of a panel.
export function anchorFor(panel, args = {}) {
  if (args.x != null && args.y != null) return { x: args.x, y: args.y };
  const row = panel.$.rows.querySelector('.row.cur .n') || panel.$.rows.querySelector('.row.cur');
  if (row) { const r = row.getBoundingClientRect(); return { x: r.left + 24, y: r.bottom }; }
  return panel.$.list;
}

// ---- opening files (Enter, Shift+Enter, Open with) ----

// openEntry is the last open handler of Enter on a file (after archives and the like): shortcut files are followed,
// programs run, documents open with their app.
async function openEntry(app, panel, e, path) {
  if (app.demo || e.broken || !isLocal(path)) return false;
  try {
    const r = await go.SystemLaunch.Open(path, panel.path);
    if (r.kind === 'dir') { await panel.go(r.dir, { focus: r.focus || null }); return true; }
    app.flash(r.kind === 'started' ? `Started ${e.name}.` : `Opened ${e.name}.`);
  } catch (err) {
    app.flash(errText(err), 'err');
  }
  return true;
}

command({
  // Shift+Enter (F-199, F-265): a program or script in a terminal that stays open; a document with its app; a folder in
  // Finder / Explorer. With text in the command line, Shift+Enter runs that instead (core/keyboard.js).
  name: 'cm_RunKeepOpen', label: 'Run in a terminal',
  run: async (app) => {
    const p = app.cur, e = p.cur;
    if (!e || e.up) return false;
    const path = p.fullPath(e.name);
    if (!isLocal(path)) { app.flash('Only files on this computer can be run.', 'err'); return false; }
    const r = await go.SystemLaunch.RunKeepOpen(path, p.path);
    app.flash(r.kind === 'started' ? `Started ${e.name}.` : `Opened ${e.name}.`);
    return true;
  },
});

// openWith shows the apps that can open the files (F-200): the default first, then "Other program…" (and the system's
// own chooser on Windows).
export async function openWith(app, args = {}) {
  const p = app.cur;
  const files = p.targets().filter((e) => !e.dir || /\.app$/i.test(e.name));
  if (!files.length) { app.flash('Choose a file to open.'); return false; }
  const paths = files.map((e) => p.fullPath(e.name));
  if (!paths.every(isLocal)) { app.flash('Only files on this computer can be opened with another app.', 'err'); return false; }
  const apps = await go.SystemLaunch.AppsFor(paths[0]).catch((e) => { app.flash(errText(e), 'err'); return []; });
  const items = (apps || []).map((a) => ({ label: a.Name, detail: a.Default ? 'default' : '', value: a }));
  if (items.length) items.push({ sep: true });
  items.push({ label: 'Other program…', value: 'other' });
  if (await go.SystemLaunch.HasChooser().catch(() => false)) items.push({ label: 'Choose another app…', value: 'chooser' });
  const what = files.length === 1 ? files[0].name : plural(files.length, 'file');
  const r = await showList({ title: `Open ${what} with`, items, anchor: anchorFor(p, args), kind: 'open-with' });
  if (!r) return false;
  if (r.item.value === 'chooser') { await go.SystemLaunch.OpenWithChooser(paths[0]); return true; }
  if (r.item.value === 'other') {
    const last = app.setting('openWithLast', '');
    const prog = await inputDialog({ title: `Open ${what} with`, label: 'Program', value: last, okLabel: 'Open', kind: 'open-with-program',
      note: app.platform === 'darwin' ? 'An app name such as TextEdit, or the path of a program.' : 'The name or the path of a program.',
      validate: (v) => (v.trim() ? '' : 'Type the name or the path of a program.') });
    if (prog === null) return false;
    app.setSetting('openWithLast', prog.trim());
    await go.SystemLaunch.OpenWithProgram(paths, prog);
    app.flash(`Opened ${what} with ${prog.trim()}.`);
    return true;
  }
  await go.SystemLaunch.OpenWith(paths, r.item.value);
  app.flash(`Opened ${what} with ${r.item.value.Name}.`);
  return true;
}
command({ name: 'cm_OpenWith', label: 'Open with…', menu: 'Files', menuAfter: 'cm_InternalAssociate', run: (app, args) => openWith(app, args) });

// ---- terminal, Finder / Explorer, desktop ----

command({
  // Ctrl+Alt+T (Ctrl+Option+T on a Mac), the usual terminal key of Linux desktops; also "cmd", "terminal" or "term"
  // in the command line (features/cmdline.js)
  name: 'cm_ExecuteDOS', label: 'Open terminal here', keys: ['Ctrl+Alt+T'],
  run: async (app) => {
    await go.SystemLaunch.Terminal(app.cur.path);
    app.flash(`Opened a terminal in ${app.cur.path}.`);
  },
});
command({
  // cm_Explorer: the file under the cursor selected in Finder / Explorer, or the folder itself on "..".
  name: 'cm_Explorer', label: 'Show in Finder',
  run: async (app) => {
    const p = app.cur, e = p.cur;
    if (e && !e.up) await go.SystemLaunch.Reveal(p.fullPath(e.name));
    else await go.SystemLaunch.OpenFolder(p.path);
    return true;
  },
});
command({
  name: 'cm_OpenDesktop', label: 'Go to desktop folder',
  run: async (app) => app.cur.go(await go.SystemLaunch.Desktop()),
});

// ---- the command line runs programs (F-293, F-294, F-302) ----

addCmdlineHandler(async (text, { shift }) => {
  const app = theApp, p = app.cur;
  let r;
  try { r = await go.SystemLaunch.RunLine(text, p.path, !!shift); } catch (e) { app.flash(errText(e), 'err'); return true; }
  if (r.kind === 'cd') await p.go(r.target);
  else if (r.kind === 'back') await p.goBack();
  else if (r.kind === 'internal') await app.run(r.command);
  else if (r.kind === 'run') app.flash(`Started: ${text}`);
  return true;
});

// ---- the files clipboard (F-237, F-739, F-740) ----

// The visible cut state (board issue #3): the files of the last cut are drawn dimmed (class "cut") until a paste moves
// them, a copy replaces them, Esc cancels the cut, or the clipboard is found to hold something else.
const cutFiles = createCutState(() => theApp.platform);
const dimCut = (app) => app.setting('dimCutFiles', true) !== false;
Panel.rowClassHooks.push((panel, e) => (cutFiles.size && !e.up && cutFiles.has(panel.fullPath(e.name)) && dimCut(theApp) ? 'cut' : ''));

// setCut remembers the files of a cut ([] after a copy or a paste) and redraws the panels.
function setCut(app, paths) {
  const had = cutFiles.size;
  cutFiles.set(paths);
  if (had || cutFiles.size) for (const p of app.panels || []) p.renderRows();
}

// cancelCut forgets the cut (Esc) and takes the files off the system clipboard, but only while the clipboard still
// holds exactly that cut, so whatever was copied since (here or in another app) stays.
async function cancelCut(app) {
  const paths = cutFiles.list();
  setCut(app, []);
  try { await go.SystemClip.CancelCut(paths); } catch { /* the dimming is gone either way */ }
  app.flash('The cut was cancelled.');
}

// syncCut forgets the cut when the clipboard no longer holds it (another app copied something meanwhile).
async function syncCut(app) {
  if (!cutFiles.size) return;
  let now;
  try { now = await go.SystemClip.CutFiles(); } catch { return; }
  if (!cutFiles.same(now || [])) setCut(app, []);
}

async function toClipboard(app, cut) {
  const p = app.cur;
  const list = p.targets();
  if (!list.length) { app.flash('Nothing to copy: the cursor is on "..".'); return false; }
  const paths = list.map((e) => p.fullPath(e.name));
  if (!paths.every(isLocal)) { app.flash('Only files on this computer can go to the clipboard.', 'err'); return false; }
  await go.SystemClip.Copy(paths, cut);
  setCut(app, cut ? paths : []);
  const what = list.length === 1 ? list[0].name : plural(list.length, 'file');
  app.flash(cut ? `${what} will be moved by the next paste (${app.keyLabel('cm_PasteFromClipboard') || 'Ctrl+V'}).` : `Copied ${what} to the clipboard.`);
  return true;
}
command({ name: 'cm_CopyToClipboard', label: 'Copy to clipboard', run: (app) => toClipboard(app, false) });
command({ name: 'cm_CutToClipboard', label: 'Cut to clipboard', run: (app) => toClipboard(app, true) });

// pasteData saves text or an image from the clipboard as a new file (F-740), asking for its name.
async function pasteData(app, dir, content) {
  const ext = content.image ? '.png' : '.txt';
  const name0 = await go.SystemClip.SuggestName(dir, 'clipboard', ext);
  const name = await inputDialog({
    title: content.image ? 'Paste image as a file' : 'Paste text as a file', label: 'File name', value: name0, okLabel: 'Save',
    select: [0, name0.length - ext.length], kind: 'paste-data',
    validate: (v) => (!v.trim() ? 'Type a name for the new file.' : /[\\/]/.test(v) ? 'A file name can\'t contain / or \\.' : ''),
  });
  if (name === null) return false;
  const path = await go.SystemClip.PasteData(dir, name.trim());
  await Promise.all(app.panels.filter((p) => p.path === dir).map((p) => p.reload()));
  focusIn(app, dir, [baseName(path)]);
  app.flash(`Saved ${baseName(path)}.`);
  return true;
}

command({
  name: 'cm_PasteFromClipboard', label: 'Paste from clipboard',
  run: async (app) => {
    const dir = app.cur.path;
    if (!isLocal(dir)) { app.flash('Files can only be pasted into folders on this computer.', 'err'); return false; }
    const c = await go.SystemClip.Content();
    if (!c.files) {
      if (c.image || c.text) return pasteData(app, dir, c);
      app.flash('The clipboard is empty.');
      return false;
    }
    const t = await go.SystemClip.PlanPaste(dir);
    if (!t.move) setCut(app, []);
    const names = await runTransfer(app, t);
    if (names && t.move) { await go.SystemClip.Clear().catch(() => {}); setCut(app, []); }
    focusIn(app, dir, names);
    return !!names;
  },
});

// ---- automatic refresh (F-065) ----

function watchOptions(app) {
  const ex = app.setting('refreshExclude', '');
  return {
    enabled: app.setting('autoRefresh', true) !== false,
    onlyActive: !!app.setting('refreshOnlyActive', false),
    exclude: (Array.isArray(ex) ? ex : String(ex).split(/[;\n]/)).map((s) => s.trim()).filter(Boolean),
    network: app.setting('refreshNetwork', true) !== false,
  };
}

const pending = new Map(); // panel → timer
// refreshPanel rereads a panel after a change on disk, unless the user is typing in it (inline rename) or it is busy;
// then it tries again a little later. The cursor stays on its file and the selection is kept (Panel.reload).
// Options > Refresh: "Delay before refreshing" is the pause after a change (120 ms by default) and "Refresh removable
// drives" off leaves panels on USB sticks and memory cards to Ctrl+R.
function refreshPanel(app, p) {
  const cv = (k, d) => app.config?.values?.[k] ?? d;
  if (cv('Refresh.RefreshRemovabl', '1') === '0' && app.volumeFor(p.path)?.kind === 'removable') return;
  const delay = Math.max(100, Math.min(60000, Number(cv('Refresh.IntervalMs', '120')) || 120));
  clearTimeout(pending.get(p));
  pending.set(p, setTimeout(async () => {
    pending.delete(p);
    const ae = document.activeElement;
    if (p.loading || (ae && p.el.contains(ae) && ae.matches('input, textarea'))) { refreshPanel(app, p); return; }
    const path = p.path;
    await p.reload();
    if (p.path === path) go.FS.VolumeOf(path).then((v) => p.renderDriveInfo(v), () => {});
  }, delay));
}

// leaveGone moves a panel whose folder was deleted, renamed or unmounted to the nearest folder that is still there.
async function leaveGone(app, p) {
  const gone = p.path;
  let dir = gone;
  for (let i = 0; i < 64; i++) {
    const up = dirName(dir);
    if (!up || up === dir) break;
    dir = up;
    if (await p.go(dir, { history: false, focus: baseName(gone) })) { app.flash(`The folder ${gone} is not there any more.`); return; }
  }
  await p.go(app.home);
}

// ---- volumes (F-709): the drive bar follows USB sticks and shares being mounted and ejected ----

async function volumesChanged(app) {
  await app.loadVolumes();
  for (const p of app.panels) {
    if (!isLocal(p.path) || app.demo) continue;
    const ok = await go.FS.Stat(p.path).then(() => true, () => false);
    if (!ok) leaveGone(app, p);
  }
}

// ---- the error log window (F-718) ----

command({
  name: 'cm_ErrorLog', label: 'Error log…', menu: 'Commands', menuAfter: 'cm_SyncDirs',
  run: (app) => modal((box, done) => {
    box.classList.add('mc-system-log');
    const paint = () => {
      const list = box.querySelector('.mc-system-loglist');
      list.innerHTML = errorLog.length ? errorLog.slice().reverse().map((x) => `<li><time>${esc(new Date(x.time).toLocaleTimeString())}</time> <span>${esc(x.text)}</span>${x.details.length ? `<ul>${x.details.slice(0, 50).map((d) => `<li>${esc(d)}</li>`).join('')}</ul>` : ''}</li>`).join('')
        : '<li class="mc-system-empty">No errors so far.</li>';
      box.querySelector('.mc-msg').textContent = errorLog.length ? `${plural(errorLog.length, 'error')} since mormiCommander started.` : 'No errors since mormiCommander started.';
    };
    box.innerHTML = `<h2>Error log</h2><p class="mc-msg"></p><ul class="mc-system-loglist"></ul><p class="mc-note"></p>
      <div class="mc-btns"><button type="button" class="copy">Copy all</button><button type="button" class="clear">Clear</button><button type="button" class="primary close">Close</button></div>`;
    paint();
    const off = onLog(paint);
    go.SystemLog.Path().then((p) => { if (app.setting('enableLog', false)) box.querySelector('.mc-note').textContent = `Also written to ${p}.`; }, () => {});
    box.querySelector('.copy').onclick = async () => {
      const text = errorLog.map((x) => [`${new Date(x.time).toISOString()} ${x.text}`, ...x.details.map((d) => `  ${d}`)].join('\n')).join('\n');
      try { await go.SystemClip.WriteText(text); app.flash('Copied the error log to the clipboard.'); } catch (e) { app.flash(errText(e), 'err'); }
    };
    box.querySelector('.clear').onclick = () => { errorLog.length = 0; paint(); };
    const close = () => { off(); done(); };
    box.querySelector('.close').onclick = close;
    box.onsubmit = (ev) => { ev.preventDefault(); close(); };
  }, undefined, { kind: 'error-log' }),
});

// ---- safety confirmations (F-719) ----

// Each confirmation is kept where the feature that asks reads it, so this dialog, Options > Copy/Delete and Options >
// Misc show and change the same thing: the copy dialog and the delete questions in settings "fileops"
// (features/fileops.js), the overwrite question as fileops.overwriteDefault ('' = ask), the drop question in
// "confirmDrop" (system-dnd.js), the quit question in the settings file's Misc.ConfirmExit (config/layout.js).
const fileopsGet = (app, k, d) => { const v = (app.setting('fileops', {}) || {})[k]; return v === undefined ? d : !!v; };
const fileopsSet = (app, part) => app.setSetting('fileops', { ...(app.setting('fileops', {}) || {}), ...part });
const CONFIRMS = [
  ['confirmCopy', 'Show the copy and move dialog (F5, F6)', (app) => fileopsGet(app, 'confirmCopy', true), (app, v) => fileopsSet(app, { confirmCopy: v })],
  ['confirmDelete', 'Ask before deleting files', (app) => fileopsGet(app, 'confirmDelete', true), (app, v) => fileopsSet(app, { confirmDelete: v })],
  ['confirmDeleteDirs', 'Ask before deleting folders that are not empty', (app) => fileopsGet(app, 'confirmDeleteDirs', true), (app, v) => fileopsSet(app, { confirmDeleteDirs: v })],
  ['confirmOverwrite', 'Ask before overwriting files',
    (app) => !(app.setting('fileops', {}) || {}).overwriteDefault,
    (app, v) => fileopsSet(app, { overwriteDefault: v ? '' : 'overwrite' })],
  ['confirmDrop', 'Ask before dropping files (drag and drop)', (app) => app.setting('confirmDrop', true) !== false, (app, v) => app.setSetting('confirmDrop', v)],
  ['confirmExit', 'Ask before quitting',
    (app) => app.config?.values?.['Misc.ConfirmExit'] === '1',
    (app, v) => applyOptions({ 'Misc.ConfirmExit': v ? '1' : '0' })],
];
command({
  name: 'cm_SafetyConfirmations', label: 'Confirmations…', menu: 'Configuration', menuAfter: 'cm_ConfigHotkeys',
  run: (app) => modal((box, done) => {
    const was = Object.fromEntries(CONFIRMS.map(([k, , get]) => [k, !!get(app)]));
    box.innerHTML = `<h2>Confirmations</h2><p class="mc-note">Which actions ask before they are done.</p>
      ${CONFIRMS.map(([k, l]) => `<label class="mc-system-check"><input type="checkbox" id="${k}"${was[k] ? ' checked' : ''}> ${esc(l)}</label>`).join('')}
      <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">OK</button></div>`;
    box.querySelector('.cancel').onclick = () => done(false);
    box.onsubmit = async (ev) => {
      ev.preventDefault();
      try {
        // only what was changed is written, so a choice like "skip" for overwriting stays until this box changes it
        for (const [k, , , set] of CONFIRMS) { const v = box.querySelector(`#${k}`).checked; if (v !== was[k]) await set(app, v); }
      } catch (e) { app.flash(errText(e), 'err'); }
      app.emit('config', null);
      app.emit('settings');
      done(true);
    };
  }, false, { kind: 'confirmations' }),
});

// ---- start-up ----

export function init(app) {
  // Enter on a file: last of the open handlers, so archives and the like are asked first
  app.on('ready', () => app.openHandlers.push((panel, e, path) => openEntry(app, panel, e, path)));

  // every error message also goes to the error log (job errors come with their full list, see below)
  const flash = app.flash.bind(app);
  let lastJobErr = '';
  app.flash = (msg, kind = '') => {
    if (kind === 'err' && !(lastJobErr && String(msg).startsWith(lastJobErr))) logError(msg);
    return flash(msg, kind);
  };
  on('job-done', (p) => {
    if (p?.errors?.length) {
      lastJobErr = p.errors[0];
      logError(`${({ copy: 'Copying', move: 'Moving', delete: 'Deleting', trash: 'Moving to the trash', wipe: 'Wiping' })[p.kind] || 'A job'} had ${plural(p.errors.length, 'error')}.`, p.errors);
    }
  });

  // automatic refresh: each panel watches its folder; changes reread it
  const configure = () => go.SystemWatch.Configure(watchOptions(app)).catch(() => {});
  configure();
  app.on('settings', configure);
  app.on('panel', (p, { moved }) => { if (moved) go.SystemWatch.Watch(p.side, isLocal(p.path) ? p.path : '').catch(() => {}); });
  on('system-fswatch', (ev) => {
    for (const p of app.panels) {
      if (p.path !== ev.path) continue;
      if (ev.kind === 'changed') refreshPanel(app, p);
      else if (ev.kind === 'gone') leaveGone(app, p);
    }
  });
  const active = () => go.SystemWatch.SetActive(document.hasFocus() && document.visibilityState === 'visible').catch(() => {});
  window.addEventListener('focus', active);
  window.addEventListener('blur', active);
  document.addEventListener('visibilitychange', active);
  active();
  on('system-volumes', () => volumesChanged(app));

  // Esc with an empty command line cancels a pending cut; it still clears the command line and stops slow folders
  addKeyHandler((ev, combo) => {
    if (combo === 'Esc' && cutFiles.size && !cmdline.text && dimCut(app)) cancelCut(app);
    return false;
  });
  // coming back to the window: the cut may have been replaced by a copy in another app
  window.addEventListener('focus', () => syncCut(app));

  // the menu and the command browser name Finder / Explorer / the file manager as the system calls it
  const ex = getCommand('cm_Explorer');
  if (ex) ex.label = revealLabel(app);
  // Quit asks first when Misc.ConfirmExit is on: config/layout.js wraps cm_Exit (F-719)
}

addControlCommand('errorlog', async () => ({ ok: true, errors: errorLog.slice() }));
addControlCommand('cut_state', async () => ({ ok: true, paths: cutFiles.list() }));
addControlCommand('clipboard_content', async () => ({ ok: true, content: await go.SystemClip.Content() }));
