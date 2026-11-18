// @vitest-environment happy-dom
// Custom columns (issue #80, docs/FEATURES.md section 32): hiding columns from the header's right-click menu (F-672)
// and the column width commands (F-673). The pure helpers are tested directly; the commands are checked to be
// registered and listed in the Show menu. Go is a stand-in that answers nothing.
import { beforeAll, expect, test, vi } from 'vitest';
import { viewKey, hiddenFor, visibleCols, toggleHidden, equalWidths, fitWidths } from '../src/features/views-cols-lib.js';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

test('viewKey names the views that have columns', () => {
  expect(viewKey({ mode: 'full' })).toBe('full');
  expect(viewKey({ mode: 'comments' })).toBe('comments');
  expect(viewKey({ mode: 'custom', set: 'Pics' })).toBe('custom:Pics');
  expect(viewKey({ mode: 'custom', set: '' })).toBe('');
  expect(viewKey({ mode: 'brief' })).toBe('');
  expect(viewKey({ mode: 'thumbs' })).toBe('');
  expect(viewKey(null)).toBe('');
});

test('toggleHidden hides a column per view and shows it again', () => {
  let m = toggleHidden(undefined, 'full', 'Attr');
  expect(m).toEqual({ full: ['Attr'] });
  m = toggleHidden(m, 'custom:Pics', 'Size');
  m = toggleHidden(m, 'full', 'Date');
  expect(hiddenFor(m, 'full')).toEqual(['Attr', 'Date']);
  expect(hiddenFor(m, 'custom:Pics')).toEqual(['Size']);
  m = toggleHidden(m, 'custom:Pics', 'Size');
  expect(m).toEqual({ full: ['Attr', 'Date'] });
  expect(hiddenFor(m, 'comments')).toEqual([]);
  expect(hiddenFor({ full: 'bad' }, 'full')).toEqual([]);
});

test('visibleCols drops hidden columns by title and keeps the order', () => {
  const cols = [{ title: 'Size' }, { title: 'Date' }, { title: 'Attr' }];
  expect(visibleCols(cols, [])).toBe(cols);
  expect(visibleCols(cols, ['Date']).map((c) => c.title)).toEqual(['Size', 'Attr']);
});

test('equalWidths shares the room; fitWidths takes the widest text', () => {
  expect(equalWidths(416, 4, 8)).toEqual([98, 98, 98, 98]);
  expect(equalWidths(50, 4, 8, 28)).toEqual([28, 28, 28, 28]);
  expect(equalWidths(100, 0)).toEqual([]);
  const measure = (t) => t.length * 10;
  expect(fitWidths([['Name', 'a'], ['Size', '1,234,567'], ['', '']], measure, { pad: 10, min: 28, max: 80 })).toEqual([50, 80, 28]);
});

let reg;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  vi.useRealTimers();
});

test('the column commands are registered and put themselves in the Show menu (F-672, F-673)', () => {
  for (const n of ['cm_ColumnsShowHide', 'cm_ColumnsFitWidth', 'cm_ColumnsEqualWidth', 'cm_ColumnsDefaultWidth']) {
    expect(reg.hasCommand(n), n).toBe(true);
    expect(reg.getCommand(n).menu, n).toBe('Show');
  }
});
