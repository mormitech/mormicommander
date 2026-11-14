// Remove quarantine flag (cm_RemoveQuarantine, ours; board #29, F-NEW-13): clears macOS's "downloaded from the
// internet" flag (com.apple.quarantine) from the selected files and from everything inside the selected folders, so
// Gatekeeper stops asking before they open. It is in the Files menu after "Change attributes…" and works on macOS
// only; the Properties dialog offers the same for one file that has the flag. The work is Tools.RemoveQuarantine
// (feature_tools_xattr.go, internal/xattr).

import { command } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { runTool, needLocal, targets, showError, failureList, deselect } from './common.js';
import { quarantineMessage } from './xattr-lib.js';

// removeQuarantine clears the flag from paths and tells the user what happened. It answers the result, or null.
export async function removeQuarantine(app, paths) {
  let r;
  try { r = await runTool('Removing the quarantine flag', (id) => go.Tools.RemoveQuarantine(id, paths)); } catch (e) { await showError(e.message); return null; }
  if (r.failed?.length) await failureList('Remove quarantine flag', r.failed, quarantineMessage(r));
  else app.flash(quarantineMessage(r));
  return r;
}

command({
  name: 'cm_RemoveQuarantine', label: 'Remove quarantine flag', menu: 'Files', menuAfter: 'cm_SetAttrib',
  enabled: (app) => !app.demo && app.platform === 'darwin',
  run: async (app) => {
    if (app.platform !== 'darwin') { app.flash('The quarantine flag is a macOS thing: there is none here.'); return; }
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    if (!paths.length) { app.flash('Nothing is selected.'); return; }
    const r = await removeQuarantine(app, paths);
    if (r && !r.failed?.length && !r.canceled) deselect(p, entries.map((e) => e.name));
  },
});
