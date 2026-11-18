// Pure parts of the trash commands (FEATURES.md section 21), kept apart so they can be tested without a page.
//
// noTrashReason (F-833): whether a drive has no trash the system can use, from the volume list the drive bar has
// (Go's internal/disk kinds). Network shares have none on any system (macOS Finder and the Windows shell delete there
// at once, Linux trash tools refuse); USB sticks and other removable drives have none on Windows only (macOS and Linux
// make a trash folder on them). Only read when Options > Copy/Delete "Say when a drive has no trash" is on.
//
// restoreReport sums up a put-back from the trash (F-501, cm_RestoreFromTrash).

import { plural } from '../core/format.js';

// noTrashReason says why the drive of path has no trash ('' when it has one, or that is not known).
export function noTrashReason(platform, volume, path = '') {
  if (platform === 'windows' && /^[\\/]{2}[^\\/?.]/.test(path)) return 'it is a network share';
  if (!volume) return '';
  if (volume.kind === 'network') return 'it is a network share';
  if (volume.kind === 'removable' && platform === 'windows') return 'it is a removable drive';
  return '';
}

// restoreReport turns go.Jobs.RestoreFromTrash results into a line for the status flash.
export function restoreReport(results, T = 'trash') {
  const list = results || [];
  const done = list.filter((r) => !r.error).length;
  const failed = list.filter((r) => r.error);
  if (!failed.length) return { text: `${plural(done, 'item')} put back from the ${T}.`, err: false };
  const why = failed.length === 1 ? `${base(failed[0].path)}: ${failed[0].error}` : `${plural(failed.length, 'item')} stayed in the ${T}.`;
  return { text: done ? `${plural(done, 'item')} put back. ${why}` : `Nothing was put back. ${why}`, err: true };
}

const base = (p) => String(p || '').split(/[\\/]/).filter(Boolean).pop() || p;
