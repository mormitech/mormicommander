import { expect, test } from 'vitest';
import { size, attr, splitExt, date, F, plural } from '../src/core/format.js';

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
});

test('splitExt like vfs.SplitExt', () => {
  expect(splitExt('a.tar.gz')).toEqual(['a.tar', 'gz']);
  expect(splitExt('.bashrc')).toEqual(['.bashrc', '']);
  expect(splitExt('README')).toEqual(['README', '']);
  expect(splitExt('x.')).toEqual(['x.', '']);
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
