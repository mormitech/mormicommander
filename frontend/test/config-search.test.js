// Tests of features/config/search.js (board issue #109): the Options search matches any part of a word by default, as
// it always did, and only whole words when "Whole words" is ticked ("two" then no longer finds "network").
import { expect, test } from 'vitest';
import { containsWord, matchesAll, searchWords } from '../src/features/config/search.js';

test('by default a search word may be part of a longer word', () => {
  expect(containsWord('refresh network folders', 'two')).toBe(true);
  expect(matchesAll('Refresh network folders', ['two'])).toBe(true);
});

test('whole words leave out matches inside longer words', () => {
  expect(containsWord('refresh network folders', 'two', true)).toBe(false);
  expect(containsWord('two drive button bars', 'two', true)).toBe(true);
  expect(containsWord('bars two', 'two', true)).toBe(true);
  expect(containsWord('layout.two-panel', 'two', true)).toBe(true);
  expect(containsWord('two2', 'two', true)).toBe(false);
  expect(containsWord('network and two', 'two', true)).toBe(true); // a later place still counts
});

test('letters beyond plain ASCII count as parts of words', () => {
  expect(containsWord('zwei über', 'über', true)).toBe(true);
  expect(containsWord('drüber', 'über', true)).toBe(false);
  expect(containsWord('a𝒜two', 'two', true)).toBe(false); // (a letter outside the BMP)
});

test('matchesAll needs every word, ignoring case', () => {
  const words = searchWords('  Drive   TWO ');
  expect(words).toEqual(['drive', 'two']);
  expect(matchesAll('Two drive button bars', words, true)).toBe(true);
  expect(matchesAll('Network drive', words, true)).toBe(false);
  expect(matchesAll('Network drive', words, false)).toBe(true);
  expect(searchWords('')).toEqual([]);
  expect(matchesAll('anything', [], true)).toBe(true);
});
