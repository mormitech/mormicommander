// The wording around extended attributes (board #29, F-NEW-13), kept free of the DOM and of Go so vitest can test
// it: the lines the Properties dialog shows for a file's attribute names, the sentence after "Remove quarantine
// flag", the questions before removing the flag or an attribute (they name the files), and the one-time note when a
// copy goes onto a file system that drops extended attributes (FAT, exFAT) or into a cloud-synced folder.
// features/tools/xattr.js and features/fileops-xattr.js use it.

export const QUARANTINE = 'com.apple.quarantine';

// Friendly words for the attributes people meet most; any other name is shown as it is.
const KNOWN = {
  'com.apple.quarantine': 'downloaded from the internet',
  'com.apple.metadata:_kMDItemUserTags': 'Finder tags',
  'com.apple.FinderInfo': 'Finder info (colour label, flags)',
  'com.apple.ResourceFork': 'resource fork',
  'com.apple.metadata:kMDItemWhereFroms': 'where it was downloaded from',
  'com.apple.provenance': 'which app made it',
  'com.apple.macl': 'apps allowed to open it',
  'com.apple.lastuseddate#PS': 'last opened',
  'system.posix_acl_access': 'access control list',
  'system.posix_acl_default': 'default access control list',
  'security.selinux': 'SELinux label',
};

// xattrLines turns [{name, size}] into one line each: "com.apple.quarantine — downloaded from the internet (57 bytes)".
export function xattrLines(list) {
  if (!Array.isArray(list)) return [];
  return list.filter((a) => a && a.name).map((a) => {
    const words = KNOWN[a.name];
    const n = Number(a.size) || 0;
    return `${a.name}${words ? ` — ${words}` : ''} (${n} ${n === 1 ? 'byte' : 'bytes'})`;
  });
}

// hasQuarantine says whether a list of attributes holds the quarantine flag.
export const hasQuarantine = (list) => Array.isArray(list) && list.some((a) => a && a.name === QUARANTINE);

// quarantineMessage is the sentence after "Remove quarantine flag": {checked, removed, failed, canceled}.
export function quarantineMessage(r) {
  if (!r) return '';
  const items = (n) => `${n} ${n === 1 ? 'item' : 'items'}`;
  const failed = (r.failed || []).length;
  let s;
  if (r.removed === 0) s = r.checked === 1 ? 'It had no quarantine flag.' : `None of the ${items(r.checked)} had a quarantine flag.`;
  else if (r.checked === 1) s = 'Removed the quarantine flag.';
  else s = `Removed the quarantine flag from ${items(r.removed)} (of ${items(r.checked)} looked at).`;
  if (failed) s += ` ${items(failed)} kept it.`;
  if (r.canceled) s += ' Stopped before the end.';
  return s;
}

// baseName is the last part of a path.
const baseName = (p) => String(p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop() || String(p || '');

// nameList names up to max files by their names: "a.app", "a.app and b.dmg", "a.app, b.dmg, c.pkg and 2 more".
export function nameList(paths, max = 5) {
  const names = (paths || []).map(baseName).filter(Boolean);
  if (names.length <= 1) return names[0] ? `"${names[0]}"` : '';
  const shown = names.slice(0, names.length > max ? max : names.length).map((n) => `"${n}"`);
  if (names.length > max) return `${shown.join(', ')} and ${names.length - max} more`;
  return `${shown.slice(0, -1).join(', ')} and ${shown.at(-1)}`;
}

// quarantineQuestion is the question before "Remove quarantine flag"; anyDir tells whether a folder is among the
// paths (its contents lose the flag too).
export function quarantineQuestion(paths, anyDir = false) {
  const n = (paths || []).length;
  const inside = anyDir ? ' and from everything inside the selected folders' : '';
  return `Remove the quarantine flag from ${nameList(paths)}${inside}?\n\nmacOS then opens ${n === 1 && !anyDir ? 'it' : 'them'} without asking whether you trust where ${n === 1 && !anyDir ? 'it' : 'they'} came from. Only do this for files you trust.`;
}

// removeAttrQuestion is the question before removing one extended attribute from one file.
export function removeAttrQuestion(path, name) {
  const words = KNOWN[name];
  return `Remove the extended attribute "${name}"${words ? ` (${words})` : ''} from "${baseName(path)}"?\n\nThis cannot be undone.`;
}

// FS_WORDS names file systems the way people know them.
const FS_WORDS = { msdos: 'FAT', vfat: 'FAT', fat: 'FAT', fat12: 'FAT', fat16: 'FAT', fat32: 'FAT32', exfat: 'exFAT', 'fuse.exfat': 'exFAT', udf: 'UDF', cd9660: 'CD', iso9660: 'CD' };

// xattrLossNote is the one-time note when a copy loses extended attributes: {loses, fileSystem, example, attribute}.
// It is '' when nothing is lost.
// A cloud target ({cloud: "Dropbox"}) may keep some of them, so its note says "may not keep".
export function xattrLossNote(check) {
  if (!check || !check.loses) return '';
  const what = check.example ? String(check.example).replace(/[\\/]+$/, '').split(/[\\/]/).pop() : '';
  if (check.cloud) {
    return `The target is in a cloud-synced folder (${check.cloud}), which may not keep extended attributes such as Finder tags${what ? `: ${what} and others may lose them` : ''}. The copy goes on; this note is shown once.`;
  }
  const fs = String(check.fileSystem || '');
  const name = FS_WORDS[fs.toLowerCase()] || fs || 'this';
  return `The target drive (${name}) keeps no extended attributes such as Finder tags${what ? `: ${what} and others lose them` : ''}. The copy goes on; this note is shown once.`;
}

// onceGate returns a function that answers true the first time it is called with a key and false after that.
export function onceGate() {
  const seen = new Set();
  return (key = '') => {
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  };
}
