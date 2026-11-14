// Compare folders' pure parts (src/features/comparedirs-lib.js): the folder mode of a compare and the "hide equal
// files" filter that a reread, Ctrl+F10 or the next plain compare removes (board issue #9).
import { expect, test } from 'vitest';
import { compareSubdirs, hideEqualFilter, isCompareFilter } from '../src/features/comparedirs-lib.js';

test('plain Shift+F2 leaves folders alone unless oneSidedFolders is on', () => {
  expect(compareSubdirs('', undefined)).toBe('');
  expect(compareSubdirs('', {})).toBe('');
  expect(compareSubdirs('', { oneSidedFolders: false })).toBe('');
  expect(compareSubdirs('', { oneSidedFolders: true })).toBe('missing');
});

test('a command that asks for a folder mode gets it whatever the setting', () => {
  expect(compareSubdirs('recursive', { oneSidedFolders: true })).toBe('recursive');
  expect(compareSubdirs('missing', {})).toBe('missing');
});

test('hideEqualFilter hides the equal names and is recognisable', () => {
  const fn = hideEqualFilter(['a.txt', 'b']);
  expect(isCompareFilter(fn)).toBe(true);
  expect(fn({ name: 'a.txt' })).toBe(false);
  expect(fn({ name: 'c.txt' })).toBe(true);
  expect(hideEqualFilter([])).toBe(null);
  expect(hideEqualFilter(undefined)).toBe(null);
});

test('other filters are not taken for the compare filter', () => {
  const quick = Object.assign(() => true, { mcQuick: 'x' });
  expect(isCompareFilter(quick)).toBe(false);
  expect(isCompareFilter(null)).toBe(false);
  expect(isCompareFilter(() => true)).toBe(false);
});
