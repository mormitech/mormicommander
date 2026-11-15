// The command palette's pure logic (board #21, F-857, F-373): fuzzy matching of what the user typed against the
// palette's items, ranking with recent use, and the list of recently used items kept in the settings. It has no DOM and
// no app, so vitest tests it alone (frontend/test/palette-lib.test.js); features/palette.js draws the dialog and
// gathers the items (commands, folder hotlist, folder history, open tabs, Options pages).
//
// An item is { key, label, detail?, words?, group, disabled? }: key is unique and stable ("cmd:cm_SyncDirs",
// "dir:/Users/me"), label is what the user reads and mostly searches, words are extra search words (the cm_* name, the
// folder path) that count a little less.

export const RECENT_MAX = 30;

const lower = (s) => String(s ?? '').toLowerCase();
const isWordStart = (text, i) => i === 0 || /[\s_\-./\\:()[\]]/.test(text[i - 1]) || (/[a-z]/.test(text[i - 1]) && /[A-Z]/.test(text[i]));

// matchWord scores one query word (lower case, no spaces) against text: a higher number is a better match, -1 is no
// match. A plain substring wins over letters spread out; a match at the start of the text or of a word wins over one in
// the middle; spread-out letters (a subsequence, "sydi" for "Sync dirs") score by how many land on word starts and how
// many follow each other.
export function matchWord(word, text) {
  if (!word) return 0;
  const t = String(text ?? '');
  const low = t.toLowerCase();
  const at = low.indexOf(word);
  if (at >= 0) {
    // the best place for a substring: the start of a word if there is one
    let best = at, i = at;
    while (i >= 0) { if (isWordStart(t, i)) { best = i; break; } i = low.indexOf(word, i + 1); }
    let s = 100 + word.length * 4;
    if (best === 0) s += 40;
    else if (isWordStart(t, best)) s += 25;
    else s -= Math.min(20, best);
    if (word.length === low.length) s += 20; // the whole text
    return s;
  }
  // spread-out letters, in order
  let s = 0, pos = 0, prev = -2;
  for (const ch of word) {
    const i = low.indexOf(ch, pos);
    if (i < 0) return -1;
    s += 2;
    if (isWordStart(t, i)) s += 8;
    if (i === prev + 1) s += 5;
    else s -= Math.min(4, i - pos);
    prev = i;
    pos = i + 1;
  }
  return Math.max(1, s);
}

// scoreItem scores a query (any number of words) against an item: every word must match the label or the extra words;
// the label counts fully, the extra words at about two thirds. -1 when a word matches neither.
export function scoreItem(query, item) {
  const words = lower(query).split(/\s+/).filter(Boolean);
  if (!words.length) return 0;
  const extra = [item.detail, ...(item.words || [])].filter(Boolean).join(' ');
  let total = 0;
  for (const w of words) {
    const a = matchWord(w, item.label);
    const b = extra ? matchWord(w, extra) : -1;
    const s = Math.max(a, b >= 0 ? Math.round(b * 0.65) : -1);
    if (s < 0) return -1;
    total += s;
  }
  return total;
}

// rankItems is what the palette shows for a query: with nothing typed, the recently used items first (newest first),
// then the rest in the order given; with a query, only the items that match, best first, where recent use adds a
// bonus that fades with age, so "sync" puts the Synchronize command you ran yesterday above a folder called "sync".
// Disabled items keep their place among equals but sink below usable ones of the same score. limit cuts the list.
export function rankItems(items, query, recent = [], limit = Infinity) {
  const age = new Map();
  (recent || []).forEach((k, i) => { if (!age.has(k)) age.set(k, i); });
  const q = String(query ?? '').trim();
  let out;
  if (!q) {
    const rec = items.filter((it) => age.has(it.key)).sort((a, b) => age.get(a.key) - age.get(b.key));
    const recKeys = new Set(rec.map((it) => it.key));
    out = [...rec, ...items.filter((it) => !recKeys.has(it.key))];
  } else {
    const scored = [];
    items.forEach((it, i) => {
      const s = scoreItem(q, it);
      if (s < 0) return;
      const bonus = age.has(it.key) ? Math.max(5, 60 - age.get(it.key) * 2) : 0;
      scored.push({ it, s: s + bonus - (it.disabled ? 30 : 0), i });
    });
    scored.sort((a, b) => b.s - a.s || a.i - b.i);
    out = scored.map((x) => x.it);
  }
  return Number.isFinite(limit) ? out.slice(0, limit) : out;
}

// noteRecent puts key at the front of the recent list (once), keeping at most max entries. It returns a new array.
export function noteRecent(recent, key, max = RECENT_MAX) {
  const list = Array.isArray(recent) ? recent.filter((k) => typeof k === 'string' && k !== key) : [];
  return [key, ...list].slice(0, max);
}

// commandWords are a command's extra search words: its cm_* name whole and split into words ("cm_SyncDirs" → "SyncDirs
// sync dirs"), so typing the Total Commander name finds it too.
export function commandWords(name) {
  const bare = String(name || '').replace(/^(cm|em)_/, '');
  return [String(name || ''), bare, bare.replace(/([a-z0-9])([A-Z])/g, '$1 $2').toLowerCase()];
}

// uniqueFolders lists folders once each (the first spelling wins), case-insensitively unless caseSensitive, ignoring a
// trailing separator; empty entries are dropped.
export function uniqueFolders(paths, caseSensitive = false) {
  const seen = new Set();
  const out = [];
  for (const p of paths || []) {
    if (!p) continue;
    const t = p.length > 1 ? p.replace(/[\\/]+$/, '') || p : p;
    const k = caseSensitive ? t : t.toLowerCase();
    if (seen.has(k)) continue;
    seen.add(k);
    out.push(p);
  }
  return out;
}

// flattenHotlist turns the hotlist tree (entries {kind: 'cmd' | 'menu' | 'sep', title/label, cmd, items}) into a flat
// list of {entry, label, trail}: every runnable entry once, with the submenus it sits in as its trail ("Work › Sites").
export function flattenHotlist(tree, trail = []) {
  const out = [];
  for (const it of tree || []) {
    if (!it || it.kind === 'sep') continue;
    const name = it.label || it.title || '';
    if (it.kind === 'menu') { out.push(...flattenHotlist(it.items || [], [...trail, name])); continue; }
    if (!it.cmd && !name) continue;
    out.push({ entry: it, label: name || String(it.cmd || '').replace(/^cd\s+/i, '').replace(/^"(.*)"$/, '$1'), trail: trail.join(' › ') });
  }
  return out;
}

// assignTarget is the command a palette item stands for, for "Assign key…" (F2) in the command browser (F-373): the
// cm_* or em_* name of a command item, "" for anything else (a folder, a tab, an Options page) or no item at all.
export function assignTarget(item) {
  if (!item || item.group !== 'cmd' || typeof item.key !== 'string' || !item.key.startsWith('cmd:')) return '';
  const name = item.key.slice(4);
  return /^(cm|em)_\w+$/.test(name) ? name : '';
}
