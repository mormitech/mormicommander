// The page's half of real file drag-out (board issue #108), kept free of the DOM and of Go so the tests can run it.
// With the setting "nativeDragOut" (Options > Drag & drop, off by default) a drag that starts in a local panel is
// handed to the system (Go's SystemDrag, feature_system_dragout.go) instead of being an HTML drag, so Finder and
// Explorer get real files. A drop of those files back inside the window then comes in as a file drop from the system, with the paths;
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

// ---- drags out of archives and servers ----
// Their files have no path other programs could copy, so Go's SystemDrag.StartFrom copies them to a temporary folder
// first (while the mouse button stays down) and drags the copies. Only drags up to this size go that way (Go checks
// the folders too, which the panel knows no size of).
export const VFS_DRAG_LIMIT = 200 * 1024 * 1024;

// VFS_KINDS are the kinds of panel place (FS.PathInfo's kind) whose files are copied out before a native drag.
const VFS_KINDS = new Set(['archive', 'ftp', 'sftp', 'other']);

// dragPlan says what a drag starting in a panel does:
//   'native'  the files are local: the system drags them (SystemDrag.Start);
//   'copy'    inside an archive or on a server: copied out first, then the system drags the copies (StartFrom);
//   'big'     inside an archive or on a server, but the files the panel knows the size of are over the limit: the drag
//             goes on as without the option (the page says why);
//   'html'    no native drag (the option off, or the system can't): the HTML drag as always.
// kind is the panel's place kind ('local' when unknown), entries the dragged panel entries ({size, dir}).
export function dragPlan({ setting, supported, kind, entries, limit = VFS_DRAG_LIMIT }) {
  if (!useNativeDrag(setting, supported)) return 'html';
  if (!kind || kind === 'local') return 'native';
  if (!VFS_KINDS.has(kind)) return 'html';
  const known = (entries || []).reduce((n, e) => n + (Number(e?.size) > 0 ? Number(e.size) : 0), 0);
  return known > limit ? 'big' : 'copy';
}

// prepText is the line shown while a drag's files are copied out ({done, total, name} of a "native-drag-prep" event).
export function prepText(p) {
  const pct = p?.total > 0 ? Math.min(100, Math.floor((p.done / p.total) * 100)) : 0;
  return `Preparing ${p?.name || 'files'} for the drag… ${pct}% (keep the mouse button down)`;
}

// nativeHoverEl is the element to highlight while our own native drag d is over the target t (targetAt's answer):
// the same as for an HTML drag of ours, so nothing lights up where the drop would not be taken. allowed is
// system-dnd.js's own check.
export function nativeHoverEl(d, t, allowed) {
  if (!d?.native || !t) return null;
  return allowed(d, t) ? t.el || null : null;
}
