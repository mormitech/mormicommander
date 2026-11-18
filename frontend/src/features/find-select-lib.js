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
