// Real file drag-out (board issue #108): when a drag goes to the system (the setting on and the system able), and how
// the drop of our own native drag back into the window is told apart from files dragged in from another app.
import { expect, test } from 'vitest';
import { dragPlan, NATIVE_DROP_WINDOW_MS, nativeHoverEl, ownNativeDrop, prepText, sameFiles, useNativeDrag, VFS_DRAG_LIMIT } from '../src/features/system-dragout.js';
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

test('dragPlan: local panels drag natively, archives and servers copy out first, big ones stay HTML', () => {
  const on = { setting: true, supported: true };
  expect(dragPlan({ ...on, kind: 'local', entries: [{ size: 1 }] })).toBe('native');
  expect(dragPlan({ ...on, kind: undefined, entries: [] })).toBe('native');
  expect(dragPlan({ ...on, kind: 'archive', entries: [{ size: 10 }, { dir: true, size: -1 }] })).toBe('copy');
  expect(dragPlan({ ...on, kind: 'sftp', entries: [{ size: 10 }] })).toBe('copy');
  expect(dragPlan({ ...on, kind: 'ftp', entries: [{ size: VFS_DRAG_LIMIT }, { size: 1 }] })).toBe('big');
  expect(dragPlan({ ...on, kind: 'archive', entries: [{ size: 600 }], limit: 500 })).toBe('big');
  expect(dragPlan({ ...on, kind: 'results', entries: [] })).toBe('html');
  expect(dragPlan({ ...on, kind: 'network', entries: [] })).toBe('html');
  expect(dragPlan({ setting: false, supported: true, kind: 'archive', entries: [] })).toBe('html');
  expect(dragPlan({ setting: true, supported: false, kind: 'local', entries: [] })).toBe('html');
});

test('prepText shows the progress of copying out', () => {
  expect(prepText({ done: 50, total: 200, name: 'a.txt' })).toMatch(/a\.txt.*25%.*mouse button/);
  expect(prepText({ done: 5, total: 0 })).toMatch(/files.*0%/);
  expect(prepText({ done: 300, total: 200, name: 'x' })).toMatch(/100%/);
});

test('nativeHoverEl lights only allowed targets of our own native drag', () => {
  const el = {};
  const d = { native: true, paths: ['/a/x'] };
  expect(nativeHoverEl(d, { el }, () => true)).toBe(el);
  expect(nativeHoverEl(d, { el }, () => false)).toBe(null);
  expect(nativeHoverEl({ paths: ['/a/x'] }, { el }, () => true)).toBe(null);
  expect(nativeHoverEl(d, null, () => true)).toBe(null);
  expect(nativeHoverEl(d, { kind: 'cmdline' }, () => true)).toBe(null);
});
