import { expect, test } from 'vitest';
import { isBranchPath, isSelectionBranch, canBranch, branchPathFor, leafName, dirPart, firstPart, realPath, progressText, doneText, pickWalk } from '../src/features/branch-lib.js';
import { quickViewSource, downloads, viewsFromCopy } from '../src/features/viewer/qvsource.js';

test('branch view paths (F-086, F-087)', () => {
  expect(isBranchPath('branch:///Users/me')).toBe(true);
  expect(isBranchPath('branch://C:\\photos')).toBe(true);
  expect(isBranchPath('branchsel://3')).toBe(true);
  expect(isBranchPath('/Users/me/branch')).toBe(false);
  expect(isBranchPath('find://results-1')).toBe(false);
  expect(isSelectionBranch('branchsel://3')).toBe(true);
  expect(isSelectionBranch('branch:///x')).toBe(false);
  expect(branchPathFor('/Users/me')).toBe('branch:///Users/me');
  expect(branchPathFor('C:\\x')).toBe('branch://C:\\x');
  expect(canBranch('/Users/me')).toBe(true);
  expect(canBranch('C:\\x')).toBe(true);
  for (const p of ['ftp://host/x', 'find://results-1', 'branch:///x', '']) expect(canBranch(p)).toBe(false);
});

test('names of the flat list', () => {
  expect(leafName('sub/deeper/c.txt', 'darwin')).toBe('c.txt');
  expect(leafName('c.txt', 'darwin')).toBe('c.txt');
  // a backslash is part of a name on macOS and Linux, a separator on Windows
  expect(leafName('a\\b.txt', 'linux')).toBe('a\\b.txt');
  expect(leafName('sub\\b.txt', 'windows')).toBe('b.txt');
  expect(dirPart('sub/deeper/c.txt', 'darwin')).toBe('sub/deeper/');
  expect(dirPart('c.txt', 'darwin')).toBe('');
  expect(firstPart('sub/deeper/c.txt', 'darwin')).toBe('sub');
  expect(firstPart('sub\\c.txt', 'windows')).toBe('sub');
  expect(firstPart('top.txt', 'darwin')).toBe('top.txt');
  expect(realPath('/Users/me', 'sub/a.txt', '/')).toBe('/Users/me/sub/a.txt');
  expect(realPath('/', 'a.txt', '/')).toBe('/a.txt');
  expect(realPath('C:\\x', 'sub\\a.txt', '\\')).toBe('C:\\x\\sub\\a.txt');
  expect(realPath('', 'a', '/')).toBe('');
});

test('notices and messages', () => {
  expect(progressText(0, '/x').title).toBe('Reading the subfolders…');
  expect(progressText(1, '/x').title).toContain('1 file so far');
  expect(progressText(1200, '/x', (n) => n.toLocaleString('en-US')).title).toContain('1,200 files');
  expect(progressText(5, '/x').hint).toBe('Esc or Stop stays in /x.');
  expect(doneText({ files: 3 })).toMatch(/^3 files from all subfolders/);
  expect(doneText({ files: 200000, truncated: true, limit: 200000 })).toMatch(/^Only the first 200000 files/);
  expect(doneText({ files: 2, skipped: 1 })).toContain('1 folder could not be read');
});

test('pickWalk matches a reading panel to a listing of Go', () => {
  const walks = [{ path: 'branch:///a', files: 1 }, { path: 'branch:///b', files: 2 }];
  expect(pickWalk([], '/x', '/y')).toBe(null);
  expect(pickWalk(walks, 'branch:///b', '/y')).toBe(walks[1]); // a reread of the branch view the panel shows
  expect(pickWalk(walks, '/x', 'branch:///a')).toBe(walks[1]); // not the other panel's reread
  expect(pickWalk([walks[0]], '/x', '/y')).toBe(walks[0]);
});

test('quick view treats branch views as local files', () => {
  const local = { kind: 'local', remote: false };
  expect(quickViewSource(local, 'branch:///Users/me', { size: 1 << 30 })).toBe('local');
  expect(downloads(local, 'branchsel://2')).toBe(false);
  // F3 takes the copy route of result lists (LocalCopy hands a local file back as it is)
  expect(viewsFromCopy(local, 'branch:///Users/me')).toBe(true);
  // other places are unchanged
  expect(quickViewSource(local, '/Users/me', { size: 1 })).toBe('local');
  expect(quickViewSource({ kind: 'results' }, 'find://results-1', { size: 1 })).toBe('remote');
});
