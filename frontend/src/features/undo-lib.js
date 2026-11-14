// The words of the undo of file operations (board issue #13), kept apart from the command so they can be tested
// without a page: what the flash line says after an undo, from what Go did at once (UndoResult) and how the file jobs
// that finished it ended.

// undoReport is {text, err}: one or two sentences for app.flash, err true when something was left alone or failed.
//   res:  {what, done, jobs, problems} from go.Undo.Run
//   ends: the final Progress of every job run for it ({state, errors})
export function undoReport(res, ends = []) {
  const what = res?.what || 'the last operation';
  const problems = [...(res?.problems || [])];
  for (const p of ends) {
    if (p?.state === 'failed') { problems.push(...(p.errors || []).slice(0, 3)); }
    if (p?.state === 'cancelled') problems.push('The undo was stopped before it was finished.');
  }
  const did = (res?.done || 0) > 0 || ends.some((p) => p?.state === 'done' || p?.state === 'failed');
  const head = did ? `Undid ${what}.` : `Nothing of ${what} could be undone.`;
  if (!problems.length) return { text: head, err: false };
  const shown = problems.slice(0, 3).join('\n');
  const more = problems.length > 3 ? `\n…and ${problems.length - 3} more.` : '';
  return { text: `${head}\n${shown}${more}`, err: true };
}

// undoListItems turns go.Undo.List() into items for showList, newest first, with the time as the detail.
export function undoListItems(list, fmtDate = (ms) => new Date(ms).toLocaleString()) {
  return (list || []).map((e) => ({ label: e.what.charAt(0).toUpperCase() + e.what.slice(1), detail: fmtDate(e.time), value: e.id }));
}

// overwriteNote is the line the copy/move dialog shows under "When a file is already there" when the choice replaces
// files without asking (Overwrite all, Overwrite all older): the undo journal can't take a replaced file back, so the
// dialog says so, as the overwrite question does (core/jobs.js). '' for the other choices, and with the journal off,
// where there is no undo to fall short of.
export function overwriteNote(policy, journalOn = true) {
  if (!journalOn) return '';
  return policy === 'overwrite' || policy === 'older' ? 'Files replaced this way can\'t be undone.' : '';
}
