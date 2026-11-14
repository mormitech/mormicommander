// The pure parts of Compare folders (comparedirs.js), kept apart so the tests can load them without the page:
// which folder mode a compare runs with, and the panel filter "hide equal files" puts on (marked so a reread, Ctrl+F10
// or the next plain compare can tell it from other filters and remove it). Tested in test/comparedirs-lib.test.js.

// compareSubdirs is the folder mode a compare runs with: what the command asked ("missing", "recursive"), or, for
// plain Shift+F2 (nothing asked), "missing" when the compareDirs.oneSidedFolders setting is on and "" (folders are
// left alone, the behaviour Shift+F2 always had) when it is off.
export function compareSubdirs(asked, settings) {
  if (asked) return asked;
  return settings?.oneSidedFolders ? 'missing' : '';
}

// hideEqualFilter is the panel filter that hides the names that compared equal, or null when none did.
export function hideEqualFilter(equalNames) {
  const eq = new Set(equalNames || []);
  if (!eq.size) return null;
  return Object.assign((e) => !eq.has(e.name), { mcCompare: true });
}

// isCompareFilter tells a filter made by hideEqualFilter from other panel filters (quick filter, hotlist mask).
export const isCompareFilter = (fn) => !!fn?.mcCompare;
