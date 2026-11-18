// The folder size treemap's pure part (shared/treemap-lib.js, board #24, F-872): which entries become blocks, the
// squarified layout (every block inside the area, areas proportional to sizes, the area filled exactly, shapes close
// to squares), the "others" block for very many items, colours, labels, shares and the arrow-key order.
import { describe, expect, test } from 'vitest';
import { treemapItems, squarify, blockHue, labelFits, share, stepIndex, ageText, MAX_BLOCKS } from '../src/shared/treemap-lib.js';

const file = (name, size, ext = name.includes('.') ? name.split('.').pop() : '') => ({ name, size, ext, dir: false });
const dir = (name, dirSize) => ({ name, size: 0, dir: true, dirSize });
const sizeOf = (e) => (e.dir ? (e.dirSize ?? null) : e.size);

describe('treemapItems', () => {
  test('picks files and counted folders, biggest first, and reports the rest', () => {
    const r = treemapItems([{ name: '..', up: true, dir: true }, file('a.txt', 10), dir('big', 500), dir('unknown'), file('empty.txt', 0), dir('nil', 0), file('b.mp4', 10)], { sizeOf });
    expect(r.items.map((i) => i.name)).toEqual(['big', 'a.txt', 'b.mp4']);
    expect(r.items[0]).toMatchObject({ dir: true, value: 500, counting: false });
    expect(r.items[2]).toMatchObject({ ext: 'mp4', dir: false });
    expect(r.pending).toEqual(['unknown']);
    expect(r.empty).toEqual(['empty.txt', 'nil']);
    expect(r.total).toBe(520);
    expect(r.others).toBe(0);
  });

  test('a folder still being counted is drawn with its size so far, or pending while that is zero', () => {
    const doneOf = (e) => e.name !== 'busy' && e.name !== 'zero';
    const r = treemapItems([dir('busy', 40), dir('zero', 0), dir('done', 0)], { sizeOf, doneOf });
    expect(r.items).toEqual([expect.objectContaining({ name: 'busy', counting: true, value: 40 })]);
    expect(r.pending).toEqual(['zero']);
    expect(r.empty).toEqual(['done']);
  });

  test('beyond the limit the smallest items share one block', () => {
    const many = Array.from({ length: 10 }, (_, i) => file(`f${i}`, 100 - i));
    const r = treemapItems(many, { sizeOf, max: 4 });
    expect(r.items).toHaveLength(4);
    expect(r.items.slice(0, 3).map((i) => i.name)).toEqual(['f0', 'f1', 'f2']);
    expect(r.items[3]).toMatchObject({ others: true, value: 97 + 96 + 95 + 94 + 93 + 92 + 91 });
    expect(r.others).toBe(7);
    expect(r.total).toBe(many.reduce((s, e) => s + e.size, 0));
    expect(MAX_BLOCKS).toBeGreaterThan(50);
  });
});

describe('squarify', () => {
  const area = (r) => r.w * r.h;

  test('areas follow the sizes and fill the rectangle exactly, all inside it', () => {
    const items = [60, 30, 20, 10, 6, 4, 2, 1].map((v, i) => ({ key: `k${i}`, value: v }));
    const rect = { x: 0, y: 0, w: 640, h: 360 };
    const rs = squarify(items, rect);
    expect(rs.map((r) => r.key)).toEqual(items.map((i) => i.key));
    const total = items.reduce((s, i) => s + i.value, 0);
    let sum = 0;
    rs.forEach((r, i) => {
      sum += area(r);
      expect(area(r)).toBeCloseTo((items[i].value / total) * rect.w * rect.h, 3);
      expect(r.x).toBeGreaterThanOrEqual(-1e-9);
      expect(r.y).toBeGreaterThanOrEqual(-1e-9);
      expect(r.x + r.w).toBeLessThanOrEqual(rect.w + 1e-6);
      expect(r.y + r.h).toBeLessThanOrEqual(rect.h + 1e-6);
    });
    expect(sum).toBeCloseTo(rect.w * rect.h, 3);
    // no two blocks overlap
    for (let a = 0; a < rs.length; a++) for (let b = a + 1; b < rs.length; b++) {
      const A = rs[a], B = rs[b];
      const ox = Math.min(A.x + A.w, B.x + B.w) - Math.max(A.x, B.x);
      const oy = Math.min(A.y + A.h, B.y + B.h) - Math.max(A.y, B.y);
      expect(ox > 1e-6 && oy > 1e-6).toBe(false);
    }
  });

  test('equal sizes come out close to squares', () => {
    const items = Array.from({ length: 16 }, (_, i) => ({ key: i, value: 1 }));
    const rs = squarify(items, { x: 0, y: 0, w: 400, h: 400 });
    for (const r of rs) expect(Math.max(r.w / r.h, r.h / r.w)).toBeLessThan(2.5);
  });

  test('one item fills everything; nothing, zero sizes and an empty area give no blocks', () => {
    expect(squarify([{ key: 'a', value: 5 }], { x: 10, y: 20, w: 100, h: 50 })).toEqual([{ key: 'a', x: 10, y: 20, w: 100, h: 50 }]);
    expect(squarify([], { x: 0, y: 0, w: 100, h: 100 })).toEqual([]);
    expect(squarify([{ key: 'a', value: 0 }], { x: 0, y: 0, w: 100, h: 100 })).toEqual([]);
    expect(squarify([{ key: 'a', value: 1 }], { x: 0, y: 0, w: 0, h: 100 })).toEqual([]);
  });

  test('a tall area is filled too', () => {
    const items = [5, 3, 2].map((v, i) => ({ key: i, value: v }));
    const rs = squarify(items, { x: 0, y: 0, w: 100, h: 900 });
    expect(rs.reduce((s, r) => s + area(r), 0)).toBeCloseTo(90000, 3);
  });
});

describe('looks and keys', () => {
  test('colours: folders blue, files by extension (any case), no extension grey', () => {
    expect(blockHue({ dir: true })).toBe(212);
    expect(blockHue({ ext: 'MP4' })).toBe(blockHue({ ext: 'mp4' }));
    expect(blockHue({ ext: 'mp4' })).toBeGreaterThanOrEqual(0);
    expect(blockHue({ ext: 'mp4' })).toBeLessThan(360);
    expect(blockHue({ ext: '' })).toBeNull();
    expect(blockHue({ others: true, ext: 'x' })).toBeNull();
  });

  test('labels only in blocks big enough, shares as short percentages', () => {
    expect(labelFits({ w: 60, h: 20 })).toBe(true);
    expect(labelFits({ w: 30, h: 40 })).toBe(false);
    expect(labelFits({ w: 80, h: 10 })).toBe(false);
    expect(share(42, 100)).toBe('42%');
    expect(share(4.25, 100)).toBe('4.3%');
    expect(share(1, 100000)).toBe('<0.1%');
    expect(share(1, 0)).toBe('0%');
  });

  test('arrow keys walk the blocks in size order', () => {
    expect(stepIndex(-1, 5, 'Down')).toBe(0);
    expect(stepIndex(-1, 5, 'End')).toBe(4);
    expect(stepIndex(0, 5, 'Right')).toBe(1);
    expect(stepIndex(4, 5, 'Down')).toBe(4);
    expect(stepIndex(2, 5, 'Up')).toBe(1);
    expect(stepIndex(0, 5, 'Left')).toBe(0);
    expect(stepIndex(3, 5, 'Home')).toBe(0);
    expect(stepIndex(1, 5, 'End')).toBe(4);
    expect(stepIndex(0, 0, 'Down')).toBe(-1);
  });
});

describe('remembered sizes (#24)', () => {
  test('oldOf marks folders whose size is a remembered one', () => {
    const r = treemapItems([dir('old', 50), dir('new', 30), file('f.txt', 5)], { sizeOf, doneOf: (e) => e.name !== 'old', oldOf: (e) => e.name === 'old' || e.name === 'f.txt' });
    expect(r.items.find((i) => i.name === 'old')).toMatchObject({ old: true, counting: true, value: 50 });
    expect(r.items.find((i) => i.name === 'new').old).toBeUndefined();
    expect(r.items.find((i) => i.name === 'f.txt').old).toBeUndefined(); // files are never remembered sizes
  });

  test('ageText words how long ago', () => {
    const now = new Date(2026, 10, 19, 15, 0, 0).getTime();
    const at = (y, m, d, h = 12) => new Date(y, m, d, h).getTime() / 1000;
    expect(ageText(at(2026, 10, 19, 1), now)).toBe('today');
    expect(ageText(at(2026, 10, 18, 23), now)).toBe('yesterday');
    expect(ageText(at(2026, 10, 14), now)).toBe('5 days ago');
    expect(ageText(at(2025, 0, 2), now)).toBe('on 2025-01-02');
    expect(ageText(0, now)).toBe('earlier');
  });
});
