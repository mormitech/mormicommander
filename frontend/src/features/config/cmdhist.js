// The command line history a Total Commander import brings over (F-295). The import keeps TC's [Command line history]
// in mormicommander.ini, but the command line shows its own history, kept in settings.json ("cmdHistory"). So the lines
// of the ini file are merged into that history: once each (the ones merged before are remembered in
// "cmdHistoryImported", so a line the history has dropped since does not come back at every start), after the app's
// own lines, without doubles, within the history size. With "Remember command line history" off nothing is merged.

import { go } from '../../core/go.js';
import { cmdHistoryMax } from '../../core/cmdline.js';

// mergeCmdHistory adds the imported lines not merged before (not in seen) after the own history, without doubles, cut
// to max. It returns the new history and the new seen list (the imported lines as they are now), or null when there
// is nothing new.
export function mergeCmdHistory(own, imported, seen, max) {
  own = Array.isArray(own) ? own : [];
  const was = new Set(Array.isArray(seen) ? seen : []);
  const fresh = (Array.isArray(imported) ? imported : []).filter((l) => typeof l === 'string' && l.trim() && !was.has(l));
  if (!fresh.length) return null;
  const history = [...own];
  for (const l of fresh) if (!history.includes(l)) history.push(l);
  return { history: history.slice(0, max), seen: imported.slice(0, 500) };
}

// importCmdHistory merges the ini file's command line history into the page's (at start and after the settings
// change, an import among them).
export async function importCmdHistory(app) {
  if (app.demo || app.config?.values?.['Operation.SaveCommands'] === '0') return;
  let lines;
  try { lines = await go.Configuration.CmdHistory(); } catch { return; }
  const r = mergeCmdHistory(app.setting('cmdHistory', []), lines || [], app.setting('cmdHistoryImported', []),
    cmdHistoryMax(app.setting('cmdHistoryLength', 50)));
  if (!r) return;
  app.setSetting('cmdHistory', r.history);
  app.setSetting('cmdHistoryImported', r.seen);
}
