// Tests of core/progresshold.js: a job's progress box must not open on top of a question the job asks before the box
// is due (overwrite, folder not empty, read-only, a server transfer's overwrite): it waits until the question is
// answered (board issue #123).
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { holdProgress, progressHeld, showWhenFree } from '../src/core/progresshold.js';

beforeEach(() => { vi.useFakeTimers(); });
afterEach(() => { vi.useRealTimers(); });

test('without a question the box opens after the delay', () => {
  const show = vi.fn();
  showWhenFree(show, 400);
  vi.advanceTimersByTime(399);
  expect(show).not.toHaveBeenCalled();
  vi.advanceTimersByTime(1);
  expect(show).toHaveBeenCalledTimes(1);
});

test('a question asked before the box is due keeps it closed until it is answered', () => {
  const show = vi.fn();
  const release = holdProgress(); // the job asks at once ("Folder is not empty")
  showWhenFree(show, 400);
  vi.advanceTimersByTime(5000);
  expect(progressHeld()).toBe(true);
  expect(show).not.toHaveBeenCalled();
  release();
  release(); // a second call changes nothing
  expect(progressHeld()).toBe(false);
  vi.advanceTimersByTime(300);
  expect(show).toHaveBeenCalledTimes(1);
  vi.advanceTimersByTime(5000);
  expect(show).toHaveBeenCalledTimes(1);
});

test('a question asked just after the job starts still wins the race with the timer', () => {
  const show = vi.fn();
  showWhenFree(show, 400);
  vi.advanceTimersByTime(200);
  const release = holdProgress();
  vi.advanceTimersByTime(1000);
  expect(show).not.toHaveBeenCalled();
  release();
  vi.advanceTimersByTime(300);
  expect(show).toHaveBeenCalledTimes(1);
});

test('two questions open at once both have to be answered', () => {
  const show = vi.fn();
  const a = holdProgress(), b = holdProgress();
  showWhenFree(show, 400);
  a();
  vi.advanceTimersByTime(1000);
  expect(show).not.toHaveBeenCalled();
  b();
  vi.advanceTimersByTime(300);
  expect(show).toHaveBeenCalledTimes(1);
});

test('a job that ends while its question is open never opens the box', () => {
  const show = vi.fn();
  const release = holdProgress();
  const cancel = showWhenFree(show, 400);
  vi.advanceTimersByTime(700);
  cancel();
  release();
  vi.advanceTimersByTime(5000);
  expect(show).not.toHaveBeenCalled();
});
