// Sorting a panel. The ".." line always comes first and folders before files (Total Commander's default). Folders are
// always sorted by name (docs/spec area 2, 5.3/5.4); equal sizes and dates fall back to the name, ascending, whatever the
// direction. Pure, unit-tested (test/sort.test.js).
//
// sort = {key: 'name' | 'ext' | 'size' | 'date' | 'none', desc: bool, then?: {key, desc}}. The keys that start
// newest/largest first (date, size) are handled by the commands that pick them (desc: true), not here. "then" is the
// second sort key (F-114, Ctrl+click on a column title): files equal by the first key are sorted by it, then by name.

// opts.method picks how names are compared (settings key "sortMethod", F-116; the default is the order the app always had):
//   ''     alphabetical by the language rules (Intl.Collator), upper and lower case alike: "Ágnes" sits with "Agota";
//   'case' the same, but upper and lower case apart, upper case first for the same letter ("Apple" < "apple" < "Banana");
//   'code' by character code (Unicode code points, no language rules, so "Ä" comes after "Z"), case ignored.
// opts.locale is a BCP 47 language ("hu", "de", "sv"; settings key "sortLocale") for the language rules; empty means
// the system's language. Names are compared in Unicode NFC form (F-754), so a name macOS Finder made in decomposed form
// (NFD, "a" + combining acute) sorts exactly like the same name typed in composed form.

// SORT_METHODS lists the methods for a dialog: [value, label].
export const SORT_METHODS = [
  ['', 'Alphabetical, by the language rules'],
  ['case', 'Alphabetical, upper and lower case apart (upper case first)'],
  ['code', 'By character code (Unicode order)'],
];

// validLocale tells whether s is a language Intl knows how to sort by ("hu", "de-CH").
export function validLocale(s) {
  try { return Intl.Collator.supportedLocalesOf([s]).length > 0; } catch { return false; }
}

const collators = new Map();
function collator(natural, method = '', locale = '') {
  const k = `${natural ? 'n' : 'p'}|${method}|${locale}`;
  if (!collators.has(k)) {
    const o = method === 'case' ? { sensitivity: 'variant', caseFirst: 'upper', numeric: !!natural } : { sensitivity: 'base', numeric: !!natural };
    let c;
    try { c = new Intl.Collator(locale || undefined, o); } catch { c = new Intl.Collator(undefined, o); } // a mistyped language: the system's
    collators.set(k, c);
  }
  return collators.get(k);
}

const isDigit = (c) => c >= 48 && c <= 57;
const fold = (s) => s.normalize('NFC').toLowerCase();

// compareFolded compares two already folded strings (fold) code point by code point; with natural, runs of digits
// compare by value ("file2" < "file10"), leading zeros only breaking ties. It walks the strings without copying them,
// so sorting a folder of 100,000 names stays quick.
function compareFolded(x, y, natural) {
  let i = 0, j = 0, tie = 0;
  while (i < x.length && j < y.length) {
    const ca = x.codePointAt(i), cb = y.codePointAt(j);
    if (natural && isDigit(ca) && isDigit(cb)) {
      let i2 = i, j2 = j;
      while (i2 < x.length && isDigit(x.charCodeAt(i2))) i2++;
      while (j2 < y.length && isDigit(y.charCodeAt(j2))) j2++;
      let za = i, zb = j; // skip leading zeros
      while (za < i2 - 1 && x.charCodeAt(za) === 48) za++;
      while (zb < j2 - 1 && y.charCodeAt(zb) === 48) zb++;
      const la = i2 - za, lb = j2 - zb;
      if (la !== lb) return la < lb ? -1 : 1;
      for (let k = 0; k < la; k++) {
        const d = x.charCodeAt(za + k) - y.charCodeAt(zb + k);
        if (d) return d < 0 ? -1 : 1;
      }
      if (!tie && i2 - i !== j2 - j) tie = i2 - i < j2 - j ? -1 : 1;
      i = i2; j = j2;
      continue;
    }
    if (ca !== cb) return ca < cb ? -1 : 1;
    i += ca > 0xffff ? 2 : 1;
    j += cb > 0xffff ? 2 : 1;
  }
  if (i < x.length) return 1;
  if (j < y.length) return -1;
  return tie;
}

// codeCompare compares two names by character code with case ignored (method 'code').
export function codeCompare(a, b, natural = false) { return compareFolded(fold(a), fold(b), natural); }

// cached wraps a string transform with a memo, kept for one sort.
function cached(fn) {
  const m = new Map();
  return (s) => { let v = m.get(s); if (v === undefined) { v = fn(s); m.set(s, v); } return v; };
}

// nameComparer returns the compare function for names under opts (natural, method, locale). Build one per sort: it
// remembers the normalised form of each name it has seen.
export function nameComparer(opts = {}) {
  const method = opts.method || '';
  if (method === 'code') {
    const key = cached(fold);
    return (a, b) => compareFolded(key(a), key(b), !!opts.natural);
  }
  const col = collator(opts.natural, method, opts.locale || '');
  if (!method && !opts.locale) return col.compare; // the app's original order, unchanged
  const key = cached((s) => s.normalize('NFC'));
  return (a, b) => col.compare(key(a), key(b));
}

export function sortEntries(entries, sort = { key: 'name', desc: false }, opts = {}) {
  const up = entries.filter((e) => e.up);
  const dirs = entries.filter((e) => !e.up && e.dir);
  const files = entries.filter((e) => !e.up && !e.dir);
  if (sort.key === 'none') return [...up, ...dirs, ...files];
  const col = { compare: nameComparer(opts) };
  const byName = (a, b) => col.compare(a.name, b.name) || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0);
  const sign = sort.desc ? -1 : 1;
  let cmp;
  switch (sort.key) {
    case 'ext': cmp = (a, b) => col.compare(a.ext, b.ext) || byName(a, b); break;
    case 'size': cmp = (a, b) => (a.size - b.size) || sign * byName(a, b); break; // equal sizes stay in name order either way
    case 'date': cmp = (a, b) => (a.mtime - b.mtime) || sign * byName(a, b); break;
    default: cmp = byName;
  }
  let fileCmp = (a, b) => sign * cmp(a, b);
  const raw = { name: byName, ext: (a, b) => col.compare(a.ext, b.ext), size: (a, b) => a.size - b.size, date: (a, b) => a.mtime - b.mtime };
  const then = sort.then;
  if (then && raw[sort.key] && raw[then.key] && then.key !== sort.key) {
    const tsign = then.desc ? -1 : 1;
    fileCmp = (a, b) => sign * raw[sort.key](a, b) || tsign * raw[then.key](a, b) || byName(a, b);
  }
  // folders: always by name (TC's default "SortDirsByName"), reversed with a reversed name sort; dirsLikeFiles sorts
  // them by date like files (sizes of folders are unknown until calculated)
  const dirCmp = sort.key === 'name' || (opts.dirsLikeFiles && sort.key === 'date') ? fileCmp : byName;
  dirs.sort(dirCmp);
  files.sort(fileCmp);
  return [...up, ...dirs, ...files];
}
