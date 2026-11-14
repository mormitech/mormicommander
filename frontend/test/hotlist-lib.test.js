// The folder hotlist's pure helpers (src/features/hotlist-lib.js, board issue #111): a mask entry ("cd folder/*.txt")
// keeps matching files that appear after it was chosen, and the typed filter of the popup can find entries that sit in
// submenus.
import { describe, expect, test } from 'vitest';
import { hotlistMaskFilter, nestedEntries } from '../src/features/hotlist-lib.js';

describe('hotlistMaskFilter', () => {
  test('files match the mask, folders always stay', () => {
    const f = hotlistMaskFilter('*.txt | b*');
    expect(f({ name: 'a.txt' })).toBe(true);
    expect(f({ name: 'b.txt' })).toBe(false);
    expect(f({ name: 'c.md' })).toBe(false);
    expect(f({ name: 'docs', dir: true })).toBe(true);
    expect(f({ name: '..', up: true })).toBe(true);
    expect(f.mcMask).toBe('*.txt | b*');
    expect(f.mcSrc).toBeUndefined(); // (not the Show > Custom filter)
  });
  test('a file that appears later is judged by its name, not by an earlier listing', () => {
    const f = hotlistMaskFilter('*.TXT');
    const before = [{ name: 'a.txt' }, { name: 'x.md' }].filter(f).map((e) => e.name);
    const after = [{ name: 'a.txt' }, { name: 'x.md' }, { name: 'new.txt' }].filter(f).map((e) => e.name);
    expect(before).toEqual(['a.txt']);
    expect(after).toEqual(['a.txt', 'new.txt']);
  });
});

describe('nestedEntries', () => {
  const tree = [
    { kind: 'cmd', title: 'Top', cmd: 'cd /top' },
    { kind: 'sep' },
    { kind: 'menu', title: 'Work', items: [
      { kind: 'cmd', title: 'Src', cmd: 'cd /src' },
      { kind: 'menu', title: 'Deep', items: [{ kind: 'cmd', label: 'Inner', cmd: 'cd /in' }, { kind: 'sep' }] },
    ] },
    { kind: 'menu', title: 'Empty' },
  ];
  test('lists the entries inside submenus with their path, not the level\'s own', () => {
    expect(nestedEntries(tree).map((x) => [x.crumbs, x.it.cmd])).toEqual([
      [['Work'], 'cd /src'],
      [['Work', 'Deep'], 'cd /in'],
    ]);
  });
  test('nothing to list', () => {
    expect(nestedEntries([])).toEqual([]);
    expect(nestedEntries(undefined)).toEqual([]);
  });
});
