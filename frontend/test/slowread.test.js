// Tests of core/slowread.js: when the "Not responding, retrying…" notice shows over a panel whose folder read hangs
// (issue #16), and what it says.
import { expect, test } from 'vitest';
import { SLOW_MS, slowStep, slowText } from '../src/core/slowread.js';

test('the notice shows after 3 seconds of reading, not before', () => {
  expect(SLOW_MS).toBeLessThanOrEqual(3000);
  let r = slowStep(null, true, 1000);
  expect(r).toEqual({ since: 1000, show: false });
  r = slowStep(r.since, true, 1000 + SLOW_MS - 1);
  expect(r.show).toBe(false);
  r = slowStep(r.since, true, 1000 + SLOW_MS);
  expect(r).toEqual({ since: 1000, show: true });
});

test('the notice goes away when the read ends, and the next read starts the clock again', () => {
  let r = slowStep(0, true, 5000);
  expect(r.show).toBe(true);
  r = slowStep(r.since, false, 5100);
  expect(r).toEqual({ since: null, show: false });
  r = slowStep(r.since, true, 9000);
  expect(r).toEqual({ since: 9000, show: false });
});

test('the words name the folder the panel stays in', () => {
  expect(slowText('/Users/me').title).toBe('Not responding, retrying…');
  expect(slowText('/Users/me').hint).toContain('/Users/me');
  expect(slowText('').hint).toBe('Esc or Cancel stops waiting.');
});
