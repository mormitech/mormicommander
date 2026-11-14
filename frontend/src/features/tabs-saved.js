// How a folder tab is turned into what Go stores and back (the tabs of both panels after a restart, .tab files and
// favourite tab sets), kept in a module of its own with no imports so the tests can load it in Node. Board issue #12
// added the lock folder: a "locked, but folder changes allowed" tab that shows another folder than the one it was
// locked to saves that folder as lockPath, so after a restart it can still go back to it; a tab still in its locked
// folder, or not locked at all, saves none (older files have none either, and the lock folder is then the tab's path).

// tabFields is the options for a new tab made from a saved one (the makeTab options of tabs.js).
export function tabFields(x) {
  const lock = +x.lock || 0;
  return { path: x.path, caption: x.caption, lock, lockPath: lock ? x.lockPath || x.path : '', sort: { key: x.sort || 'name', desc: !!x.desc }, saved: x };
}

// lockField is the lockPath to save with a tab: only for a locked tab whose lock folder is not the folder it shows.
export function lockField(lock, lockPath, path) {
  return lock && lockPath && lockPath !== path ? { lockPath } : {};
}
