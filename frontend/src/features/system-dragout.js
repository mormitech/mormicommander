// The page's half of real file drag-out (board issue #108), kept free of the DOM and of Go so the tests can run it.
// With the setting "nativeDragOut" (Options > Drag & drop, off by default) a drag that starts in a local panel is
// handed to the system (Go's SystemDrag, feature_system_dragout.go) instead of being an HTML drag, so Finder gets real
// files. A drop of those files back inside the window then comes in as a file drop from the system, with the paths;
// ownNativeDrop tells such a drop apart from files dragged in from another app, so system-dnd.js can treat it like
// one of its own HTML drops (target checks, copy / move / link from the modifier keys) and not as a Finder drop.

// How long after a native drag ended its drop may still come in (the drop event and the end event travel apart).
export const NATIVE_DROP_WINDOW_MS = 3000;

// useNativeDrag says whether a drag starting now goes to the system: the setting is on and the system can do it.
export function useNativeDrag(setting, supported) {
  return !!setting && supported === true;
}

// norm drops a trailing slash (the system may give a folder's path with one), but not the root's.
const norm = (p) => (String(p).length > 1 ? String(p).replace(/[\\/]+$/, '') || String(p) : String(p));

// sameFiles: the two lists hold the same paths (in any order, a folder's trailing slash aside).
export function sameFiles(a, b) {
  if (!Array.isArray(a) || !Array.isArray(b) || !a.length) return false;
  const sa = new Set(a.map(norm));
  const sb = new Set(b.map(norm));
  return sa.size === sb.size && [...sb].every((p) => sa.has(p));
}

// ownNativeDrop returns the native drag d ({from, paths, native: true, endedAt?}) when the dropped paths are its files
// and it is still under way or ended only a moment ago; otherwise null (the files came from another app).
export function ownNativeDrop(d, paths, now) {
  if (!d?.native || !sameFiles(d.paths, paths)) return null;
  if (d.endedAt && now - d.endedAt > NATIVE_DROP_WINDOW_MS) return null;
  return d;
}
