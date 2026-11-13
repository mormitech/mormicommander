// Tests of core/hidden.js: the status-line marker, the starting state of Find files' "Include hidden files" and the
// menu label with its key (board issue #4).
import { expect, test } from 'vitest';
import { HIDDEN_MARK, hiddenMarkHTML, findIncludeHidden, menuLabel } from '../src/core/hidden.js';

test('the marker shows only while hidden files are shown', () => {
  expect(hiddenMarkHTML(false)).toBe('');
  expect(hiddenMarkHTML(true)).toContain(HIDDEN_MARK);
  expect(hiddenMarkHTML(true)).toContain('class="mc-hidden-mark"');
  expect(HIDDEN_MARK).toBe('Hidden: shown');
});

test('by default a new search includes hidden files as the last search did', () => {
  expect(findIncludeHidden()).toBe(true);
  expect(findIncludeHidden({ last: null, showHidden: false })).toBe(true);
  expect(findIncludeHidden({ last: { skipHidden: false }, showHidden: false })).toBe(true);
  expect(findIncludeHidden({ last: { skipHidden: true }, showHidden: true })).toBe(false);
});

test('with the option on, a new search follows the panels', () => {
  expect(findIncludeHidden({ followPanel: true, showHidden: true, last: { skipHidden: true } })).toBe(true);
  expect(findIncludeHidden({ followPanel: true, showHidden: false, last: { skipHidden: false } })).toBe(false);
});

test('menu labels carry the key when there is one', () => {
  expect(menuLabel('Hidden files', 'Ctrl+H')).toBe('Hidden files (Ctrl+H)');
  expect(menuLabel('Hidden files', '⇧⌘.')).toBe('Hidden files (⇧⌘.)');
  expect(menuLabel('Hidden files', '')).toBe('Hidden files');
});
