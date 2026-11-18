// Mark > Select search results (cm_SelectSearchResults, FEATURES.md F-353): the pure part. The last Find files search
// (its results stay after the dialog closes, until the next search) gives full paths; the panel's entries whose full
// path is one of them are the ones to select. Windows and macOS paths compare ignoring case, as those file systems do.

const norm = (p, fold) => {
  const s = String(p || '').replace(/[\\/]+$/, '');
  return fold ? s.toLowerCase() : s;
};

// searchMatches returns the entries (not ".." ) whose fullPath(name) is among paths. fold: compare ignoring case.
export function searchMatches(entries, fullPath, paths, fold = false) {
  const want = new Set((paths || []).map((p) => norm(p, fold)));
  if (!want.size) return [];
  return entries.filter((e) => !e.up && e.name !== '..' && want.has(norm(fullPath(e.name), fold)));
}

// templateName (F-537): a saved Find files search used as a filter. The mask ">name" names it when name is one of the
// saved searches and not a saved filter of Select files (those keep their meaning); names compare ignoring case. It
// answers the saved search's name as stored, or '' when the mask is something else.
export function templateName(mask, filterNames, searchNames) {
  const m = /^\s*>\s*(.+?)\s*$/.exec(String(mask || ''));
  if (!m) return '';
  const want = m[1].toLowerCase();
  if ((filterNames || []).some((n) => String(n).toLowerCase() === want)) return '';
  return (searchNames || []).find((n) => String(n).toLowerCase() === want) || '';
}

// kindIndexes keeps the indexes whose entry fits the group kind: "files", "folders" or "both".
export function kindIndexes(entries, idx, kind) {
  return (idx || []).filter((i) => {
    const e = entries[i];
    if (!e || e.up) return false;
    return kind === 'both' || (kind === 'folders' ? !!e.dir : !e.dir);
  });
}
