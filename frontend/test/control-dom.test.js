// Control mode's "dom" command takes a limit (issue #43): 50 elements by default as before, any other number, or all.
import { describe, expect, test } from 'vitest';
import { domInfo, domLimit, DOM_DEFAULT_LIMIT } from '../src/core/control-dom.js';

const els = (n) => Array.from({ length: n }, (_, i) => ({ textContent: `t${i}`, className: i % 2 ? 'odd' : 'even', offsetWidth: i % 3 ? 10 : 0, offsetHeight: 0 }));

describe('domLimit', () => {
  test('left out it is 50, as before', () => {
    expect(domLimit(undefined)).toBe(DOM_DEFAULT_LIMIT);
    expect(domLimit(null)).toBe(50);
    expect(domLimit('x')).toBe(50);
  });
  test('a number is used as it is; 0 or less means all', () => {
    expect(domLimit(3)).toBe(3);
    expect(domLimit(2.7)).toBe(2);
    expect(domLimit(0)).toBe(Infinity);
    expect(domLimit(-1)).toBe(Infinity);
  });
});

describe('domInfo', () => {
  test('default: the first 50 of 120, count of all', () => {
    const r = domInfo(els(120));
    expect(r.count).toBe(120);
    expect(r.texts).toHaveLength(50);
    expect(r.visible).toHaveLength(50);
    expect(r.classes).toHaveLength(50);
    expect(r.more).toBe(true);
  });
  test('limit 0 describes them all', () => {
    const r = domInfo(els(120), 0);
    expect(r.texts).toHaveLength(120);
    expect(r.texts[119]).toBe('t119');
    expect(r.more).toBe(false);
  });
  test('a small limit', () => {
    const r = domInfo(els(5), 2);
    expect(r).toEqual({ ok: true, count: 5, more: true, texts: ['t0', 't1'], visible: [false, true], classes: ['even', 'odd'] });
  });
  test('no matches', () => {
    expect(domInfo([], 10)).toEqual({ ok: true, count: 0, more: false, texts: [], visible: [], classes: [] });
  });
});
