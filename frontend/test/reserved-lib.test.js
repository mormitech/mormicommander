// Tests of reserved-lib.js: the custom filter's TC mask (same rules as internal/config/mask.go), the Programs filter,
// the command browser's search and the command line history items (board issue #38).
import { expect, test } from 'vitest';
import { parseMask, matchMask, maskFilter, execFilter, filterCommands, historyItems } from '../src/features/reserved-lib.js';

test('parseMask splits patterns, quotes and the exclude part', () => {
  expect(parseMask('*.txt *.md;*.log')).toEqual({ include: ['*.txt', '*.md', '*.log'], exclude: [] });
  expect(parseMask('"my file*" | *.bak')).toEqual({ include: ['my file*'], exclude: ['*.bak'] });
  expect(parseMask('| *.bak')).toEqual({ include: ['*'], exclude: ['*.bak'] });
  expect(parseMask('')).toEqual({ include: [], exclude: [] });
});

test('matchMask follows the TC rules', () => {
  expect(matchMask('*.txt', 'A.TXT')).toBe(true);
  expect(matchMask('*.txt', 'a.md')).toBe(false);
  expect(matchMask('*.*', 'README')).toBe(true);
  expect(matchMask('read*.*', 'README')).toBe(true);
  expect(matchMask('?.c', 'a.c')).toBe(true);
  expect(matchMask('?.c', 'ab.c')).toBe(false);
  expect(matchMask('*.txt | b*', 'a.txt')).toBe(true);
  expect(matchMask('*.txt | b*', 'b.txt')).toBe(false);
  expect(matchMask('[a].txt', '[a].txt')).toBe(true);
  expect(matchMask('src/', 'src', true)).toBe(true);
  expect(matchMask('src/', 'src', false)).toBe(false);
  expect(matchMask('a/b.txt', 'b.txt')).toBe(false);
  expect(matchMask('', 'x')).toBe(false);
});

test('maskFilter keeps folders and matching files, and is marked', () => {
  const f = maskFilter('*.jpg *.png');
  expect(f({ name: 'a.jpg' })).toBe(true);
  expect(f({ name: 'a.txt' })).toBe(false);
  expect(f({ name: 'docs', dir: true })).toBe(true);
  expect(f({ name: '..', dir: true, up: true })).toBe(true);
  expect(f.mcSrc).toBe('user');
  expect(f.mcMask).toBe('*.jpg *.png');
});

test('execFilter keeps folders and programs', () => {
  const f = execFilter();
  expect(f({ name: 'run.sh', exec: true })).toBe(true);
  expect(f({ name: 'notes.txt', exec: false })).toBe(false);
  expect(f({ name: 'bin', dir: true })).toBe(true);
  expect(f.mcSrc).toBe('execs');
});

test('filterCommands matches every word in name or label, sorted by label', () => {
  const list = [
    { name: 'cm_Copy', label: 'Copy…' },
    { name: 'cm_MkDir', label: 'New folder…' },
    { name: 'cm_CopyNamesToClip', label: 'Copy names' },
    { name: 'cm_Delete', label: 'Delete…' },
  ];
  expect(filterCommands(list, '').map((c) => c.name)).toEqual(['cm_Copy', 'cm_CopyNamesToClip', 'cm_Delete', 'cm_MkDir']);
  expect(filterCommands(list, 'copy').map((c) => c.name)).toEqual(['cm_Copy', 'cm_CopyNamesToClip']);
  expect(filterCommands(list, 'COPY clip').map((c) => c.name)).toEqual(['cm_CopyNamesToClip']);
  expect(filterCommands(list, 'mkdir').map((c) => c.name)).toEqual(['cm_MkDir']);
  expect(filterCommands(list, 'nothing')).toEqual([]);
});

test('historyItems keeps text lines, newest first, up to max', () => {
  expect(historyItems(['ls', '', '  ', 7, 'cd /tmp'])).toEqual([{ label: 'ls', value: 'ls' }, { label: 'cd /tmp', value: 'cd /tmp' }]);
  expect(historyItems(null)).toEqual([]);
  expect(historyItems(['a', 'b', 'c'], 2).map((x) => x.value)).toEqual(['a', 'b']);
});
