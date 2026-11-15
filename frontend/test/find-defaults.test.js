// Tests of features/find-defaults.js (board issue #94): with no last search, a new Find files search starts with the
// fields Options > Find files sets, and with the program's own defaults (which are also the options' defaults) when
// the options are not there.
import { expect, test } from 'vitest';
import { findDefaults } from '../src/features/find-defaults.js';
import { findIncludeHidden } from '../src/core/hidden.js';

test('without options a search starts as it always did', () => {
  expect(findDefaults(() => undefined)).toEqual({
    searchFor: '*', depth: -1, archives: false, caseSensitive: false, textRegex: false, skipHidden: false, followLinks: false,
  });
  expect(findDefaults(() => '')).toEqual(findDefaults(() => undefined));
});

test('the options fill the fields', () => {
  const values = {
    'FindFiles.Depth': '2', 'FindFiles.Archives': '1', 'FindFiles.CaseSensitive': '1', 'FindFiles.RegEx': '1',
    'FindFiles.Hidden': '0', 'FindFiles.FollowLinks': '1',
  };
  expect(findDefaults((n) => values[n])).toEqual({
    searchFor: '*', depth: 2, archives: true, caseSensitive: true, textRegex: true, skipHidden: true, followLinks: true,
  });
  expect(findDefaults((n) => (n === 'FindFiles.Depth' ? '500' : undefined)).depth).toBe(99);
});

test('"Search hidden files" counts only when there is no last search', () => {
  expect(findIncludeHidden({ last: null, fresh: false })).toBe(false);
  expect(findIncludeHidden({ last: { skipHidden: false }, fresh: false })).toBe(true);
  expect(findIncludeHidden({ last: null, fresh: false, followPanel: true, showHidden: true })).toBe(true);
});
