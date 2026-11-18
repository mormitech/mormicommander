// The field lines under thumbnails (F-679, Thumbnails.FieldLines): none keeps the one size/dimensions line the view
// always had; with lines, each tile gets one line per template (escaped), and grows 15 pixels per extra line.
import { expect, test } from 'vitest';
import { cleanThumbLines, thumbTileH, thumbSubHtml } from '../src/features/views-thumbs-lib.js';

const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;');

test('the stored lines are trimmed, empty ones dropped, at most four kept', () => {
  expect(cleanThumbLines(undefined)).toEqual([]);
  expect(cleanThumbLines('x')).toEqual([]);
  expect(cleanThumbLines([' [=tc.size.k] ', '', null, 'a', 'b', 'c', 'd'])).toEqual(['[=tc.size.k]', 'a', 'b', 'c']);
});

test('the tile is as tall as before with zero or one line, taller with more', () => {
  expect(thumbTileH(96, 0)).toBe(146);
  expect(thumbTileH(96, 1)).toBe(146);
  expect(thumbTileH(96, 3)).toBe(176);
});

test('without field lines the tile shows the old line; with them, one escaped line per template', () => {
  expect(thumbSubHtml([], null, '640 × 480', esc)).toBe('<div class="mc-views-sub">640 × 480</div>');
  expect(thumbSubHtml(null, null, '1 k', esc)).toBe('<div class="mc-views-sub">1 k</div>');
  expect(thumbSubHtml(['[=tc.size]', '[=tc.writedate]'], ['1 k', '<b>'], 'x', esc))
    .toBe('<div class="mc-views-sub">1 k</div><div class="mc-views-sub">&lt;b></div>');
  expect(thumbSubHtml(['[=tc.size]', '[=tc.ext]'], undefined, 'x', esc)).toBe('<div class="mc-views-sub"></div><div class="mc-views-sub"></div>');
});
