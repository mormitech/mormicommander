// Real file drag-out (board issue #108): when a drag goes to the system (the setting on and the system able), and how
// the drop of our own native drag back into the window is told apart from files dragged in from another app.
import { expect, test } from 'vitest';
import { NATIVE_DROP_WINDOW_MS, ownNativeDrop, sameFiles, useNativeDrag } from '../src/features/system-dragout.js';
import { boolKey, MORE_BRIDGE } from '../src/features/config/optbridge.js';

test('native drags only with the setting on and a system that can', () => {
  expect(useNativeDrag(true, true)).toBe(true);
  expect(useNativeDrag(false, true)).toBe(false);
  expect(useNativeDrag(undefined, true)).toBe(false);
  expect(useNativeDrag(true, false)).toBe(false);
  expect(useNativeDrag(true, undefined)).toBe(false);
});

test('the option is off unless set, and stored under nativeDragOut', () => {
  const store = {};
  const app = { setting: (k, d) => (k in store ? store[k] : d), setSetting: (k, v) => { store[k] = v; } };
  const opt = MORE_BRIDGE['DragDrop.NativeDragOut'];
  expect(opt.get(app)).toBe('0');
  opt.set(app, '1');
  expect(store.nativeDragOut).toBe(true);
  expect(opt.get(app)).toBe('1');
  expect(typeof boolKey).toBe('function');
});

test('sameFiles ignores order and a folder\'s trailing slash', () => {
  expect(sameFiles(['/a/x', '/a/sub'], ['/a/sub/', '/a/x'])).toBe(true);
  expect(sameFiles(['/a/x'], ['/a/x', '/a/y'])).toBe(false);
  expect(sameFiles(['/a/x'], ['/a/y'])).toBe(false);
  expect(sameFiles([], [])).toBe(false);
  expect(sameFiles(null, ['/a'])).toBe(false);
  expect(sameFiles(['/'], ['/'])).toBe(true);
});

test('ownNativeDrop: our files, while the drag runs or just after it', () => {
  const live = { from: 'left', paths: ['/a/x', '/a/y'], native: true };
  expect(ownNativeDrop(live, ['/a/y', '/a/x'], 1000)).toBe(live);
  expect(ownNativeDrop(live, ['/b/other'], 1000)).toBe(null);

  const ended = { ...live, endedAt: 10_000 };
  expect(ownNativeDrop(ended, ['/a/x', '/a/y'], 10_000 + NATIVE_DROP_WINDOW_MS)).toBe(ended);
  expect(ownNativeDrop(ended, ['/a/x', '/a/y'], 10_001 + NATIVE_DROP_WINDOW_MS)).toBe(null);

  // an HTML drag (not native) or none at all: the files came from another app
  expect(ownNativeDrop({ from: 'left', paths: ['/a/x'] }, ['/a/x'], 0)).toBe(null);
  expect(ownNativeDrop(null, ['/a/x'], 0)).toBe(null);
});
