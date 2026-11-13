// What the Files-menu tools share on the page side: running a Go operation with a progress box and a Cancel button,
// the "Overwrite?" question, paths of the selection, remembered dialog choices, the clipboard. Not a feature module of
// its own (main.js only imports src/features/*.js); features/tools.js and its parts import it.

import { go, on, errText, setClipboardText } from '../../core/go.js';
import { modal, messageDialog, esc } from '../../core/dialogs.js';
import { size, plural } from '../../core/format.js';
import { track } from '../../core/busy.js';

export { esc };

let seq = 0;
export const newId = (what) => `${what}-${Date.now().toString(36)}-${++seq}`;

// Progress events from Go ("tools-progress", {id, ...}) go to whoever waits for that id.
const listeners = new Map();
let listening = false;
export function onProgress(id, fn) {
  if (!listening) {
    listening = true;
    on('tools-progress', (p) => listeners.get(p?.id)?.(p));
  }
  listeners.set(id, fn);
  return () => listeners.delete(id);
}

// runTool runs fn(id) (a Go call) and shows a progress box with a Cancel button when it takes more than a moment. It
// resolves with what Go answered and rejects with a sentence. The box has kind "tools-progress" for control mode.
export function runTool(title, fn, { delay = 350 } = {}) {
  const id = newId('tool');
  return track((async () => {
    let p = {};
    let paint = () => {};
    let close = () => {};
    const stop = onProgress(id, (np) => { if (np.itemDone === undefined) { p = np; paint(); } });
    const timer = setTimeout(() => {
      modal((box, done) => {
        close = () => done();
        box.classList.add('mc-tools-progress');
        box.innerHTML = `<h2>${esc(title)}</h2><p class="mc-msg"></p><div class="mc-progress"><i></i></div><p class="mc-note"></p>
          <div class="mc-btns"><button type="button" class="cancel">Cancel</button></div>`;
        const msg = box.querySelector('.mc-msg'), bar = box.querySelector('.mc-progress i'), note = box.querySelector('.mc-note');
        paint = () => {
          msg.textContent = p.text || (p.current ? baseName(p.current) : '');
          const frac = p.bytes > 0 ? p.bytesDone / p.bytes : p.total > 0 ? p.done / p.total : p.itemsTotal > 0 ? p.itemsDone / p.itemsTotal : 0;
          bar.style.width = `${Math.round(Math.min(1, frac || 0) * 100)}%`;
          if (p.files !== undefined) note.textContent = `${size(p.bytes || 0)} in ${plural(p.files || 0, 'file')}, ${plural(p.dirs || 0, 'folder')}`;
          else if (p.bytes > 0) note.textContent = `${size(p.bytesDone || 0)} of ${size(p.bytes)}`;
          else if (p.total > 0) note.textContent = `${p.done} of ${plural(p.total, 'item')}`;
        };
        paint();
        box.querySelector('.cancel').onclick = () => go.Tools.Cancel(id);
        box.onsubmit = (ev) => ev.preventDefault();
      }, undefined, { kind: 'tools-progress' }).then(() => go.Tools.Cancel(id).catch(() => {}));
    }, delay);
    try {
      return await fn(id).catch((e) => { throw new Error(errText(e)); });
    } finally {
      clearTimeout(timer);
      stop();
      close();
    }
  })());
}

// askOverwrite asks before a tool replaces a file that is already there.
export async function askOverwrite(path, what = '') {
  const b = await messageDialog({
    Title: 'Overwrite?', Message: `${what || baseName(path) || 'The file'} is already there.\n\nReplace it?`,
    Buttons: ['Overwrite', 'Cancel'], DefaultButton: 'Overwrite', CancelButton: 'Cancel', Kind: 'tools-overwrite',
  });
  return b === 'Overwrite';
}

// withOverwrite calls run(false); when the answer says a file is in the way (exists), it asks and calls run(true).
export async function withOverwrite(run) {
  const r = await run(false);
  if (r && r.exists) {
    if (!(await askOverwrite(r.exists === true ? '' : r.exists))) return null;
    return run(true);
  }
  return r;
}

// showError shows a sentence in a message box (for results the user must read, unlike a flash).
export const showError = (msg, title = 'mormiCommander') => messageDialog({ Title: title, Message: msg, Kind: 'tools-error' });

// failureList shows the files a tool could not handle: [{path, error}].
export function failureList(title, failed, intro = '') {
  const lines = failed.slice(0, 12).map((f) => `${baseName(f.path)}: ${f.error}`);
  if (failed.length > 12) lines.push(`…and ${failed.length - 12} more.`);
  return messageDialog({ Title: title, Message: `${intro ? intro + '\n\n' : ''}${lines.join('\n')}`, Kind: 'tools-failed' });
}

export const baseName = (p) => String(p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop();
// dirName is the folder of a path: "/a/b.txt" → "/a", "/b.txt" → "/", "C:\x.txt" → "C:\".
export function dirName(p) {
  const s = String(p || '');
  const i = Math.max(s.lastIndexOf('/'), s.lastIndexOf('\\'));
  if (i < 0) return '';
  if (i === 0) return s[0];
  return /^[a-z]:$/i.test(s.slice(0, i)) ? s.slice(0, i + 1) : s.slice(0, i);
}

// joinPath joins a folder and a name with the folder's separator.
export function joinPath(app, dir, name) {
  const sep = app.sepFor(dir);
  return dir.endsWith(sep) || dir.endsWith('/') ? dir + name : dir + sep + name;
}

// isLocal says whether a panel folder is on this computer (not the demo, an archive URL or a server).
export const isLocal = (app, path) => !app.demo && !!path && !path.includes('://');

// targetPaths is what a command works on (the selection or the entry under the cursor) as full paths.
export function targets(app, p = app.cur) {
  const list = p.targets();
  return { panel: p, entries: list, paths: list.map((e) => p.fullPath(e.name)) };
}

// otherDir is the default output folder: the other panel's folder, or this one when the other is not on this computer.
export const otherDir = (app) => (isLocal(app, app.other.path) ? app.other.path : app.cur.path);

// needLocal shows why a tool can't run here, and returns false then.
export function needLocal(app, p = app.cur) {
  if (app.demo) { app.flash('This is the demo: its files are not real, so this tool can\'t work on them.', 'err'); return false; }
  if (!isLocal(app, p.path)) { app.flash('This tool works on files on this computer only, not in archives or on servers.', 'err'); return false; }
  return true;
}

// Remembered choices of the dialogs, all under the settings key "tools".
export const pref = (app, key, def) => { const v = app.setting('tools', {})?.[key]; return v === undefined ? def : v; };
export const setPref = (app, key, value) => app.setSetting('tools', { ...(app.setting('tools', {}) || {}), [key]: value });

// reloadAll rereads both panels and puts the cursor on a new file where it is shown.
export async function reloadAll(app, focus = '') {
  await Promise.all(app.panels.map((pn) => pn.reload()));
  if (focus) {
    const dir = dirName(focus), name = baseName(focus);
    for (const pn of app.panels) if (pn.path === dir || pn.path.replace(/[\\/]+$/, '') === dir) pn.focusName(name);
  }
}

// deselect unselects the entries a tool handled (TC unselects files once an operation is done with them).
export function deselect(p, names) {
  const set = new Set(names);
  let any = false;
  for (const e of p.selectedEntries()) if (set.has(e.name)) { p.select(e, false); any = true; }
  if (any) p.selectionChanged();
}

// copyText puts text on the clipboard.
export async function copyText(text) {
  if (await setClipboardText(text)) return true;
  try { await navigator.clipboard.writeText(text); return true; } catch { return false; }
}

const pad = (n, w = 2) => String(n).padStart(w, '0');

// dateInput turns Unix ms into the value of an <input type="datetime-local" step="1"> (local time, seconds included).
export function dateInput(ms) {
  if (!ms) return '';
  const d = new Date(ms);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

// parseDateInput reads such a value back (also "2026-10-14 09:30", without seconds); NaN when it is not a date.
export function parseDateInput(s) {
  const m = String(s || '').trim().match(/^(\d{4})-(\d{1,2})-(\d{1,2})[T ](\d{1,2}):(\d{2})(?::(\d{2})(?:\.(\d{1,3}))?)?$/);
  if (!m) return NaN;
  const [y, mo, d, h, mi, se = 0, ms = 0] = m.slice(1).map((x) => Number(x || 0));
  const t = new Date(y, mo - 1, d, h, mi, se, ms);
  if (t.getMonth() !== mo - 1 || t.getDate() !== d || h > 23) return NaN;
  return t.getTime();
}
