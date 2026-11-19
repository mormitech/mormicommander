// Shared pieces of the desktop-integration feature (system.js, system-dnd.js, system-menus.js): which paths are on
// this computer, the error log, and turning a paste or a drop into file jobs. It registers no commands.

import { go, errText } from '../core/go.js';
import { runJob } from '../core/jobs.js';
import { modal, esc } from '../core/dialogs.js';
import { plural } from '../core/format.js';
import { tm, tn, tText } from '../core/i18n.js';

// isLocal: a path on this computer (not inside an archive, not on an FTP/SFTP/WebDAV server).
export const isLocal = (p) => !!p && !/^[a-z][a-z0-9+.-]*:\/\//i.test(p);

export const baseName = (p) => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || p;

export function dirName(p) {
  const t = p.replace(/[\\/]+$/, '');
  const i = Math.max(t.lastIndexOf('/'), t.lastIndexOf('\\'));
  if (i < 0) return t;
  if (i === 0) return t[0];
  if (/^[a-z]:$/i.test(t.slice(0, i))) return t.slice(0, i + 1);
  return t.slice(0, i);
}

// quoteArg quotes a path for the command line when it has spaces or shell characters.
export function quoteArg(s, platform) {
  if (platform === 'windows') return /[\s&^|<>()"]/.test(s) ? `"${s}"` : s;
  return /[\s"'&|;<>()$`\\*?[\]{}!#~]/.test(s) ? `'${s.replace(/'/g, "'\\''")}'` : s;
}

// ---- the error log (F-718, section 39) ----

const LOG_MAX = 500;
export const errorLog = [];   // {time: ms, text, details: [lines]}
const logListeners = new Set();
export const onLog = (fn) => { logListeners.add(fn); return () => logListeners.delete(fn); };

// logError keeps an error for the error log window and writes it to the log file when that is on.
export function logError(text, details = []) {
  if (!text) return;
  errorLog.push({ time: Date.now(), text: String(text), details: details.map(String) });
  if (errorLog.length > LOG_MAX) errorLog.splice(0, errorLog.length - LOG_MAX);
  go.SystemLog.Write([`Error: ${text}`, ...details.map((d) => `  ${d}`)]).catch(() => {});
  logListeners.forEach((f) => { try { f(); } catch { /* a closed window */ } });
}

// ---- paste and drop as file jobs ----

const OPS = { copy: 'Copy', move: 'Move', link: 'Link' };

// askDrop is the drop dialog (F-694, F-719 confirmDrop): what to do and where, with the target folder editable. The
// answer is {op, dest} or null. Control mode sees it as kind "drop" with the field "drop-target".
export function askDrop(app, op, paths, dest) {
  const what = paths.length === 1 ? baseName(paths[0]) : plural(paths.length, 'file');
  const verbs = { copy: 'Copy', move: 'Move', link: 'Create links to' };
  return modal((box, done) => {
    box.classList.add('mc-system-dropbox');
    box.innerHTML = `<h2>${esc(verbs[op])} ${esc(what)}</h2>
      <div class="mc-row"><label for="drop-target">To</label><input id="drop-target" type="text" spellcheck="false" value="${esc(dest)}"></div>
      <p class="mc-note">Copy, move, or create symbolic links to the dropped files in this folder.</p><p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" data-op="cancel" class="cancel">Cancel</button>${Object.entries(OPS).map(([k, l]) => `<button type="button" data-op="${k}"${k === op ? ' class="primary"' : ''}>${l}</button>`).join('')}</div>`;
    const input = box.querySelector('#drop-target');
    const answer = (o) => {
      const d = input.value.trim();
      if (!d) { const e = box.querySelector('.mc-err'); e.hidden = false; e.textContent = tText('Type the folder to put the files in.'); return; }
      done({ op: o, dest: app.resolvePath(d, dest) });
    };
    box.querySelectorAll('[data-op]').forEach((b) => { b.onclick = () => (b.dataset.op === 'cancel' ? done(null) : answer(b.dataset.op)); });
    box.onsubmit = (ev) => { ev.preventDefault(); answer(op); };
    setTimeout(() => box.querySelector('.primary')?.focus(), 0);
  }, null, { kind: 'drop' });
}

// runTransfer starts the jobs of a planned paste or drop (go.SystemClip.PlanPaste / PlanTransfer): one job for the
// files going to another folder, and a renamed copy for each file copied into its own folder. Resolves with the
// names now in the target folder (for the cursor) or null when a job failed or was cancelled.
export async function runTransfer(app, t) {
  const kind = t.move ? 'move' : 'copy';
  const names = [];
  let ok = true;
  if (t.sources.length) {
    const r = await runJob({ kind, sources: t.sources, target: t.dest }, { title: t.move ? 'Moving' : 'Copying' });
    if (r.state === 'failed') logError(`${t.move ? 'Moving' : 'Copying'} to ${t.dest} had errors.`, r.errors || []);
    if (r.state !== 'done') ok = false;
    names.push(...t.sources.map(baseName));
  }
  for (const s of t.same || []) {
    const r = await runJob({ kind: 'copy', sources: [s.source], target: t.dest, rename: s.newName }, { title: 'Copying' });
    if (r.state === 'failed') logError(`Copying ${baseName(s.source)} had errors.`, r.errors || []);
    if (r.state !== 'done') ok = false;
    names.push(s.newName);
  }
  if (t.skipped && !t.sources.length && !(t.same || []).length) app.flash('The files are already in this folder.');
  return ok ? names : null;
}

// focusIn puts the cursor of the panel that shows dest on the first of names.
export function focusIn(app, dest, names) {
  if (!names?.length) return;
  for (const p of app.panels) if (p.path === dest) p.focusName(names[0]);
}

// transferFiles copies, moves or links paths into dest, asking first when confirm is set (a drop).
export async function transferFiles(app, op, paths, dest, { confirm = false } = {}) {
  if (!paths.length) return false;
  if (confirm) {
    const a = await askDrop(app, op, paths, dest);
    if (!a) return false;
    ({ op, dest } = a);
  }
  if (op === 'link') {
    const errs = await go.SystemFiles.CreateLinks(paths, dest);
    await Promise.all(app.panels.map((p) => p.reload()));
    if (errs?.length) { logError(`Some links could not be made in ${dest}.`, errs); app.flash(errs.slice(0, 3).join('\n'), 'err'); return false; }
    app.flash(tm('Made {what} in {name}.', { what: tn(paths.length, 'link'), name: baseName(dest) }));
    focusIn(app, dest, paths.map(baseName));
    return true;
  }
  let t;
  try { t = await go.SystemClip.PlanTransfer(paths, op === 'move', dest); } catch (e) { app.flash(errText(e), 'err'); return false; }
  const names = await runTransfer(app, t);
  focusIn(app, dest, names);
  return !!names;
}
