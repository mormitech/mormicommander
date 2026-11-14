// Tests of core/session.js: the session is written a moment after a change, staged on the Go side at once, and a
// waiting change can be written right away before quitting (board issue #131).
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { sessionSaver } from '../src/core/session.js';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

test('a run of changes is written once, after the delay, with the last state', () => {
  const written = [];
  const s = sessionSaver({ write: (v) => written.push(v) });
  s.schedule(() => 'a');
  s.schedule(() => 'b');
  vi.advanceTimersByTime(399);
  expect(written).toEqual([]);
  vi.advanceTimersByTime(1);
  expect(written).toEqual(['b']);
  expect(s.pending).toBe(false);
});

test('every change is staged at once', () => {
  const staged = [];
  const s = sessionSaver({ write: () => {}, stage: (v) => staged.push(v) });
  s.schedule(() => 1);
  s.schedule(() => 2);
  expect(staged).toEqual([1, 2]);
});

test('flush writes a waiting change now and only once', async () => {
  const written = [];
  const s = sessionSaver({ write: (v) => { written.push(v); return Promise.resolve('ok'); } });
  expect(s.flush()).toBeUndefined();
  s.schedule(() => 'sorted by size');
  expect(await s.flush()).toBe('ok');
  expect(written).toEqual(['sorted by size']);
  vi.advanceTimersByTime(1000);
  expect(written).toEqual(['sorted by size']);
});

test('a failing stage does not stop the write', () => {
  const written = [];
  const s = sessionSaver({ write: (v) => written.push(v), stage: () => { throw new Error('no Go'); } });
  s.schedule(() => 'x');
  vi.advanceTimersByTime(400);
  expect(written).toEqual(['x']);
});
