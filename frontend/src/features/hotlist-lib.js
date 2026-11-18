// Pure helpers of the folder hotlist (features/hotlist.js), kept free of the DOM and of Go so vitest can run them in
// Node (board issue #111). hotlistMaskFilter is the panel filter of a hotlist entry with a mask ("cd /folder/*.txt"):
// it tests each name against the mask when the list is drawn, so files that appear later (an automatic refresh, a
// reread) are shown when they match, instead of only the names that were there when the entry was chosen.
// nestedEntries lists the entries inside the submenus of a hotlist level, for the popup's typed filter: they show only
// while something is typed, so the filter finds an entry wherever it is, as the Search… item does.

import { matchMask, parseMask } from './reserved-lib.js';

// hotlistMaskFilter: folders always stay (so one can walk on, as before); files show when they match the mask. The
// mask rules are those of internal/config/mask.go for a bare name, which is what the Go side used for this filter.
export function hotlistMaskFilter(mask) {
  const m = parseMask(mask);
  const fn = (e) => !!e.up || !!e.dir || matchMask(m, e.name, false);
  fn.mcMask = String(mask);
  return fn;
}

// nestedEntries walks the submenus of a hotlist level (not the level's own entries, which the popup already lists)
// and returns each command entry as {crumbs, it}: crumbs are the submenu names on the way to it.
export function nestedEntries(level) {
  const out = [];
  const walk = (list, crumbs) => {
    for (const it of list || []) {
      if (it.kind === 'menu') walk(it.items, [...crumbs, it.label || it.title || '']);
      else if (it.kind === 'cmd' && crumbs.length) out.push({ crumbs, it });
    }
  };
  walk(level, []);
  return out;
}

// appendImported puts entries read from Total Commander's [DirMenu] at the end of a hotlist level, with a separator
// between the old and the new entries when both have some (the Configure dialog's Import button and
// cm_ImportDirHotlist, F-663). It returns a new list; nothing already in the hotlist changes.
export function appendImported(tree, items) {
  const old = tree || [], add = items || [];
  return [...old, ...(old.length && add.length ? [{ kind: 'sep', title: '' }] : []), ...add];
}
