// Undo of file operations (board issue #13, F-NEW-2; ours, Total Commander has none). Go keeps the journal
// (feature_undo.go, internal/undo): renames in place, new folders, copies and moves on this computer's disks. Here are
// the commands: cm_UndoFileOp takes the newest operation back (Cmd+Z on macOS; no default key elsewhere, where TC's
// Ctrl+Z edits the comment), and pressing it again goes one further back; cm_UndoList lists what can be undone and
// takes back the one chosen. Each undo first asks, saying what will happen (copies go to the trash) and what will be
// left alone (an item that was changed or moved since). The setting undoJournal false stops the recording.

import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { messageDialog } from '../core/dialogs.js';
import { showList } from '../core/popup.js';
import { runJob } from '../core/jobs.js';
import { date } from '../core/format.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { undoReport, undoListItems } from './undo-lib.js';

// undo takes back the operation id (0: the newest). args.confirm false skips the question (scripts).
async function undo(app, id = 0, { confirm = true } = {}) {
  let pv;
  try { pv = await go.Undo.Preview(id); } catch (e) { app.flash(errText(e), 'err'); return false; }
  if (!pv?.id) { app.flash('There is nothing to undo.'); return false; }
  if (!pv.steps) {
    // nothing of it can be put back any more: say why, and take it off the list so the next undo goes further back
    if (confirm) await messageDialog({ Title: 'Undo', Message: `${pv.question}\n\nIt was taken off the undo list.`, Kind: 'undo' });
    else app.flash(pv.question, 'err');
    await go.Undo.Run(pv.id).catch(() => {});
    return false;
  }
  if (confirm) {
    const b = await messageDialog({ Title: 'Undo', Message: pv.question, Buttons: ['Undo', 'Cancel'], DefaultButton: 'Undo', Kind: 'undo' });
    if (b !== 'Undo') return false;
  }
  let res;
  try { res = await go.Undo.Run(pv.id); } catch (e) { app.flash(errText(e), 'err'); return false; }
  const ends = [];
  for (const j of res.jobs || []) {
    try { ends.push(await runJob(j.request, { title: j.title })); } catch (e) { ends.push({ state: 'failed', errors: [errText(e)] }); }
  }
  if (!(res.jobs || []).length) await Promise.all(app.panels.map((p) => p.reload())); // (runJob rereads them itself)
  const r = undoReport(res, ends);
  app.flash(r.text, r.err ? 'err' : undefined);
  return !r.err;
}

command({
  name: 'cm_UndoFileOp',
  label: 'Undo last file operation…',
  short: 'Undo',
  menu: 'Files',
  menuAfter: 'cm_Wipe',
  run: (app, args) => undo(app, Number(args?.id) || 0, { confirm: args?.confirm !== false }),
});

command({
  name: 'cm_UndoList',
  label: 'Undo list…',
  menu: 'Files',
  menuAfter: 'cm_UndoFileOp',
  run: async (app) => {
    const list = await go.Undo.List().catch(() => []);
    if (!list?.length) { app.flash('There is nothing to undo.'); return false; }
    const r = await showList({ title: 'Undo', items: undoListItems(list, date), anchor: app.cur?.$.pathbar, kind: 'undo-list' });
    if (!r) return false;
    return undo(app, r.item.value);
  },
});

// control mode: {"cmd": "undo"} undoes the newest operation without asking; {"cmd": "undo", "id": n} a given one
addControlCommand('undo', async (m) => {
  const ok = await undo(theApp, Number(m?.id) || 0, { confirm: false });
  return { ok };
});
