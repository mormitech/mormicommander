// Internal associations (Files > Internal associations…, cm_InternalAssociate, F-323): file masks mapped to programs or
// cm_* commands inside mormiCommander, independent of the system. The setting "internalAssociations" holds one
// "mask list = command" per line ("*.txt *.md = code --wait "%1"", "*.log = cm_List"); Enter on a matching file uses the
// first matching line before the system's app (system.js asks openAssociated first). Go parses and runs it
// (SystemLaunch.OpenInternal over internal/launch: TC masks, %1 and the other placeholders). Empty, the default, leaves
// Enter exactly as it was, and then Go is not even asked.

import { command } from '../core/registry.js';
import { go } from '../core/go.js';
import { formDialog, esc } from '../core/dialogs.js';
import { realCommand } from './config/run.js';

export const ASSOC_KEY = 'internalAssociations';

const assocText = (app) => String(app.setting(ASSOC_KEY, '') || '');

// openAssociated applies the internal associations to Enter on a file; true when one matched and was carried out
// (errors are thrown to the caller, which shows them like any other open error).
export async function openAssociated(app, e, path, dir) {
  if (!assocText(app).trim()) return false;
  const a = await go.SystemLaunch.OpenInternal(path, dir);
  if (!a?.handled) return false;
  if (a.internal) await app.run(realCommand(a.internal), a.args ? { params: a.args } : undefined);
  else app.flash(`Opened ${e.name}.`);
  return true;
}

// internalAssociations is the dialog: the lines in a text box, saved as they are (only blank text clears them).
export async function internalAssociations(app) {
  const text = assocText(app);
  const r = await formDialog('Internal associations',
    `<p class="mc-note">One association per line: file masks, "=", then a program with its parameters (%1 is the file) or an internal command. Enter on a matching file uses it instead of the system's app; the first matching line wins. Lines starting with # are notes.</p>
    <div class="mc-row"><label for="assoc">Associations</label><textarea id="assoc" rows="8" spellcheck="false" placeholder="*.txt *.md = code --wait &quot;%1&quot;&#10;*.log = cm_List">${esc(text)}</textarea></div>`,
    (box) => ({ text: box.querySelector('#assoc').value }), null, { kind: 'internal-associations' });
  if (r === null) return false;
  const v = r.text.trim() ? r.text : '';
  await app.setSetting(ASSOC_KEY, v);
  app.flash(v ? 'Internal associations saved.' : 'No internal associations: Enter uses the system\'s apps.');
  return true;
}

command({ name: 'cm_InternalAssociate', label: 'Internal associations…', run: (app) => internalAssociations(app) });
