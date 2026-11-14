// Tests of core/keyrepeat.js: the faster repeat of held Up/Down keys.
import { afterEach, expect, test, vi } from 'vitest';
import { takeRepeat, stopRepeat, repeatInterval } from '../src/core/keyrepeat.js';

afterEach(() => { stopRepeat(); vi.useRealTimers(); });

test('the speeds', () => {
  expect(repeatInterval(0)).toBe(0);
  expect(repeatInterval('1')).toBe(30);
  expect(repeatInterval(3)).toBe(10);
  expect(repeatInterval(undefined)).toBe(0);
});

test('with the system setting (0) or a key that is not repeated, nothing is taken', () => {
  expect(takeRepeat({ repeat: true, code: 'ArrowDown' }, 'Down', '0', () => {})).toBe(false);
  expect(takeRepeat({ repeat: true, code: 'KeyA' }, 'A', '2', () => {})).toBe(false);
  expect(takeRepeat({ repeat: false, code: 'ArrowDown' }, 'Down', '2', () => {})).toBe(false);
});

test('from the first system repeat on, the command runs at our pace and the system repeats are swallowed', async () => {
  vi.useFakeTimers();
  let n = 0;
  const step = () => { n++; };
  expect(takeRepeat({ repeat: true, code: 'ArrowDown' }, 'Down', '1', step)).toBe(true);
  await vi.advanceTimersByTimeAsync(0);
  expect(n).toBe(1);
  await vi.advanceTimersByTimeAsync(300);
  expect(n).toBe(11);
  expect(takeRepeat({ repeat: true, code: 'ArrowDown' }, 'Down', '1', step)).toBe(true); // the system's own repeat
  await vi.advanceTimersByTimeAsync(0);
  expect(n).toBe(11);
  stopRepeat();
  await vi.advanceTimersByTimeAsync(300);
  expect(n).toBe(11);
});
