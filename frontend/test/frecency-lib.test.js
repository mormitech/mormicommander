// The frecency jump's pure helpers (src/features/frecency-lib.js, F-867, board issue #27): typed words match folders the
// way zoxide matches them, the last word may be fuzzy within the folder's name, and Go's ranked list becomes popup items
// in the same order. Also the popup's match/query options the jump relies on (core/popup.js).
import { describe, expect, test } from 'vitest';
import { frecencyMatch, frecencyItems, frecencyItemMatch, lastName } from '../src/features/frecency-lib.js';

describe('frecencyMatch', () => {
  test('the last word must be in the folder\'s own name', () => {
    expect(frecencyMatch('/home/a/projects/mormi', 'mor')).toBe(2);
    expect(frecencyMatch('/home/a/projects/mormi', 'proj')).toBe(0);
    expect(frecencyMatch('/home/a/projects/mormi/', 'MORMI')).toBe(2);
    expect(frecencyMatch('C:\\Users\\a\\Downloads', 'down')).toBe(2);
  });
  test('earlier words appear in order anywhere in the path', () => {
    expect(frecencyMatch('/home/a/projects/mormi/src', 'proj src')).toBe(2);
    expect(frecencyMatch('/home/a/projects/mormi/src', 'src proj')).toBe(0);
    expect(frecencyMatch('/home/a/projects/mormi/src', 'nope src')).toBe(0);
    expect(frecencyMatch('/a/src_main', 'src main')).toBe(2);
  });
  test('the last word can be fuzzy', () => {
    expect(frecencyMatch('/Users/a/Downloads', 'dwnl')).toBe(1);
    expect(frecencyMatch('/Users/a/Downloads', 'dlw')).toBe(0);
  });
  test('an empty query matches everything', () => {
    expect(frecencyMatch('/x', '   ')).toBe(2);
  });
  test('lastName', () => {
    expect(lastName('/a/b/')).toBe('b');
    expect(lastName('C:\\x\\y')).toBe('y');
    expect(lastName('ftp://host/dir')).toBe('dir');
    expect(lastName('plain')).toBe('plain');
  });
});

describe('frecencyItems', () => {
  const list = [{ path: '/a', source: 'app' }, { path: '/b', source: 'zoxide' }, { path: '/c', source: 'both' }];
  test('keeps the order, marks zoxide-only folders, hides the tail until typing', () => {
    const items = frecencyItems(list, 2);
    expect(items.map((i) => i.value)).toEqual(['/a', '/b', '/c']);
    expect(items.map((i) => i.detail)).toEqual(['', 'zoxide', '']);
    expect(items.map((i) => i.only)).toEqual([false, false, true]);
    expect(items.every((i) => i.path)).toBe(true);
    expect(frecencyItems(null)).toEqual([]);
  });
  test('the popup match leaves out rows that are not folders', () => {
    expect(frecencyItemMatch({ label: '/x/docs', path: true }, 'doc')).toBe(true);
    expect(frecencyItemMatch({ label: 'Include zoxide\'s folders' }, 'zox')).toBe(false);
  });
});
