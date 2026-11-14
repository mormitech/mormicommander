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

// ---- name order options (F-116) and Unicode forms (F-754) ----

import { codeCompare, nameComparer, validLocale } from '../src/core/sort.js';

const nfd = (s) => s.normalize('NFD');

test('a name in decomposed form (NFD, from Finder) sorts like the composed one', () => {
  const hu = ['Zoltán', nfd('Ágnes'), 'Adél', 'Agota', 'Béla', nfd('Éva'), 'Edit'].map((n) => f(n));
  for (const method of ['', 'case', 'code']) {
    const got = names(sortEntries(hu, { key: 'name', desc: false }, { method })).map((n) => n.normalize('NFC'));
    const want = names(sortEntries(hu.map((e) => f(e.name.normalize('NFC'))), { key: 'name', desc: false }, { method }));
    expect(got).toEqual(want);
  }
  expect(names(sortEntries(hu)).map((n) => n.normalize('NFC'))).toEqual(['Adél', 'Ágnes', 'Agota', 'Béla', 'Edit', 'Éva', 'Zoltán']);
});

test('the default method is the order the app always had', () => {
  const plain = sortEntries(list, { key: 'name', desc: false }, { natural: true });
  expect(names(sortEntries(list, { key: 'name', desc: false }, { natural: true, method: '', locale: '' }))).toEqual(names(plain));
});

test('case-sensitive: alphabetical first, upper case before lower case of the same letter', () => {
  const l = ['banana', 'Apple', 'apple', 'Banana'].map((n) => f(n));
  expect(names(sortEntries(l, { key: 'name', desc: false }, { method: 'case' }))).toEqual(['Apple', 'apple', 'Banana', 'banana']);
});

test('by character code: no language rules, case ignored, natural numbers still work', () => {
  const l = ['Zebra', 'Äpfel', 'apfel', 'file10', 'file2'].map((n) => f(n));
  expect(names(sortEntries(l, { key: 'name', desc: false }, { method: 'code' }))).toEqual(['apfel', 'file10', 'file2', 'Zebra', 'Äpfel']);
  expect(names(sortEntries(l, { key: 'name', desc: false }, { method: 'code', natural: true })).slice(1, 3)).toEqual(['file2', 'file10']);
  expect(codeCompare('A', 'a')).toBe(0);
  expect(codeCompare('file007', 'file7', true)).toBe(1); // equal values: fewer leading zeros first
  expect(codeCompare('x\u{1F600}', 'x�')).toBe(1); // astral characters compare by code point, not UTF-16 unit
});

test('a language picks its alphabet: Hungarian puts "cs" after "cu"', () => {
  const cmp = nameComparer({ locale: 'hu' });
  expect(['dob', 'csiga', 'cukor'].sort(cmp)).toEqual(['cukor', 'csiga', 'dob']);
  expect(nameComparer({ locale: 'not a locale!' })('a', 'b')).toBe(-1); // a bad language falls back to the system's
  expect(validLocale('hu')).toBe(true);
  expect(validLocale('not a locale!')).toBe(false);
});
