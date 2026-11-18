// When a folder read stops answering (a network share whose server is gone, a disk pulled out without ejecting): the
// rule for when a panel shows "Not responding, retrying…", and its words. Pure functions, so they test without a page;
// the notice itself is drawn by features/slowread.js.

// SLOW_MS is how long a panel may be reading a folder before the notice appears (issue #16: within 3 seconds).
export const SLOW_MS = 3000;

// slowStep follows one panel over time. since: when its current read started (null when it is not reading), loading:
// whether it is reading now, now: the time. Returns the new since, and whether the notice should show.
export function slowStep(since, loading, now, delay = SLOW_MS) {
  if (!loading) return { since: null, show: false };
  const s = since ?? now;
  return { since: s, show: now - s >= delay };
}

// slowText is what the notice says. prev is the folder the panel still shows (where Esc or Cancel leaves it), or ''
// when nothing was shown yet (the app is starting).
export function slowText(prev) {
  return {
    title: 'Not responding, retrying…',
    hint: prev ? `Esc or Cancel stays in ${prev}.` : 'Esc or Cancel stops waiting.',
  };
}

// countText is the line under the notice with how many entries of the folder were read so far (F-743), from
// go.FS.ReadCount; '' while none were, or when the count is not known (-1: an archive, a server).
export function countText(n) {
  n = Number(n);
  if (!Number.isFinite(n) || n <= 0) return '';
  return `${new Intl.NumberFormat().format(n)} ${n === 1 ? 'entry' : 'entries'} read so far`;
}
