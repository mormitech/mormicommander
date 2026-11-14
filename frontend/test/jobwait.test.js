// Tests of core/jobwait.js: a server transfer that ends before StartJob returns its number (a same-server rename, a
// tiny upload) must still resolve its wait, so the progress box closes and the app is not busy for good (board issue
// #140).
import { expect, test, vi } from 'vitest';
import { jobWaiter } from '../src/core/jobwait.js';

test('a done event that comes before wait() resolves the wait at once', async () => {
  const w = jobWaiter();
  w.done({ id: 7, state: 'done' }); // the job ended while StartJob was still returning
  const close = vi.fn();
  const final = await w.wait(7, { close });
  expect(final).toEqual({ id: 7, state: 'done' });
  expect(close).not.toHaveBeenCalled(); // no box was opened yet
  expect(w.get(7)).toBeUndefined();
});

test('a done event after wait() closes the box and resolves', async () => {
  const w = jobWaiter();
  const update = vi.fn(), close = vi.fn();
  const pending = w.wait(3, { update, close });
  w.get(3).update({ id: 3, state: 'running' });
  expect(update).toHaveBeenCalledTimes(1);
  w.done({ id: 3, state: 'failed' });
  expect(await pending).toEqual({ id: 3, state: 'failed' });
  expect(close).toHaveBeenCalledTimes(1);
  expect(w.get(3)).toBeUndefined();
});

test('an early end is used once, and only for its own job', async () => {
  const w = jobWaiter();
  w.done({ id: 1, state: 'done' });
  expect(await w.wait(1)).toEqual({ id: 1, state: 'done' });
  let resolved = false;
  w.wait(1).then(() => { resolved = true; });
  w.wait(2).then(() => { resolved = true; });
  await Promise.resolve();
  expect(resolved).toBe(false);
});

test('only the last ends nobody waits for are kept', async () => {
  const w = jobWaiter();
  for (let i = 1; i <= 60; i++) w.done({ id: i, state: 'done' });
  expect(await w.wait(60)).toEqual({ id: 60, state: 'done' });
  let first = false;
  w.wait(1).then(() => { first = true; });
  await Promise.resolve();
  expect(first).toBe(false);
});
