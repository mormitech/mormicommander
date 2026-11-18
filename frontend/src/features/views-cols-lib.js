// Pure helpers for the column header (FEATURES.md F-672, F-673): which columns of a view the user hid from the header's
// right-click menu, and the widths of "fit columns to their contents" and "equal column widths". views.js and
// columns.js use them; frontend/test/views-cols.test.js tests them.
//
// Hidden columns are kept in the "views" setting as hiddenCols: {"<view key>": ["Size", ...]} by column title; the view
// key is "full", "comments" or "custom:<set name>", so each column set keeps its own. The name column is never hidden.

// viewKey names the view a panel shows for hiddenCols ("" when the view has no columns to hide).
export function viewKey(vs) {
  if (!vs) return '';
  if (vs.mode === 'full' || vs.mode === 'comments') return vs.mode;
  if (vs.mode === 'custom' && vs.set) return `custom:${vs.set}`;
  return '';
}

// hiddenFor lists the titles hidden in a view.
export function hiddenFor(map, key) {
  const v = map && key ? map[key] : null;
  return Array.isArray(v) ? v.filter((t) => typeof t === 'string') : [];
}

// visibleCols drops the hidden columns (by title) from a column list.
export function visibleCols(cols, hidden) {
  if (!hidden?.length) return cols;
  const h = new Set(hidden);
  return cols.filter((c) => !h.has(c.title || ''));
}

// toggleHidden returns a new hiddenCols map with title hidden or shown again in view key; a view with nothing hidden
// is removed from the map.
export function toggleHidden(map, key, title) {
  const out = { ...(map || {}) };
  const now = hiddenFor(out, key);
  const next = now.includes(title) ? now.filter((t) => t !== title) : [...now, title];
  if (next.length) out[key] = next;
  else delete out[key];
  return out;
}

// equalWidths gives every one of n columns (the name column included) the same share of total pixels, with gap pixels
// between columns; never narrower than min.
export function equalWidths(total, n, gap = 8, min = 28) {
  if (!(n > 0)) return [];
  const w = Math.max(min, Math.floor((total - gap * (n - 1)) / n));
  return Array(n).fill(w);
}

// fitWidths gives each column the width of its widest text: texts[k] is the header title and the cells of column k,
// measure(text) their width in pixels. pad is added for the cell padding; the result stays within [min, max].
export function fitWidths(texts, measure, { pad = 14, min = 28, max = 600 } = {}) {
  return texts.map((list) => {
    let w = 0;
    for (const t of list) if (t) w = Math.max(w, measure(String(t)));
    return Math.max(min, Math.min(max, Math.ceil(w + pad)));
  });
}
