// Sorting a panel. The ".." line always comes first and folders before files (Total Commander's default). Folders are
// always sorted by name (docs/spec area 2, 5.3/5.4); equal sizes and dates fall back to the name, ascending, whatever the
// direction. Pure, unit-tested (test/sort.test.js).
//
// sort = {key: 'name' | 'ext' | 'size' | 'date' | 'none', desc: bool}. The keys that start newest/largest first (date,
// size) are handled by the commands that pick them (desc: true), not here.

const collators = new Map();
function collator(natural) {
  const k = natural ? 'n' : 'p';
  if (!collators.has(k)) collators.set(k, new Intl.Collator(undefined, { sensitivity: 'base', numeric: !!natural }));
  return collators.get(k);
}

export function sortEntries(entries, sort = { key: 'name', desc: false }, opts = {}) {
  const up = entries.filter((e) => e.up);
  const dirs = entries.filter((e) => !e.up && e.dir);
  const files = entries.filter((e) => !e.up && !e.dir);
  if (sort.key === 'none') return [...up, ...dirs, ...files];
  const col = collator(opts.natural);
  const byName = (a, b) => col.compare(a.name, b.name) || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0);
  const sign = sort.desc ? -1 : 1;
  let cmp;
  switch (sort.key) {
    case 'ext': cmp = (a, b) => col.compare(a.ext, b.ext) || byName(a, b); break;
    case 'size': cmp = (a, b) => (a.size - b.size) || sign * byName(a, b); break; // equal sizes stay in name order either way
    case 'date': cmp = (a, b) => (a.mtime - b.mtime) || sign * byName(a, b); break;
    default: cmp = byName;
  }
  const fileCmp = (a, b) => sign * cmp(a, b);
  // folders: always by name (TC's default "SortDirsByName"), reversed with a reversed name sort; dirsLikeFiles sorts
  // them by date like files (sizes of folders are unknown until calculated)
  const dirCmp = sort.key === 'name' || (opts.dirsLikeFiles && sort.key === 'date') ? fileCmp : byName;
  dirs.sort(dirCmp);
  files.sort(fileCmp);
  return [...up, ...dirs, ...files];
}
