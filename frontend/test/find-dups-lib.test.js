// The duplicate actions' pure helpers (src/features/find-dups-lib.js, board issue #32): which file of each duplicate
// group is kept under each rule, which results are left out, and how much space the other copies take.
import { describe, expect, test } from 'vitest';
import { planDuplicates, hasDuplicates, dropGone } from '../src/features/find-dups-lib.js';

const items = [
  { p: '/b/x.txt', s: 10, t: 300, g: 1 },
  { p: '/a/x.txt', s: 10, t: 100, g: 1 },
  { p: '/c/x.txt', s: 10, t: 200, g: 1 },
  { p: '/z/y.bin', s: 5, t: 50, g: 2 },
  { p: '/A/y.bin', s: 5, t: 50, g: 2 },        // same time: the first path wins
  { p: '/only/one', s: 99, t: 1, g: 3 },       // a group of one
  { p: '/arc.zip/x.txt', s: 10, t: 999, g: 1, a: true }, // inside an archive: never counted
  { p: '/d', s: 0, t: 1, g: 4, f: true },
  { p: '/e', s: 0, t: 1, g: 4, f: true },      // folders: never counted
  { p: '/plain', s: 1, t: 1 },                 // not a duplicate
];

describe('planDuplicates', () => {
  test('newest keeps the latest file of each group', () => {
    const r = planDuplicates(items, 'newest');
    expect(r.keep).toEqual([0, 4]);
    expect(r.drop).toEqual([{ i: 1, keep: 0 }, { i: 2, keep: 0 }, { i: 3, keep: 4 }]);
    expect(r.groups).toBe(2);
    expect(r.bytes).toBe(25);
  });
  test('oldest keeps the earliest file', () => {
    const r = planDuplicates(items, 'oldest');
    expect(r.keep).toEqual([1, 4]);
    expect(r.drop.map((d) => d.i)).toEqual([0, 2, 3]);
  });
  test('first keeps the first path A to Z, ignoring case', () => {
    const r = planDuplicates(items, 'first');
    expect(r.keep).toEqual([1, 4]);
  });
  test('nothing to do without groups', () => {
    expect(planDuplicates([{ p: '/a', s: 1, t: 1 }])).toEqual({ keep: [], drop: [], groups: 0, bytes: 0 });
    expect(planDuplicates(null).drop).toEqual([]);
    expect(hasDuplicates(items)).toBe(true);
    expect(hasDuplicates(items.slice(5))).toBe(false);
  });
});

describe('dropGone', () => {
  test('removes the gone paths and keeps group numbers', () => {
    const r = dropGone(items.slice(0, 3), ['/a/x.txt']);
    expect(r.map((x) => x.p)).toEqual(['/b/x.txt', '/c/x.txt']);
    expect(r.every((x) => x.g === 1)).toBe(true);
  });
});
