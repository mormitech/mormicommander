// How a folder tab is turned into what Go stores and back (the tabs of both panels after a restart, .tab files and
// favourite tab sets), kept in a module of its own with no imports so the tests can load it in Node. Board issue #12
// added the lock folder: a "locked, but folder changes allowed" tab that shows another folder than the one it was
// locked to saves that folder as lockPath, so after a restart it can still go back to it; a tab still in its locked
// folder, or not locked at all, saves none (older files have none either, and the lock folder is then the tab's path).
// Board issue #112 added each tab's own folder history: the tabs that are not active save theirs with the tab (newest
// first, like TC's [LeftHistory]), so Alt+Left still works in every tab after a restart, not only in the active one.

// tabFields is the options for a new tab made from a saved one (the makeTab options of tabs.js).
export function tabFields(x) {
  const lock = +x.lock || 0;
  return { path: x.path, caption: x.caption, lock, lockPath: lock ? x.lockPath || x.path : '', sort: sortFields(x), saved: x };
}

// sortFields is a saved tab's sort as the panel keeps it, with the second sort key (sort.then, F-114) when one was saved.
export function sortFields(x) {
  const key = x.sort || 'name';
  return x.sort2 && x.sort2 !== key ? { key, desc: !!x.desc, then: { key: x.sort2, desc: !!x.desc2 } } : { key, desc: !!x.desc };
}

// sortSaved is what a tab saves of a panel sort: the key and direction, and the second key when there is one (F-114).
export const sortSaved = (so) => ({ sort: so.key, desc: !!so.desc, ...(so.then?.key ? { sort2: so.then.key, desc2: !!so.then.desc } : {}) });

// sameSort says whether two panel sorts are the same, the second key included.
export const sameSort = (a, b) => a.key === b.key && !!a.desc === !!b.desc && (a.then?.key || '') === (b.then?.key || '') && !!a.then?.desc === !!b.then?.desc;

// lockField is the lockPath to save with a tab: only for a locked tab whose lock folder is not the folder it shows.
export function lockField(lock, lockPath, path) {
  return lock && lockPath && lockPath !== path ? { lockPath } : {};
}

// tabHistory is the folder history to save with a tab, newest first: none for the active tab (its history is the
// panel's, saved as leftHistory/rightHistory), else the back list of the state the tab was left with, or what the tab
// was read back with when it has not been shown since.
export function tabHistory(t, active) {
  if (active || !t) return [];
  if (t.state && Array.isArray(t.state.back)) return [...t.state.back].reverse();
  return Array.isArray(t.saved?.history) ? [...t.saved.history] : [];
}

// backFromHistory turns a saved newest-first history into a panel's back list (oldest first), leaving out empty
// entries and the folder the tab shows (same compares two folders with the platform's rules).
export function backFromHistory(history, path, same) {
  return (Array.isArray(history) ? [...history] : []).reverse().filter((x) => x && !same(x, path));
}
