// Tests of core/histfocus.js: moving back and forward through the folder history puts the cursor on the entry it
// was on when the folder was left, or on the subfolder just left, instead of on ".." (board issue #126).
import { expect, test } from 'vitest';
import { historyFocus } from '../src/core/histfocus.js';

test('going back to a folder brings back its last cursor entry', () => {
  const last = new Map([['/A', 'c.log']]);
  expect(historyFocus('/A', '/', last)).toBe('c.log');
  expect(historyFocus('/A', '/A/sub2', last)).toBe('c.log');
});

test('without a remembered entry, going back to the parent lands on the folder left', () => {
  expect(historyFocus('/A', '/A/sub2', new Map())).toBe('sub2');
  expect(historyFocus('/A', '/A/sub2/deeper/x', new Map())).toBe('sub2');
  expect(historyFocus('/', '/A', new Map())).toBe('A');
  expect(historyFocus('C:\\', 'C:\\Users\\me', new Map(), '\\')).toBe('Users');
  expect(historyFocus('C:\\Users', 'C:\\Users\\me', null, '\\')).toBe('me');
});

test('an unrelated folder with nothing remembered keeps the default', () => {
  expect(historyFocus('/A', '/B', new Map())).toBe(null);
  expect(historyFocus('/A', '/AB/x', new Map())).toBe(null);
  expect(historyFocus('/A/sub2', '/A', new Map())).toBe(null);
  expect(historyFocus('/A', '/A', new Map())).toBe(null);
});
