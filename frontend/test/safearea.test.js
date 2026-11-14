// Tests of core/safearea.js: telling full screen from the window size.
import { expect, test } from 'vitest';
import { isFullscreen } from '../src/core/safearea.js';

test('a window that fills the screen is full screen', () => {
  expect(isFullscreen(1728, 1117, 1728, 1117)).toBe(true);
  expect(isFullscreen(1727.5, 1117, 1728, 1117)).toBe(true);
  expect(isFullscreen(1728, 1080, 1728, 1117)).toBe(false); // maximised (zoomed), menu bar still there
  expect(isFullscreen(1400, 900, 1728, 1117)).toBe(false);
});
