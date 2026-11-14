// The frecency jump's pure helpers (src/features/frecency.js, F-867, board issue #27): how typed text matches a folder,
// and how the ranked list from Go becomes popup items. No DOM here, so vitest checks it alone.
//
// Matching follows zoxide, so people who use it get what they expect: the typed words must appear in the folder's path
// in the order they were typed, ignoring case, and the last word must match the folder's own name (the last part of
// the path). The last word may also be "fuzzy": its letters in order with others between ("dwn" finds Downloads).
// The list keeps Go's order (best score first), so the matches stay ranked by frequency and recency.

// lastName is the folder's own name: the part after the last separator (trailing separators ignored).
export function lastName(path) {
  const t = String(path || '').replace(/[\\/]+$/, '');
  const i = Math.max(t.lastIndexOf('/'), t.lastIndexOf('\\'));
  return i >= 0 ? t.slice(i + 1) : t;
}

// subsequence says whether the letters of w appear in s in order.
function subsequence(w, s) {
  let j = 0;
  for (let i = 0; i < s.length && j < w.length; i++) if (s[i] === w[j]) j++;
  return j === w.length;
}

// frecencyMatch tells whether a folder path matches the typed query: 0 no, 2 the last word is in the folder's name,
// 1 it matches the name only fuzzily. An empty query matches everything (2).
export function frecencyMatch(path, query) {
  const words = String(query || '').toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return 2;
  const p = String(path || '').toLowerCase().replace(/[\\/]+$/, '');
  const name = lastName(p);
  const head = p.slice(0, p.length - name.length);
  // the words before the last one, in order, anywhere in the path
  let at = 0;
  for (const w of words.slice(0, -1)) {
    const i = p.indexOf(w, at);
    if (i < 0) return 0;
    at = i + w.length;
  }
  const last = words[words.length - 1];
  // the last word goes in the name, and after the earlier words unless they reached into the name themselves
  const from = Math.max(0, at - head.length);
  if (name.indexOf(last, from) >= 0) return 2;
  if (subsequence(last, name.slice(from))) return 1;
  return 0;
}

// frecencyItems turns Go's ranked list into popup items: the first `top` show at once, the rest only while something
// is typed. Folders that came from zoxide alone say so on the right.
export function frecencyItems(list, top = 30) {
  return (list || []).map((it, n) => ({
    label: it.path, detail: it.source === 'zoxide' ? 'zoxide' : '', value: it.path, path: true, only: n >= top,
  }));
}

// frecencyItemMatch is the popup's match function: folders match by frecencyMatch, other rows (the settings at the
// bottom) disappear while something is typed.
export function frecencyItemMatch(it, query) {
  return !!it.path && frecencyMatch(it.label, query) > 0;
}
