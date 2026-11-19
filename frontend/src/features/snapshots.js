// Versions from snapshots (board #34, F-874): Files > Versions from snapshots… (cm_SnapshotVersions) lists the copies
// of the file or folder under the cursor (the panel's folder on "..") that the file system's snapshots keep — ZFS,
// Btrfs snapper, macOS APFS local snapshots and Time Machine backups, Windows shadow copies — and opens the chosen one
// in the other panel, where it can be compared or copied back with the usual commands. Snapshots are only read:
// nothing here creates, deletes, mounts or restores one, and fileops refuses writing inside one (snapshotRefusal).
// Not in the AI tool allow-list.

import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { formDialog, messageDialog, esc } from '../core/dialogs.js';
import { date, size } from '../core/format.js';
import { versionRows, openTarget } from './snapshots-lib.js';

async function snapshotVersions(app) {
  const src = app.cur;
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(src.path || '')) { app.flash('Snapshots are only kept for files and folders on this computer\'s disks.', 'err'); return false; }
  const e = src.cur;
  const target = !e || e.up ? src.path : src.fullPath(e.name);
  app.flash('Looking for snapshots…');
  let r;
  try { r = await go.FS.SnapshotVersions(target); } catch (err) { app.flash(errText(err), 'err'); return false; }
  const notes = (r?.notes || []).join('\n');
  const versions = r?.versions || [];
  if (!versions.length) {
    await messageDialog({ Title: 'Versions from snapshots', Message: `No snapshot holds ${r?.live || target}.${notes ? `\n\n${notes}` : ''}`, Kind: 'snapshots' });
    return false;
  }
  const rows = versionRows(versions, date, size);
  const first = rows.findIndex((x) => !x.disabled);
  const opts = rows.map((x, i) => `<option value="${i}"${x.disabled ? ' disabled' : ''}${i === first ? ' selected' : ''} title="${esc(x.title)}">${esc(x.label)}</option>`).join('');
  const html = `<p class="mc-msg">${esc(r.live)}</p>
    <div class="mc-row"><select id="snap-list" size="${Math.min(12, Math.max(4, rows.length))}" autofocus style="width:100%;min-width:32em">${opts}</select></div>
    <p class="mc-note">The chosen version opens in the other panel, read-only: compare it or copy it back from there.${notes ? `<br>${esc(notes).replace(/\n/g, '<br>')}` : ''}</p>`;
  const pick = await formDialog('Versions from snapshots', html, (box) => {
    const i = Number(box.querySelector('#snap-list').value);
    if (!Number.isInteger(i) || !rows[i] || rows[i].disabled) return 'Choose a version that can be opened.';
    return versions[i];
  }, (box, done) => {
    const sel = box.querySelector('#snap-list');
    sel.addEventListener('dblclick', () => { const i = Number(sel.value); if (rows[i] && !rows[i].disabled) done(versions[i]); });
  }, { okLabel: 'Open in other panel', kind: 'snapshots' });
  if (!pick) return false;
  const t = openTarget(pick, app.platform === 'windows' ? '\\' : '/');
  return app.other.go(t.dir, { focus: t.focus });
}

command({ name: 'cm_SnapshotVersions', label: 'Versions from snapshots…', menu: 'Files', menuAfter: 'cm_RestoreFromTrash', run: (app) => snapshotVersions(app) });
