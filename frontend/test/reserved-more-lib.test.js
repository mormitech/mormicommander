// Tests of reserved-more-lib.js (board issue #38, second slice): the separate tree commands mapped onto the panels'
// tree view, the items of the in-window menu (F9, F10), the folder tree popup (Alt+F10) and the printed file list.
import { expect, test } from 'vitest';
import { treeLevel, treeTarget, menuItems, topMenuItems, dirNames, joinPath, folderItems, isoDate, groupDigits, fileListText, subEntries } from '../src/features/reserved-more-lib.js';

const S = (active, other) => ({ active, other });

test('treeLevel counts the panels in the tree view', () => {
  expect(treeLevel(S(false, false))).toBe('off');
  expect(treeLevel(S(true, false))).toBe('one');
  expect(treeLevel(S(false, true))).toBe('one');
  expect(treeLevel(S(true, true))).toBe('two');
  expect(treeLevel(undefined)).toBe('off');
});

test('treeTarget: off, one, two', () => {
  for (const s of [S(false, false), S(true, false), S(false, true), S(true, true)]) {
    expect(treeTarget(s, 'off')).toEqual(S(false, false));
    expect(treeTarget(s, 'two')).toEqual(S(true, true));
  }
  // one tree: the active panel gets it, unless one panel already has it (then nothing changes)
  expect(treeTarget(S(false, false), 'one')).toEqual(S(true, false));
  expect(treeTarget(S(false, true), 'one')).toEqual(S(false, true));
  expect(treeTarget(S(true, false), 'one')).toEqual(S(true, false));
  // from two trees down to one, the active panel keeps its tree
  expect(treeTarget(S(true, true), 'one')).toEqual(S(true, false));
  // an unknown wish changes nothing
  expect(treeTarget(S(false, true), 'what')).toEqual(S(false, true));
});

test('treeTarget: the cycle goes off -> one -> two -> off', () => {
  expect(treeTarget(S(false, false), 'cycle')).toEqual(S(true, false));
  expect(treeTarget(S(true, false), 'cycle')).toEqual(S(true, true));
  expect(treeTarget(S(false, true), 'cycle')).toEqual(S(true, true));
  expect(treeTarget(S(true, true), 'cycle')).toEqual(S(false, false));
  let s = S(false, false);
  const seen = [];
  for (let i = 0; i < 4; i++) { s = treeTarget(s, 'cycle'); seen.push(treeLevel(s)); }
  expect(seen).toEqual(['one', 'two', 'off', 'one']);
});

test('menuItems: commands with keys and ticks, submenus, tidy separators', () => {
  const keys = { cm_SrcShort: 'Ctrl+F1' };
  const items = menuItems([
    { sep: true },
    { label: 'Brief', cmd: 'cm_SrcShort', disabled: false, checked: true },
    { label: 'Full', cmd: 'cm_SrcLong', disabled: false, checked: false },
    { sep: true }, { sep: true },
    { label: 'Separate tree', items: [{ label: 'Off', cmd: 'cm_SeparateTreeOff' }] },
    { label: 'Not there yet', cmd: 'cm_Nothing', disabled: true },
    { sep: true },
  ], (c) => keys[c]);
  expect(items).toEqual([
    { label: '✓ Brief', detail: 'Ctrl+F1', value: { cmd: 'cm_SrcShort' }, disabled: false },
    { label: '   Full', detail: '', value: { cmd: 'cm_SrcLong' }, disabled: false },
    { sep: true },
    { label: 'Separate tree ▸', value: { items: [{ label: 'Off', cmd: 'cm_SeparateTreeOff' }], label: 'Separate tree' }, sub: true },
    { label: 'Not there yet', detail: '', value: { cmd: 'cm_Nothing' }, disabled: true },
  ]);
  expect(menuItems(null)).toEqual([]);
});

test('topMenuItems lists the menus that have items', () => {
  const spec = [{ label: 'Files', items: [{ label: 'Quit', cmd: 'cm_Exit' }] }, { label: 'Empty', items: [] }, { label: 'Help', items: [{ label: 'Help', cmd: 'cm_HelpIndex' }] }];
  expect(topMenuItems(spec).map((it) => [it.label, it.sub, it.value.label])).toEqual([['Files ▸', true, 'Files'], ['Help ▸', true, 'Help']]);
  expect(topMenuItems(undefined)).toEqual([]);
});

test('dirNames keeps folders, hides hidden ones, sorts numbers naturally', () => {
  const rows = [['b10', 0, 0, 1], ['b2', 0, 0, 1], ['file.txt', 5, 0, 0], ['.git', 0, 0, 1], ['Secret', 0, 0, 1 | 4], ['A', 0, 0, 1], ['..', 0, 0, 1]];
  expect(dirNames(rows)).toEqual(['A', 'b2', 'b10']);
  expect(dirNames(rows, { showHidden: true })).toEqual(['.git', 'A', 'b2', 'b10', 'Secret']);
  expect(dirNames(null)).toEqual([]);
});

test('folderItems: go to here first, then up, then the subfolders', () => {
  expect(joinPath('/Users/me', 'x')).toBe('/Users/me/x');
  expect(joinPath('/', 'x')).toBe('/x');
  expect(joinPath('C:\\', 'x', '\\')).toBe('C:\\x');
  const items = folderItems('/Users/me', '/Users', ['Documents', 'Music']);
  expect(items[0]).toEqual({ label: 'Go to /Users/me', value: { go: '/Users/me' } });
  expect(items[1]).toEqual({ label: '..', detail: 'up one level', value: { open: '/Users', from: '/Users/me' } });
  expect(items[2]).toEqual({ sep: true });
  expect(items.slice(3).map((it) => it.value.open)).toEqual(['/Users/me/Documents', '/Users/me/Music']);
  expect(items.slice(3).every((it) => it.sub)).toBe(true);
  // the root has no ".." and an empty folder no separator
  expect(folderItems('/', '', [])).toEqual([{ label: 'Go to /', value: { go: '/' } }]);
});

test('isoDate and groupDigits do not depend on the language', () => {
  const ms = new Date(2026, 0, 5, 7, 3).getTime();
  expect(isoDate(ms)).toBe('2026-01-05 07:03');
  expect(isoDate(0)).toBe('');
  expect(isoDate(NaN)).toBe('');
  expect(groupDigits(1234567)).toBe('1 234 567');
  expect(groupDigits(999)).toBe('999');
  expect(groupDigits(undefined)).toBe('0');
});

test('fileListText lists the entries in panel order with a summary', () => {
  const t = new Date(2026, 10, 15, 9, 30).getTime();
  const text = fileListText({
    path: '/Users/me/Docs', when: t, filter: '*.txt',
    entries: [
      { name: '..', up: true, dir: true },
      { name: 'Old', dir: true, mtime: t },
      { name: 'a.txt', size: 1200, mtime: t },
      { name: 'b.txt', size: 5, mtime: t },
    ],
  });
  const lines = text.split('\n');
  expect(lines[0]).toBe('Folder: /Users/me/Docs');
  expect(lines[1]).toBe('Printed: 2026-11-15 09:30   Filter: *.txt');
  expect(lines[3]).toMatch(/^Name\s+Size {2}Date$/);
  expect(lines[5]).toBe('Old    <DIR>  2026-11-15 09:30');
  expect(lines[6]).toBe('a.txt  1 200  2026-11-15 09:30');
  expect(lines[7]).toBe('b.txt      5  2026-11-15 09:30');
  expect(text).not.toContain('..');
  expect(lines.at(-2)).toBe('2 files (1 205 bytes), 1 folder');
  expect(text.endsWith('\n')).toBe(true);
  expect(fileListText({ path: '/e', entries: [], when: t }).split('\n').at(-2)).toBe('0 files (0 bytes), 0 folders');
});

test('subEntries turns a branch listing into printable entries, sorted by path (cm_PrintDirSub, F-334)', () => {
  const list = subEntries({ entries: [['sub/b.txt', 5, 2, 0], ['..', 0, 0, 1], ['a.txt', 7, 1, 0], ['sub/deep', 0, 3, 1]] });
  expect(list.map((e) => e.name)).toEqual(['a.txt', 'sub/b.txt', 'sub/deep']);
  expect(list[0]).toEqual({ name: 'a.txt', size: 7, mtime: 1, dir: false });
  expect(list[2].dir).toBe(true);
  expect(subEntries(null)).toEqual([]);
  expect(fileListText({ path: '/x', entries: list, when: 0 }).split('\n').at(-2)).toBe('2 files (12 bytes), 1 folder');
});
