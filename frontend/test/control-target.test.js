// Control mode's clicks follow a redraw (issue #42): when the press redraws the element that was clicked (a panel row),
// the rest of the click (mouseup, contextmenu) goes to the element now in its place, not to the removed one.
import { describe, expect, test } from 'vitest';
import { liveTarget } from '../src/core/control-target.js';

const node = (name, connected = true) => ({ name, isConnected: connected, got: [], dispatchEvent(ev) { this.got.push(ev); } });

describe('liveTarget', () => {
  test('an element still in the page keeps the events', () => {
    const el = node('row');
    expect(liveTarget(el, 1, 2, () => node('other'), { elementFromPoint: () => node('point') })).toBe(el);
  });
  test('a removed element is replaced by what refind finds', () => {
    const old = node('old', false), fresh = node('fresh');
    expect(liveTarget(old, 1, 2, () => fresh, { elementFromPoint: () => node('point') })).toBe(fresh);
  });
  test('without a replacement, what is at the point now', () => {
    const old = node('old', false), there = node('there');
    expect(liveTarget(old, 5, 6, () => null, { elementFromPoint: (x, y) => (x === 5 && y === 6 ? there : null) })).toBe(there);
    expect(liveTarget(old, 5, 6, () => node('gone', false), { elementFromPoint: () => there })).toBe(there);
  });
  test('nothing better: the old element, so a click never throws', () => {
    const old = node('old', false);
    expect(liveTarget(old, 0, 0, null, { elementFromPoint: () => null })).toBe(old);
    expect(liveTarget(old, 0, 0, null, null)).toBe(old);
  });
  test('a right-click whose press redraws the row still delivers the contextmenu', () => {
    // a list of rows that is redrawn on mousedown, as the panel does when the cursor moves to the pressed row
    const list = { rows: [node('row0')], redraw() { this.rows = [node('row0')]; } };
    const first = list.rows[0];
    const refind = () => list.rows[0];
    const send = (type) => liveTarget(first, 0, 0, refind, { elementFromPoint: () => null }).dispatchEvent({ type });
    send('mousedown');
    first.isConnected = false; list.redraw();
    send('mouseup');
    send('contextmenu');
    expect(first.got.map((e) => e.type)).toEqual(['mousedown']);
    expect(list.rows[0].got.map((e) => e.type)).toEqual(['mouseup', 'contextmenu']);
  });
});
