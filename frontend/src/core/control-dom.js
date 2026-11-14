// What control mode's "dom" command answers about the elements matching a selector (core/control.js, issue #43).
//
// It used to describe only the first 50 matches; a script checking a long list (every row of a big folder, every
// button of a bar) could not see the rest. "limit" now says how many to describe: left out it stays 50, as before; 0
// or a negative number describes them all. "count" is always the number of all matches, and "more" says whether some
// were left out.

export const DOM_DEFAULT_LIMIT = 50;

// domLimit turns the command's "limit" into how many elements to describe (Infinity for all).
export function domLimit(limit) {
  if (limit === undefined || limit === null || limit === '') return DOM_DEFAULT_LIMIT;
  const n = Math.floor(Number(limit));
  if (!Number.isFinite(n)) return DOM_DEFAULT_LIMIT;
  return n <= 0 ? Infinity : n;
}

// domInfo describes the elements els (an array) for the "dom" command.
export function domInfo(els, limit) {
  const shown = els.slice(0, domLimit(limit));
  return {
    ok: true,
    count: els.length,
    more: shown.length < els.length,
    texts: shown.map((e) => e.textContent),
    visible: shown.map((e) => !!(e.offsetWidth || e.offsetHeight)),
    classes: shown.map((e) => e.className),
  };
}
