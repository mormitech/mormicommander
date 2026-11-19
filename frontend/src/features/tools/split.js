// Split and combine (cm_FileSplit, cm_FileCombine, F-328, F-329, F-687, F-688) and encode / decode (cm_UUEncode,
// cm_UUDecode, F-330, F-331, F-692) over internal/fileutil. Output goes to the other panel's folder by default, as in
// Total Commander. Split writes name.001, name.002, … and name.crc; combine finds the parts from any one of them and
// checks size and CRC32 against name.crc.

import { command } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { formDialog, messageDialog } from '../../core/dialogs.js';
import { bytes, size, plural } from '../../core/format.js';
import { esc, runTool, withOverwrite, needLocal, targets, showError, reloadAll, deselect, pref, setPref, baseName, joinPath, otherDir } from './common.js';
import { tm, tn, Msg, tText } from '../../core/i18n.js';

let presets = null;

command({
  name: 'cm_FileSplit', menuKey: true, label: 'Split file…',
  enabled: (app) => !app.demo,
  run: async (app) => {
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    if (entries.length !== 1 || entries[0].dir) { await showError('Please select a single file to split.', 'Split file'); return; }
    const e = entries[0];
    presets ||= await go.Tools.SplitPresets().catch(() => []);
    let values = { target: otherDir(app), size: pref(app, 'splitSize', '1.44 MB'), crc: pref(app, 'splitCRC', true) };
    for (;;) {
      const list = presets.filter((x) => x.size > 0).map((x) => `<option value="${esc(x.name)}">`).join('');
      const html = `<div class="mc-row"><label for="target">${esc(tm('Split {name} ({size}) to the folder', { name: e.name, size: size(e.size) }))}</label><input type="text" id="target" spellcheck="false" value="${esc(values.target)}"></div>
        <div class="mc-row"><label for="size">Bytes per part (a size like 700M or 4.7G, or a choice from the list)</label><input type="text" id="size" list="mc-tools-sizes" spellcheck="false" value="${esc(values.size)}" autofocus><datalist id="mc-tools-sizes">${list}</datalist></div>
        <label class="mc-check"><input type="checkbox" id="crc"${values.crc ? ' checked' : ''}> Write a .crc file (name, size and CRC32, for combining)</label>`;
      const req = await formDialog('Split file', html, (box) => {
        const target = box.querySelector('#target').value.trim(), sz = box.querySelector('#size').value.trim();
        if (!target) return 'Type the folder for the parts.';
        if (!sz) return 'Type the size of a part.';
        return { target, size: sz, crc: box.querySelector('#crc').checked };
      }, null, { kind: 'split', okLabel: 'Split' });
      if (!req) return;
      values = req;
      let partSize;
      try { partSize = await go.Tools.ParseSize(req.size); } catch (err) { await showError(String(err?.message ?? err), 'Split file'); continue; }
      if (partSize > 0 && partSize >= e.size) {
        const b = await messageDialog({ Title: 'Split file', Message: tm('{name} ({size} bytes) is not bigger than one part ({part} bytes).\n\nCreate one part anyway?', { name: e.name, size: bytes(e.size), part: bytes(partSize) }), Buttons: ['Yes', 'No'], Kind: 'split-small' });
        if (b !== 'Yes') continue;
      }
      break;
    }
    setPref(app, 'splitSize', values.size);
    setPref(app, 'splitCRC', values.crc);
    let res;
    try {
      res = await withOverwrite((overwrite) => runTool(tm('Splitting {name}', { name: e.name }), (id) => go.Tools.Split(id, { source: paths[0], targetDir: values.target, size: values.size, noCRC: !values.crc, overwrite })));
    } catch (err) { await showError(err.message, 'Split file'); return; }
    if (!res) return;
    await reloadAll(app, res.parts[0]);
    deselect(p, [e.name]);
    app.flash(res.crcFile ? tm('Split {name} into {what} and {crc}.', { name: e.name, what: tn(res.parts.length, 'part'), crc: baseName(res.crcFile) }) : tm('Split {name} into {what}.', { name: e.name, what: tn(res.parts.length, 'part') }));
  },
});

command({
  name: 'cm_FileCombine', menuKey: true, label: 'Combine files…',
  enabled: (app) => !app.demo,
  run: async (app) => {
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    const files = paths.filter((x, i) => !entries[i].dir);
    if (!files.length) { await showError('Select the first part (such as name.001) or the parts to combine.', 'Combine files'); return; }
    let info;
    try { info = await go.Tools.CombineInfo(files); } catch (err) { await showError(String(err?.message ?? err), 'Combine files'); return; }
    const first = baseName(info.parts[0]), last = baseName(info.parts[info.parts.length - 1]);
    const html = `<p class="mc-msg">${esc(info.parts.length === 1 ? first : `${tn(info.parts.length, 'part')}: ${first} … ${last}`)}</p>
      <div class="mc-row"><label for="target">Combine to</label><input type="text" id="target" spellcheck="false" value="${esc(joinPath(app, otherDir(app), info.name))}" autofocus></div>
      ${info.crc ? `<p class="mc-note">${esc(tm('The size ({size} bytes) and CRC32 will be checked against {name}.', { size: bytes(info.size), name: baseName(info.crc) }))}</p>` : '<p class="mc-note">There is no .crc file, so the result can\'t be checked.</p>'}`;
    const ans = await formDialog('Combine files', html, (box) => { const v = box.querySelector('#target').value.trim(); return v ? { target: v } : 'Type the name of the combined file.'; }, null, { kind: 'combine', okLabel: 'Combine' });
    if (!ans) return;
    const { target } = ans;
    let res;
    try {
      res = await withOverwrite((overwrite) => runTool(tm('Combining {name}', { name: first }), (id) => go.Tools.Combine(id, { parts: info.parts, auto: info.auto, target, overwrite })));
    } catch (err) { await reloadAll(app); await showError(err.message, 'Combine files'); return; }
    if (!res) return;
    await reloadAll(app, res.target);
    deselect(p, entries.map((x) => x.name));
    const v = { what: tn(res.parts, 'part'), name: baseName(res.target), size: size(res.size) };
    app.flash(res.checked ? tm('Combined {what} into {name} ({size}); size and CRC32 match.', v) : tm('Combined {what} into {name} ({size}).', v));
  },
});

const ENCODINGS = [['mime', 'MIME (Base64)'], ['uue', 'UUE'], ['xxe', 'XXE'], ['binhex', 'BinHex']];

command({
  name: 'cm_UUEncode', menuKey: true, label: 'Encode file…',
  enabled: (app) => !app.demo,
  run: async (app) => {
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    const files = paths.filter((x, i) => !entries[i].dir);
    if (!files.length) { await showError('Select the files to encode; folders can\'t be encoded.', 'Encode file'); return; }
    const enc = pref(app, 'encoding', 'mime');
    const html = `<div class="mc-row"><label for="target">${esc(tm('Encode {what} to the folder', { what: files.length === 1 ? baseName(files[0]) : tn(files.length, 'file') }))}</label><input type="text" id="target" spellcheck="false" value="${esc(otherDir(app))}" autofocus></div>
      <div class="mc-row"><label for="encoding">Encoding</label><select id="encoding">${ENCODINGS.map(([v, l]) => `<option value="${v}"${v === enc ? ' selected' : ''}>${l}</option>`).join('')}</select></div>
      <div class="mc-row"><label for="lines">Split into parts of this many lines (0 = one file; UUE and XXE only)</label><input type="text" id="lines" inputmode="numeric" value="${esc(String(pref(app, 'encodeLines', 0)))}"></div>`;
    const req = await formDialog('Encode file', html, (box) => {
      const target = box.querySelector('#target').value.trim();
      const lines = Number(box.querySelector('#lines').value.trim() || 0);
      if (!target) return 'Type the folder for the encoded files.';
      if (!Number.isInteger(lines) || lines < 0) return 'The number of lines must be 0 or more.';
      return { sources: files, targetDir: target, encoding: box.querySelector('#encoding').value, linesPerPart: lines };
    }, (box) => {
      const sync = () => { box.querySelector('#lines').disabled = !['uue', 'xxe'].includes(box.querySelector('#encoding').value); };
      box.querySelector('#encoding').addEventListener('change', sync);
      sync();
    }, { kind: 'encode', okLabel: 'Encode' });
    if (!req) return;
    if (!['uue', 'xxe'].includes(req.encoding)) req.linesPerPart = 0;
    setPref(app, 'encoding', req.encoding);
    setPref(app, 'encodeLines', req.linesPerPart);
    let res;
    try { res = await withOverwrite((overwrite) => runTool(tm('Encoding'), (id) => go.Tools.Encode(id, { ...req, overwrite }))); } catch (err) { await showError(err.message, 'Encode file'); return; }
    if (!res) return;
    await reloadAll(app, res.files[0] || '');
    deselect(p, entries.map((x) => x.name));
    app.flash(tm('Wrote {what}.', { what: res.files.length === 1 ? baseName(res.files[0]) : tn(res.files.length, 'file') }));
  },
});

command({
  name: 'cm_UUDecode', menuKey: true, label: 'Decode file…',
  enabled: (app) => !app.demo,
  run: async (app) => {
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    const files = paths.filter((x, i) => !entries[i].dir);
    if (!files.length) { await showError('Select the encoded file (.uue, .xxe, .b64, .hqx, .eml…).', 'Decode file'); return; }
    const html = `<div class="mc-row"><label for="target">${esc(files.length === 1 ? tm('Decode {what} to the folder', { what: baseName(files[0]) }) : tm('Decode {what} (read as one, in this order) to the folder', { what: tn(files.length, 'file') }))}</label><input type="text" id="target" spellcheck="false" value="${esc(otherDir(app))}" autofocus></div>
      <p class="mc-note">The format is found by itself: MIME, UUE, XXE, BinHex or Base64.</p>`;
    const ans = await formDialog('Decode file', html, (box) => { const v = box.querySelector('#target').value.trim(); return v ? { target: v } : 'Type the folder for the decoded files.'; }, null, { kind: 'decode', okLabel: 'Decode' });
    if (!ans) return;
    const { target } = ans;
    let res;
    try { res = await withOverwrite((overwrite) => runTool(tm('Decoding'), (id) => go.Tools.Decode(id, { sources: files, targetDir: target, overwrite }))); } catch (err) { await showError(err.message, 'Decode file'); return; }
    if (!res) return;
    await reloadAll(app, res.files[0] || '');
    deselect(p, entries.map((x) => x.name));
    const msg = tm('Decoded {what}.', { what: res.files.length === 1 ? baseName(res.files[0]) : tn(res.files.length, 'file') });
    if (res.warnings.length) await messageDialog({ Title: 'Decode file', Message: new Msg(`${msg.en}\n\n${res.warnings.slice(0, 8).join('\n')}`, `${msg}\n\n${res.warnings.slice(0, 8).map(tText).join('\n')}`), Kind: 'decode-warnings' });
    else app.flash(msg);
  },
});
