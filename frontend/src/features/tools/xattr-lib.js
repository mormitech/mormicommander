// The wording around extended attributes (board #29, F-NEW-13), kept free of the DOM and of Go so vitest can test
// it: the lines the Properties dialog shows for a file's attribute names, the sentence after "Remove quarantine
// flag", and the one-time note when a copy goes onto a file system that drops extended attributes (FAT, exFAT).
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

// FS_WORDS names file systems the way people know them.
const FS_WORDS = { msdos: 'FAT', vfat: 'FAT', fat: 'FAT', fat12: 'FAT', fat16: 'FAT', fat32: 'FAT32', exfat: 'exFAT', 'fuse.exfat': 'exFAT', udf: 'UDF', cd9660: 'CD', iso9660: 'CD' };

// xattrLossNote is the one-time note when a copy loses extended attributes: {loses, fileSystem, example, attribute}.
// It is '' when nothing is lost.
export function xattrLossNote(check) {
  if (!check || !check.loses) return '';
  const fs = String(check.fileSystem || '');
  const name = FS_WORDS[fs.toLowerCase()] || fs || 'this';
  const what = check.example ? String(check.example).replace(/[\\/]+$/, '').split(/[\\/]/).pop() : '';
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
