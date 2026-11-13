// How sizes, dates and attributes look in the panels and status lines. Pure functions, unit-tested (test/format.test.js).

const group = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 });
const one = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1, minimumFractionDigits: 0 });

// bytes is a size in plain bytes with thousands separators ("12,345,678"), the way the Size column shows files.
export const bytes = (n) => group.format(n);

// size is a short size for status lines and dialogs: "0 B", "999 B", "1.2 KB", "34 MB", "1.5 GB" (1 KB = 1024 bytes, as
// Total Commander counts).
export function size(n) {
  n = Number(n) || 0;
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let v = n / 1024, i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v >= 100 ? group.format(v) : one.format(v)} ${units[i]}`;
}

const pad = (n) => String(n).padStart(2, '0');

// date is a modification time (Unix ms) as "2026-10-14 09:30", in the computer's time zone. One fixed shape keeps the
// column narrow and sortable by eye; F-094 adds a choice.
export function date(ms) {
  if (!ms) return '';
  const d = new Date(ms);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// Flags as the Go side sends them (internal/vfs).
export const F = { DIR: 1, LINK: 2, HIDDEN: 4, EXEC: 8, BROKEN: 16, SYSTEM: 32, READONLY: 64 };

// attr is the Attr column: Unix permissions "rwxr-xr-x" (with a leading "d" or "l" left out, the icon tells), or the
// Windows letters r (read-only), a, h (hidden), s (system) as Total Commander shows them.
export function attr(mode, flags, platform) {
  if (platform === 'windows') {
    return (flags & F.READONLY ? 'r' : '-') + (flags & F.DIR ? '-' : 'a') + (flags & F.HIDDEN ? 'h' : '-') + (flags & F.SYSTEM ? 's' : '-');
  }
  const m = mode & 0o777;
  let s = '';
  for (let i = 8; i >= 0; i--) s += m & (1 << i) ? 'rwx'[(8 - i) % 3] : '-';
  return s;
}

// splitExt is the name split at the last dot for the Ext column: "a.tar.gz" → ["a.tar", "gz"]; a dotfile like ".bashrc"
// is all name, so is "README". (Same rule as vfs.SplitExt in Go.)
export function splitExt(name) {
  const i = name.lastIndexOf('.');
  if (i <= 0 || i === name.length - 1) return [name, ''];
  return [name.slice(0, i), name.slice(i + 1)];
}

// plural: "1 file", "2 files".
export const plural = (n, word, many = word + 's') => `${group.format(n)} ${n === 1 ? word : many}`;
