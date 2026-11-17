// Archives and remote folders inside the panels (FEATURES.md section 29): Enter on a zip/tar/7z/rar opens it as a
// folder and ".." leaves it (the listing goes through Go's vfs.Router, feature_fs.go); Ctrl+PgDn opens any file whose
// content is an archive (.jar, .docx, renamed zips); F5/F6/F8 into, out of and inside archives and server folders
// run as jobs of Go's VfsJobs (feature_vfs_jobs.go), which offers itself to core/jobs.js, so the copy and delete
// commands need nothing special; F3 and Enter on a file inside an archive use a temporary copy. Pack (Alt+F5,
// Alt+Shift+F5 with "move to archive"), Unpack (Alt+F9) and Test archives (Alt+Shift+F9) are here, with TC's dialogs.
//
// F4 on a file inside an archive edits a watched temporary copy (Go: feature_archive_edit.go); each save asks whether
// to update the archive.
//
// Dialog kinds for control mode: "pack", "unpack", "password", "test-report", "archive-edit", "archive-edit-error";
// the jobs show the usual "job" progress box and "overwrite" question. Control command "vfs" tells what kind of place each panel shows.

import { command, commandForKey, hasCommand } from '../core/registry.js';
import { go, on, errText } from '../core/go.js';
import { runJob, jobBackends, holdProgress } from '../core/jobs.js';
import { formDialog, infoDialog, messageDialog, esc } from '../core/dialogs.js';
import { updateQuestion, panelsShowing, UPDATE } from './archive-edit-lib.js';
import { addKeyHandler } from '../core/keyboard.js';
import { addControlCommand } from '../core/control.js';
import { plural } from '../core/format.js';
import { app as theApp } from '../core/app.js';
import { num as optNum } from './config/util.js';
import './vfs.css';

const base = (p) => String(p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop() || p;
const dirOf = (p) => { const s = String(p).replace(/[\\/]+$/, ''); const i = Math.max(s.lastIndexOf('/'), s.lastIndexOf('\\')); return i <= 0 ? s.slice(0, i + 1) : s.slice(0, i); };

// ---- jobs on archives and servers go to VfsJobs (0 = a plain local job: fileops takes it) ----
jobBackends.push({
  start: (req) => go.VfsJobs.Start(req).then((id) => id || 0, (e) => { if (/is not available/.test(errText(e))) return 0; throw e; }),
  cancel: (id) => go.VfsJobs.Cancel(id),
  answer: (id, a) => go.VfsJobs.Answer(id, a),
});

// ---- what a panel shows: a badge in the path bar for archives and servers ----
async function updateInfo(p) {
  let info = { kind: 'local' };
  try { info = await go.FS.PathInfo(p.path); } catch { /* the demo or no Go: local */ }
  p.vfs = info;
  p.el.classList.toggle('mc-vfs-archive', info.kind === 'archive');
  p.el.classList.toggle('mc-vfs-remote', !!info.remote);
  let badge = p.$.pbtns.querySelector('.mc-vfs-badge');
  if (info.kind === 'local' || info.kind === 'network') { badge?.remove(); return; } // (the Network panel has its own banner)
  if (!badge) {
    badge = document.createElement('span');
    badge.className = 'mc-vfs-badge';
    p.$.pbtns.prepend(badge);
  }
  const what = info.kind === 'archive' ? (info.format || 'archive').toUpperCase() : info.kind.toUpperCase();
  badge.textContent = info.readOnly ? `${what} · read-only` : what;
  badge.title = info.kind === 'results' ? 'Search results (Feed to listbox): file commands work on the real files. ".." goes back.'
    : info.kind === 'archive'
    ? `Inside the archive ${info.archive}${info.readOnly ? ' (it can be read, not changed)' : ''}. ".." leaves it.`
    : `On a server${info.readOnly ? ' (read-only)' : ''}.`;
}

// ---- Enter on an archive opens it as a folder ----
// With Options > Operation > Enter on an archive set to "Open with associated program" (ArchiveEnter 1), Enter leaves
// archives to the system's app for them; Ctrl+PgDn (opts.intoArchive) still opens them like a folder, as in TC.
async function openArchive(panel, entry, path, opts = {}) {
  if (entry.dir) return false;
  if (!opts.intoArchive && optNum('Operation.ArchiveEnter', 0) === 1) return false;
  // Options > Packer > Treat archives like folders off: Enter gives archives to their program; Ctrl+PgDn still opens them
  if (!opts.intoArchive && theApp.config?.values?.['Packer.OpenFolderLike'] === '0') return false;
  if (!(await go.FS.IsArchive(entry.name).catch(() => false))) return false;
  await panel.go(path);
  return true;
}

// the temporary local copy of a file inside an archive or on a server ("" for a local file)
async function localCopyOf(p, e) {
  if (!p.vfs || p.vfs.kind === 'local' || !e || e.dir || e.up) return '';
  return go.FS.LocalCopy(p.fullPath(e.name));
}

export function init(app) {
  app.openHandlers.unshift(openArchive);
  app.on('panel', (p, info) => { if (info?.moved || !p.vfs) updateInfo(p); });
  for (const p of app.panels) if (p.path) updateInfo(p);

  addKeyHandler((ev, combo) => {
    const name = commandForKey(combo);
    const p = app.cur, e = p?.cur;
    if (!e || e.up || e.dir) return false;
    if (name === 'cm_EnterDir') {
      // Ctrl+PgDn on a file: open it as an archive when its content is one (TC's "open archive under cursor")
      const path = p.fullPath(e.name);
      go.FS.IsArchiveFile(path).then((yes) => (yes ? p.go(path) : app.run('cm_EnterDir')), () => app.run('cm_EnterDir'));
      return true;
    }
    if (name === 'cm_List' && p.vfs && p.vfs.kind !== 'local') {
      // F3 inside an archive or on a server: view a temporary copy
      localCopyOf(p, e).then(async (local) => {
        if (!local) return;
        // N / P in the Lister walk the selected files when the cursor is on one of them, else all files of the folder
        // (unpacked when reached), and the cursor follows (spec area 4, 1.6)
        const sel = p.selectedEntries().filter((x) => !x.dir);
        const walk = sel.includes(e) ? sel : p.entries.filter((x) => !x.dir && !x.up);
        if (hasCommand('cm_List')) {
          await app.run('cm_List', { path: local, paths: [local], name: e.name, from: p.fullPath(e.name),
            sources: walk.map((x) => p.fullPath(x.name)), side: p.side, dir: p.path });
        }
        else { await go.FS.OpenFile(local); app.flash(`Opened ${e.name}.`); }
      }, (err) => app.flash(errText(err), 'err'));
      return true;
    }
    return false;
  });

  on('vfs-password', askPassword);
  on('archive-edit-changed', (c) => askArchiveUpdate(app, c));
}

// ---- F4 on a file inside an archive: the editor saved the temporary copy ----
async function askArchiveUpdate(app, c) {
  // Options > Viewer/Editor > Ask before uploading an edited remote file off: each save goes into the archive at once
  const ask = app.config?.values?.['ViewerEditor.EditTempUpload'] !== '0';
  const update = ask ? (await messageDialog(updateQuestion(c))) === UPDATE : true;
  try {
    await go.ListerEdit.AnswerArchiveEdit(c.id, update);
  } catch (e) {
    await messageDialog({ Title: 'Edit inside archive', Message: errText(e), Kind: 'archive-edit-error' });
    return;
  }
  if (!update) return;
  app.flash(`Updated ${c.name} in ${base(c.archive)}.`);
  for (const p of panelsShowing(app.panels, c.archive)) p.reload();
}

// ---- the password question of encrypted archives ----
async function askPassword(q) {
  const where = q.entry ? `${base(q.archive)} (${q.entry})` : base(q.archive);
  const html = `<p class="mc-msg">${esc(q.attempt > 1 ? `The password was wrong. Try again for ${where}.` : `${where} is encrypted. Enter its password.`)}</p>
    <div class="mc-row"><label for="mc-password">Password</label><input id="mc-password" type="password" class="mc-vfs-pw" autofocus></div>`;
  // (a job's progress box must not open over the question: the job waits for it)
  const release = holdProgress();
  let r;
  try {
    r = await formDialog('Password', html, (box) => ({ pw: box.querySelector('#mc-password').value }), null, { kind: 'password' });
  } finally { release(); }
  go.FS.AnswerPassword(q.id, r ? r.pw : '', !!r).catch(() => {});
}

// ---- Pack (Alt+F5, Alt+Shift+F5) ----
let packersCache = null;
async function packers() {
  if (!packersCache) packersCache = await go.VfsJobs.Packers().catch(() => []);
  return packersCache;
}

// sizeBytes reads a part size: "1.44" (MB), "700 MB", "650m", "4g", "100 KB".
export function sizeBytes(text) {
  const m = String(text).trim().toLowerCase().replace(',', '.').match(/^(\d+(?:\.\d+)?)\s*(b|k|kb|m|mb|g|gb)?$/);
  if (!m) return 0;
  const mult = { b: 1, k: 1024, kb: 1024, m: 1024 ** 2, mb: 1024 ** 2, g: 1024 ** 3, gb: 1024 ** 3 }[m[2] || 'mb'];
  return Math.floor(parseFloat(m[1]) * mult);
}

// the extensions each packer is known by, longest first: "tar.gz" and "tgz" are both TGZ
const EXT_ALIASES = { 'tar.gz': ['tgz'], 'tar.bz2': ['tbz', 'tbz2', 'tb2'], 'tar.xz': ['txz'], 'tar.zst': ['tzst'] };
function packerExts(list) {
  const out = [];
  for (const p of list) for (const e of new Set([p.ext, p.name, ...(EXT_ALIASES[p.name] || [])])) if (e) out.push([e.toLowerCase(), p]);
  return out.sort((a, b) => b[0].length - a[0].length);
}

// packerOfName is the packer a typed archive name asks for by its extension (null: no known extension).
function packerOfName(path, list) {
  const lower = base(path).toLowerCase();
  return packerExts(list).find(([e]) => lower.endsWith(`.${e}`))?.[1] || null;
}

function withExt(path, ext, list) {
  const lower = path.toLowerCase();
  for (const [e] of packerExts(list)) if (lower.endsWith(`.${e}`)) return `${path.slice(0, -(e.length + 1))}.${ext}`;
  return `${path}.${ext}`;
}

async function pack(app, args = {}) {
  const p = app.cur;
  const items = p.targets();
  if (!items.length) return false;
  const sources = items.map((e) => p.fullPath(e.name));
  const list = await packers();
  const saved = app.setting('packer', {});
  // Options > Packer and Zip: Move to archive / One archive per file / Include subfolders by default, and AES-256 (off:
  // ZipCrypto) as the encryption a new archive starts with; what the dialog chose last time still wins
  const ov = (k, d) => app.config?.values?.[k] ?? d;
  const req = {
    kind: 'pack', sources, format: args.format || saved.format || 'zip', level: args.level ?? saved.level ?? 0,
    noPaths: args.noPaths ?? saved.noPaths ?? false, noRecurse: args.noRecurse ?? saved.noRecurse ?? ov('Packer.ArchiveSubdirs', '1') === '0',
    move: args.move ?? ov('Packer.MoveToArchive', '0') === '1', separate: args.separate ?? ov('Packer.SeparateFiles', '0') === '1', password: args.password || '',
    encryption: args.encryption || saved.encryption || (ov('Zip.AES', '1') === '0' ? 'zipcrypto' : 'aes256'),
    volumeSize: args.volumeSize || 0,
  };
  let pk = list.find((x) => x.name === req.format && x.available) || list[0];
  req.format = pk?.name || 'zip';
  const one = items.length === 1 ? items[0] : null;
  const stem = one ? (one.dir ? one.name : (one.base || one.name)) : (base(p.path).replace(/\.[^.]+$/, '') || 'archive');
  const sep = app.sepFor(app.other.path);
  const dir = app.other.path.endsWith(sep) || app.other.path.endsWith('/') ? app.other.path : app.other.path + sep;
  const defName = (ext, separate) => dir + (separate && !one ? '*' : stem) + '.' + ext;
  req.target = args.target || defName(pk?.ext || 'zip', req.separate);

  if (!args.noDialog) {
    const opts = list.map((x) => `<option value="${esc(x.name)}"${x.available ? '' : ' disabled'}${x.name === req.format ? ' selected' : ''}>${esc(x.label)}${x.available ? '' : ' (not installed)'}</option>`).join('');
    const levels = ['<option value="0">Default</option>', ...[1, 2, 3, 4, 5, 6, 7, 8, 9].map((n) => `<option value="${n}"${n === req.level ? ' selected' : ''}>${n}${n === 1 ? ' (fastest)' : n === 9 ? ' (smallest)' : ''}</option>`)].join('');
    const check = (id, label, on) => `<label class="mc-check"><input type="checkbox" id="${id}"${on ? ' checked' : ''}> ${esc(label)}</label>`;
    const html = `<p class="mc-msg">${esc(one ? `Pack ${one.name} to the archive:` : `Pack ${plural(items.length, 'file')} to the archive:`)}</p>
      <div class="mc-row"><input id="mc-pack-target" type="text" spellcheck="false" value="${esc(req.target)}" autofocus></div>
      <div class="mc-pack-grid">
        <div class="mc-row"><label for="mc-pack-format">Packer</label><select id="mc-pack-format">${opts}</select></div>
        <div class="mc-row"><label for="mc-pack-level">Compression level</label><select id="mc-pack-level">${levels}</select></div>
      </div>
      <p class="mc-note mc-pack-note" hidden></p>
      ${check('mc-pack-paths', 'Also pack path names (only recursed)', !req.noPaths)}
      ${check('mc-pack-recurse', 'Recursively pack subfolders', !req.noRecurse)}
      ${check('mc-pack-split', 'Multiple disk archive (split into parts)', req.volumeSize > 0)}
      <div class="mc-row mc-pack-sub mc-pack-splitrow"><label for="mc-pack-size">Part size (MB, or with KB / GB)</label><input id="mc-pack-size" type="text" value="${esc(saved.partSize || '100')}"></div>
      ${check('mc-pack-move', 'Move to archive (delete the files after packing and testing)', req.move)}
      ${check('mc-pack-separate', 'Create separate archives, one per selected file/folder', req.separate)}
      ${check('mc-pack-encrypt', 'Encrypt', !!req.password)}
      <div class="mc-pack-sub mc-pack-cryptrow">
        <div class="mc-row"><label for="mc-pack-cipher">Encryption</label><select id="mc-pack-cipher">
          <option value="aes256">AES-256</option><option value="aes192">AES-192</option><option value="aes128">AES-128</option>
          <option value="zipcrypto">ZipCrypto (weak; for old unzip tools)</option></select></div>
        <div class="mc-row"><label for="mc-pack-password">Password</label><input id="mc-pack-password" type="password" class="mc-vfs-pw"></div>
        <div class="mc-row"><label for="mc-pack-password2">Password again</label><input id="mc-pack-password2" type="password" class="mc-vfs-pw"></div>
      </div>`;
    const setup = (box) => {
      const $ = (s) => box.querySelector(s);
      $('#mc-pack-cipher').value = req.encryption;
      const sync = () => {
        pk = list.find((x) => x.name === $('#mc-pack-format').value) || pk;
        $('#mc-pack-level').disabled = !pk.levels;
        for (const [id, ok] of [['#mc-pack-encrypt', pk.encrypt], ['#mc-pack-split', pk.volumes]]) {
          $(id).disabled = !ok;
          if (!ok) $(id).checked = false;
        }
        $('#mc-pack-cipher').disabled = pk.name !== 'zip';
        $('.mc-pack-splitrow').hidden = !$('#mc-pack-split').checked;
        $('.mc-pack-cryptrow').hidden = !$('#mc-pack-encrypt').checked;
        const note = $('.mc-pack-note');
        note.textContent = pk.single ? `${pk.label} packs one file only.` : pk.note || '';
        note.hidden = !note.textContent;
      };
      $('#mc-pack-format').addEventListener('change', () => {
        const t = $('#mc-pack-target');
        sync();
        t.value = withExt(t.value, pk.ext, list);
      });
      $('#mc-pack-separate').addEventListener('change', () => {
        const t = $('#mc-pack-target');
        if (!one) t.value = $('#mc-pack-separate').checked ? dirOf(t.value) + sep + '*.' + pk.ext : defName(pk.ext, false);
      });
      for (const id of ['#mc-pack-split', '#mc-pack-encrypt']) $(id).addEventListener('change', sync);
      sync();
    };
    const read = (box) => {
      const $ = (s) => box.querySelector(s);
      let target = $('#mc-pack-target').value.trim();
      if (!target) return 'Type the name of the archive.';
      let f = list.find((x) => x.name === $('#mc-pack-format').value);
      // the typed extension and the packer list must agree: a known extension picks its packer ("mv.zip" is a ZIP even
      // when TGZ was chosen last time), a name without one gets the packer's extension
      const byName = packerOfName(target, list);
      if (byName && byName !== f) {
        if (!byName.available) return byName.note || `The ${byName.label} packer is not available, so ${base(target)} can't be made.`;
        f = byName;
        $('#mc-pack-format').value = f.name;
      } else if (!byName && f) {
        target = `${target}.${f.ext}`;
      }
      if (!f?.available) return f?.note || 'This packer is not available.';
      if ($('#mc-pack-encrypt').checked && !f.encrypt) return `${f.label} archives can't be encrypted; choose ZIP or 7Z.`;
      if ($('#mc-pack-split').checked && !f.volumes) return `${f.label} archives can't be split into parts; choose ZIP or 7Z.`;
      const out = { target, format: f.name, level: Number($('#mc-pack-level').value) || 0, noPaths: !$('#mc-pack-paths').checked,
        noRecurse: !$('#mc-pack-recurse').checked, move: $('#mc-pack-move').checked, separate: $('#mc-pack-separate').checked,
        password: '', encryption: $('#mc-pack-cipher').value, volumeSize: 0, partSize: $('#mc-pack-size').value };
      if (f.single && (items.length > 1 || items.some((e) => e.dir)) && !out.separate) return `${f.label} packs one file only; choose ZIP or TAR for several files or folders.`;
      if (f.single && items.some((e) => e.dir)) return `${f.label} packs files only, not folders.`;
      if ($('#mc-pack-split').checked) {
        out.volumeSize = sizeBytes(out.partSize);
        if (out.volumeSize < 64 * 1024) return 'Type a part size of at least 64 KB, for example 100 (MB) or 4 GB.';
      }
      if ($('#mc-pack-encrypt').checked) {
        out.password = $('#mc-pack-password').value;
        if (!out.password) return 'Type the password.';
        if (out.password !== $('#mc-pack-password2').value) return 'The two passwords are not the same.';
      }
      if (out.separate && !one && !target.includes('*') && items.length > 1) out.target = dirOf(target) + sep + '*.' + f.ext;
      return out;
    };
    const r = await formDialog('Pack files', html, read, setup, { kind: 'pack' });
    if (!r) return false;
    Object.assign(req, r);
    app.setSetting('packer', { format: r.format, level: r.level, noPaths: r.noPaths, noRecurse: r.noRecurse, encryption: r.encryption, partSize: r.partSize });
    delete req.partSize;
  }
  if (args.noDialog && !args.format) { // (a scripted pack: the name's extension picks the packer, as in the dialog)
    const byName = packerOfName(req.target, list);
    if (byName?.available) req.format = byName.name;
  }
  p.saveSelection?.();
  const final = await runJob(req, { title: 'Packing' });
  if (final.state === 'done') {
    if (!req.separate) {
      const o = app.other;
      if (o.path === dirOf(req.target) || o.path + sep === dirOf(req.target) + sep) o.focusName(base(req.target));
    }
    p.clearSelection?.();
    app.flash(final.summary || 'Packed.');
  }
  return final.state === 'done';
}

command({ name: 'cm_PackFiles', label: 'Pack…', short: 'Pack', run: (app, args) => pack(app, args) });
command({ name: 'cm_PackFilesMove', label: 'Pack and delete…', run: (app, args) => pack(app, { ...args, move: true }) });

// ---- Unpack (Alt+F9) ----
command({
  name: 'cm_UnpackFiles', label: 'Unpack…', short: 'Unpack',
  run: async (app, args = {}) => {
    const p = app.cur;
    const items = p.targets().filter((e) => !e.dir);
    if (!items.length) { app.flash('No supported archive selected.', 'err'); return false; }
    const saved = app.setting('unpack', {});
    const req = { kind: 'unpack', sources: items.map((e) => p.fullPath(e.name)), target: args.target || app.other.path,
      mask: args.mask ?? '*.*', noPaths: args.noPaths ?? !(saved.paths ?? true), overwrite: (args.overwrite ?? saved.overwrite) ? 'overwrite' : '',
      separate: args.separate ?? saved.separate ?? false, password: args.password || '' };
    if (!args.noDialog) {
      const check = (id, label, on) => `<label class="mc-check"><input type="checkbox" id="${id}"${on ? ' checked' : ''}> ${esc(label)}</label>`;
      const html = `<p class="mc-msg">${esc(items.length === 1 ? `Unpack specific files from ${items[0].name} to:` : `Unpack ${plural(items.length, 'archive')} to:`)}</p>
        <div class="mc-row"><input id="mc-unpack-target" type="text" spellcheck="false" value="${esc(req.target)}" autofocus></div>
        <div class="mc-row"><label for="mc-unpack-mask">Files to unpack</label><input id="mc-unpack-mask" type="text" spellcheck="false" value="${esc(req.mask)}"></div>
        ${check('mc-unpack-paths', 'Unpack path names if stored with files', !req.noPaths)}
        ${check('mc-unpack-overwrite', 'Overwrite existing files', !!req.overwrite)}
        ${check('mc-unpack-separate', 'Unpack each archive to a separate subfolder (name of the archive)', req.separate)}`;
      const r = await formDialog('Unpack files', html, (box) => {
        const $ = (s) => box.querySelector(s);
        const target = $('#mc-unpack-target').value.trim();
        if (!target) return 'Type the folder to unpack to.';
        return { target: app.resolvePath(target, p.path), mask: $('#mc-unpack-mask').value.trim() || '*.*', paths: $('#mc-unpack-paths').checked,
          overwrite: $('#mc-unpack-overwrite').checked, separate: $('#mc-unpack-separate').checked };
      }, null, { kind: 'unpack' });
      if (!r) return false;
      Object.assign(req, { target: r.target, mask: r.mask, noPaths: !r.paths, overwrite: r.overwrite ? 'overwrite' : '', separate: r.separate });
      app.setSetting('unpack', { paths: r.paths, overwrite: r.overwrite, separate: r.separate });
    }
    const final = await runJob(req, { title: 'Unpacking' });
    if (final.state === 'done') { p.clearSelection?.(); app.flash(final.summary || 'Unpacked.'); }
    return final.state === 'done';
  },
});

// ---- Test archives (Alt+Shift+F9) ----
command({
  name: 'cm_TestArchive', label: 'Test archives',
  run: async (app) => {
    const p = app.cur;
    const items = p.targets().filter((e) => !e.dir);
    if (!items.length) { app.flash('No supported archive selected.', 'err'); return false; }
    const final = await runJob({ kind: 'test', sources: items.map((e) => p.fullPath(e.name)) }, { title: 'Testing archives' });
    if (final.state === 'cancelled') return false;
    const rows = (final.report || []).map((r) => `<li class="${r.ok ? 'ok' : 'bad'}"><b>${esc(base(r.archive))}</b>: ${r.ok ? `OK (${esc(plural(r.files, 'file'))})` : esc(r.problems.join(' · '))}</li>`).join('');
    const msg = final.report?.length ? final.summary : (final.errors || []).join(' ');
    await infoDialog(`<h2>Test archives</h2><p class="mc-msg">${esc(msg)}</p>${rows ? `<ul class="mc-vfs-report">${rows}</ul>` : ''}`, 'mc-vfs-test', 'test-report');
    return final.state === 'done';
  },
});

addControlCommand('vfs', async () => {
  const info = async (p) => (p?.path ? go.FS.PathInfo(p.path).catch(() => ({ kind: 'local' })) : null);
  return { ok: true, left: await info(theApp.left), right: await info(theApp.right), packers: await packers() };
});
