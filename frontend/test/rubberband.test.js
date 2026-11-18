// @vitest-environment happy-dom
// Rubber-band selection (F-700): the rectangle, the boxes of each view, which entries a band touches, the selection it
// makes, and the feature's wiring (off by default, starts only on empty space, Esc puts the selection back).
import { describe, expect, test, vi } from 'vitest';
import { bandRect, bandSelection, geoBox, hitIndices } from '../src/features/rubberband-lib.js';

describe('rubberband-lib', () => {
  test('bandRect spans two points in any order', () => {
    expect(bandRect({ x: 50, y: 10 }, { x: 20, y: 40 })).toEqual({ x: 20, y: 10, w: 30, h: 30 });
  });

  test('geoBox per view', () => {
    expect(geoBox({ kind: 'rows', rowH: 20, width: 300 }, 3)).toEqual({ x: 0, y: 60, w: 300, h: 20 });
    expect(geoBox({ kind: 'brief', rows: 5, colW: 100, rowH: 20 }, 7)).toEqual({ x: 100, y: 40, w: 100, h: 20 });
    expect(geoBox({ kind: 'grid', cols: 4, tileW: 120, tileH: 150 }, 6)).toEqual({ x: 240, y: 150, w: 120, h: 150 });
  });

  test('Full: rows the band crosses', () => {
    const g = { kind: 'rows', rowH: 20, width: 300 };
    expect(hitIndices(100, g, { x: 10, y: 45, w: 50, h: 40 })).toEqual([2, 3, 4]);
    expect(hitIndices(3, g, { x: 10, y: 45, w: 50, h: 400 })).toEqual([2]); // (past the last entry)
    expect(hitIndices(100, g, { x: 10, y: 45, w: 0, h: 0 })).toEqual([]);
  });

  test('Brief: columns filled top to bottom', () => {
    const g = { kind: 'brief', rows: 5, colW: 100, rowH: 20 };
    // second and third column, rows 1..2
    expect(hitIndices(20, g, { x: 150, y: 25, w: 100, h: 30 })).toEqual([6, 7, 11, 12]);
  });

  test('Thumbnails: tiles filled left to right', () => {
    const g = { kind: 'grid', cols: 4, tileW: 100, tileH: 100 };
    expect(hitIndices(10, g, { x: 150, y: 50, w: 100, h: 100 })).toEqual([1, 2, 5, 6]);
    expect(hitIndices(10, g, { x: 250, y: 150, w: 200, h: 200 })).toEqual([6, 7]); // (no tiles past the last)
  });

  test('bandSelection replaces, or adds with a key, and never selects ".."', () => {
    const entries = [{ name: '..', up: true }, { name: 'a' }, { name: 'b' }, { name: 'c' }];
    expect([...bandSelection(entries, new Set(['c']), [0, 1, 2], false)]).toEqual(['a', 'b']);
    expect([...bandSelection(entries, new Set(['c']), [1], true)].sort()).toEqual(['a', 'c']);
  });
});

describe('rubberband feature', () => {
  async function setup(on) {
    vi.resetModules();
    document.body.innerHTML = '<div class="panel"><div class="list"><div class="spacer"></div><div class="rows"><div class="row" data-i="0"></div></div></div></div>';
    const list = document.querySelector('.list');
    Object.defineProperties(list, { clientWidth: { value: 300 }, clientHeight: { value: 400 }, scrollWidth: { value: 300 }, scrollHeight: { value: 400 } });
    list.getBoundingClientRect = () => ({ left: 0, top: 0, right: 300, bottom: 400, width: 300, height: 400 });
    const p = {
      $: { list }, rowH: 20, selected: new Set(['x']), changed: 0,
      entries: [{ name: '..', up: true }, { name: 'a' }, { name: 'b' }, { name: 'c' }, { name: 'x' }],
      selectionChanged() { this.changed++; },
    };
    const app = { panels: [p], config: { values: on ? { 'DragDrop.RubberBand': '1' } : {} } };
    const m = await import('../src/features/rubberband.js');
    m.init(app);
    return { p, list, m };
  }
  const press = (el, x, y, extra = {}) => el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0, clientX: x, clientY: y, detail: 1, ...extra }));
  const move = (x, y, buttons = 1) => document.dispatchEvent(new MouseEvent('pointermove', { bubbles: true, clientX: x, clientY: y, buttons }));
  const up = () => document.dispatchEvent(new MouseEvent('pointerup', { bubbles: true, button: 0 }));

  test('off by default: nothing happens', async () => {
    const { p, list } = await setup(false);
    press(list, 100, 200); move(100, 10); up();
    expect([...p.selected]).toEqual(['x']);
    expect(list.querySelector('.mc-rubberband')).toBeNull();
  });

  test('a band from empty space selects what it touches, replacing the selection', async () => {
    const { p, list } = await setup(true);
    press(list, 150, 200);
    move(150, 198); // (not far enough yet)
    expect(p.changed).toBe(0);
    move(100, 30); // rows 1..9 (y 30..200): a, b, c, x (and ".." is not selected)
    expect([...p.selected].sort()).toEqual(['a', 'b', 'c', 'x']);
    move(100, 50); // band shrinks: rows 2.. → b, c, x
    expect([...p.selected].sort()).toEqual(['b', 'c', 'x']);
    up();
    expect(list.querySelector('.mc-rubberband')).toBeNull();
    expect([...p.selected].sort()).toEqual(['b', 'c', 'x']);
  });

  test('a press on a row is no band; Esc puts the selection back', async () => {
    const { p, list } = await setup(true);
    press(list.querySelector('.row'), 150, 10); move(150, 90);
    expect([...p.selected]).toEqual(['x']);
    up();
    press(list, 150, 200); move(150, 25);
    expect(p.selected.size).toBe(4);
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect([...p.selected]).toEqual(['x']);
  });

  test('Ctrl adds to the selection', async () => {
    const { p, list } = await setup(true);
    press(list, 150, 200, { ctrlKey: true });
    move(150, 25); // rows 1..9 → a..x; then shrink to row 1 only
    move(150, 199);
    expect([...p.selected]).toEqual(['x']);
    move(150, 30);
    expect([...p.selected].sort()).toEqual(['a', 'b', 'c', 'x']);
    up();
  });
});
