// Synchronize folders' pure parts (F-555): what the window says about a side that is an archive or a server folder.
// The Go side tells for each side what kind of place it is ({kind, readOnly, why, trash, times}, see SyncSideInfo in
// feature_syncdirs_vfs.go); these functions turn that into the notes of the window and of the Synchronize dialog, so
// they can be tested without a page.

const KIND_NAMES = { archive: 'an archive', ftp: 'an FTP server', sftp: 'an SFTP server', other: 'a server' };

// isVirtual says whether a side is an archive or a server folder (not a folder on this computer).
export const isVirtual = (side) => !!side && side.kind !== 'local';

// readOnlyNote is the line the window shows after a compare when a side can't be changed, or ''.
export function readOnlyNote(sides) {
  const out = [];
  for (const s of ['left', 'right']) {
    const i = sides?.[s];
    if (i?.readOnly) out.push(`The ${s} side can't be changed, so nothing is copied to it or deleted on it: ${i.why || 'it is read-only.'}`);
  }
  return out.join(' ');
}

// dialogNotes are the extra lines of the Synchronize dialog: read-only sides, deletes that can't go to the trash, and
// sides that can't keep file dates.
export function dialogNotes(plan, sides, trash) {
  const notes = [];
  const del = { left: plan?.deletesLeft || 0, right: plan?.deletesRight || 0 };
  const copies = { left: plan?.filesRightToLeft || 0, right: plan?.filesLeftToRight || 0 };
  for (const s of ['left', 'right']) {
    const i = sides?.[s];
    if (!i) continue;
    if (i.readOnly) {
      notes.push(`The ${s} side is read-only: ${i.why || 'nothing can be written there.'}`);
      continue;
    }
    if (!isVirtual(i)) continue;
    const where = KIND_NAMES[i.kind] || 'a server';
    // With the trash off every delete is permanent anyway, on any side, so only the trash case needs a word.
    if (del[s] && trash) notes.push(`Deleting on the ${s} side (${where}) is permanent: it has no trash.`);
    if (copies[s] && !i.times) notes.push(`The ${s} side can't set file dates: copied files get the time of the copy, so compare them by content afterwards.`);
  }
  return notes;
}

// contentTip is the tooltip of the "By content" checkbox: on a server or in an archive the files are read for it.
export function contentTip(left, right) {
  const remote = [left, right].some((p) => /^[a-z][a-z0-9+.-]*:\/\//i.test(String(p || '').trim()));
  return 'Compare files of the same size byte by byte' + (remote ? ' (files on a server are downloaded for this, which can take long)' : '');
}
