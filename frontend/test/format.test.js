import { expect, test } from 'vitest';
import { size, attr, splitExt, date, F, plural, cellSize, setDisplay, dateWidth } from '../src/core/format.js';

test('size', () => {
  expect(size(0)).toBe('0 B');
  expect(size(1023)).toBe('1023 B');
  expect(size(1024)).toBe('1 KB');
  expect(size(1536)).toBe('1.5 KB');
  expect(size(5 * 1024 * 1024)).toBe('5 MB');
  expect(size(250 * 1024 * 1024 * 1024)).toBe('250 GB');
});

test('attr on Unix and Windows', () => {
  expect(attr(0o755, 0, 'darwin')).toBe('rwxr-xr-x');
  expect(attr(0o640, 0, 'linux')).toBe('rw-r-----');
  expect(attr(0, F.READONLY | F.HIDDEN, 'windows')).toBe('rah-');
  expect(attr(0, F.DIR | F.SYSTEM, 'windows')).toBe('---s');
  // a server or share in the Network panel has no attributes: blank, while a real folder with no permissions shows dashes
  expect(attr(0o755, F.DIR | F.NOATTR, 'darwin')).toBe('');
  expect(attr(0, F.DIR | F.NOATTR, 'windows')).toBe('');
  expect(attr(0, F.DIR, 'linux')).toBe('---------');
});

test('attr shows the real Windows archive bit when Go read it (F-686)', () => {
  expect(attr(0, F.WINATTRS, 'windows')).toBe('----');
  expect(attr(0, F.WINATTRS | F.ARCHIVE | F.READONLY, 'windows')).toBe('ra--');
  expect(attr(0, F.DIR | F.WINATTRS | F.ARCHIVE, 'windows')).toBe('-a--');
  expect(attr(0, F.DIR | F.WINATTRS, 'windows')).toBe('----');
  // the archive bit never leaks into the Unix permission string
  expect(attr(0o644, F.WINATTRS | F.ARCHIVE, 'linux')).toBe('rw-r--r--');
});

test('splitExt like vfs.SplitExt', () => {
  expect(splitExt('a.tar.gz')).toEqual(['a.tar', 'gz']);
  expect(splitExt('.bashrc')).toEqual(['.bashrc', '']);
  expect(splitExt('README')).toEqual(['README', '']);
  expect(splitExt('x.')).toEqual(['x.', '']);
  // names with folders (result lists, branch view): only the last part has an extension
  expect(splitExt('sub/a.txt')).toEqual(['sub/a', 'txt']);
  expect(splitExt('sub.d/Makefile')).toEqual(['sub.d/Makefile', '']);
  expect(splitExt('sub/.bashrc')).toEqual(['sub/.bashrc', '']);
  expect(splitExt('a.b\\c.txt')).toEqual(['a.b\\c', 'txt']);
});

test('date', () => {
  const ms = new Date(2026, 9, 14, 9, 5).getTime();
  expect(date(ms)).toBe('2026-10-14 09:05');
  expect(date(0)).toBe('');
});

test('plural', () => {
  expect(plural(1, 'file')).toBe('1 file');
  expect(plural(3, 'folder')).toBe('3 folders');
});

test('display options: sizes and dates (F-414)', () => {
  const ms = new Date(2026, 10, 13, 18, 6, 7).getTime();
  try {
    setDisplay({ sizeStyle: 0, thousands: false });
    expect(cellSize(123456)).toBe('123456');
    setDisplay({ sizeStyle: 1, thousands: true });
    expect(cellSize(123456)).toBe('121 k');
    setDisplay({ sizeStyle: 2 });
    expect(cellSize(123456)).toBe('0.2 M');
    expect(cellSize(6)).toBe('0.1 M');
    expect(cellSize(0)).toBe('0 M');
    setDisplay({ dateFormat: 'yyyy', seconds: true });
    expect(date(ms)).toBe('2026 18:06:07');
    setDisplay({ dateFormat: 'dd.MM.yy', seconds: false, time24: false });
    expect(date(ms)).toBe('13.11.26 6:06 PM');
  } finally {
    setDisplay({ sizeStyle: 0, thousands: true, dateFormat: '', time24: true, seconds: false });
  }
  expect(date(ms)).toBe('2026-11-13 18:06');
});

test('dateWidth fits the widest date of the Display options (Sync window date columns)', () => {
  try {
    expect(dateWidth()).toBe('calc(17ch + 12px)'); // "2000-12-28 22:58" is 16 characters
    setDisplay({ seconds: true, time24: false });
    expect(dateWidth(10)).toBe('calc(23ch + 10px)'); // "2000-12-28 10:58:58 PM" is 22
  } finally {
    setDisplay({ sizeStyle: 0, thousands: true, dateFormat: '', time24: true, seconds: false });
  }
});
