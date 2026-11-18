// Tests of the file tags helpers (features/tags-lib.js, issue #28): settings defaults, the marks after a name, the
// Tags dialog's list and what its choices change, and the folder's tag list for Select by tag.
import { describe, it, expect } from 'vitest';
import { tagsConfig, DEFAULTS, marksHTML, summary, parseNames, changes, folderTags, hasTag, defaultColor, colorOf, tagText } from '../src/features/tags-lib.js';

describe('tagsConfig', () => {
  it('is off by default and keeps only booleans', () => {
    expect(tagsConfig(undefined)).toEqual(DEFAULTS);
    expect(DEFAULTS.enabled).toBe(false);
    expect(tagsConfig({ enabled: true, names: 'yes' })).toEqual({ enabled: true, names: false });
  });
});

describe('marks', () => {
  it('draws a dot per tag, hollow without colour, names only when asked', () => {
    const l = [{ name: 'Red', color: 6 }, { name: 'a<b', color: 0 }];
    const h = marksHTML(l);
    expect(h.match(/mc-tags-dot/g)).toHaveLength(2);
    expect(h).toContain('mc-tags-dot none');
    expect(h).not.toContain('mc-tags-names');
    expect(marksHTML(l, { names: true })).toContain('Red, a&lt;b');
    expect(marksHTML([])).toBe('');
    expect(colorOf({ color: 6 })).toBe('#ff3b30');
    expect(colorOf({ color: 9 })).toBe('');
    expect(tagText(l)).toBe('Red, a<b');
  });
});

describe('summary and changes', () => {
  const lists = [[{ name: 'Work', color: 0 }, { name: 'Red', color: 6 }], [{ name: 'work', color: 0 }]];
  it('counts the tags of the selection, then the folder, then the colour tags', () => {
    const s = summary(lists, { colours: true, extra: [{ name: 'Home', color: 0 }] });
    expect(s.slice(0, 3)).toEqual([{ name: 'Work', color: 0, count: 2 }, { name: 'Red', color: 6, count: 1 }, { name: 'Home', color: 0, count: 0 }]);
    expect(s.map((t) => t.name)).toContain('Orange');
    expect(s.filter((t) => t.name === 'Red')).toHaveLength(1);
    expect(summary(lists).map((t) => t.name)).toEqual(['Work', 'Red']);
  });
  it('adds what is checked, removes what is unchecked, leaves mixed ones', () => {
    const rows = summary(lists, { extra: [{ name: 'Home', color: 0 }] });
    // Work (2 of 2) unchecked, Red (1 of 2) mixed, Home checked
    expect(changes(rows, [false, 'mixed', true], 2)).toEqual({ add: [{ name: 'Home', color: 0 }], remove: ['Work'] });
    // Red checked: the one without it gets it
    expect(changes(rows, [true, true, false], 2)).toEqual({ add: [{ name: 'Red', color: 6 }], remove: [] });
    // typed names: colour from the choice, from a known row or Finder's colour name; a typed name is not removed
    expect(changes(rows, [false, 'mixed', false], 2, ['work', 'Blue', 'New'], 0)).toEqual({
      add: [{ name: 'work', color: 0 }, { name: 'Blue', color: 4 }, { name: 'New', color: 0 }], remove: [],
    });
    expect(changes(rows, [true, 'mixed', false], 2, ['New'], 3).add).toEqual([{ name: 'New', color: 3 }]);
  });
  it('parses typed names', () => {
    expect(parseNames(' a, b ,,A\nc')).toEqual(['a', 'b', 'c']);
    expect(parseNames('')).toEqual([]);
  });
});

describe('folder tags', () => {
  it('lists each tag once, by name', () => {
    const m = { '/d/a': [{ name: 'b', color: 0 }, { name: 'A', color: 6 }], '/d/b': [{ name: 'a', color: 0 }] };
    expect(folderTags(m)).toEqual([{ name: 'A', color: 6 }, { name: 'b', color: 0 }]);
    expect(hasTag(m['/d/a'], 'B')).toBe(true);
    expect(hasTag(undefined, 'x')).toBe(false);
    expect(defaultColor(' orange ')).toBe(7);
    expect(defaultColor('None')).toBe(0);
  });
});
