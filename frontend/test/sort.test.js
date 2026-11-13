import { expect, test } from 'vitest';
import { sortEntries } from '../src/core/sort.js';
import { splitExt } from '../src/core/format.js';

const f = (name, size = 0, mtime = 0) => ({ name, ext: splitExt(name)[1], size, mtime, dir: false });
const d = (name, mtime = 0) => ({ name, ext: '', size: 0, mtime, dir: true });
const up = { name: '..', up: true, dir: true };
const names = (es) => es.map((e) => e.name);

const list = [f('b.txt', 30, 3), f('A.log', 10, 1), d('zdir', 5), up, f('c', 20, 2), d('adir', 9), f('file10.txt', 1, 4), f('file2.txt', 1, 4)];

test('".." first, folders before files, by name, case-insensitive', () => {
  expect(names(sortEntries(list))).toEqual(['..', 'adir', 'zdir', 'A.log', 'b.txt', 'c', 'file10.txt', 'file2.txt']);
});

test('natural order puts file2 before file10', () => {
  expect(names(sortEntries(list, { key: 'name', desc: false }, { natural: true })).slice(-2)).toEqual(['file2.txt', 'file10.txt']);
});

test('descending name reverses folders and files, ".." stays first', () => {
  expect(names(sortEntries(list, { key: 'name', desc: true }))).toEqual(['..', 'zdir', 'adir', 'file2.txt', 'file10.txt', 'c', 'b.txt', 'A.log']);
});

test('extension: no extension first, then by name', () => {
  expect(names(sortEntries(list, { key: 'ext', desc: false })).slice(3)).toEqual(['c', 'A.log', 'b.txt', 'file10.txt', 'file2.txt']);
});

test('size: largest first; equal sizes by name ascending; folders by name', () => {
  expect(names(sortEntries(list, { key: 'size', desc: true }, { natural: true }))).toEqual(['..', 'adir', 'zdir', 'b.txt', 'c', 'A.log', 'file2.txt', 'file10.txt']);
  expect(names(sortEntries(list, { key: 'size', desc: false }, { natural: true })).slice(3)).toEqual(['file2.txt', 'file10.txt', 'A.log', 'c', 'b.txt']);
});

test('date: newest first; folders by name unless dirsLikeFiles', () => {
  expect(names(sortEntries(list, { key: 'date', desc: true }, { natural: true }))).toEqual(['..', 'adir', 'zdir', 'file2.txt', 'file10.txt', 'b.txt', 'c', 'A.log']);
  expect(names(sortEntries(list, { key: 'date', desc: true }, { dirsLikeFiles: true })).slice(1, 3)).toEqual(['adir', 'zdir']);
  expect(names(sortEntries([d('old', 1), d('new', 9)], { key: 'date', desc: true }, { dirsLikeFiles: true }))).toEqual(['new', 'old']);
});

test('unsorted keeps the order, folders still first', () => {
  expect(names(sortEntries(list, { key: 'none' }))).toEqual(['..', 'zdir', 'adir', 'b.txt', 'A.log', 'c', 'file10.txt', 'file2.txt']);
});
