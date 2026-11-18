// Copy, move, delete, make folder and rename in place: F5, Shift+F5, F6, Shift+F6, F7, F8/Delete, Shift+F8/Shift+Delete,
// Wipe and the background transfer window (FEATURES.md sections 7, 20, 21, 23; behaviour from docs/spec/area-1.md).
// The work is done by internal/fileops through feature_jobs.go / feature_fileops.go; core/jobs.js shows progress and
// asks the engine's questions. This file has the dialogs and the TC rules around them: what a command acts on, how the
// target text is read, the F8/Shift+F8 swap when the trash is off, where the cursor goes afterwards.
//
// Settings (key "fileops", all optional; the Configuration dialog's Copy/Delete page can write them):
//   useTrash (true)            F8 moves to the trash; off: F8 deletes permanently and Shift+F8 uses the trash
//   confirmDelete (true)       ask before deleting
//   confirmDeleteDirs (true)   permanent delete: ask before each non-empty folder (Yes / All / Skip)
//   confirmDeleteRO (true)     permanent delete: ask before each read-only file
//   confirmOverwriteRO (true)  ask before a read-only target is overwritten
//   confirmCopy (true)         F5/F6 show the dialog; off: copy/move to the other panel at once
//   background (false)         OK in the copy dialog runs the job in the background (TC's AlwaysCopyInBackground)
//   verify, copyTimes, copyPermissions, copyOwner, copyAttributes, followLinks, checkFreeSpace, ignoreErrors,
//   bigFileMode, bufferKB, overwriteDefault ('' ask, 'overwrite', 'skip', 'older', 'rename', 'rename_target',
//   'larger', 'smaller'), showOptions, renameSelectExt (false: Shift+F6 selects the name without the extension),
//   speedLimitOn / speedLimitKB (the transfer window's limit for background jobs).
//   trashReplaced (false)      copy/move: files and links an overwrite replaces go to the trash first (board #99); the
//                              dialog's checkbox remembers the last choice
//   skipErrorsBackground (false) background jobs don't stop to ask when a file fails: the failures are listed at the
//                              end, with Retry failed, Copy list and Save log (board issue #15, core/joberrors.js)
// Histories: "copyHistory" (copy/move targets) and "mkdirHistory" (F7 names), newest first.

import { attachPathComplete } from '../core/pathcomplete.js';
import './fileops.css';
import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { modal, messageDialog, esc } from '../core/dialogs.js';
import { runJob, showTransfers, jobList, backgroundSpeedLimit } from '../core/jobs.js';
import { onErrorFor } from '../core/joberrors.js';
import { plural, F } from '../core/format.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { sameNameTarget, targetHistory } from './fileops-target.js';
import { tabList } from './tabs.js';
import { copiedNames } from './fileops-sel.js';
import { archiveRefusal } from './fileops-readonly.js';
import { noteXattrLoss } from './fileops-xattr.js';
import { overwriteNote } from './undo-lib.js';
import { noTrashReason } from './trash-lib.js';

const DEFAULTS = {
  useTrash: true, confirmDelete: true, confirmDeleteDirs: true, confirmDeleteRO: true, confirmOverwriteRO: true, confirmCopy: true,
  background: false, verify: false, copyTimes: true, copyPermissions: true, copyOwner: false, copyAttributes: true, followLinks: false,
  checkFreeSpace: true, ignoreErrors: false, bigFileMode: false, bufferKB: 1024, overwriteDefault: '', showOptions: false, renameSelectExt: false,
  skipErrorsBackground: false, trashReplaced: false,
};
const opt = (app, key) => { const s = app.setting('fileops', {}) || {}; return s[key] ?? DEFAULTS[key]; };
const setOpt = (app, key, v) => app.setSetting('fileops', { ...(app.setting('fileops', {}) || {}), [key]: v });

function remember(app, key, value, max = 20) {
  if (!value) return;
  const list = (app.setting(key, []) || []).filter((x) => x !== value);
  list.unshift(value);
  app.setSetting(key, list.slice(0, max));
}

// targetsOf is what F5/F6/F8 act on: the selection, or the entry under the cursor; on ".." nothing (F-832).
function targetsOf(app) {
  const t = app.cur.targets();
  if (!t.length) app.flash('No files selected.');
  return t;
}

// describe: "a.txt", "folder", "2 files and 1 folder".
function describe(items) {
  if (items.length === 1) return `"${items[0].name}"`;
  const dirs = items.filter((e) => e.dir).length, files = items.length - dirs;
  return [files ? plural(files, 'file') : '', dirs ? plural(dirs, 'folder') : ''].filter(Boolean).join(' and ');
}

const trashWord = (app) => (app.platform === 'windows' ? 'Recycle Bin' : app.platform === 'darwin' ? 'Trash' : 'trash');
// (a Feed to listbox result list, find://results-N, is not remote: its names map to the real files, see find.js; the
// same goes for a branch view, branch://<folder> and branchsel://N, see branch.js)
const isFeed = (p) => /^(find|branch|branchsel):\/\//i.test(p || '');
const isRemote = (p) => !isFeed(p) && /^[a-z][a-z0-9+.-]*:\/\//i.test(p || '');
// fromOf: the folder a relative target is read from (for a result list, the folder of the first item)
const fromOf = (src, items) => (isFeed(src.path) && items.length ? src.fullPath(items[0].name).replace(/[\\/][^\\/]*$/, '') || '/' : src.path);
const sepOf = (app, p) => app.sepFor(p);
const withSep = (app, p) => (p.endsWith('/') || p.endsWith('\\') ? p : p + sepOf(app, p));

function samePath(app, a, b) {
  const norm = (p) => { let x = String(p || ''); while (x.length > 1 && /[\\/]$/.test(x) && !/^[a-z]:\\$/i.test(x)) x = x.slice(0, -1); return x; };
  const [x, y] = [norm(a), norm(b)];
  return app.platform === 'linux' ? x === y : x.toLowerCase() === y.toLowerCase();
}

// insidePath says whether p is the folder dir or lies anywhere below it (copying dir there would copy it into itself).
function insidePath(app, p, dir) {
  if (samePath(app, p, dir)) return true;
  const fold = (s) => (app.platform === 'linux' ? String(s || '') : String(s || '').toLowerCase());
  const d = withSep(app, fold(dir));
  return fold(p).startsWith(d);
}

// ---- a dropdown inside a dialog: history, templates, and the folder tree of the copy dialog (F10) ----

class Drop {
  constructor(box, input, after) {
    this.input = input;
    this.el = document.createElement('div');
    this.el.className = 'mc-fileops-drop';
    this.el.setAttribute('role', 'listbox');
    this.el.hidden = true;
    after.after(this.el);
    this.items = [];
    this.at = 0;
    this.mode = '';
    this.el.addEventListener('mousedown', (ev) => {
      ev.preventDefault();
      const it = ev.target.closest('[data-i]');
      if (it) { this.at = +it.dataset.i; this.choose(); }
    });
  }

  get open() { return !this.el.hidden; }

  show(items, mode, title = '') {
    this.items = items;
    this.mode = mode;
    this.at = 0;
    this.title = title;
    this.el.hidden = false;
    this.paint();
  }

  paint() {
    this.el.innerHTML = (this.title ? `<div class="t">${esc(this.title)}</div>` : '') + (this.items.length
      ? this.items.map((it, i) => `<div class="it${i === this.at ? ' on' : ''}" data-i="${i}" role="option">${esc(it.label)}${it.detail ? `<span class="d">${esc(it.detail)}</span>` : ''}</div>`).join('')
      : '<div class="none">Nothing here.</div>');
    this.el.querySelector('.it.on')?.scrollIntoView({ block: 'nearest' });
  }

  close() { this.el.hidden = true; this.mode = ''; this.input.focus(); }

  choose() {
    const it = this.items[this.at];
    if (!it) return;
    if (it.open) { it.open(); return; } // a folder of the tree: go into it, the list stays
    this.input.value = it.value;
    this.input.dispatchEvent(new Event('input', { bubbles: true }));
    this.close();
  }

  // key handles the keys of an open list; true when used.
  key(ev) {
    if (!this.open) return false;
    switch (ev.key) {
      case 'ArrowDown': this.at = Math.min(this.items.length - 1, this.at + 1); this.paint(); return true;
      case 'ArrowUp': this.at = Math.max(0, this.at - 1); this.paint(); return true;
      case 'PageDown': this.at = Math.min(this.items.length - 1, this.at + 8); this.paint(); return true;
      case 'PageUp': this.at = Math.max(0, this.at - 8); this.paint(); return true;
      case 'Enter': this.choose(); return true;
      case 'Escape': this.close(); return true;
      case 'Backspace': if (this.mode === 'tree' && this.up) { this.up(); return true; } return false;
      default: return false;
    }
  }
}

// treeList fills a Drop with the folders inside path, with ".." on top; choosing a folder writes it into the field.
async function treeList(app, drop, path) {
  let l;
  try { l = await go.FS.List('fileops-tree', path); } catch (e) { app.flash(errText(e), 'err'); return; }
  const sep = sepOf(app, l.path);
  const join = (n) => (l.path.endsWith(sep) || l.path.endsWith('/') ? l.path + n : l.path + sep + n);
  const go2 = (p) => { drop.input.value = withSep(app, p); treeList(app, drop, p); };
  const dirs = (l.entries || []).filter((r) => r[3] & F.DIR).map((r) => r[0]).filter((n) => app.showHidden || !n.startsWith('.'))
    .sort((a, b) => a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' }));
  const items = [];
  if (l.parent) items.push({ label: '..', detail: 'up', open: () => go2(l.parent) });
  for (const n of dirs) items.push({ label: n, open: () => go2(join(n)) });
  drop.up = l.parent ? () => go2(l.parent) : null;
  drop.show(items, 'tree', l.path);
  drop.input.value = withSep(app, l.path);
}

// ---- copy / move dialog (F5, Shift+F5, F6) ----

const OVERWRITE = [['', 'Ask'], ['overwrite', 'Overwrite all'], ['skip', 'Skip all'], ['older', 'Overwrite all older'], ['rename', 'Auto-rename copied files'],
  ['rename_target', 'Auto-rename target files'], ['larger', 'Copy all larger'], ['smaller', 'Copy all smaller']];

function copyDialog(app, { move, same, items }) {
  const src = app.cur, dst = app.other;
  const verb = move ? 'Move' : 'Copy';
  let initial, select;
  if (same) {
    initial = items.length === 1 ? items[0].name : '*.*';
    const e = items[0];
    select = items.length === 1 && !e.dir && e.ext ? [0, e.base.length] : [0, initial.length];
  } else {
    initial = withSep(app, dst.path);
    // Options > FTP > Default download folder: copying from a server while the other panel is not a folder on this
    // computer (another server, an archive) offers that folder instead
    const dl = String(app.config?.values?.['FTP.DownloadDir'] ?? '').trim();
    const remote = (p) => /^[a-z][a-z0-9+.-]*:\/\//i.test(p || '');
    if (dl && remote(src.path) && remote(dst.path)) initial = withSep(app, dl);
    select = [0, initial.length];
  }
  const ch = (id, label, on, title = '') => `<label class="mc-check"${title ? ` title="${esc(title)}"` : ''}><input type="checkbox" id="${id}"${on ? ' checked' : ''}> ${esc(label)}</label>`;
  const attrsLabel = app.platform === 'windows' ? 'Copy alternate data streams' : app.platform === 'darwin' ? 'Copy extended attributes and resource forks' : 'Copy extended attributes';
  const ow = opt(app, 'overwriteDefault');
  const showOpts = !!opt(app, 'showOptions');
  // with the undo journal on (board #13), a choice that replaces files without asking says it can't be undone
  const journalOn = app.setting('undoJournal', true) !== false;
  return modal((box, done) => {
    box.classList.add('mc-fileops-copy');
    queueMicrotask(() => { const t = box.querySelector('#target'); if (t) attachPathComplete(t, () => src.path); }); // (F-782)
    box.innerHTML = `<h2>${verb}</h2>
      <div class="mc-row"><label for="target" class="mc-msg">${esc(`${verb} ${describe(items)} to:`)}</label>
        <div class="mc-fileops-field"><input id="target" type="text" spellcheck="false" autofocus value="${esc(initial)}" data-select="${select.join(',')}">
        <button type="button" class="mc-fileops-hist" tabindex="-1" title="Recent targets (Alt+Down)" aria-label="Recent targets">▾</button></div></div>
      <div class="mc-row"><label for="filter">Only files of this type:</label><input id="filter" type="text" spellcheck="false" value="*.*" title="For example *.txt *.md | old* — after | what to leave out; a name ending in / leaves out folders"></div>
      <div class="mc-fileops-opts"${showOpts ? '' : ' hidden'}>
        <div class="mc-row"><label for="overwrite">When a file is already there:</label><select id="overwrite">${OVERWRITE.map(([v, l]) => `<option value="${v}"${v === ow ? ' selected' : ''}>${esc(l)}</option>`).join('')}</select></div>
        <p class="mc-note mc-fileops-noundo"${overwriteNote(ow, journalOn) ? '' : ' hidden'}>${esc(overwriteNote(ow, journalOn))}</p>
        <div class="mc-fileops-checks">
          ${ch('verify', 'Verify after copying', opt(app, 'verify'), 'Read every copy back and compare it with the original')}
          ${ch('dates', 'Keep file dates', opt(app, 'copyTimes'))}
          ${ch('perms', app.platform === 'windows' ? 'Copy attributes' : 'Copy permissions', opt(app, 'copyPermissions'))}
          ${ch('owner', 'Copy owner and group (needs administrator rights)', opt(app, 'copyOwner'))}
          ${ch('attrs', attrsLabel, opt(app, 'copyAttributes'))}
          ${ch('links', 'Copy links as links (off: copy what they point to)', !opt(app, 'followLinks'))}
          ${ch('bigfile', 'Big file mode (large buffer, no system cache)', opt(app, 'bigFileMode'))}
          ${ch('ignoreErrors', 'Don\'t stop on files that can\'t be read (list them at the end)', opt(app, 'ignoreErrors'))}
          ${ch('trashReplaced', 'Move files that are overwritten to the trash', opt(app, 'trashReplaced'), 'The old file or link goes to the trash before the new one takes its place (local folders)')}
        </div>
      </div>
      <p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" class="opts">Options</button><button type="button" class="tree" title="Pick the target folder in a tree (F10)">Tree</button>
        <span class="mc-fileops-gap"></span><button type="button" class="cancel">Cancel</button><button type="button" class="queue" title="Add to the background queue (F2)">F2 Queue</button><button type="submit" class="primary">OK</button></div>`;
    const $ = (s) => box.querySelector(s);
    const input = $('#target');
    const drop = new Drop(box, input, $('.mc-fileops-field'));
    box._drop = drop;
    // the other panel's folder, the folders of its other tabs, then the recent targets (F-830)
    const history = () => targetHistory({ other: dst.path, tabs: tabList(dst.side).map((t) => t.path), recent: app.setting('copyHistory', []) || [],
      withSep: (p) => withSep(app, p) });
    box._history = () => (drop.open && drop.mode === 'hist' ? drop.close() : drop.show(history(), 'hist', 'Recent targets'));
    box._tree = () => {
      if (drop.open && drop.mode === 'tree') { drop.close(); return; }
      const v = input.value.trim();
      const start = v && !v.includes('*') && !v.includes('?') ? app.resolvePath(v, src.path) : dst.path;
      treeList(app, drop, start).catch(() => {});
    };
    $('.mc-fileops-hist').onclick = () => box._history();
    $('.tree').onclick = () => box._tree();
    $('#overwrite').onchange = () => { const t = overwriteNote($('#overwrite').value, journalOn); const n = $('.mc-fileops-noundo'); n.hidden = !t; n.textContent = t; };
    $('.opts').onclick = () => { const o = $('.mc-fileops-opts'); o.hidden = !o.hidden; setOpt(app, 'showOptions', !o.hidden); };
    $('.cancel').onclick = () => done(null);
    let queued = false;
    $('.queue').onclick = () => { queued = true; box.requestSubmit(); };
    const err = (msg) => { const e = $('.mc-err'); e.hidden = !msg; e.textContent = msg || ''; };
    let busy = false;
    box.onsubmit = async (ev) => {
      ev.preventDefault();
      if (busy) return;
      const q = queued;
      queued = false;
      const text = input.value.trim();
      if (!text) { err('Type the folder to copy to.'); return; }
      busy = true;
      try {
        let t;
        try { t = await go.Jobs.ParseTarget(text, items.length, fromOf(src, items)); } catch (e) { err(errText(e)); return; }
        // Shift+F5 on one folder with its name unchanged: the same name in the same folder, not into itself (#133)
        t = sameNameTarget({ same, items, text, srcPath: src.path, t });
        // Like TC, refuse a folder into itself here, before asking to make (and so making) a target inside it.
        const self = items.filter((e) => e.dir && insidePath(app, t.dir, src.fullPath(e.name)));
        if (self.length && self.length === items.length) {
          err(`Can't ${move ? 'move' : 'copy'} the folder ${self[0].name} into itself.`);
          return;
        }
        if (!t.dirExists) {
          const b = await messageDialog({ Title: 'Make the folder?', Message: `The folder ${t.dir} is not there.\n\nMake it?`, Buttons: ['Make it', 'Cancel'], Kind: 'make-target' });
          if (b !== 'Make it') return;
        }
        done({
          text, t, queued: q,
          filter: $('#filter').value.trim(), overwrite: $('#overwrite').value, verify: $('#verify').checked, dates: $('#dates').checked,
          perms: $('#perms').checked, owner: $('#owner').checked, attrs: $('#attrs').checked, links: $('#links').checked,
          bigfile: $('#bigfile').checked, ignoreErrors: $('#ignoreErrors').checked, trashReplaced: $('#trashReplaced').checked,
        });
        if ($('#trashReplaced').checked !== !!opt(app, 'trashReplaced')) setOpt(app, 'trashReplaced', $('#trashReplaced').checked);
      } finally { busy = false; }
    };
  }, null, {
    kind: move ? 'move' : 'copy',
    onKey: (ev) => {
      const box = document.querySelector('.mc-fileops-copy');
      if (!box) return false;
      if (box._drop.key(ev)) return true;
      if (ev.key === 'F2') { box.querySelector('.queue').click(); return true; }
      if (ev.key === 'F10') { box._tree(); return true; }
      if ((ev.key === 'ArrowDown' && ev.altKey) || ev.key === 'F4') { box._history(); return true; }
      if (ev.key === 'Enter' && ev.target.tagName !== 'BUTTON') { box.requestSubmit(); return true; }
      return false;
    },
  });
}

const normFilter = (f) => (!f || f === '*' || f === '*.*' ? '' : f);

// copyOrMove runs F5, Shift+F5 and F6.
async function copyOrMove(app, { move = false, same = false } = {}) {
  const src = app.cur, dst = app.other;
  const items = targetsOf(app);
  if (!items.length) return false;
  let a;
  if (!same && !opt(app, 'confirmCopy')) {
    // no dialog (Configuration: confirmCopy off): straight to the other panel with the default options
    const t = await go.Jobs.ParseTarget(withSep(app, dst.path), items.length, fromOf(src, items));
    a = { text: '', t, queued: false, filter: '', overwrite: opt(app, 'overwriteDefault'), verify: opt(app, 'verify'), dates: opt(app, 'copyTimes'),
      perms: opt(app, 'copyPermissions'), owner: opt(app, 'copyOwner'), attrs: opt(app, 'copyAttributes'), links: !opt(app, 'followLinks'),
      bigfile: opt(app, 'bigFileMode'), ignoreErrors: opt(app, 'ignoreErrors'), trashReplaced: opt(app, 'trashReplaced') };
  } else {
    a = await copyDialog(app, { move, same, items });
    if (!a) return false;
    remember(app, 'copyHistory', a.text);
  }
  const t = a.t;
  // into an archive that can't be changed: say why now, before any "Replace file?" question (board #143)
  const why = archiveRefusal(await go.FS.PathInfo(t.dir).catch(() => null));
  if (why) { await messageDialog({ Title: move ? 'Move' : 'Copy', Message: why }); return false; }
  let rename = t.rename === '*.*' || t.rename === '*' ? '' : t.rename;
  let overwrite = a.overwrite;
  // onto itself (Shift+F5 with the name unchanged): a copy gets a free name, "a(2).txt" (F-479, F-464); a move does nothing
  if (samePath(app, t.dir, src.path) && (!rename || (items.length === 1 && rename === items[0].name))) {
    if (move) { app.flash('The target is the same as the source: nothing to move.'); return false; }
    rename = '';
    overwrite = 'rename';
  }
  const background = a.queued || !!opt(app, 'background');
  // Options > Copy/Delete > Big file threshold: a file at least that big switches the job to big file mode by itself;
  // Move across drives: delete only after a full copy (the engine's deleteAfterAll)
  const cfgv = (k, d) => app.config?.values?.[k] ?? d;
  const bigMB = Number(cfgv('CopyDelete.BigFileThresholdMB', '1024')) || 1024;
  const bigfile = !!a.bigfile || items.some((e) => !e.dir && e.size >= bigMB * 1048576);
  const req = {
    kind: move ? 'move' : 'copy',
    sources: items.map((e) => src.fullPath(e.name)),
    target: t.dir, rename, overwrite,
    readOnly: opt(app, 'confirmOverwriteRO') ? '' : 'overwrite',
    // Options > Copy/Delete > Confirm overwriting hidden and system files (off by default, F-467)
    confirmHidden: cfgv('CopyDelete.ConfirmOverwriteHidden', '0') === '1',
    filter: normFilter(a.filter),
    links: a.links ? '' : 'follow',
    noDates: !a.dates, noPerms: !a.perms, noAttrs: !a.attrs, owner: !!a.owner, verify: !!a.verify, trashReplaced: !!a.trashReplaced,
    bufferSize: bigfile ? 16 << 20 : Math.max(16, Number(opt(app, 'bufferKB')) || 1024) * 1024, noCache: bigfile,
    onError: onErrorFor({ ignoreErrors: !!a.ignoreErrors, background, skipErrorsBackground: !!opt(app, 'skipErrorsBackground') }),
    ignoreSpace: !opt(app, 'checkFreeSpace'),
    queued: !!a.queued,
    deleteAfterAll: move && cfgv('CopyDelete.MoveDeleteAfterCopy', '0') === '1',
    speedLimit: background ? backgroundSpeedLimit() : 0,
  };
  src.saveSelection();
  const names = items.map((e) => e.name);
  const firstName = items.length === 1 && rename && !/[*?]/.test(rename) ? rename : names[0];
  const after = (final) => {
    // only the sources that got across are unselected; the skipped or failed ones stay selected (#132)
    const copied = copiedNames(final, req.sources, names);
    if (copied.length) {
      for (const n of copied) src.selected.delete(n);
      src.selectionChanged();
    }
    for (const p of app.panels) if (samePath(app, p.path, t.dir) && p !== src) p.focusName(firstName);
    if (samePath(app, src.path, t.dir) && !move) src.focusName(firstName);
    return final;
  };
  const title = move ? (t.sameVolume ? 'Moving' : 'Moving (copy, then delete)') : 'Copying';
  // to, from or between servers: the network transfers (features/net.js runNetJob: progress, overwrite with Resume)
  const onServer = app.isRemote || (() => false);
  if (app.netJob && (onServer(src.path) || onServer(t.dir))) {
    const NET_POLICY = { overwrite: 'overwrite', skip: 'skip', older: 'older' };
    if (!t.dirExists && onServer(t.dir)) await go.FS.Mkdir(t.dir).catch(() => {}); // ("Make it?" was answered)
    const final = await app.netJob({ kind: req.kind, sources: req.sources, target: t.dir, rename, overwrite: NET_POLICY[overwrite] || '' }, { title });
    return after(final);
  }
  noteXattrLoss(app, req.sources, t.dir); // a flash at most once when the target drops Finder tags etc. (#29)
  const job = runJob(req, { title, background });
  if (background) {
    // the command ends once the job has started (a refusal shows as its error); the job goes on and the panels
    // follow when it is done
    job.then(after, () => {});
    await job.started;
    return true;
  }
  return after(await job);
}

// ---- delete (F8 / Delete, Shift+F8 / Shift+Delete, Wipe) ----

async function deleteFiles(app, { permanent = false, wipe = false } = {}) {
  const src = app.cur;
  const items = targetsOf(app);
  if (!items.length) return false;
  // inside an archive or on a server there is no trash: the entries are removed for good (VfsJobs), as TC asks
  let place = isRemote(src.path) ? 'server' : '';
  let why = '';
  if (!place && !isFeed(src.path)) {
    const info = await go.FS.PathInfo(src.path).catch(() => null);
    if (info?.kind === 'archive') place = 'archive';
    why = archiveRefusal(info);
  }
  if (place && wipe) { app.flash(`Wiping works only for files on this computer, not in ${place === 'archive' ? 'an archive' : 'a server folder'}.`, 'err'); return false; }
  // an archive that can't be changed says so instead of asking to delete "for good" first (board #143)
  if (why) { await messageDialog({ Title: 'Delete', Message: why }); return false; }
  const useTrash = !!opt(app, 'useTrash');
  const trashOK = place ? false : await go.Jobs.TrashSupported().catch(() => false);
  let toTrash = !place && !wipe && (useTrash ? !permanent : permanent); // F-810: with the trash off, Shift+F8 is the trash
  // F-833, Options > Copy/Delete "Say when a drive has no trash" (off by default): a drive known to have no trash gets
  // the permanent delete question at once, instead of offering the trash
  const volReason = toTrash && trashOK && app.config?.values?.['CopyDelete.CheckVolumeTrash'] === '1'
    ? noTrashReason(app.platform, app.volumeFor?.(src.path), src.path) : '';
  const noTrash = toTrash && (!trashOK || !!volReason);
  if (noTrash) toTrash = false;
  const T = trashWord(app);
  const what = describe(items);
  let title, msg, button;
  if (place) {
    title = 'Delete';
    const them = items.length === 1 ? 'It' : 'They';
    msg = place === 'archive'
      ? `Delete ${what} from the archive?\n\n${them} will be removed from the archive for good: there is no ${T} for files in an archive.`
      : `Delete ${what} from the server?\n\n${them} will be deleted for good: there is no ${T} on a server.`;
    button = 'Delete';
  } else if (wipe) {
    title = 'Wipe';
    msg = `Wipe ${what}?\n\nThe contents are overwritten before they are deleted, so they can't be brought back, not even with recovery tools.`;
    button = 'Wipe';
  } else if (toTrash) {
    title = 'Delete';
    msg = `Move ${what} to the ${T}?`;
    if (app.volumeFor(src.path)?.kind === 'network') msg += `\n\nNetwork volumes often have no ${T}. If this one has none, you will be asked before anything is deleted permanently.`;
    button = `Move to ${T}`;
  } else {
    title = 'Delete permanently';
    msg = noTrash ? `There is no ${T} here${volReason ? ` (${volReason})` : ''}, so ${what} will be deleted permanently.\n\nDelete ${items.length === 1 ? 'it' : 'them'}? This can't be undone.`
      : `Delete ${what} permanently?\n\n${items.length === 1 ? 'It' : 'They'} will not go to the ${T}, and can't be brought back.`;
    button = 'Delete';
  }
  if (wipe || volReason || opt(app, 'confirmDelete')) { // (F-833: a delete that would have gone to the trash always asks)
    const b = await messageDialog({ Title: title, Message: msg, Buttons: [button, 'Cancel'], DefaultButton: button, Danger: !toTrash, Kind: wipe ? 'wipe' : 'delete' });
    if (b !== button) return false;
  }
  // the cursor goes to the entry after the first deleted one (or before, at the end of the list)
  const gone = new Set(items.map((e) => e.name));
  const first = src.entries.findIndex((e) => gone.has(e.name));
  let next = null;
  for (let i = first + 1; i < src.entries.length && next == null; i++) if (!gone.has(src.entries[i].name)) next = src.entries[i].name;
  for (let i = first - 1; i >= 0 && next == null; i--) if (!gone.has(src.entries[i].name)) next = src.entries[i].name;
  src.saveSelection();
  const req = {
    kind: wipe ? 'wipe' : toTrash ? 'trash' : 'delete',
    sources: items.map((e) => src.fullPath(e.name)),
    onError: 'ask',
    confirmDirs: !toTrash && !!opt(app, 'confirmDeleteDirs'),
    confirmReadOnly: !toTrash && !!opt(app, 'confirmDeleteRO'),
  };
  const final = await runJob(req, { title: wipe ? 'Wiping' : toTrash ? `Moving to the ${T}` : 'Deleting' });
  if (next != null && !src.index.has(items[0].name)) src.focusName(next); // (the first one is gone: deleting happened)
  return final;
}

// ---- make folder (F7) ----

const MKDIR_TEMPLATES = [['[Y]-[M]-[D]', 'today'], ['[Y]-[M]-[D] [h].[m]', 'now'], ['[Y][M][D]', 'today, compact']];

async function makeFolder(app) {
  const src = app.cur;
  const e = src.cur;
  // in an archive that can't be changed: say why instead of offering the dialog (board #143)
  const why = isFeed(src.path) ? '' : archiveRefusal(await go.FS.PathInfo(src.path).catch(() => null));
  if (why) { await messageDialog({ Title: 'New folder', Message: why }); return false; }
  const initial = e && !e.up ? (e.dir ? e.name : e.base) : '';
  const first = await modal((box, done) => {
    box.classList.add('mc-fileops-mkdir');
    box.innerHTML = `<h2>New folder</h2>
      <div class="mc-row"><label for="name">Name of the new folder:</label>
        <div class="mc-fileops-field"><input id="name" type="text" spellcheck="false" autofocus value="${esc(initial)}" data-select="0,${initial.length}">
        <button type="button" class="mc-fileops-hist" tabindex="-1" title="Recent names and date templates (Alt+Down)" aria-label="Recent names">▾</button></div></div>
      <p class="mc-note">Use / to make folders inside folders (a/b/c). [Y] [M] [D] [h] [m] [s] put in the date and time.</p>
      <p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">OK</button></div>`;
    const input = box.querySelector('#name');
    const drop = new Drop(box, input, box.querySelector('.mc-fileops-field'));
    box._drop = drop;
    box._history = () => {
      if (drop.open) { drop.close(); return; }
      const list = (app.setting('mkdirHistory', []) || []).map((n) => ({ label: n, value: n }));
      for (const [tpl, d] of MKDIR_TEMPLATES) list.push({ label: tpl, value: tpl, detail: d });
      drop.show(list, 'hist');
    };
    box.querySelector('.mc-fileops-hist').onclick = () => box._history();
    box.querySelector('.cancel').onclick = () => done(null);
    const err = (msg) => { const x = box.querySelector('.mc-err'); x.hidden = !msg; x.textContent = msg || ''; };
    let busy = false;
    box.onsubmit = async (ev) => {
      ev.preventDefault();
      if (busy) return;
      const name = input.value;
      if (!name.trim()) { err('Type a name for the new folder.'); return; }
      busy = true;
      try {
        const made = await go.Jobs.MkDir(src.path, name);
        remember(app, 'mkdirHistory', name.trim());
        done({ made });
      } catch (x) { err(errText(x)); } finally { busy = false; }
    };
  }, null, {
    kind: 'mkdir',
    onKey: (ev) => {
      const box = document.querySelector('.mc-fileops-mkdir');
      if (!box) return false;
      if (box._drop.key(ev)) return true;
      if ((ev.key === 'ArrowDown' && ev.altKey) || ev.key === 'F4') { box._history(); return true; }
      if (ev.key === 'Enter' && ev.target.tagName !== 'BUTTON') { box.requestSubmit(); return true; }
      return false;
    },
  });
  if (!first) return false;
  await src.reload();
  if (first.made) src.focusName(first.made);
  return true;
}

// ---- rename in place (Shift+F6) ----

let editor = null; // {panel, entry, input, mode, close(commit)}

// subPathPrefix is the folder part of a name that holds folders ("sub/deeper/" of "sub/deeper/a.txt"; "" for "a.txt").
// Only "/" separates folders on macOS and Linux, where "\" can be part of a file name.
function subPathPrefix(name, platform) {
  const i = platform === 'windows' ? Math.max(name.lastIndexOf('/'), name.lastIndexOf('\\')) : name.lastIndexOf('/');
  return i >= 0 ? name.slice(0, i + 1) : '';
}

export function renameSelection(e, mode) {
  const n = e.name;
  if (e.dir || !e.ext) return [0, n.length];
  if (mode === 0) return [0, e.base.length];
  if (mode === 1) return [e.base.length + 1, n.length];
  return [0, n.length];
}

function startRename(app) {
  const p = app.cur;
  const e = p.cur;
  if (editor) { // Shift+F6 again: name only → extension only → the whole name (F-502)
    editor.mode = (editor.mode + 1) % 3;
    const [a, b] = renameSelection(editor.entry, editor.mode);
    editor.input.setSelectionRange(a, b);
    return true;
  }
  if (!e || e.up) { app.flash('No file under the cursor.'); return false; }
  // In a result list or a branch view a name can hold folders ("sub/a.txt"): only the file's own name is edited, as a
  // rename never moves a file, and the folder part is put back for the panel's name afterwards.
  const pre = isFeed(p.path) ? subPathPrefix(e.name, app.platform) : '';
  const own = pre ? { ...e, name: e.name.slice(pre.length), base: e.dir ? e.name.slice(pre.length) : e.base.slice(pre.length) } : e;
  p.ensureVisible();
  p.renderRows();
  const list = p.$.list;
  const row = p.$.rows.querySelector(`.row[data-i="${p.cursor}"]`);
  const lr = list.getBoundingClientRect();
  const nameEl = row?.querySelector('.n b') || row?.querySelector('.n');
  const extEl = row?.querySelector('.e');
  const left = nameEl ? nameEl.getBoundingClientRect().left - lr.left + list.scrollLeft - 3 : 24;
  const right = extEl ? extEl.getBoundingClientRect().right - lr.left + list.scrollLeft : list.clientWidth - 8;
  const input = document.createElement('input');
  input.type = 'text';
  input.id = 'mc-rename';
  input.className = 'mc-fileops-rename';
  input.spellcheck = false;
  input.autocomplete = 'off';
  input.value = own.name;
  input.setAttribute('aria-label', `New name for ${own.name}`);
  input.style.top = `${p.cursor * p.rowH}px`;
  input.style.left = `${Math.max(0, left)}px`;
  input.style.width = `${Math.max(160, right - left)}px`;
  input.style.height = `${p.rowH}px`;
  list.append(input);
  const mode = opt(app, 'renameSelectExt') ? 2 : 0;
  const ed = { panel: p, entry: own, input, mode, closed: false };
  editor = ed;
  const finish = () => {
    if (ed.closed) return;
    ed.closed = true;
    input.remove();
    if (editor === ed) editor = null;
    if (app.cur === p && !document.querySelector('.mc-modal:not(.mc-modeless)')) p.$.list.focus({ preventScroll: true });
  };
  ed.close = finish;
  let committing = false;
  const commit = async (keepOnError) => {
    if (committing || ed.closed) return;
    const name = input.value;
    if (!name.trim() || name === own.name) { finish(); return; }
    committing = true;
    try {
      await go.Jobs.Rename(p.fullPath(e.name), name);
      if (p.selected.delete(e.name)) p.selected.add(pre + name);
      finish();
      await p.reload();
      p.focusName(pre + name);
    } catch (x) {
      app.flash(errText(x), 'err');
      if (keepOnError && !ed.closed) { input.focus(); } else finish();
    } finally { committing = false; }
  };
  ed.commit = commit;
  input.addEventListener('keydown', (ev) => {
    ev.stopPropagation();
    if (ev.key === 'Enter') { ev.preventDefault(); commit(true); }
    else if (ev.key === 'Escape') { ev.preventDefault(); finish(); }
    else if (ev.key === 'F6' && ev.shiftKey) { ev.preventDefault(); startRename(app); }
    else if (ev.key === 'Tab') { ev.preventDefault(); commit(false); }
  });
  input.addEventListener('blur', () => { setTimeout(() => { if (!ed.closed && document.activeElement !== input) commit(false); }, 0); });
  input.addEventListener('mousedown', (ev) => ev.stopPropagation());
  input.focus();
  const [a, b] = renameSelection(own, mode);
  input.setSelectionRange(a, b);
  return true;
}

// ---- commands ----

command({ name: 'cm_Copy', label: 'Copy…', short: 'Copy', run: (app) => copyOrMove(app) });
// (in the Files menu with its key, Shift+F5: F-479)
command({ name: 'cm_CopySamepanel', label: 'Copy in the same folder…', short: 'Copy here', menu: 'Files', menuAfter: 'cm_MultiRenameFiles', menuKey: true, run: (app) => copyOrMove(app, { same: true }) });
// Move or rename (F6) and the inline rename (Shift+F6) sit in the Files menu too, with their keys (F-502, F-503)
command({ name: 'cm_RenMov', label: 'Move or rename…', short: 'Move', menu: 'Files', menuAfter: 'cm_CopySamepanel', menuKey: true, run: (app) => copyOrMove(app, { move: true }) });
command({ name: 'cm_RenameOnly', label: 'Rename', short: 'Rename', menu: 'Files', menuAfter: 'cm_RenMov', menuKey: true, run: (app) => startRename(app) });
// New folder (F7) sits in the Files menu with its key too; the same dialog makes a/b/c and fills in [Y] [M] [D]… (F-520..F-522)
command({ name: 'cm_MkDir', label: 'New folder…', short: 'New folder', menu: 'Files', menuAfter: 'cm_RenameOnly', menuKey: true, run: (app) => makeFolder(app) });
command({ name: 'cm_Delete', label: 'Delete…', short: 'Delete', run: (app) => deleteFiles(app) });
command({ name: 'cm_DeletePermanent', label: 'Delete permanently…', short: 'Delete now', run: (app) => deleteFiles(app, { permanent: true }) });
command({ name: 'cm_Wipe', label: 'Wipe…', run: (app) => deleteFiles(app, { wipe: true }) });
command({ name: 'cm_BackgroundTransferManager', label: 'Background transfers…', run: () => showTransfers() });

export function init(app) {
  // a folder change in the panel being renamed in ends the editing (nothing is renamed)
  app.on('panel', (p, info) => { if (editor && editor.panel === p && info?.moved) editor.close(); });
  app.on('active', () => { if (editor && editor.panel !== app.cur) editor.commit?.(false); });
}

// Control mode: the inline rename editor and the jobs this page knows.
addControlCommand('rename_editor', async () => {
  if (!editor) return { ok: true, open: false };
  const i = editor.input;
  return { ok: true, open: true, name: editor.entry.name, value: i.value, selection: [i.selectionStart, i.selectionEnd], mode: editor.mode, side: editor.panel.side };
});
addControlCommand('transfers', async () => ({ ok: true, jobs: jobList(), panel: theApp.cur.side }));
