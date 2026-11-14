// Actions on duplicate search results (board issue #32, F-862): the pure part. After a Find files search for duplicate
// files every result carries its group number (g); planDuplicates picks the one file to keep in each group by a rule
// (newest, oldest, or the first path A to Z) and lists all the others with the file they duplicate, so the dialog can
// mark them, move them to the trash or replace them with hard links. Folders and files found inside archives can't be
// acted on, so they never count; a group with fewer than two usable files is left out.

export const DUP_RULES = [
  ['newest', 'Keep the newest file'],
  ['oldest', 'Keep the oldest file'],
  ['first', 'Keep the first path (A to Z)'],
];

// comparePaths orders paths A to Z ignoring case, then exactly, so the choice never depends on the search order.
function comparePaths(a, b) {
  const x = a.toLowerCase(), y = b.toLowerCase();
  if (x !== y) return x < y ? -1 : 1;
  return a < b ? -1 : a > b ? 1 : 0;
}

// better tells whether result a should be kept rather than result b under the rule (ties go to the first path).
function better(rule, a, b) {
  if (rule === 'newest' && a.t !== b.t) return (a.t || 0) > (b.t || 0);
  if (rule === 'oldest' && a.t !== b.t) return (a.t || 0) < (b.t || 0);
  return comparePaths(a.p, b.p) < 0;
}

// planDuplicates plans the action for the results items (Find's {p, n, d, s, t, f, g, a} records).
// The answer: keep (indexes of the kept files), drop ([{i, keep}] — the index of every other copy and of the file kept
// in its place, in list order), groups (how many groups take part) and bytes (the space the copies take up).
export function planDuplicates(items, rule = 'newest') {
  const groups = new Map();
  (items || []).forEach((it, i) => {
    if (!it || !it.g || it.f || it.a) return;
    if (!groups.has(it.g)) groups.set(it.g, []);
    groups.get(it.g).push(i);
  });
  const keep = [], drop = [];
  let bytes = 0, used = 0;
  for (const idx of groups.values()) {
    if (idx.length < 2) continue;
    used++;
    let k = idx[0];
    for (const i of idx.slice(1)) if (better(rule, items[i], items[k])) k = i;
    keep.push(k);
    for (const i of idx) if (i !== k) { drop.push({ i, keep: k }); bytes += items[i].s || 0; }
  }
  keep.sort((a, b) => a - b);
  drop.sort((a, b) => a.i - b.i);
  return { keep, drop, groups: used, bytes };
}

// hasDuplicates tells whether the Duplicates… button has anything to work on.
export function hasDuplicates(items) {
  return planDuplicates(items).drop.length > 0;
}

// dropGone removes the results whose paths are gone and renumbers nothing: groups keep their numbers, so the list
// still shows which files belonged together. Returns the new list.
export function dropGone(items, gonePaths) {
  const gone = new Set(gonePaths || []);
  return (items || []).filter((it) => !gone.has(it.p));
}
