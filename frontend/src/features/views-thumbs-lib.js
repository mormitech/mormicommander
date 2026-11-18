// Pure pieces of the thumbnails view (features/views.js): the field lines under each picture (Thumbnails.FieldLines,
// F-679) and the tile height they need.

// cleanThumbLines keeps at most four non-empty column templates from the stored "views" → thumbLines value.
export const cleanThumbLines = (v) => (Array.isArray(v) ? v : []).map((x) => String(x ?? '').trim()).filter(Boolean).slice(0, 4);

// thumbTileH is the height of a tile: the picture, the name and one line under it, 15 pixels more per extra line.
export const thumbTileH = (size, lines) => size + 50 + 15 * Math.max(0, lines - 1);

// thumbSubHtml is what goes under the name of a tile: with field lines, one line per template from the engine's cells
// (empty until they come, so the tiles keep their height); without, the one line the view always had (fallback).
export function thumbSubHtml(lines, cells, fallback, esc) {
  if (!lines?.length) return `<div class="mc-views-sub">${fallback}</div>`;
  return lines.map((_, k) => `<div class="mc-views-sub">${esc(cells?.[k] ?? '')}</div>`).join('');
}
