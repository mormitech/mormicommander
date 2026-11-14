// Tests of the selection a copy or move leaves behind (features/fileops-sel.js, issue #132): only the sources that
// did not get across stay selected; jobs without the engine's "left" list keep the older all-or-nothing rule.
import { describe, expect, test } from 'vitest';
import { copiedNames } from '../src/features/fileops-sel.js';

const names = ['r1.txt', 'r2.txt', 'r3.txt'];
const sources = names.map((n) => `/a/${n}`);

describe('copiedNames', () => {
  test('skipped sources stay selected', () => {
    const final = { state: 'done', skipped: 2, errors: [], left: ['/a/r2.txt', '/a/r3.txt'] };
    expect(copiedNames(final, sources, names)).toEqual(['r1.txt']);
  });
  test('failed sources stay selected', () => {
    const final = { state: 'failed', skipped: 0, errors: ['x'], left: ['/a/r1.txt'] };
    expect(copiedNames(final, sources, names)).toEqual(['r2.txt', 'r3.txt']);
  });
  test('a clean job unselects all', () => {
    expect(copiedNames({ state: 'done', skipped: 0, errors: [], left: [] }, sources, names)).toEqual(names);
  });
  test('a cancelled job keeps the selection', () => {
    expect(copiedNames({ state: 'cancelled', left: [] }, sources, names)).toEqual([]);
  });
  test('without a left list: all or nothing', () => {
    expect(copiedNames({ state: 'done', skipped: 0, errors: [] }, sources, names)).toEqual(names);
    expect(copiedNames({ state: 'done', skipped: 1, errors: [] }, sources, names)).toEqual([]);
    expect(copiedNames({ state: 'failed', skipped: 0, errors: ['x'], left: null }, sources, names)).toEqual([]);
  });
});
