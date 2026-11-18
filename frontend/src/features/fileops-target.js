// Small rules about the copy/move dialog's target, kept apart from features/fileops.js so they can be tested without
// a page. Today it holds one: Shift+F5 / Shift+F6 on a single folder with the name left as it is. The engine's
// ParseTarget reads "dx" as the existing folder dx (copy into it), which would trip the "folder into itself" check;
// what the user means is "the same name in the same folder", so the target becomes the source folder with dx as the
// new name, and copyOrMove() then gives a copy a free name, "dx(2)", and tells a move there is nothing to do, exactly
// as it already does for a file (board #133).

// sameNameTarget returns the target to use for the dialog's answer t (from go.Jobs.ParseTarget). When the dialog is
// the same-folder one (same), there is one item, it is a folder and the typed text is exactly its name (no separator
// at the end: "dx/" still means into the folder), the answer is the source folder (srcPath) with the folder's name as
// the new name. Otherwise t is returned unchanged.
export function sameNameTarget({ same, items, text, srcPath, t }) {
  if (!same || !items || items.length !== 1) return t;
  const e = items[0];
  if (!e.dir || text !== e.name) return t;
  return { ...t, dir: srcPath, rename: e.name, dirExists: true, sameVolume: true };
}

// targetHistory is the list of the copy dialog's target dropdown (F-830): the other panel's folder first, then the
// folders of the other panel's tabs, then the recent targets, each once. withSep adds the trailing separator.
export function targetHistory({ other, tabs = [], recent = [], withSep = (p) => p }) {
  const seen = new Set();
  const out = [];
  const add = (value, detail) => { if (value && !seen.has(value)) { seen.add(value); out.push({ label: value, value, detail }); } };
  if (other) add(withSep(other), 'other panel');
  for (const p of tabs) if (p) add(withSep(p), 'tab');
  for (const h of recent) add(h, '');
  return out;
}
