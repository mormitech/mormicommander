// Rubber-band selection (F-700), the parts without a page: the rectangle between two points, the boxes of the entries
// in each view, and which entries a rectangle touches. All coordinates are in the list's content (scroll) space.

// bandRect is the rectangle {x, y, w, h} spanned by the points a and b.
export function bandRect(a, b) {
  const x = Math.min(a.x, b.x), y = Math.min(a.y, b.y);
  return { x, y, w: Math.abs(a.x - b.x), h: Math.abs(a.y - b.y) };
}

const overlaps = (r, b) => r.x < b.x + b.w && b.x < r.x + r.w && r.y < b.y + b.h && b.y < r.y + r.h;

// geoBox makes the box of entry i from a view's geometry:
//   {kind: 'rows', rowH, width}            one entry per row (Full, Comments, custom columns)
//   {kind: 'brief', rows, colW, rowH}      columns filled top to bottom
//   {kind: 'grid', cols, tileW, tileH}     tiles filled left to right (Thumbnails)
export function geoBox(g, i) {
  if (g.kind === 'brief') { const c = Math.floor(i / g.rows), r = i % g.rows; return { x: c * g.colW, y: r * g.rowH, w: g.colW, h: g.rowH }; }
  if (g.kind === 'grid') { const r = Math.floor(i / g.cols), c = i % g.cols; return { x: c * g.tileW, y: r * g.tileH, w: g.tileW, h: g.tileH }; }
  return { x: 0, y: i * g.rowH, w: g.width, h: g.rowH };
}

// hitIndices lists the entries (0..n-1) whose boxes the rectangle touches, in order. Only the range of rows or columns
// the rectangle can reach is looked at, so a big folder costs no more than what is under the band.
export function hitIndices(n, g, band) {
  if (!g || n <= 0 || band.w <= 0 && band.h <= 0) return [];
  let lo = 0, hi = n - 1;
  if (g.kind === 'rows') { lo = Math.floor(band.y / g.rowH); hi = Math.floor((band.y + band.h) / g.rowH); }
  else if (g.kind === 'brief') { lo = Math.floor(band.x / g.colW) * g.rows; hi = (Math.floor((band.x + band.w) / g.colW) + 1) * g.rows - 1; }
  else if (g.kind === 'grid') { lo = Math.floor(band.y / g.tileH) * g.cols; hi = (Math.floor((band.y + band.h) / g.tileH) + 1) * g.cols - 1; }
  lo = Math.max(0, lo);
  hi = Math.min(n - 1, hi);
  const out = [];
  for (let i = lo; i <= hi; i++) if (overlaps(band, geoBox(g, i))) out.push(i);
  return out;
}

// bandSelection is the selected names after a band: the names selected before (kept when the band adds, as with Ctrl,
// Cmd or Shift held) plus the entries it touches ("..", entries with up, is never selected).
export function bandSelection(entries, before, hits, add) {
  const out = new Set(add ? before : []);
  for (const i of hits) { const e = entries[i]; if (e && !e.up) out.add(e.name); }
  return out;
}
