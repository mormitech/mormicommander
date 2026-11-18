// Compare folders (Shift+F2, Mark > Compare folders: cm_CompareDirs, cm_CompareDirsWithSubdirs; FEATURES.md F-133,
// F-134, F-212, F-351, F-352; behaviour from docs/spec area 3, 4.1): the selection of both panels is replaced by the
// files that are newer or missing on the other side (and both copies of files that differ at the same time). With
// subfolders, folders that exist on one side only are selected too, and so is a folder present on both sides when
// something inside it differs. "Compare folders, hide equal files" (ours) also hides the files that compared equal;
// Ctrl+F10 (all files), a reread, or a plain Compare folders shows them again. The comparing is internal/dircompare's
// (feature_dircmp.go).
//
// Plain Shift+F2 leaves folders alone, as it always did. "Compare folders, mark one-sided folders" (ours, Mark menu)
// also selects the folders that exist on one side only, without going into subfolders; the setting
// compareDirs.oneSidedFolders (default off) makes plain Shift+F2 do that too.
//
// Setting "compareDirs": {byContent, ignoreDate, ignoreHour, tolerance, oneSidedFolders} (tolerance in seconds; default
// 2 for FAT and network drives).

import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { messageDialog } from '../core/dialogs.js';
import { plural } from '../core/format.js';
import { isCompareFilter, compareSubdirs, hideEqualFilter } from './comparedirs-lib.js';

const entriesOf = (p) => p.all.filter((e) => !e.up && e.name !== '..').map((e) => ({ name: e.name, size: e.size || 0, mtime: e.mtime || 0, dir: !!e.dir, hidden: !!e.hidden }));

export async function compareDirs(app, { subdirs = '', hideEqual = false } = {}) {
  const L = app.left, R = app.right;
  const o = { byContent: false, ignoreDate: false, ignoreHour: false, tolerance: 2, oneSidedFolders: false, ...(app.setting('compareDirs', {}) || {}) };
  const req = {
    left: L.path, right: R.path, subdirs: compareSubdirs(subdirs, o), byContent: !!o.byContent, ignoreDate: !!o.ignoreDate, ignoreHour: !!o.ignoreHour,
    tolerance: Number(o.tolerance) || 0, includeHidden: !!app.showHidden, leftEntries: entriesOf(L), rightEntries: entriesOf(R),
  };
  const res = await go.CompareDirs.Compare(req).catch((e) => { throw new Error(errText(e)); });
  for (const [p, names, equal] of [[L, res.leftSelect, res.leftEqual], [R, res.rightSelect, res.rightEqual]]) {
    if (isCompareFilter(p.filter)) p.setFilter(null); // an earlier "hide equal files" belongs to the earlier compare
    const want = new Set(names);
    p.selected.clear();
    for (const e of p.entries) if (!e.up && want.has(e.name)) p.select(e, true);
    if (hideEqual) {
      const fn = hideEqualFilter(equal);
      p.setFilter(fn, fn ? 'Differing files' : '');
    }
    p.selectionChanged();
  }
  const nl = res.leftSelect.length, nr = res.rightSelect.length;
  if (res.errors.length) app.flash(`Some files could not be compared: ${res.errors.slice(0, 2).join('; ')}${res.errors.length > 2 ? ` …and ${res.errors.length - 2} more` : ''}`, 'err');
  if (!nl && !nr) {
    await messageDialog({ Title: 'Compare folders', Message: 'No differences found.', Kind: 'comparedirs' });
  } else if (!res.errors.length) {
    app.flash(`Selected ${plural(nl, 'entry', 'entries')} on the left and ${plural(nr, 'entry', 'entries')} on the right.`);
  }
  return { left: res.leftSelect, right: res.rightSelect, counts: res.counts };
}

command({ name: 'cm_CompareDirs', menuKey: true, label: 'Compare folders', run: (app) => compareDirs(app, { subdirs: '' }) });
command({ name: 'cm_CompareDirsWithSubdirs', label: 'Compare folders with subfolders', run: (app) => compareDirs(app, { subdirs: 'recursive' }) });
command({
  name: 'cm_CompareDirsHideEqual', label: 'Compare folders, hide equal files', menu: 'Mark', menuAfter: 'cm_CompareDirsWithSubdirs',
  run: (app) => compareDirs(app, { subdirs: '', hideEqual: true }),
});
command({
  name: 'cm_CompareDirsOneSidedFolders', label: 'Compare folders, mark one-sided folders', menu: 'Mark', menuAfter: 'cm_CompareDirsHideEqual',
  run: (app) => compareDirs(app, { subdirs: 'missing' }),
});
