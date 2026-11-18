// The one-time note when a copy or move goes onto a file system that keeps no extended attributes (FAT, exFAT; board
// #29, F-NEW-13), or into a cloud-synced folder that may not keep them (iCloud Drive, ~/Library/CloudStorage): Finder
// tags, colour labels and other attributes of the sources are lost there. The check runs beside
// the copy (Tools.XattrLossCheck only looks) and shows a flash at most once per session, so the copy itself starts and
// runs exactly as before. Setting "tools" → xattrLossNote: false turns the note off. Not a feature of its own: it
// exports one function that features/fileops.js calls.

import { go } from '../core/go.js';
import { xattrLossNote, onceGate } from './tools/xattr-lib.js';

const once = onceGate();
let shown = false;

// noteXattrLoss checks, without waiting, whether copying sources to target loses extended attributes, and shows the
// note the first time it does.
export function noteXattrLoss(app, sources, target) {
  try {
    if (shown || app.demo || app.platform === 'windows' || !target || String(target).includes('://')) return;
    if (sources.some((s) => String(s).includes('://'))) return;
    if ((app.setting('tools', {}) || {}).xattrLossNote === false) return;
    if (!go.Tools?.XattrLossCheck) return;
    go.Tools.XattrLossCheck(sources, target).then((check) => {
      const note = xattrLossNote(check);
      if (note && once('xattr-loss')) {
        shown = true;
        app.flash(note);
      }
    }).catch(() => {});
  } catch { /* a note is never worth stopping a copy */ }
}
