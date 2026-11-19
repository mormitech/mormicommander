// Remove quarantine flag (cm_RemoveQuarantine, ours; board #29, F-NEW-13): clears macOS's "downloaded from the
// internet" flag (com.apple.quarantine) from the selected files and from everything inside the selected folders, so
// Gatekeeper stops asking before they open. It is in the Files menu after "Change attributes…" and works on macOS
// only; the Properties dialog offers the same for one file that has the flag, and "Remove attribute…" there removes
// any one extended attribute of the file. Both are security-relevant and can't be undone, so both ask first and name
// the files; neither is on the AI tool allow-list (aitools-allow.js). The work is Tools.RemoveQuarantine and
// Tools.RemoveXattr (feature_tools_xattr.go, internal/xattr).

import { command } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { messageDialog, formDialog } from '../../core/dialogs.js';
import { runTool, needLocal, targets, showError, failureList, deselect, esc } from './common.js';
import { quarantineMessage, quarantineQuestion, removeAttrQuestion, xattrLines } from './xattr-lib.js';
import { tm } from '../../core/i18n.js';

// ask shows a question with a "Remove" button and answers whether the user chose it.
async function ask(title, message) {
  const b = await messageDialog({ Title: title, Message: message, Buttons: ['Remove', 'Cancel'], Danger: true, Kind: 'tools-xattr-confirm' });
  return b === 'Remove';
}

// removeQuarantine asks first (naming the files), clears the flag from paths and tells the user what happened. It
// answers the result, or null when the user said no or it failed. anyDir: a folder is among the paths.
export async function removeQuarantine(app, paths, anyDir = false) {
  if (!paths?.length) return null;
  if (!(await ask('Remove quarantine flag', quarantineQuestion(paths, anyDir)))) return null;
  let r;
  try { r = await runTool('Removing the quarantine flag', (id) => go.Tools.RemoveQuarantine(id, paths)); } catch (e) { await showError(e.message); return null; }
  if (r.failed?.length) await failureList('Remove quarantine flag', r.failed, quarantineMessage(r));
  else app.flash(quarantineMessage(r));
  return r;
}

// removeAttribute lets the user pick one of a file's extended attributes ({name, size} list), asks again naming the
// file and the attribute, and removes it. It answers the name removed, or ''.
export async function removeAttribute(app, path, list) {
  const attrs = (list || []).filter((a) => a?.name);
  if (!attrs.length) return '';
  const lines = xattrLines(attrs);
  const options = attrs.map((a, i) => `<option value="${i}">${esc(lines[i])}</option>`).join('');
  const pick = await formDialog('Remove extended attribute',
    `<div class="mc-row"><label for="xa-name">Attribute</label><select id="xa-name">${options}</select></div>`,
    (box) => attrs[+box.querySelector('#xa-name').value]?.name || 'Choose an attribute.', null, { okLabel: 'Remove…', kind: 'tools-xattr-pick' });
  if (!pick) return '';
  if (!(await ask('Remove extended attribute', removeAttrQuestion(path, pick)))) return '';
  try {
    const ok = await go.Tools.RemoveXattr(path, pick);
    app.flash(ok ? tm('Removed {name}.', { name: pick }) : tm('It had no {name} any more.', { name: pick }));
    return pick;
  } catch (e) { await showError(String(e?.message ?? e)); return ''; }
}

command({
  name: 'cm_RemoveQuarantine', label: 'Remove quarantine flag…', menu: 'Files', menuAfter: 'cm_SetAttrib',
  enabled: (app) => !app.demo && app.platform === 'darwin',
  run: async (app) => {
    if (app.platform !== 'darwin') { app.flash('The quarantine flag is a macOS thing: there is none here.'); return; }
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    if (!paths.length) { app.flash('Nothing is selected.'); return; }
    const r = await removeQuarantine(app, paths, entries.some((e) => e.dir));
    if (r && !r.failed?.length && !r.canceled) deselect(p, entries.map((e) => e.name));
  },
});
