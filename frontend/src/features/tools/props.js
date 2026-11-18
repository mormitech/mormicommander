// Properties (cm_Properties, Alt+Enter, Cmd+I on macOS; F-184, F-263, F-324, F-681, F-682, F-691): our own window on
// every platform with name, type, place, size and size on disk (folders are counted at once, internal/dirsize), dates,
// permissions, owner, attributes, link target, extended attribute names (with "Remove quarantine flag" on macOS when the
// file has it, board #29); a button for the system's own window (Finder Get Info, Explorer
// properties, the file manager's properties); checksums of a file on demand, and a field to compare a pasted
// checksum. With several items selected it shows what they hold together.

import { command, hasCommand } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { modal } from '../../core/dialogs.js';
import { bytes, size, date, plural } from '../../core/format.js';
import { esc, needLocal, targets, showError, newId, onProgress, dirName, copyText } from './common.js';
import { xattrLines, hasQuarantine } from './xattr-lib.js';
import { removeQuarantine } from './xattr.js';
import { noteFor, saveNote, pickerHTML, pickerSetup, pickerValue } from '../notes.js';
import { onServer, remoteProps } from '../net-attrs.js';

const SYS_LABEL = { darwin: 'Get Info in Finder', windows: 'Windows properties…', linux: 'File manager properties…' };
const LINK_WORDS = { symlink: 'Symbolic link', junction: 'Junction', hardlink: 'Hard link', url: 'Shortcut file', webloc: 'Web location file', desktop: 'Desktop link file', lnk: 'Windows shortcut', alias: 'Finder alias' };

const full = (ms) => {
  if (!ms) return '';
  const d = new Date(ms);
  return `${date(ms)}:${String(d.getSeconds()).padStart(2, '0')}`;
};
const row = (label, value, cls = '') => (value === '' || value == null ? '' : `<tr${cls ? ` class="${cls}"` : ''}><th>${esc(label)}</th><td>${value}</td></tr>`);
const sizeText = (n) => (n < 1024 ? `${bytes(n)} ${n === 1 ? 'byte' : 'bytes'}` : `${size(n)} (${bytes(n)} bytes)`);

function propsTable(app, pr) {
  let h = '<table class="mc-tools-props">';
  h += row('Name', esc(pr.name));
  h += row('Type', esc(pr.type || pr.kind) + (pr.mime ? ` <span class="mc-tools-dim">${esc(pr.mime)}</span>` : ''));
  h += row('Location', esc(pr.folder));
  if (pr.isDir) {
    h += row('Size', '<span id="tp-size">Counting…</span>');
    h += row('Size on disk', '<span id="tp-disk"></span>');
    h += row('Contains', '<span id="tp-contains"></span>');
  } else {
    h += row('Size', esc(sizeText(pr.size)));
    if (pr.sizeOnDisk >= 0) h += row('Size on disk', esc(sizeText(pr.sizeOnDisk)));
  }
  if (pr.linkKind) {
    h += row('Link', esc(LINK_WORDS[pr.linkKind] || pr.linkKind) + (pr.linkKind === 'hardlink' ? ` (${pr.hardLinks} names)` : ''));
    if (pr.url) h += row('Address', esc(pr.url));
    else if (pr.linkTarget) h += row('Points to', esc(pr.linkTarget));
    if (pr.linkResolved && pr.linkResolved !== pr.linkTarget) h += row('Resolves to', esc(pr.linkResolved));
    if (pr.linkLoop) h += row('', '<span class="mc-tools-bad">The link goes round in a loop.</span>');
    else if (pr.linkBroken) h += row('', '<span class="mc-tools-bad">Broken: what it points to is not there.</span>');
    if (pr.linkChain?.length) h += row('Through', pr.linkChain.map(esc).join(' → '));
    if (pr.arguments) h += row('Arguments', esc(pr.arguments));
    if (pr.workingDir) h += row('Starts in', esc(pr.workingDir));
  }
  h += '<tr class="mc-tools-sep"><td colspan="2"></td></tr>';
  h += row('Created', esc(full(pr.created)));
  h += row('Modified', esc(full(pr.modified)));
  h += row('Accessed', esc(full(pr.accessed)));
  if (app.platform !== 'windows') h += row('Changed', esc(full(pr.changed)) + (pr.changed ? ' <span class="mc-tools-dim">(permissions or owner)</span>' : ''));
  h += '<tr class="mc-tools-sep"><td colspan="2"></td></tr>';
  if (pr.perm) h += row('Permissions', `<code>${esc(pr.mode || pr.perm)}</code> (${esc(pr.octal)})`);
  if (pr.owner) h += row('Owner', esc(pr.owner) + (pr.uid >= 0 ? ` <span class="mc-tools-dim">(${pr.uid})</span>` : ''));
  if (pr.group) h += row('Group', esc(pr.group) + (pr.gid >= 0 ? ` <span class="mc-tools-dim">(${pr.gid})</span>` : ''));
  if (app.platform === 'windows') h += row('Attributes', esc(pr.attrs || 'none'));
  if (pr.hidden) h += row('Hidden', 'yes');
  if (!pr.isDir && pr.hardLinks > 1 && pr.linkKind !== 'hardlink') h += row('Hard links', esc(String(pr.hardLinks)));
  // extended attributes (macOS, Linux; board #29): one name per line, shown only when the file has any
  const xl = xattrLines(pr.xattrs);
  if (xl.length) h += row('Extended attributes', `<span id="tp-xattrs">${xl.map(esc).join('<br>')}</span>`);
  if (pr.volume) h += row('Drive', esc(pr.volume) + (pr.fileSystem ? ` <span class="mc-tools-dim">(${esc(pr.fileSystem)}${pr.blockSize ? `, ${bytes(pr.blockSize)}-byte blocks` : ''})</span>` : ''));
  return h + '</table>';
}

function checksumPart(algs) {
  const opts = algs.map((a) => `<option${a.name === 'SHA256' ? ' selected' : ''}>${esc(a.name)}</option>`).join('');
  return `<fieldset class="mc-tools-group mc-tools-sum"><legend>Checksum</legend>
    <div class="mc-tools-inline"><select id="sumAlgorithm" aria-label="Checksum method">${opts}</select><button type="button" class="mc-tools-calc">Calculate</button></div>
    <p class="mc-tools-digest" id="tp-digest"></p>
    <div class="mc-row"><label for="sumCompare">Compare with a checksum (paste it here)</label><input type="text" id="sumCompare" spellcheck="false"></div>
    <p class="mc-note" id="tp-match"></p></fieldset>`;
}

// countInto counts a folder or several items and writes the result into the dialog while it runs.
function countInto(box, paths, selCount) {
  const id = newId('props');
  const set = (sel, text) => { const el = box.querySelector(sel); if (el) el.textContent = text; };
  const stop = onProgress(id, (p) => {
    if (p.itemDone !== undefined) return;
    set('#tp-size', `${sizeText(p.bytes || 0)}…`);
    set('#tp-contains', `${plural(p.files || 0, 'file')}, ${plural(p.dirs || 0, 'folder')}…`);
  });
  go.Tools.Space(id, paths).then((r) => {
    const t = r.total;
    set('#tp-size', sizeText(t.bytes) + (r.canceled ? ' (stopped)' : ''));
    set('#tp-disk', sizeText(t.allocated));
    const dirs = selCount ? t.dirs : Math.max(0, t.dirs - 1); // a single folder: what is inside it
    set('#tp-contains', `${plural(t.files, 'file')}, ${plural(dirs, 'folder')}${t.errors ? `, ${plural(t.errors, 'item')} not readable` : ''}`);
  }).catch((e) => set('#tp-size', String(e?.message ?? e))).finally(stop);
  return () => go.Tools.Cancel(id).catch(() => {});
}

// notePart is the note and colour label that mormiCommander keeps for the item (features/notes.js, #147); they are
// saved when the dialog closes.
function notePart(n) {
  return `<fieldset class="mc-tools-group"><legend>Note and colour label</legend>
    <div class="mc-row"><textarea id="tp-note" class="mc-notes-text" spellcheck="true" aria-label="Note">${esc(n.note || '')}</textarea></div>
    <div class="mc-row"><label>Colour label</label>${pickerHTML(n.color || '', 'tp')}</div>
    <p class="mc-note">Kept by mormiCommander, not in the file.</p></fieldset>`;
}

async function showProps(app, path, entry) {
  let pr;
  try { pr = await go.Tools.Properties(path); } catch (e) { await showError(String(e?.message ?? e)); return; }
  const algs = pr.isDir ? [] : await go.Tools.ChecksumAlgorithms().catch(() => []);
  let stopCount = () => {}, stopSum = () => {};
  const quarantined = app.platform === 'darwin' && hasQuarantine(pr.xattrs);
  const noteBefore = await noteFor(path);
  let readNote = () => null;
  const after = await modal((box, done) => {
    box.classList.add('mc-info', 'mc-tools-propsbox');
    box.innerHTML = `<button type="button" class="mc-close" aria-label="Close" title="Close (Esc)">×</button>
      <h2>Properties of ${esc(pr.name)}</h2>${propsTable(app, pr)}${notePart(noteBefore)}${algs.length ? checksumPart(algs) : ''}
      <div class="mc-btns"><button type="button" class="mc-tools-sys">${esc(SYS_LABEL[app.platform] || SYS_LABEL.linux)}</button>
      ${quarantined ? '<button type="button" class="mc-tools-unq">Remove quarantine flag</button>' : ''}
      <button type="button" class="mc-tools-attr">Change attributes…</button><button type="submit" class="primary">Close</button></div>`;
    box.querySelector('.mc-close').onclick = () => done();
    box.onsubmit = (ev) => { ev.preventDefault(); done(); };
    box.querySelector('.mc-tools-attr').onclick = () => done('attrs');
    pickerSetup(box, 'tp');
    const ta = box.querySelector('#tp-note');
    ta.addEventListener('keydown', (ev) => { if (ev.key === 'Enter' && !ev.ctrlKey && !ev.metaKey) ev.stopPropagation(); });
    readNote = () => ({ note: ta.value, color: pickerValue(box, 'tp') });
    const unq = box.querySelector('.mc-tools-unq');
    if (unq) {
      unq.onclick = async () => {
        unq.disabled = true;
        const r = await removeQuarantine(app, [path]);
        if (!r || r.failed?.length) { unq.disabled = false; return; }
        unq.remove();
        const fresh = await go.Tools.Properties(path).catch(() => null);
        const cell = box.querySelector('#tp-xattrs');
        if (fresh && cell) {
          const lines = xattrLines(fresh.xattrs);
          if (lines.length) cell.innerHTML = lines.map(esc).join('<br>');
          else cell.closest('tr')?.remove();
        }
      };
    }
    box.querySelector('.mc-tools-sys').onclick = () => go.Tools.OpenSystemProperties(path).catch((e) => app.flash(String(e?.message ?? e), 'err'));
    if (pr.isDir) stopCount = countInto(box, [path], 0);
    const calc = box.querySelector('.mc-tools-calc');
    if (calc) {
      const out = box.querySelector('#tp-digest');
      calc.onclick = async () => {
        const alg = box.querySelector('#sumAlgorithm').value;
        const id = newId('sum');
        out.textContent = 'Calculating…';
        const off = onProgress(id, (p) => { if (p.bytes) out.textContent = `Calculating… ${Math.round((p.bytesDone / p.bytes) * 100)}%`; });
        stopSum = () => go.Tools.Cancel(id).catch(() => {});
        try {
          const d = await go.Tools.Checksums(id, path, [alg]);
          out.innerHTML = `<code>${esc(d[alg] || '')}</code> <button type="button" class="mc-tools-copy">Copy</button>`;
          out.querySelector('.mc-tools-copy').onclick = async () => { if (await copyText(d[alg])) app.flash(`Copied the ${alg} checksum.`); };
        } catch (e) { out.textContent = String(e?.message ?? e); } finally { off(); }
      };
      const cmp = box.querySelector('#sumCompare'), match = box.querySelector('#tp-match');
      let timer = 0;
      cmp.addEventListener('input', () => {
        clearTimeout(timer);
        const v = cmp.value.trim();
        if (!v) { match.textContent = ''; return; }
        if (!/^[0-9a-f]{8,128}$/i.test(v)) { match.textContent = 'That does not look like a checksum (8 to 128 hex digits).'; return; }
        timer = setTimeout(async () => {
          match.textContent = 'Comparing…';
          const id = newId('match');
          try {
            const r = await go.Tools.MatchChecksum(id, path, v);
            match.textContent = !r.algorithm ? 'No checksum method has that length.' : r.ok ? `It matches (${r.algorithm}).` : `It does not match (${r.algorithm}).`;
            match.className = `mc-note ${r.ok ? 'mc-tools-good' : 'mc-tools-bad'}`;
          } catch (e) { match.textContent = String(e?.message ?? e); }
        }, 300);
      });
    }
    box.parentElement.addEventListener('mousedown', (ev) => { if (ev.target === ev.currentTarget) done(); });
  }, undefined, { kind: 'properties' });
  stopCount();
  stopSum();
  const n = readNote();
  if (n) await saveNote(path, noteBefore, n.note, n.color);
  if (after === 'attrs') {
    const p = app.cur;
    if (entry && !p.selectedEntries().length) p.focusName(entry.name);
    await app.run('cm_SetAttrib');
  }
}

async function showMany(app, paths, entries) {
  const files = entries.filter((e) => !e.dir).length, dirs = entries.length - files;
  let stop = () => {};
  const after = await modal((box, done) => {
    box.classList.add('mc-info', 'mc-tools-propsbox');
    box.innerHTML = `<button type="button" class="mc-close" aria-label="Close" title="Close (Esc)">×</button>
      <h2>Properties of ${plural(entries.length, 'item')}</h2><table class="mc-tools-props">
      ${row('Selected', esc(`${plural(files, 'file')}, ${plural(dirs, 'folder')}`))}
      ${row('Location', esc(dirName(paths[0])))}
      ${row('Size', '<span id="tp-size">Counting…</span>')}${row('Size on disk', '<span id="tp-disk"></span>')}${row('Contains', '<span id="tp-contains"></span>')}
      </table><div class="mc-btns"><button type="button" class="mc-tools-attr">Change attributes…</button><button type="submit" class="primary">Close</button></div>`;
    box.querySelector('.mc-close').onclick = () => done();
    box.onsubmit = (ev) => { ev.preventDefault(); done(); };
    box.querySelector('.mc-tools-attr').onclick = () => done('attrs');
    stop = countInto(box, paths, entries.length);
    box.parentElement.addEventListener('mousedown', (ev) => { if (ev.target === ev.currentTarget) done(); });
  }, undefined, { kind: 'properties' });
  stop();
  if (after === 'attrs') await app.run('cm_SetAttrib');
}

command({
  name: 'cm_Properties', menuKey: true, label: 'Properties…',
  enabled: (app) => !app.demo,
  run: async (app) => {
    // inside an archive: TC's "Info on packed file" (features/vfs.js, F-847) instead of "works on local files only"
    if (app.cur?.vfs?.kind === 'archive' && hasCommand('cm_PackedFileInfo')) return app.run('cm_PackedFileInfo');
    // on a server: the file's server, permissions, link target and owner (features/net-attrs.js, F-649)
    if (!app.demo && onServer(app)) return remoteProps(app);
    if (!needLocal(app)) return;
    const { entries, paths } = targets(app);
    if (!paths.length) { // on "..": the folder itself, as the system's file managers do
      if (app.cur.path) await showProps(app, app.cur.path, null);
      return;
    }
    if (paths.length === 1) await showProps(app, paths[0], entries[0]);
    else await showMany(app, paths, entries);
  },
});
