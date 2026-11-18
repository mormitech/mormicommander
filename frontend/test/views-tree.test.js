// Refresh folder tree (cm_RescanTree, F-369, issue #62): the pure merge of a fresh folder read into the nodes of the tree
// in the panel (src/features/views-tree.js), and where the cursor goes when its folder is gone.
import { expect, test } from 'vitest';
import { mergeKids, cursorAfter } from '../src/features/views-tree.js';

const node = (path, extra = {}) => ({ name: path.split('/').pop() || path, path, depth: 1, open: false, kids: null, ...extra });

test('a new folder shows up, a deleted one goes, and an open folder stays open with its children', () => {
  const root = { name: '/', path: '/', depth: 0, open: true, kids: null, parentNode: null };
  const grand = node('/a/x', { depth: 2 });
  const a = node('/a', { open: true, kids: [grand], parentNode: root });
  root.kids = [a, node('/gone', { parentNode: root })];
  const kids = mergeKids(root, [{ name: 'a', path: '/a' }, { name: 'new', path: '/new' }]);
  expect(kids.map((k) => k.path)).toEqual(['/a', '/new']);
  expect(kids[0]).toBe(a);
  expect(kids[0].open).toBe(true);
  expect(kids[0].kids).toEqual([grand]);
  expect(kids[1]).toMatchObject({ depth: 1, open: false, kids: null, parentNode: root });
});

test('a folder that became a file (or the other way round) gets a fresh node; files have no children', () => {
  const root = { path: '/', depth: 0, kids: [node('/a', { open: true, kids: [] })] };
  const kids = mergeKids(root, [{ name: 'a', path: '/a', file: true }]);
  expect(kids[0]).toMatchObject({ file: true, open: false, kids: [] });
});

test('on Windows names match whatever their case', () => {
  const low = (s) => s.toLowerCase();
  const old = node('C:\\Data', { open: true, kids: [] });
  const root = { path: 'C:\\', depth: 0, kids: [old] };
  const kids = mergeKids(root, [{ name: 'DATA', path: 'C:\\DATA' }], low);
  expect(kids[0]).toBe(old);
  expect(kids[0].name).toBe('DATA');
});

test('the cursor stays on its folder, or goes to the nearest folder above it that is left', () => {
  const nodes = [{ path: '/' }, { path: '/a' }, { path: '/a/b' }, { path: '/c' }];
  expect(cursorAfter(nodes, '/c')).toBe(3);
  expect(cursorAfter(nodes, '/a/b/gone/deeper')).toBe(2);
  expect(cursorAfter(nodes, '/a/gone')).toBe(1);
  expect(cursorAfter(nodes, '/zz')).toBe(0);
  expect(cursorAfter([], '/a')).toBe(0);
  const win = [{ path: 'C:\\' }, { path: 'C:\\Data' }];
  expect(cursorAfter(win, 'C:\\Data\\Gone')).toBe(1);
  expect(cursorAfter(win, 'C:\\Other')).toBe(0);
});
