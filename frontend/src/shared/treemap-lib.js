// The pure part of the folder size treemap (features/treemap.js, board #24, F-872): which entries of a panel become
// blocks and how big, the squarified layout that turns sizes into rectangles, the colour of a block and the order the
// arrow keys walk through the blocks. No DOM here, so it is unit-tested (test/treemap-lib.test.js).
//
// Shared engine: this file lives in frontend/src/shared because it imports nothing (no app, no DOM, no Wails), so it
// can be copied or imported as is by another app such as mormiDiskFree, next to the Go counting engine in
// pkg/dirsize. An entry is any object {name, dir, ext, up}; the app says how big it is through sizeOf.
//
// A block's area is proportional to its size: a file's size, or a folder's counted size (from the size column, which
// Space, Alt+Shift+Enter and the treemap itself fill). Folders whose size is not known yet are left out of the layout
// and reported as "pending", so the picture never shows a guess. Empty items take no area. A folder with thousands of
// tiny files would draw thousands of unreadable slivers, so only the biggest MAX_BLOCKS items get a block of their own
// and the rest share one "others" block.

export const MAX_BLOCKS = 300;

// treemapItems picks the blocks for a panel's entries. sizeOf(entry) → bytes or null (unknown); doneOf(entry) → true
// when a folder's size is final (counted to the end), false while it is still being counted. It returns the items
// sorted by size (biggest first; equal sizes by name), the folders without a size, the empty items, the total of what
// is drawn and how many items the "others" block holds. oldOf(entry) → true when the size sizeOf gave is a remembered
// one from an earlier session (shown until the new count ends); such items get old: true.
export function treemapItems(entries, { sizeOf, doneOf = () => true, oldOf = () => false, max = MAX_BLOCKS } = {}) {
  const items = [], pending = [], empty = [];
  for (const e of entries) {
    if (!e || e.up) continue;
    const v = sizeOf(e);
    if (e.dir && (v == null || Number.isNaN(Number(v)))) { pending.push(e.name); continue; }
    const n = Math.max(0, Number(v) || 0);
    const counting = e.dir && !doneOf(e);
    if (n <= 0) { if (counting) pending.push(e.name); else empty.push(e.name); continue; }
    const it = { key: e.name, name: e.name, value: n, dir: !!e.dir, counting, ext: e.dir ? '' : (e.ext || '') };
    if (e.dir && oldOf(e)) it.old = true;
    items.push(it);
  }
  items.sort((a, b) => b.value - a.value || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  let others = 0;
  if (items.length > max) {
    const rest = items.splice(max - 1);
    others = rest.length;
    items.push({ key: '\u0000others', name: `${others} other items`, value: rest.reduce((s, it) => s + it.value, 0), dir: false, counting: false, ext: '', others: true });
  }
  const total = items.reduce((s, it) => s + it.value, 0);
  return { items, pending, empty, total, others };
}

// worst is the worst aspect ratio of a row of areas laid along a side of length side (the squarified algorithm keeps
// adding blocks to a row while this gets better).
function worst(row, side) {
  if (!row.length || side <= 0) return Infinity;
  let sum = 0, mx = 0, mn = Infinity;
  for (const a of row) { sum += a; if (a > mx) mx = a; if (a < mn) mn = a; }
  const s2 = side * side, sum2 = sum * sum;
  return Math.max((s2 * mx) / sum2, sum2 / (s2 * mn));
}

// squarify lays items ({key, value}, biggest first, values > 0) out in the rectangle {x, y, w, h}, as rectangles as
// close to squares as the sizes allow (Bruls, Huizing and van Wijk's squarified treemap). Every item gets one
// rectangle {key, x, y, w, h}; together they fill the rectangle exactly.
export function squarify(items, rect) {
  const out = [];
  const list = items.filter((it) => it.value > 0);
  const total = list.reduce((s, it) => s + it.value, 0);
  if (!list.length || rect.w <= 0 || rect.h <= 0 || total <= 0) return out;
  const scale = (rect.w * rect.h) / total;
  const areas = list.map((it) => it.value * scale);
  let { x, y, w, h } = rect;
  let i = 0;
  while (i < areas.length) {
    const side = Math.min(w, h);
    const row = [areas[i]];
    let j = i + 1;
    while (j < areas.length && worst([...row, areas[j]], side) <= worst(row, side)) row.push(areas[j++]);
    const sum = row.reduce((s, a) => s + a, 0);
    const last = j >= areas.length;
    if (w >= h) {
      // a column at the left, as wide as the row's area needs; the last row takes all that is left
      const cw = last ? w : sum / h;
      let cy = y;
      row.forEach((a, k) => {
        const bh = k === row.length - 1 ? y + h - cy : a / cw;
        out.push({ key: list[i + k].key, x, y: cy, w: cw, h: bh });
        cy += bh;
      });
      x += cw; w -= cw;
    } else {
      // a row along the top
      const rh = last ? h : sum / w;
      let cx = x;
      row.forEach((a, k) => {
        const bw = k === row.length - 1 ? x + w - cx : a / rh;
        out.push({ key: list[i + k].key, x: cx, y, w: bw, h: rh });
        cx += bw;
      });
      y += rh; h -= rh;
    }
    i = j;
  }
  return out;
}

// hash is a small stable string hash (FNV-1a), for picking a colour per extension.
function hash(s) {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 0x01000193); }
  return h >>> 0;
}

// blockHue is the colour of a block as a hue (0-359): folders all share the blue of the accent, files of one extension
// share a hue picked from the extension (so all .mp4 files look alike), files without one are grey (null).
export function blockHue(item) {
  if (item.dir) return 212;
  if (item.others || !item.ext) return null;
  return hash(item.ext.toLowerCase()) % 360;
}

// labelFits says whether a block is big enough for its name to be written in it.
export const labelFits = (r, minW = 44, minH = 18) => r.w >= minW && r.h >= minH;

// share is what part of the total a block is, for its tooltip: "42%", "0.4%", "<0.1%".
export function share(value, total) {
  if (!total) return '0%';
  const p = (value / total) * 100;
  if (p >= 10) return `${Math.round(p)}%`;
  if (p >= 0.1) return `${Math.round(p * 10) / 10}%`;
  return '<0.1%';
}

// stepIndex moves the highlighted block for an arrow key or Home/End: the blocks are in size order, so Down and Right
// go to the next smaller block, Up and Left to the next bigger one. -1 (nothing highlighted) starts at the biggest.
export function stepIndex(i, count, key) {
  if (count <= 0) return -1;
  if (i < 0 || i >= count) return key === 'End' ? count - 1 : 0;
  switch (key) {
    case 'Down': case 'Right': return Math.min(count - 1, i + 1);
    case 'Up': case 'Left': return Math.max(0, i - 1);
    case 'Home': return 0;
    case 'End': return count - 1;
    default: return i;
  }
}

// ageText says how long ago a remembered size was counted, for its tooltip: "today", "yesterday", "5 days ago", or the
// date for anything older than a month. when is in Unix seconds (as the Go store keeps it), now in milliseconds.
export function ageText(when, now = Date.now()) {
  if (!(when > 0)) return 'earlier';
  const day = 86400000;
  const startOf = (ms) => { const d = new Date(ms); d.setHours(0, 0, 0, 0); return d.getTime(); };
  const days = Math.round((startOf(now) - startOf(when * 1000)) / day);
  if (days <= 0) return 'today';
  if (days === 1) return 'yesterday';
  if (days <= 31) return `${days} days ago`;
  const d = new Date(when * 1000);
  return `on ${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
