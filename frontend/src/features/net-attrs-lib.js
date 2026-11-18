// Pure helpers of the server attributes dialogs (src/features/net-attrs.js, issue #78: F-633, F-649, F-644): the Unix
// permission bits and their octal number, and what quick connect does with a web address.

// PERM_BITS are the boxes of the rwx grid and the special bits, in the order of the dialog.
export const PERM_ROWS = [['Owner', [0o400, 0o200, 0o100]], ['Group', [0o40, 0o20, 0o10]], ['Others', [0o4, 0o2, 0o1]]];
export const SPECIAL_BITS = [[0o4000, 'Set user ID'], [0o2000, 'Set group ID'], [0o1000, 'Sticky']];

// parseOctal reads "644" or "0755" (1 to 4 octal digits), or null.
export function parseOctal(s) {
  const t = String(s ?? '').trim();
  if (!/^[0-7]{1,4}$/.test(t)) return null;
  return parseInt(t, 8);
}

// octalText is a permission number as four octal digits ("0644").
export const octalText = (bits) => (bits & 0o7777).toString(8).padStart(4, '0');

// bitsOfEntry is the Unix permission number of a panel entry's mode (Go's io/fs.FileMode: the special bits are high
// bits there), or null when the server sent no permissions.
export function bitsOfEntry(mode) {
  const m = Number(mode) >>> 0;
  if (!m) return null;
  let b = m & 0o777;
  if (m & 0x800000) b |= 0o4000; // fs.ModeSetuid
  if (m & 0x400000) b |= 0o2000; // fs.ModeSetgid
  if (m & 0x100000) b |= 0o1000; // fs.ModeSticky
  return b;
}

// permString is "rwxr-xr-x" for a permission number (with s/S, t/T for the special bits like ls).
export function permString(bits) {
  const c = (on, ch) => (on ? ch : '-');
  const x = (exec, special, low, up) => (special ? (exec ? low : up) : exec ? 'x' : '-');
  return c(bits & 0o400, 'r') + c(bits & 0o200, 'w') + x(bits & 0o100, bits & 0o4000, 's', 'S')
    + c(bits & 0o40, 'r') + c(bits & 0o20, 'w') + x(bits & 0o10, bits & 0o2000, 's', 'S')
    + c(bits & 0o4, 'r') + c(bits & 0o2, 'w') + x(bits & 0o1, bits & 0o1000, 't', 'T');
}

// isWebURL says whether quick connect got a web address (downloaded as a file, F-644) instead of a server.
export const isWebURL = (s) => /^https?:\/\/[^/\s]/i.test(String(s ?? '').trim());

// webFileName is the name a web address is saved as: the last part of its path, or "index.html".
export function webFileName(s) {
  try {
    const u = new URL(String(s).trim());
    const parts = u.pathname.split('/').filter(Boolean);
    const last = parts.length ? decodeURIComponent(parts[parts.length - 1]) : '';
    return last && last !== '.' && last !== '..' && !/[\\/]/.test(last) ? last : 'index.html';
  } catch {
    return 'index.html';
  }
}
