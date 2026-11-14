// Issue #128: each folder tab keeps its own view (src/features/views-tab.js). A tab's copy of the view state is its own
// object (changing the view of a new tab must not change the tab it came from), and the view saved with a tab comes
// back, or none for tabs saved by older versions.
import { describe, expect, test } from 'vitest';
import { freshView, copyView, viewToSaved, viewFromSaved } from '../src/features/views-tab.js';

describe('views per tab', () => {
  test('a copy is a separate object: the new tab switching to Full leaves the first tab in Brief', () => {
    const first = freshView('brief');
    first.cells = { path: '/a', map: new Map() };
    first.thumbs = { key: 'k' };
    const second = copyView(first);
    expect(second).not.toBe(first);
    expect(second.manual).not.toBe(first.manual);
    expect(second.mode).toBe('brief');
    expect(second.cells).toBe(null);
    expect(second.thumbs).toBe(null);
    second.mode = 'full';
    second.manual = { mode: 'full', set: '' };
    expect(first.mode).toBe('brief');
    expect(first.manual.mode).toBe('brief');
    expect(first.thumbs).toEqual({ key: 'k' });
  });
  test('a copy keeps the column set, and a panel in the tree gives the view under it', () => {
    const c = copyView({ ...freshView('custom', 'Media'), auto: true });
    expect([c.mode, c.set, c.auto]).toEqual(['custom', 'Media', true]);
    const tree = { ...freshView('thumbs'), mode: 'tree', tree: {}, prev: { mode: 'thumbs', set: '' } };
    const t = copyView(tree);
    expect([t.mode, t.tree, t.prev]).toEqual(['thumbs', null, null]);
    expect(copyView(null).mode).toBe('full');
  });
  test('the saved view is the one the user chose', () => {
    expect(viewToSaved(freshView('brief'))).toEqual({ view: 'brief', set: '' });
    expect(viewToSaved(freshView('custom', 'Media'))).toEqual({ view: 'custom', set: 'Media' });
    const auto = { ...freshView('full'), mode: 'custom', set: 'Pics', auto: true };
    expect(viewToSaved(auto)).toEqual({ view: 'full', set: '' });
    expect(viewToSaved(null)).toEqual({});
  });
  test('a saved view comes back; none, unknown or a deleted column set keep the panel view', () => {
    expect(viewFromSaved({ view: 'thumbs' }).mode).toBe('thumbs');
    const c = viewFromSaved({ view: 'custom', set: 'Media' }, (n) => n === 'Media');
    expect([c.mode, c.set, c.manual]).toEqual(['custom', 'Media', { mode: 'custom', set: 'Media' }]);
    expect(viewFromSaved({ path: '/a', sort: 'name' })).toBe(null);
    expect(viewFromSaved(null)).toBe(null);
    expect(viewFromSaved({ view: 'tree' })).toBe(null);
    expect(viewFromSaved({ view: 'custom', set: 'Gone' }, () => false)).toBe(null);
  });
});
