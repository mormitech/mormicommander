// Pure helpers of section 38 (drives and volumes), kept apart so they test without a page.

// DRIVES is the drive list as a folder (F-716, Go: feature_drives_fs.go).
export const DRIVES = 'drives://';
export const isDrivesPath = (p) => /^drives:\/\//i.test(String(p || '').trim());

// canRenameVolume says whether a volume of the drive list can be renamed from here (F-713): disks and removable
// drives on macOS (not the startup disk) and Windows; never network shares, optical discs, Home or mapped folders.
export function canRenameVolume(platform, v) {
  if (!v || v.stale || (platform !== 'darwin' && platform !== 'windows')) return false;
  if (v.kind !== 'fixed' && v.kind !== 'removable') return false;
  return !(platform === 'darwin' && v.path === '/');
}

// volumeLabelError says what is wrong with a new volume name ('' when it can be used); Go checks the same.
export function volumeLabelError(name) {
  const s = String(name ?? '');
  if (!s.trim()) return 'The new name is empty.';
  if ([...s.trim()].length > 32) return 'The new name is too long: at most 32 characters.';
  if (/[/\\:*?"<>|]/.test(s)) return 'The new name can\'t contain / \\ : * ? " < > or |.';
  if (/[\u0000-\u001f\u007f]/.test(s)) return 'The new name can\'t contain control characters.';
  return '';
}

// usedPercent is how full a volume is, 0–100 (null when its size is not known), for the drive bar's usage bar (F-714).
export function usedPercent(v) {
  if (!v?.total || v.stale) return null;
  const used = Math.max(0, v.total - (v.free || 0));
  return Math.min(100, Math.round((used / v.total) * 100));
}
