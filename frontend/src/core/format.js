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

// display holds the Configuration > Display options the file lists follow (F-414); features/config/layout.js sets
// them with setDisplay. sizeStyle: 0 bytes, 1 kB, 2 MB, 3 GB, 4 dynamic. dateFormat: "" for yyyy-MM-dd, else a pattern
// with yyyy yy MM M dd d.
export const display = { brackets: false, separateExt: true, sizeStyle: 0, thousands: true, dateFormat: '', time24: true, seconds: false };
export function setDisplay(o) { Object.assign(display, o); }

const plain = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0, useGrouping: false });
const tenth = (sep) => new Intl.NumberFormat(undefined, { maximumFractionDigits: 1, minimumFractionDigits: 0, useGrouping: sep });
const tenthSep = tenth(true), tenthPlain = tenth(false);

// cellSize is a file size as the Size column shows it, in the unit and with the separators of the Display options.
export function cellSize(n) {
  n = Number(n) || 0;
  const whole = display.thousands ? group : plain;
  const frac = display.thousands ? tenthSep : tenthPlain;
  const up = (v) => (n > 0 ? Math.max(0.1, Math.ceil(v * 10) / 10) : 0); // a non-empty file never shows as 0
  switch (display.sizeStyle) {
    case 1: return `${whole.format(Math.ceil(n / 1024))} k`;
    case 2: return `${frac.format(up(n / 1048576))} M`;
    case 3: return `${frac.format(up(n / 1073741824))} G`;
    case 4: return size(n);
    default: return whole.format(n);
  }
}

// date is a modification time (Unix ms), in the computer's time zone: "2026-10-14 09:30" by default, else in the date
// format, clock and seconds of the Display options.
export function date(ms) {
  if (!ms) return '';
  const d = new Date(ms);
  const y = d.getFullYear(), mo = d.getMonth() + 1, dd = d.getDate();
  const day = display.dateFormat
    ? display.dateFormat.replace(/yyyy|yy|MM|M|dd|d/g, (t) => ({ yyyy: String(y), yy: pad(y % 100), MM: pad(mo), M: String(mo), dd: pad(dd), d: String(dd) })[t])
    : `${y}-${pad(mo)}-${pad(dd)}`;
  const sec = display.seconds ? `:${pad(d.getSeconds())}` : '';
  const hr = d.getHours();
  const time = display.time24 ? `${pad(hr)}:${pad(d.getMinutes())}${sec}` : `${hr % 12 || 12}:${pad(d.getMinutes())}${sec} ${hr < 12 ? 'AM' : 'PM'}`;
  return `${day} ${time}`;
}

// dateWidth is how wide a date column must be (a CSS length) to show the widest date of the current Display options
// without cutting it off: a two-digit day, month and hour, plus the room the column's padding takes (padPx).
export function dateWidth(padPx = 12) {
  const n = date(new Date(2000, 11, 28, 22, 58, 58).getTime()).length;
  return `calc(${n + 1}ch + ${padPx}px)`;
}

// Flags as the Go side sends them (internal/vfs).
export const F = { DIR: 1, LINK: 2, HIDDEN: 4, EXEC: 8, BROKEN: 16, SYSTEM: 32, READONLY: 64, NOATTR: 128 };

// attr is the Attr column: Unix permissions "rwxr-xr-x" (with a leading "d" or "l" left out, the icon tells), or the
// Windows letters r (read-only), a, h (hidden), s (system) as Total Commander shows them.
export function attr(mode, flags, platform) {
  if (flags & F.NOATTR) return '';
  if (platform === 'windows') {
    return (flags & F.READONLY ? 'r' : '-') + (flags & F.DIR ? '-' : 'a') + (flags & F.HIDDEN ? 'h' : '-') + (flags & F.SYSTEM ? 's' : '-');
  }
  const m = mode & 0o777;
  let s = '';
  for (let i = 8; i >= 0; i--) s += m & (1 << i) ? 'rwx'[(8 - i) % 3] : '-';
  return s;
}

// splitExt is the name split at the last dot for the Ext column: "a.tar.gz" → ["a.tar", "gz"]; a dotfile like ".bashrc"
// is all name, so is "README". (Same rule as vfs.SplitExt in Go.) A name that holds folders, as in a result list or a
// branch view ("sub.d/Makefile", "sub/.bashrc"), is split in its last part only.
export function splitExt(name) {
  const i = name.lastIndexOf('.');
  const s = Math.max(name.lastIndexOf('/'), name.lastIndexOf('\\'));
  if (i <= 0 || i === name.length - 1 || i <= s + 1) return [name, ''];
  return [name.slice(0, i), name.slice(i + 1)];
}

// plural: "1 file", "2 files".
export const plural = (n, word, many = word + 's') => `${group.format(n)} ${n === 1 ? word : many}`;
