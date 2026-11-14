// Tests of features/cmdline-complete-lib.js: the rules of Tab completion in the command line.
import { expect, test } from 'vitest';
import { wordAt, splitWord, matchEntries, writeMatch } from '../src/features/cmdline-complete-lib.js';

test('the word at the caret starts after the last space outside quotes', () => {
  expect(wordAt('cd Pu', 5)).toMatchObject({ start: 3, word: 'Pu', quoted: false });
  expect(wordAt('cd "Virtual Ma', 14)).toMatchObject({ start: 3, word: 'Virtual Ma', quoted: true });
  expect(wordAt('cd ', 3)).toMatchObject({ start: 3, word: '' });
  expect(wordAt('open a b', 6)).toMatchObject({ start: 5, word: 'a' });
});

test('a word splits into its folder part and the name start', () => {
  expect(splitWord('Code/mor')).toEqual({ dir: 'Code/', name: 'mor' });
  expect(splitWord('mor')).toEqual({ dir: '', name: 'mor' });
  expect(splitWord('C:\\Users\\jp', true)).toEqual({ dir: 'C:\\Users\\', name: 'jp' });
  expect(splitWord('a\\b', false)).toEqual({ dir: '', name: 'a\\b' });
});

const E = (name, dir = false, hidden = false) => ({ name, dir, hidden });
test('matches: name start, folders first, hidden only when shown or asked for', () => {
  const es = [E('..', true), E('pub.txt'), E('Public', true), E('Pictures', true), E('.profile', false, true), E('purge')];
  expect(matchEntries(es, 'Pu').map((e) => e.name)).toEqual(['Public', 'pub.txt', 'purge']);
  expect(matchEntries(es, 'Pu', { caseless: false }).map((e) => e.name)).toEqual(['Public']);
  expect(matchEntries(es, '.p').map((e) => e.name)).toEqual(['.profile']);
  expect(matchEntries(es, '').some((e) => e.name === '.profile' || e.name === '..')).toBe(false);
  expect(matchEntries(es, '', { showHidden: true }).some((e) => e.name === '.profile')).toBe(true);
});

test('a match is written with a separator after a folder and quotes when needed', () => {
  expect(writeMatch('', E('Public', true))).toBe('Public/');
  expect(writeMatch('Code/', E('a.txt'))).toBe('Code/a.txt');
  expect(writeMatch('', E('Virtual Machines', true))).toBe('"Virtual Machines/"');
  expect(writeMatch('', E('x', true), { quoted: true })).toBe('"x/"');
  expect(writeMatch('C:\\', E('Program Files', true), { sep: '\\', windows: true })).toBe('"C:\\Program Files\\"');
});
