// Pure helpers of the reserved Total Commander commands that got a body in board issue #38 (features/reserved.js):
// the file mask of the custom filter (Ctrl+F12, "Define custom filter…"), the search of the command browser
// ("All commands…") and the items of the command line history popup (Alt+F8). No DOM here, so vitest can test it.
//
// The mask rules are the same as internal/config/mask.go (the Go side's TC mask): patterns separated by spaces or ";",
// quotes keep spaces, "|" starts the patterns to leave out, "*" and "?" are wildcards, case does not matter, "*.*"
// matches every name and "name.*" also matches "name". A pattern ending in "/" or "\" matches folders only.

// splitPatterns cuts mask text into patterns, keeping quoted parts whole.
function splitPatterns(s) {
  const out = [];
  let cur = '', inQuote = false;
  for (const ch of s) {
    if (ch === '"') inQuote = !inQuote;
    else if ((ch === ' ' || ch === ';' || ch === '\t') && !inQuote) { if (cur) out.push(cur); cur = ''; }
    else cur += ch;
  }
  if (cur) out.push(cur);
  return out;
}

// parseMask turns "*.txt *.md | *.bak" into {include, exclude}. An empty include list means everything when there is
// an exclude part, and nothing otherwise (as in Go).
export function parseMask(s) {
  const text = String(s ?? '');
  const bar = text.indexOf('|');
  const include = splitPatterns(bar < 0 ? text : text.slice(0, bar));
  const exclude = bar < 0 ? [] : splitPatterns(text.slice(bar + 1));
  if (bar >= 0 && include.length === 0) include.push('*');
  return { include, exclude };
}

// wildMatch matches * (any run of characters) and ? (one character); "[" is a plain character, as in TC.
function wildMatch(p, s) {
  if (p === '*.*' || p === '*') return true;
  if (p.endsWith('.*') && !s.includes('.') && wildMatch(p.slice(0, -2), s)) return true;
  const pr = [...p], sr = [...s];
  let pi = 0, si = 0, star = -1, mark = 0;
  while (si < sr.length) {
    if (pi < pr.length && (pr[pi] === '?' || pr[pi] === sr[si])) { pi++; si++; }
    else if (pi < pr.length && pr[pi] === '*') { star = pi; mark = si; pi++; }
    else if (star >= 0) { pi = star + 1; mark++; si = mark; }
    else return false;
  }
  while (pi < pr.length && pr[pi] === '*') pi++;
  return pi === pr.length;
}

function matchPattern(p, name, isDir) {
  if (p.endsWith('/') || p.endsWith('\\')) {
    if (!isDir) return false;
    p = p.replace(/[\\/]+$/, '');
  }
  if (/[\\/]/.test(p)) return false; // a path pattern needs a full path; the panel filter only knows names
  return wildMatch(p.toLowerCase(), String(name).toLowerCase());
}

// matchMask says whether a name passes a parsed mask (parseMask) or mask text.
export function matchMask(mask, name, isDir = false) {
  const m = typeof mask === 'string' ? parseMask(mask) : mask;
  return m.include.some((p) => matchPattern(p, name, isDir)) && !m.exclude.some((p) => matchPattern(p, name, isDir));
}

// maskFilter is the panel filter of a custom mask: folders always stay (so one can still walk around, as in TC), files
// show when they match. The function is marked with mcSrc so the menu can tick "Custom filter".
export function maskFilter(text) {
  const m = parseMask(text);
  const fn = (e) => !!e.up || !!e.dir || matchMask(m, e.name, false);
  fn.mcSrc = 'user';
  fn.mcMask = String(text);
  return fn;
}

// execFilter is the panel filter of Show > Programs (Ctrl+F11): folders and the files that can be run (the Unix x bit,
// or .exe/.bat/.cmd/.com on Windows; the Go listing sets the flag).
export function execFilter() {
  const fn = (e) => !!e.up || !!e.dir || !!e.exec;
  fn.mcSrc = 'execs';
  return fn;
}

// filterCommands lists the commands whose name or label holds every word of text (case does not matter), sorted by
// label. An empty text lists them all.
export function filterCommands(list, text = '') {
  const words = String(text).toLowerCase().split(/\s+/).filter(Boolean);
  const plain = (c) => String(c.label || c.name).replace(/…$/, '');
  return list
    .filter((c) => { const hay = `${c.name} ${c.label || ''}`.toLowerCase(); return words.every((w) => hay.includes(w)); })
    .sort((a, b) => plain(a).localeCompare(plain(b), undefined, { sensitivity: 'base' }) || a.name.localeCompare(b.name));
}

// historyItems turns the command line history (newest first) into popup items, at most max of them.
export function historyItems(history, max = 50) {
  return (Array.isArray(history) ? history : []).filter((x) => typeof x === 'string' && x.trim()).slice(0, max)
    .map((x) => ({ label: x, value: x }));
}

// selectionFilter is the panel filter of Show only selected files (keep true, cm_ShowOnlySelected) and Hide selected
// files (keep false, cm_HideSelected, F-408): it lets through only the names given, or all but them. It works on top of
// the filter the panel had (prev, with its path bar label), and is marked mcReread so a reread (Ctrl+R) drops it and
// puts that earlier filter back (Panel.reload).
export function selectionFilter(names, keep, prev = null, prevLabel = '') {
  const set = new Set(names);
  return Object.assign((e) => set.has(e.name) === keep && (!prev || prev(e)), {
    mcSrc: keep ? 'onlysel' : 'hidesel', mcReread: true, mcPrev: prev || null, mcPrevLabel: prev ? prevLabel : '',
  });
}

