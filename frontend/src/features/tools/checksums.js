// Checksums (internal/fileutil): create checksum files (cm_CRCCreate, F-332, F-689) in md5sum / shasum / SFV format,
// verify them (cm_CRCCheck, F-333, F-690; also Enter on a checksum file, as TC 11 does), and copy the checksums of the
// selected files to the clipboard (cm_ChecksumToClip, ours, F-693).

import { command } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { formDialog, modal } from '../../core/dialogs.js';
import { plural } from '../../core/format.js';
import { esc, runTool, withOverwrite, needLocal, targets, showError, failureList, reloadAll, deselect, pref, setPref, baseName, joinPath, copyText, isLocal } from './common.js';

let algCache = null;
const algorithms = async () => (algCache ||= await go.Tools.ChecksumAlgorithms().catch(() => [{ name: 'MD5', ext: '.md5' }]));
const algSelect = (algs, cur, id = 'algorithm') => `<select id="${id}">${algs.map((a) => `<option value="${esc(a.name)}"${a.name === cur ? ' selected' : ''}>${esc(a.name === 'CRC32' ? 'CRC32 (SFV)' : a.name)}</option>`).join('')}</select>`;

const CHECKSUM_EXT = /\.(sfv|md5|sha|sha1|sha224|sha256|sha384|sha512|sha3|sha3-256|sha3-512|blake2|blake2b|b2|b2sum|blake3|b3|b3sum|crc32)$/i;

command({
  name: 'cm_CRCCreate', label: 'Create checksum files…',
  enabled: (app) => !app.demo,
  run: async (app) => {
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    if (!paths.length) { app.flash('Nothing is selected.'); return; }
    const algs = await algorithms();
    const cur = pref(app, 'crcAlgorithm', 'MD5');
    const extOf = (name) => algs.find((a) => a.name === name)?.ext || '.md5';
    const base = entries.length === 1 ? (entries[0].dir ? entries[0].name : entries[0].base || entries[0].name) : (baseName(p.path) || 'checksums');
    const files = entries.filter((e) => !e.dir).length, dirs = entries.length - files;
    const what = [files ? plural(files, 'file') : '', dirs ? `${plural(dirs, 'folder')} (with everything inside)` : ''].filter(Boolean).join(' and ');
    const html = `<p class="mc-note">Checksums of ${esc(what)}.</p>
      <div class="mc-row"><label for="output">Checksum file</label><input type="text" id="output" spellcheck="false" value="${esc(joinPath(app, p.path, base + extOf(cur)))}" autofocus></div>
      <div class="mc-row"><label for="algorithm">Method</label>${algSelect(algs, cur)}</div>
      <label class="mc-check"><input type="checkbox" id="perFile"${pref(app, 'crcPerFile', false) ? ' checked' : ''}> A separate checksum file for each file</label>
      <label class="mc-check"><input type="checkbox" id="unix"${pref(app, 'crcUnix', app.platform !== 'windows') ? ' checked' : ''}> Unix format (LF line ends, / in paths)</label>`;
    const req = await formDialog('Create checksum files', html, (box) => {
      const q = (s) => box.querySelector(s);
      const output = q('#output').value.trim();
      if (!q('#perFile').checked && !output) return 'Type the name of the checksum file.';
      return { baseDir: p.path, paths, algorithm: q('#algorithm').value, onePerFile: q('#perFile').checked, output, unix: q('#unix').checked };
    }, (box) => {
      const q = (s) => box.querySelector(s);
      let prev = cur;
      q('#algorithm').addEventListener('change', () => {
        const v = q('#output').value, oldExt = extOf(prev), newExt = extOf(q('#algorithm').value);
        if (v.toLowerCase().endsWith(oldExt)) q('#output').value = v.slice(0, -oldExt.length) + newExt;
        prev = q('#algorithm').value;
      });
      const sync = () => { q('#output').disabled = q('#perFile').checked; };
      q('#perFile').addEventListener('change', sync);
      sync();
    }, { kind: 'crc-create', okLabel: 'Create' });
    if (!req) return;
    setPref(app, 'crcAlgorithm', req.algorithm);
    setPref(app, 'crcPerFile', req.onePerFile);
    setPref(app, 'crcUnix', req.unix);
    let res;
    try {
      res = await withOverwrite((overwrite) => runTool(`Creating ${req.algorithm} checksums`, (id) => go.Tools.CreateChecksums(id, { ...req, overwrite })));
    } catch (e) { await showError(e.message); return; }
    if (!res) return;
    await reloadAll(app, res.written.length === 1 ? res.written[0] : '');
    deselect(p, entries.map((e) => e.name));
    const msg = `Wrote ${res.written.length === 1 ? baseName(res.written[0]) : plural(res.written.length, 'checksum file')} for ${plural(res.hashed, 'file')}.`;
    if (res.failed.length) await failureList('Create checksum files', res.failed, `${msg} These files could not be read:`);
    else app.flash(msg);
  },
});

// showReports shows the result of a verify run: one block per checksum file, every line with its status, and
// "Only errors" to hide the OK lines.
function showReports(app, reports) {
  const total = reports.reduce((t, r) => ({ ok: t.ok + r.ok, failed: t.failed + r.failed, missing: t.missing + r.missing, errors: t.errors + r.errors }), { ok: 0, failed: 0, missing: 0, errors: 0 });
  const allOK = !total.failed && !total.missing && !total.errors && !reports.some((r) => r.badLines.length);
  const summary = `${total.ok} OK, ${total.failed} FAILED, ${total.missing} missing${total.errors ? `, ${total.errors} not readable` : ''}`;
  const text = reports.map((r) => r.text).join('\n');
  const STATUS = { OK: 'OK', FAILED: 'FAILED', MISSING: 'Not found', ERROR: 'Not readable' };
  let body = '';
  for (const r of reports) {
    body += `<h3>${esc(baseName(r.file))} <span class="mc-tools-dim">${esc(r.summary)}</span></h3><table class="mc-tools-verify">`;
    for (const e of r.entries) {
      body += `<tr class="${e.status === 'OK' ? 'ok' : 'bad'}"><td class="st">${esc(STATUS[e.status] || e.status)}</td><td>${esc(e.name)}${e.error && e.status !== 'MISSING' ? ` <span class="mc-tools-dim">${esc(e.error)}</span>` : ''}</td></tr>`;
    }
    for (const b of r.badLines) body += `<tr class="bad"><td class="st">Bad line</td><td>${esc(b)}</td></tr>`;
    body += '</table>';
  }
  return modal((box, done) => {
    box.classList.add('mc-info', 'mc-tools-verifybox');
    box.innerHTML = `<button type="button" class="mc-close" aria-label="Close" title="Close (Esc)">×</button><h2>Verify checksums</h2>
      <p class="mc-msg ${allOK ? 'mc-tools-good' : 'mc-tools-bad'}">${esc(allOK ? `All ${plural(total.ok, 'file')} OK.` : summary)}</p>
      <label class="mc-check"><input type="checkbox" id="onlyErrors"> Show only errors</label><div class="mc-tools-list">${body}</div>
      <div class="mc-btns"><button type="button" class="mc-tools-copy">Copy</button><button type="submit" class="primary">Close</button></div>`;
    box.querySelector('.mc-close').onclick = () => done();
    box.onsubmit = (ev) => { ev.preventDefault(); done(); };
    box.querySelector('#onlyErrors').addEventListener('change', (ev) => box.classList.toggle('mc-tools-onlybad', ev.target.checked));
    box.querySelector('.mc-tools-copy').onclick = async () => { if (await copyText(text)) app.flash('Copied the report.'); };
  }, undefined, { kind: 'crc-verify' });
}

async function verify(app, paths) {
  let reports;
  try { reports = await runTool('Verifying checksums', (id) => go.Tools.VerifyChecksums(id, paths)); } catch (e) { await showError(e.message); return; }
  await showReports(app, reports);
}

command({
  name: 'cm_CRCCheck', label: 'Verify checksums…',
  enabled: (app) => !app.demo,
  run: async (app) => {
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    const files = paths.filter((x, i) => !entries[i].dir);
    if (!files.length) { app.flash('Select a checksum file first (.md5, .sfv, .sha, .sha256…).'); return; }
    await verify(app, files);
    deselect(p, entries.map((e) => e.name));
  },
});

command({
  name: 'cm_ChecksumToClip', label: 'Copy checksums to clipboard…', menu: 'Files', menuAfter: 'cm_CRCCheck',
  enabled: (app) => !app.demo,
  run: async (app) => {
    if (!needLocal(app)) return;
    const { entries, paths } = targets(app);
    const files = paths.filter((x, i) => !entries[i].dir);
    if (!files.length) { app.flash('Select files first; folders have no checksum.'); return; }
    const algs = await algorithms();
    const req = await formDialog('Copy checksums to clipboard', `<p class="mc-note">${esc(files.length === 1 ? baseName(files[0]) : plural(files.length, 'file'))}</p>
      <div class="mc-row"><label for="algorithm">Method</label>${algSelect(algs, pref(app, 'clipAlgorithm', 'SHA256'))}</div>
      <label class="mc-check"><input type="checkbox" id="uppercase"> Capital letters (A–F)</label>`,
    (box) => ({ algorithm: box.querySelector('#algorithm').value, uppercase: box.querySelector('#uppercase').checked }), null, { kind: 'crc-clip', okLabel: 'Copy' });
    if (!req) return;
    setPref(app, 'clipAlgorithm', req.algorithm);
    let text;
    try { text = await runTool(`Calculating ${req.algorithm} checksums`, (id) => go.Tools.ChecksumText(id, files, req.algorithm, req.uppercase)); } catch (e) { await showError(e.message); return; }
    if (await copyText(text)) app.flash(`Copied the ${req.algorithm} checksum${files.length > 1 ? `s of ${plural(files.length, 'file')}` : ''}.`);
    else await showError(`The clipboard could not be used. The checksum is:\n\n${text}`);
  },
});

export function init(app) {
  // Enter on a checksum file verifies it (with several checksum files selected, all of them), as TC 11 does.
  app.openHandlers.push(async (panel, entry, path) => {
    if (!entry || entry.dir || !CHECKSUM_EXT.test(entry.name) || !isLocal(app, panel.path)) return false;
    const sel = panel.selectedEntries().filter((e) => !e.dir && CHECKSUM_EXT.test(e.name));
    const paths = sel.some((e) => e.name === entry.name) ? sel.map((e) => panel.fullPath(e.name)) : [path];
    await verify(app, paths);
    return true;
  });
}
