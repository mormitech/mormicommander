// Tests of the quick view panel's right-click menu (viewer/quickmenu.js, F-082 / F-589): the Lister menus rearranged
// into one flat list, with the window-only items left out.
import { describe, it, expect } from 'vitest';
import { quickMenuItems, tidy } from '../src/features/viewer/quickmenu.js';

const run = () => {};
const menus = {
  File: [{ label: 'Next file', run }, { label: 'Previous file', run }, { sep: true }, { label: 'Reload', detail: 'F2', run }, { label: '✓ Follow file (auto-reload)', run },
    { sep: true }, { label: 'Save as…', run }, { label: 'Open with another app', run }, { sep: true }, { label: 'Close', run }],
  Edit: [{ label: 'Copy', run }, { label: 'Select all', run }, { sep: true }, { label: 'Find…', detail: 'F7', run }, { label: 'Find next', run }],
  Options: [{ label: '✓ Text', detail: '1', run }, { label: 'Hex', detail: '3', run }, { label: 'HTML/Markdown', detail: '5', run }, { sep: true },
    { label: 'Wrap text', detail: 'W', run }, { sep: true }, { label: 'Lister options…', run }],
  Image: [{ label: 'Fit image to window', run, disabled: true }, { label: 'Rotate left', run }],
};
const labels = (items) => items.map((i) => (i.sep ? '-' : i.label));

describe('quickMenuItems (quick view right-click menu)', () => {
  it('puts the modes first, then copy and find, then the options, without window-only items', () => {
    expect(labels(quickMenuItems(menus))).toEqual(['✓ Text', 'Hex', 'HTML/Markdown', '-', 'Copy', 'Select all', '-', 'Find…', 'Find next', '-',
      'Wrap text', '-', 'Lister options…', '-', 'Reload', '✓ Follow file (auto-reload)']);
  });

  it('adds the usable image commands only when an image is shown, and the encoding item', () => {
    const enc = { label: 'Encoding…', run };
    const l = labels(quickMenuItems(menus, { media: true, encoding: enc }));
    expect(l).toContain('Rotate left');
    expect(l).not.toContain('Fit image to window');
    expect(l.indexOf('Encoding…')).toBeGreaterThan(l.indexOf('Rotate left'));
    expect(labels(quickMenuItems(menus))).not.toContain('Rotate left');
  });

  it('keeps the run functions of the original items', () => {
    const f = () => 42;
    const items = quickMenuItems({ Options: [{ label: 'Hex', run: f }] });
    expect(items).toHaveLength(1);
    expect(items[0].run()).toBe(42);
  });

  it('tidy removes leading, trailing and doubled separators', () => {
    expect(labels(tidy([{ sep: true }, { label: 'a' }, { sep: true }, { sep: true }, { label: 'b' }, { sep: true }]))).toEqual(['a', '-', 'b']);
    expect(tidy([])).toEqual([]);
  });
});
