// Tests for the error question's buttons (issue #36): the "without Finder" buttons appear only when the engine offers
// them (a failed move to the macOS Trash), and an ordinary error keeps the buttons it always had.
import { expect, test } from 'vitest';
import { ERROR_BUTTONS, offeredButtons } from '../src/core/jobchoices.js';

const labels = (choices) => offeredButtons(ERROR_BUTTONS, choices).map(([l]) => l);

test('an ordinary error shows the usual buttons', () => {
  expect(labels(['retry', 'skip', 'skip_all', 'cancel'])).toEqual(['Retry', 'Skip', 'Skip all', 'Cancel']);
});

test('a failed trash without a Finder-free route offers the permanent delete', () => {
  expect(labels(['retry', 'skip', 'skip_all', 'delete', 'cancel'])).toEqual(['Retry', 'Skip', 'Skip all', 'Delete permanently', 'Cancel']);
});

test('a failed Finder trash also offers the route without Finder, in a fixed order', () => {
  expect(labels(['retry', 'skip', 'skip_all', 'trash_direct', 'trash_direct_all', 'delete', 'cancel']))
    .toEqual(['Retry', 'Skip', 'Skip all', 'Trash without Finder', 'All without Finder', 'Delete permanently', 'Cancel']);
});

test('missing or unknown choices show nothing extra', () => {
  expect(labels(undefined)).toEqual([]);
  expect(labels(['retry', 'something_new'])).toEqual(['Retry']);
});
