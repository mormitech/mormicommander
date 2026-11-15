// @vitest-environment happy-dom
// Defaults behind options wired in board issue #94: the command line history size (core/cmdline.js, Options >
// Directory hotlist > Command line history size) is 50 when nothing is set, as it always was, and from 1 to 500; the
// Lister's image fit (viewer/lister-defaults.js, Options > Lister > Image size) shrinks large images and enlarges none.
import { expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});
import { cmdHistoryMax } from '../src/core/cmdline.js';
import { LISTER_DEFAULTS } from '../src/features/viewer/lister-defaults.js';

test('the command line keeps 50 commands unless the option says otherwise', () => {
  expect(cmdHistoryMax(undefined)).toBe(50);
  expect(cmdHistoryMax('x')).toBe(50);
  expect(cmdHistoryMax(10)).toBe(10);
  expect(cmdHistoryMax(0)).toBe(1);
  expect(cmdHistoryMax(9999)).toBe(500);
});

test('the Lister shrinks large images and enlarges none by default', () => {
  expect(LISTER_DEFAULTS.fitImage).toBe(true);
  expect(LISTER_DEFAULTS.fitGrow).toBe(false);
});
