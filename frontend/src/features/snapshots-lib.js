// Versions from snapshots (board #34, F-874), the parts that need no page: which paths are inside a snapshot (never
// written to), and the rows of the version list. Go finds the versions (feature_snapshots.go, internal/snapshots).

const KIND = { zfs: 'ZFS', btrfs: 'Btrfs', apfs: 'APFS', timemachine: 'Time Machine', vss: 'Shadow copy' };
export const kindLabel = (k) => KIND[k] || k;

// inSnapshot mirrors snapshots.InSnapshot: a ZFS or snapper snapshot folder, a Windows shadow copy, a mounted APFS
// local snapshot or a Time Machine backup.
export function inSnapshot(p) {
  const raw = String(p || '');
  if (/^\\\\\?\\GLOBALROOT\\Device\\HarddiskVolumeShadowCopy\d+(\\|$)/i.test(raw)) return true;
  const s = raw.replace(/\\/g, '/');
  return /\/\.zfs\/snapshot\/[^/]+(\/|$)/.test(s) || /\/\.snapshots\/\d+\/snapshot(\/|$)/.test(s) ||
    s.startsWith('/Volumes/.timemachine/') || s.startsWith('/Volumes/com.apple.TimeMachine.localsnapshots/');
}

// snapshotRefusal returns the sentence to show when a file command would change something inside a snapshot, and ""
// otherwise. Snapshots are read-only copies: files are copied out of them, never into, moved, renamed or deleted.
export function snapshotRefusal(...paths) {
  return paths.some((p) => inSnapshot(p)) ? "A snapshot can't be changed: copy files out of it instead." : '';
}

// versionRows turns SnapshotVersions' versions into list rows: {label, path, title, disabled}. fmtDate(ms) formats a
// date, fmtSize(n) a size.
export function versionRows(versions, fmtDate, fmtSize) {
  return (versions || []).map((v) => {
    const when = v.when ? fmtDate(v.when) : 'date not known';
    let state;
    if (!v.path) state = 'not mounted';
    else if (v.missing) state = 'not in this snapshot';
    else state = v.dir ? 'folder' : fmtSize(v.size);
    const label = `${when}  ·  ${kindLabel(v.kind)}  ·  ${v.name}${v.info ? ` (${v.info})` : ''}  ·  ${state}`;
    return { label, path: v.path || '', title: v.why || v.path || '', disabled: !v.path || !!v.missing };
  });
}

// openTarget says how the other panel opens a version: a folder is entered, a file's folder is opened with the
// cursor on it. sep is the path separator.
export function openTarget(v, sep = '/') {
  if (v.dir) return { dir: v.path, focus: null };
  const p = String(v.path).replace(/[\\/]+$/, '');
  const i = Math.max(p.lastIndexOf('/'), sep === '\\' ? p.lastIndexOf('\\') : -1);
  return { dir: i > 0 ? p.slice(0, i) : p.slice(0, i + 1) || sep, focus: p.slice(i + 1) };
}
