// @vitest-environment happy-dom
// Pointer gestures of control mode (core/pointer.js): a played drag must reach handlers the way a real mouse does —
// pointer capture honoured, the browser's drag and drop for draggable elements, hover over/out and enter/leave — so the
// features only a mouse reaches (dialog dragging, column resizing, row reordering, hover tooltips) can be scripted.
// happy-dom has no layout, so each test lays out its elements by giving them rectangles and an elementFromPoint.
import { describe, expect, test, beforeEach } from 'vitest';
import { pointOf, path, dragGesture, hover, unhover } from '../src/core/pointer.js';

let rects;
function place(el, left, top, width, height) { rects.set(el, { left, top, width, height }); el.getBoundingClientRect = () => ({ ...rects.get(el), x: left, y: top, right: left + width, bottom: top + height }); }
// the innermost placed element containing the point, like a browser's hit test on boxes that nest
function hit(x, y) {
  let best = null, area = Infinity;
  for (const [el, r] of rects) {
    if (!el.isConnected || x < r.left || y < r.top || x >= r.left + r.width || y >= r.top + r.height) continue;
    if (r.width * r.height < area) { best = el; area = r.width * r.height; }
  }
  return best;
}

beforeEach(() => {
  document.body.innerHTML = '';
  rects = new Map();
  document.elementFromPoint = hit;
});

describe('points', () => {
  test('pointOf: the middle by default, else an offset from the top-left corner (negative allowed)', () => {
    const el = document.createElement('div');
    document.body.append(el);
    place(el, 100, 50, 40, 20);
    expect(pointOf(el)).toEqual({ x: 120, y: 60 });
    expect(pointOf(el, -4, 3)).toEqual({ x: 96, y: 53 });
  });
  test('path: n even steps ending on the target', () => {
    expect(path({ x: 0, y: 0 }, { x: 10, y: -20 }, 2)).toEqual([{ x: 5, y: -10 }, { x: 10, y: -20 }]);
    expect(path({ x: 0, y: 0 }, { x: 3, y: 3 }, 0)).toEqual([{ x: 3, y: 3 }]);
  });
});

describe('a drag with pointer capture (a dialog dragged by its title, a column border)', () => {
  // the same shape as dialogs.js draggable(): capture on pointerdown, listen for moves on the box itself
  function dialogBox() {
    const box = document.createElement('div');
    const h2 = document.createElement('h2');
    box.append(h2);
    document.body.append(box);
    place(box, 100, 100, 300, 200);
    place(h2, 100, 100, 300, 30);
    const seen = [];
    let off = { x: 0, y: 0 };
    box.addEventListener('pointerdown', (ev) => {
      seen.push('down');
      const start = { x: ev.clientX - off.x, y: ev.clientY - off.y };
      box.setPointerCapture(ev.pointerId);
      const move = (e) => { off = { x: e.clientX - start.x, y: e.clientY - start.y }; seen.push('move'); };
      const up = () => { seen.push('up'); box.removeEventListener('pointermove', move); box.removeEventListener('pointerup', up); };
      box.addEventListener('pointermove', move);
      box.addEventListener('pointerup', up);
    });
    return { box, h2, seen, off: () => off };
  }

  test('moves and the release reach the capturing element even when the pointer leaves it', async () => {
    const { box, h2, seen, off } = dialogBox();
    const res = await dragGesture(document, { from: pointOf(h2), to: { x: pointOf(h2).x + 500, y: pointOf(h2).y + 400 }, steps: 4 });
    expect(off()).toEqual({ x: 500, y: 400 });
    expect(seen).toEqual(['down', 'move', 'move', 'move', 'move', 'up']);
    expect(res.captured).toBe('div');
    expect(res.released).toBe('div');
    expect(res.dnd).toBe(false);
    expect(box.hasPointerCapture).toBeUndefined(); // the page's own methods are back (happy-dom has none)
  });

  test('the matching mouse events come too (handlers that listen for mousemove on window, like the splitter)', async () => {
    const bar = document.createElement('div');
    document.body.append(bar);
    place(bar, 0, 0, 10, 100);
    const got = [];
    bar.addEventListener('mousedown', () => {
      const mv = (e) => got.push(e.clientX);
      window.addEventListener('mousemove', mv);
      window.addEventListener('mouseup', () => { window.removeEventListener('mousemove', mv); got.push('up'); }, { once: true });
    });
    await dragGesture(document, { from: { x: 5, y: 50 }, to: { x: 85, y: 50 }, steps: 2 });
    expect(got).toEqual([45, 85, 'up']);
  });

  test('modifier keys and the button go with every event; a failing handler still restores capture', async () => {
    const el = document.createElement('div');
    document.body.append(el);
    place(el, 0, 0, 50, 50);
    const mods = [];
    el.addEventListener('pointerdown', (ev) => { mods.push([ev.shiftKey, ev.altKey, ev.button]); el.setPointerCapture(ev.pointerId); });
    await dragGesture(document, { from: { x: 10, y: 10 }, to: { x: 20, y: 10 }, steps: 1, shift: true, alt: true });
    expect(mods).toEqual([[true, true, 0]]);
    const proto = Element.prototype;
    expect(Object.getOwnPropertyDescriptor(proto, 'setPointerCapture')).toBeUndefined();
    await expect(dragGesture(document, { from: { x: 10, y: 10 }, to: { x: 20, y: 10 }, steps: 1, wait: async () => { throw new Error('boom'); } })).rejects.toThrow('boom');
    expect(Object.getOwnPropertyDescriptor(proto, 'setPointerCapture')).toBeUndefined();
  });

  test('a page method of its own (the dialog windows patch setPointerCapture) is put back as it was', async () => {
    const mine = function () { return 'page'; };
    Element.prototype.setPointerCapture = mine;
    try {
      const el = document.createElement('div');
      document.body.append(el);
      place(el, 0, 0, 50, 50);
      el.addEventListener('pointerdown', (ev) => el.setPointerCapture(ev.pointerId));
      const res = await dragGesture(document, { from: { x: 10, y: 10 }, to: { x: 20, y: 10 }, steps: 1 });
      expect(res.captured).toBe('div');
      expect(Element.prototype.setPointerCapture).toBe(mine);
    } finally { delete Element.prototype.setPointerCapture; }
  });

  test('a click follows on the element holding both the press and the release', async () => {
    const a = document.createElement('div'), b = document.createElement('div'), wrap = document.createElement('div');
    wrap.append(a, b);
    document.body.append(wrap);
    place(wrap, 0, 0, 200, 50);
    place(a, 0, 0, 100, 50);
    place(b, 100, 0, 100, 50);
    const clicks = [];
    wrap.addEventListener('click', (ev) => clicks.push(ev.target === wrap ? 'wrap' : ev.target === a ? 'a' : 'b'));
    await dragGesture(document, { from: { x: 10, y: 10 }, to: { x: 150, y: 10 }, steps: 2 });
    await dragGesture(document, { from: { x: 10, y: 10 }, to: { x: 20, y: 10 }, steps: 1 });
    expect(clicks).toEqual(['wrap', 'a']);
  });
});

describe('drag and drop of draggable elements (multi-rename rows, panel rows, tabs)', () => {
  // the same shape as multirename.js: a draggable row list that accepts drops on rows
  function rowList() {
    const tbody = document.createElement('tbody');
    const rows = [0, 1, 2].map((i) => { const tr = document.createElement('tr'); tr.dataset.i = i; tr.setAttribute('draggable', 'true'); tbody.append(tr); return tr; });
    const table = document.createElement('table');
    table.append(tbody);
    document.body.append(table);
    place(tbody, 0, 0, 200, 60);
    rows.forEach((tr, i) => place(tr, 0, i * 20, 200, 20));
    const log = [];
    tbody.addEventListener('dragstart', (ev) => { log.push(`start ${ev.target.dataset.i}`); ev.dataTransfer.setData('text/plain', 'mc-test'); });
    tbody.addEventListener('dragover', (ev) => { if (ev.target.closest('tr')) ev.preventDefault(); });
    tbody.addEventListener('drop', (ev) => log.push(`drop ${ev.target.closest('tr').dataset.i} ${ev.dataTransfer.getData('text/plain')} ${ev.clientY < ev.target.getBoundingClientRect().top + 10 ? 'before' : 'after'}`));
    tbody.addEventListener('dragend', () => log.push('end'));
    tbody.addEventListener('mouseup', () => log.push('mouseup'));
    return { rows, log };
  }

  test('dragstart on the draggable element, drop where the last dragover was accepted, then dragend', async () => {
    const { log } = rowList();
    const res = await dragGesture(document, { from: { x: 50, y: 5 }, to: { x: 50, y: 55 }, steps: 5 });
    expect(log).toEqual(['start 0', 'drop 2 mc-test after', 'end']); // no mouseup during a drag and drop, as in a browser
    expect(res).toMatchObject({ dnd: true, dropped: true, dropTarget: 'tr' });
  });

  test('no drop where nothing accepts it', async () => {
    const { log } = rowList();
    const out = document.createElement('div');
    document.body.append(out);
    place(out, 0, 100, 200, 50);
    const res = await dragGesture(document, { from: { x: 50, y: 5 }, to: { x: 50, y: 120 }, steps: 3 });
    expect(log).toEqual(['start 0', 'end']);
    expect(res.dropped).toBe(false);
  });

  test('a cancelled dragstart leaves plain moves and a mouseup; dnd: false never starts one', async () => {
    const { rows, log } = rowList();
    rows[0].addEventListener('dragstart', (ev) => ev.preventDefault());
    const res = await dragGesture(document, { from: { x: 50, y: 5 }, to: { x: 50, y: 55 }, steps: 2 });
    expect(res).toMatchObject({ dnd: false, dragstartCancelled: true });
    expect(log).toEqual(['start 0', 'mouseup']);
    log.length = 0;
    const res2 = await dragGesture(document, { from: { x: 50, y: 25 }, to: { x: 50, y: 55 }, steps: 2, dnd: false });
    expect(res2.dnd).toBe(false);
    expect(log).toEqual(['mouseup']);
  });

  test('enter and leave follow the pointer across elements', async () => {
    const { rows } = rowList();
    const seen = [];
    rows.forEach((tr) => { tr.addEventListener('dragenter', () => seen.push(`enter ${tr.dataset.i}`)); tr.addEventListener('dragleave', () => seen.push(`leave ${tr.dataset.i}`)); });
    await dragGesture(document, { from: { x: 50, y: 5 }, to: { x: 50, y: 45 }, steps: 2 });
    expect(seen).toEqual(['enter 1', 'enter 2', 'leave 1']);
  });
});

describe('hover', () => {
  test('over/enter on the way in, out/leave on the way out, a move at the point', () => {
    const outer = document.createElement('div'), a = document.createElement('span'), b = document.createElement('span');
    outer.append(a, b);
    document.body.append(outer);
    place(outer, 0, 0, 200, 20);
    place(a, 0, 0, 100, 20);
    place(b, 100, 0, 100, 20);
    const log = [];
    for (const [n, el] of [['outer', outer], ['a', a], ['b', b]]) {
      for (const t of ['pointerenter', 'pointerleave', 'mouseenter', 'mouseleave']) el.addEventListener(t, () => log.push(`${t} ${n}`));
    }
    document.addEventListener('pointerover', (ev) => log.push(`over ${ev.target === a ? 'a' : 'b'}`));
    document.addEventListener('pointerout', (ev) => log.push(`out ${ev.target === a ? 'a' : 'b'}`));
    outer.addEventListener('mousemove', (ev) => log.push(`move ${ev.clientX}`));

    expect(hover(document, { x: 10, y: 5 })).toEqual({ over: 'span' });
    expect(log).toEqual(['over a', 'pointerenter outer', 'mouseenter outer', 'pointerenter a', 'mouseenter a', 'move 10']);
    log.length = 0;
    hover(document, { x: 150, y: 5 });
    expect(log).toEqual(['out a', 'pointerleave a', 'mouseleave a', 'over b', 'pointerenter b', 'mouseenter b', 'move 150']);
    log.length = 0;
    hover(document, { x: 160, y: 5 });
    expect(log).toEqual(['move 160']); // still over b: only a move
    log.length = 0;
    expect(unhover(document)).toEqual({ left: 'span' });
    expect(log).toEqual(['out b', 'pointerleave b', 'mouseleave b', 'pointerleave outer', 'mouseleave outer']);
  });

  test('a given target is used even when something else is drawn at the point', () => {
    const el = document.createElement('div');
    document.body.append(el);
    let over = 0;
    el.addEventListener('pointerover', () => over++);
    hover(document, { x: 999, y: 999 }, el);
    expect(over).toBe(1);
    unhover(document);
  });
});

describe('a scope the hit test cannot see (a dialog whose page copy is hidden behind its dialog window)', () => {
  test('points inside the scope are found by the boxes of its elements, so drag and drop still lands on rows', async () => {
    const back = document.createElement('div'), list = document.createElement('div');
    const rows = [0, 1].map((i) => { const r = document.createElement('div'); r.dataset.i = i; r.setAttribute('draggable', 'true'); list.append(r); return r; });
    document.body.append(back, list);
    place(list, 0, 0, 100, 40);
    rows.forEach((r, i) => place(r, 0, i * 20, 100, 20));
    document.elementFromPoint = () => back; // the dimmed page covers everything
    let dropped = null;
    list.addEventListener('dragover', (ev) => ev.preventDefault());
    list.addEventListener('drop', (ev) => { dropped = ev.target.dataset.i; });
    const res = await dragGesture(document, { from: { x: 10, y: 5 }, to: { x: 10, y: 30 }, source: rows[0], scope: list, steps: 2 });
    expect(res.dropped).toBe(true);
    expect(dropped).toBe('1');
  });
  test('innermostAt: the deepest box holding the point, the later sibling winning; null outside', async () => {
    const { innermostAt } = await import('../src/core/pointer.js');
    const root = document.createElement('div'), a = document.createElement('div'), b = document.createElement('div'), c = document.createElement('span');
    b.append(c); root.append(a, b); document.body.append(root);
    place(root, 0, 0, 100, 100); place(a, 0, 0, 100, 100); place(b, 0, 0, 50, 50); place(c, 10, 10, 5, 5);
    expect(innermostAt(root, { x: 12, y: 12 })).toBe(c);
    expect(innermostAt(root, { x: 30, y: 30 })).toBe(b);
    expect(innermostAt(root, { x: 80, y: 80 })).toBe(a);
    expect(innermostAt(root, { x: 180, y: 80 })).toBe(null);
  });
});
