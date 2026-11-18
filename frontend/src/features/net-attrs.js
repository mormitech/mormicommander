// Files on servers (issue #78): Properties (Alt+Enter, cm_Properties, F-649) of an FTP, SFTP or WebDAV file shows its
// server, folder, size, date, permissions, link target and owner; Change attributes (cm_SetAttrib, F-633) on an FTP or
// SFTP panel sets the Unix permissions with the rwx grid or the octal number, optionally through subfolders (files
// only, folders only, or both) and the modification time where the server can set it; Create symbolic link
// (cm_CreateSymbolicLink, F-633) on an SFTP panel makes a link on the server. The tools' own commands hand over to
// these when the active panel shows a server (features/tools/props.js, attrs.js, links.js); local files are unchanged.
// Go side: Net.RemoteInfo, Net.RemoteChmod, Net.RemoteSymlink (feature_net_attrs.go).

import { go, errText } from '../core/go.js';
import { modal, formDialog, infoDialog, messageDialog, esc } from '../core/dialogs.js';
import { size, date, plural } from '../core/format.js';
import { PERM_ROWS, SPECIAL_BITS, parseOctal, octalText, bitsOfEntry, permString } from './net-attrs-lib.js';

const KIND_LABEL = { ftp: 'FTP', sftp: 'SFTP', webdav: 'WebDAV' };
const row = (label, html) => `<tr><th>${esc(label)}</th><td>${html}</td></tr>`;
const isServer = (app, p) => !!app.isRemote?.(p);

// onServer says whether the active panel shows a server (the tools then use the functions below).
export const onServer = (app) => isServer(app, app.cur?.path);

// remoteProps shows the properties of the item under the cursor (or the folder itself on "..").
export async function remoteProps(app) {
  const p = app.cur;
  const items = p.targets();
  const path = items.length ? p.fullPath(items[0].name) : p.path;
  if (items.length > 1) {
    const files = items.filter((e) => !e.dir).length;
    const bytes = items.reduce((n, e) => n + (e.dir ? 0 : e.size || 0), 0);
    await infoDialog(`<h2>Properties of ${plural(items.length, 'item')}</h2><table class="mc-tools-props">
      ${row('Selected', esc(`${plural(files, 'file')}, ${plural(items.length - files, 'folder')}`))}${row('Location', esc(p.path))}
      ${row('Size of the files', esc(size(bytes)))}</table>
      <div class="mc-btns"><button type="submit" class="primary">Close</button></div>`, 'mc-tools-propsbox', 'properties');
    return true;
  }
  let info;
  try { info = await go.Net.RemoteInfo(path); } catch (e) { await messageDialog({ Title: 'Properties', Message: errText(e), Kind: 'net-error' }); return false; }
  const type = info.isLink ? `Symbolic link to a ${info.isDir ? 'folder' : 'file'}${info.broken ? ' (target missing)' : ''}` : info.isDir ? 'Folder' : 'File';
  const html = `<h2>Properties of ${esc(info.name)}</h2><table class="mc-tools-props">
    ${row('Type', esc(type))}
    ${info.linkTarget ? row('Points to', `<code>${esc(info.linkTarget)}</code>`) : ''}
    ${row('Server', esc(`${info.server} (${KIND_LABEL[info.kind] || info.kind})`))}
    ${row('Folder', esc(info.folder))}
    ${info.isDir ? '' : row('Size', esc(`${size(info.size)} (${Number(info.size).toLocaleString('en-US')} bytes)`))}
    ${info.mtime ? row('Modified', esc(date(info.mtime))) : ''}
    ${info.perms ? row('Permissions', `<code>${esc(info.perms)}</code> (${esc(info.octal)})`) : ''}
    ${info.uid >= 0 ? row('Owner / group', esc(`${info.uid} / ${info.gid}`)) : ''}
    </table>
    <div class="mc-btns">${info.canChmod ? '<button type="button" class="mc-net-attrs">Change attributes…</button>' : ''}<button type="submit" class="primary">Close</button></div>`;
  const after = await modal((box, done) => {
    box.classList.add('mc-info', 'mc-tools-propsbox');
    box.innerHTML = `<button type="button" class="mc-close" aria-label="Close" title="Close (Esc)">×</button>${html}`;
    box.querySelector('.mc-close').onclick = () => done();
    box.onsubmit = (ev) => { ev.preventDefault(); done(); };
    const attrs = box.querySelector('.mc-net-attrs');
    if (attrs) attrs.onclick = () => done('attrs');
    box.parentElement.addEventListener('mousedown', (ev) => { if (ev.target === ev.currentTarget) done(); });
  }, undefined, { kind: 'properties' });
  if (after === 'attrs') {
    if (items.length) await remoteAttrs(app);
    else app.flash('Select the files whose attributes you want to change.');
  }
  return true;
}

// remoteAttrs is Change attributes on an FTP or SFTP panel.
export async function remoteAttrs(app) {
  const p = app.cur;
  const items = p.targets();
  if (!items.length) { app.flash('Nothing is selected.'); return false; }
  const paths = items.map((e) => p.fullPath(e.name));
  let info;
  try { info = await go.Net.RemoteInfo(paths[0]); } catch (e) { await messageDialog({ Title: 'Change attributes', Message: errText(e), Kind: 'net-error' }); return false; }
  if (!info.canChmod) { app.flash('This server can\'t change attributes (WebDAV has no permissions).', 'err'); return false; }
  const start = bitsOfEntry(items[0].mode) ?? parseOctal(info.octal) ?? 0o644;
  const same = items.every((e) => bitsOfEntry(e.mode) === bitsOfEntry(items[0].mode));
  const folders = items.filter((e) => e.dir).length;
  const what = items.length === 1 ? items[0].name : `${plural(items.length, 'item')} (${plural(items.length - folders, 'file')}, ${plural(folders, 'folder')})`;
  const box = (bit, label = '') => `<label class="mc-check"><input type="checkbox" class="mc-net-perm" data-bit="${bit}"${start & bit ? ' checked' : ''}${label ? '' : ` aria-label="${esc(String(bit.toString(8)))}"`}>${label ? ` ${esc(label)}` : ''}</label>`;
  const html = `<p class="mc-note mc-tools-what">${esc(what)} on ${esc(info.server)}</p>
    <label class="mc-check"><input type="checkbox" id="setPerm"${same ? ' checked' : ''}> Change the permissions${same ? '' : ' (the selected items have different ones)'}</label>
    <fieldset class="mc-tools-group"><legend>Permissions</legend><table class="mc-tools-perm"><tr><th></th><th>Read</th><th>Write</th><th>Execute</th></tr>
    ${PERM_ROWS.map(([who, bits]) => `<tr><th>${who}</th>${bits.map((b) => `<td>${box(b)}</td>`).join('')}</tr>`).join('')}</table>
    <div class="mc-tools-special">${SPECIAL_BITS.map(([b, n]) => box(b, n)).join('')}</div>
    <div class="mc-row mc-tools-octal"><label for="octal">Octal</label><input type="text" id="octal" spellcheck="false" maxlength="4" value="${octalText(start)}"></div>
    <p class="mc-note" id="permText">${esc(permString(start))}</p></fieldset>
    <label class="mc-check"><input type="checkbox" id="recursive"${folders ? '' : ' disabled'}> Process subfolders</label>
    <div class="mc-row"><label for="applyTo">Change</label><select id="applyTo"><option value="">Files and folders</option><option value="files">Files only</option><option value="folders">Folders only</option></select></div>
    ${info.canChtimes ? `<label class="mc-check"><input type="checkbox" id="setDate"> Set the modification time</label>
      <input type="datetime-local" step="1" id="mtime" value="${esc(localInput(items[0].mtime || Date.now()))}">` : ''}`;
  const req = await formDialog('Change attributes', html, (b) => {
    const setPerm = b.querySelector('#setPerm').checked;
    const octal = b.querySelector('#octal').value.trim();
    if (setPerm && parseOctal(octal) === null) return 'Type the permissions as an octal number such as 644 or 0755.';
    let mtime = 0;
    if (b.querySelector('#setDate')?.checked) {
      mtime = new Date(b.querySelector('#mtime').value).getTime();
      if (!Number.isFinite(mtime) || mtime <= 0) return 'Type a valid date and time.';
    }
    if (!setPerm && !mtime) return 'Nothing to change: tick "Change the permissions" or "Set the modification time".';
    return { paths, octal: setPerm ? octal : '', recursive: b.querySelector('#recursive').checked, applyTo: b.querySelector('#applyTo').value, mtime };
  }, (b) => {
    const oct = b.querySelector('#octal');
    const text = b.querySelector('#permText');
    const fromBoxes = () => {
      let m = 0;
      for (const c of b.querySelectorAll('.mc-net-perm')) if (c.checked) m |= Number(c.dataset.bit);
      oct.value = octalText(m);
      text.textContent = permString(m);
      b.querySelector('#setPerm').checked = true;
    };
    for (const c of b.querySelectorAll('.mc-net-perm')) c.addEventListener('change', fromBoxes);
    oct.addEventListener('input', () => {
      const m = parseOctal(oct.value);
      if (m === null) return;
      for (const c of b.querySelectorAll('.mc-net-perm')) c.checked = !!(m & Number(c.dataset.bit));
      text.textContent = permString(m);
      b.querySelector('#setPerm').checked = true;
    });
  }, { okLabel: 'OK', kind: 'net-attrs' });
  if (!req) return false;
  let res;
  try { res = await go.Net.RemoteChmod(req); } catch (e) { await messageDialog({ Title: 'Change attributes', Message: errText(e), Kind: 'net-error' }); return false; }
  await p.reload();
  if (res.failed?.length) {
    const lines = res.failed.slice(0, 12).map((f) => `${f.path.split('/').pop()}: ${f.error}`);
    if (res.failed.length > 12) lines.push(`…and ${res.failed.length - 12} more.`);
    await messageDialog({ Title: 'Change attributes', Message: `${plural(res.done, 'item')} changed, ${res.failed.length} not:\n\n${lines.join('\n')}`, Kind: 'tools-failed' });
  } else app.flash(`Changed ${plural(res.done, 'item')}.`);
  return res;
}

// localInput is a time for a datetime-local field.
function localInput(ms) {
  const d = new Date(ms);
  const z = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${z(d.getMonth() + 1)}-${z(d.getDate())}T${z(d.getHours())}:${z(d.getMinutes())}:${z(d.getSeconds())}`;
}

// remoteSymlink is Create symbolic link on an SFTP panel: the link is made in the same server folder.
export async function remoteSymlink(app) {
  const p = app.cur;
  if (!/^(sftp|scp):\/\//i.test(p.path)) { app.flash('On servers, symbolic links can be made on SFTP servers only.', 'err'); return false; }
  const items = p.targets();
  const first = items[0];
  const values = await formDialog('Create symbolic link', `
    <div class="mc-row"><label for="link">New link (in this server folder)</label><input type="text" id="link" spellcheck="false" autofocus value="${esc(first ? `${first.name}.link` : '')}"></div>
    <div class="mc-row"><label for="target">Points to (a path on the server, relative to this folder or starting with /)</label><input type="text" id="target" spellcheck="false" value="${esc(first?.name || '')}"></div>`,
  (b) => {
    const link = b.querySelector('#link').value.trim(), target = b.querySelector('#target').value.trim();
    if (!link) return 'Type the name of the new link.';
    if (/[\\/]/.test(link) || link === '.' || link === '..') return 'The link name must be a plain name (it is made in this folder).';
    if (!target) return 'Type what the link points to.';
    return { link, target };
  }, null, { kind: 'net-symlink', okLabel: 'Create' });
  if (!values) return false;
  let made;
  try { made = await go.Net.RemoteSymlink(p.path, values.link, values.target); } catch (e) { await messageDialog({ Title: 'Create symbolic link', Message: errText(e), Kind: 'net-error' }); return false; }
  await p.reload();
  p.focusName?.(values.link);
  app.flash(`Made ${values.link}.`);
  return made;
}
