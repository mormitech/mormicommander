// The visible cut state of the files clipboard (board issue #3): files cut with Ctrl+X / Cmd+X stay dimmed in the
// panels until they are pasted, and Esc cancels the cut. This module only keeps the set of cut paths and compares
// paths the way the system does (without case on Windows); system.js wires it to the commands, the panels' rows
// (Panel.rowClassHooks), Esc and the system clipboard. It has no side effects of its own, so tests can use it directly.

// cutKey is the form a path is compared in: no trailing separator (except a root such as "/" or "C:\"), and lower
// case on Windows, where names that differ only in case name the same file.
export function cutKey(path, platform) {
  let p = String(path || '');
  while (p.length > 1 && /[\\/]$/.test(p) && !/^[a-z]:[\\/]$/i.test(p)) p = p.slice(0, -1);
  return platform === 'windows' ? p.toLowerCase() : p;
}

// createCutState keeps the files of the last cut. set() replaces them, clear() forgets them, has() tells whether a
// path is one of them, same() whether a list of paths (read back from the clipboard) is exactly the cut. platform is
// the system's name, or a function returning it (the app learns it only after start-up).
export function createCutState(platform) {
  const plat = typeof platform === 'function' ? platform : () => platform;
  const key = (p) => cutKey(p, plat());
  let keys = new Set();
  let paths = [];
  return {
    get size() { return keys.size; },
    list: () => paths.slice(),
    set(list) {
      paths = (list || []).filter(Boolean).map(String);
      keys = new Set(paths.map(key));
    },
    clear() { keys = new Set(); paths = []; },
    has: (path) => keys.size > 0 && keys.has(key(path)),
    same(list) {
      const other = new Set((list || []).map(key));
      if (other.size !== keys.size) return false;
      for (const k of other) if (!keys.has(k)) return false;
      return true;
    },
  };
}
