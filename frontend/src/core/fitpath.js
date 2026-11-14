// Shortening long paths at the start, so the end (the folder you are in) stays visible (issue #129).
//
// A panel's path bar and the command line's prompt show the current folder's path. When it doesn't fit, Total Commander
// drops the start of the path and keeps the end. CSS alone can't do that well: `text-overflow: ellipsis` cuts the end,
// and the right-to-left trick (direction: rtl with a <bdi>) leaves a wide empty gap after the "…" in WebKit. So the
// page measures and cuts here instead: the path bar hides whole leading parts behind a clickable "…" (which goes to the
// deepest hidden folder), and the prompt keeps as many trailing characters as fit after a "…". The arithmetic is in pure
// functions (crumbCut, tailCut) so it is tested without a browser; the DOM helpers only measure and apply. Where
// nothing is laid out (tests, a hidden panel) every width is 0 and nothing is cut.

export const ELLIPSIS = '…';

// crumbCut says how many leading parts of the path bar to hide so the rest fits: widths are the parts' widths in
// order, avail the room there is, ellW the width of the "…" that stands for the hidden ones. 0 when everything fits;
// never more than widths.length - 1, so the last part (the current folder) always stays.
export function crumbCut(widths, avail, ellW) {
  const n = widths.length;
  let rest = widths.reduce((a, w) => a + w, 0);
  if (n < 2 || rest <= avail) return 0;
  for (let k = 1; k < n; k++) {
    rest -= widths[k - 1];
    if (ellW + rest <= avail) return k;
  }
  return n - 1;
}

// tailCut finds how many characters to drop from the start of a text so that "…" plus the rest fits: fits(i) says
// whether "…" + text.slice(i) fits and is assumed to hold more often as i grows. 0 when the whole text fits as it is
// (fits0), else the smallest i that fits (binary search), at most text.length - 1 so one character stays.
export function tailCut(len, fits0, fits) {
  if (fits0 || len < 2) return 0;
  let lo = 1, hi = len - 1;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (fits(mid)) hi = mid; else lo = mid + 1;
  }
  return lo;
}

// overflows says whether an element's content is wider than the element (with a pixel of slack for rounding).
function overflows(el) {
  return el.scrollWidth > el.clientWidth + 1;
}

// fitCrumbs shortens a path bar's parts (spans with data-p, one per folder) from the start until they fit: the hidden
// ones stay in the page with the hidden attribute, and a "…" span (class "cut") in front of the rest goes to the
// deepest hidden folder when clicked. Calling it again starts over, so it is also what runs when the bar is resized.
// If even the last part alone is too wide, its own text is shortened at the start.
export function fitCrumbs(box) {
  for (const c of [...box.children]) if (c.classList.contains('cut')) c.remove();
  const parts = [...box.children].filter((c) => c.dataset.p != null);
  for (const s of parts) {
    s.hidden = false;
    if (s.dataset.label != null) { s.textContent = s.dataset.label; delete s.dataset.label; }
  }
  if (parts.length === 0 || !overflows(box)) return;
  const cut = document.createElement('span');
  cut.className = 'cut';
  cut.textContent = ELLIPSIS;
  box.prepend(cut);
  const ellW = cut.getBoundingClientRect().width;
  const widths = parts.map((s) => s.getBoundingClientRect().width);
  const k = crumbCut(widths, box.clientWidth, ellW);
  if (k === 0) { cut.remove(); return; }
  for (let i = 0; i < k; i++) parts[i].hidden = true;
  cut.dataset.p = parts[k - 1].dataset.p;
  cut.title = parts[k - 1].dataset.p;
  if (!overflows(box)) return;
  // Only the last part is left and it is still too wide: drop the extra "…" and cut that part's own text instead.
  cut.remove();
  const last = parts[parts.length - 1];
  fitText(last, last.textContent, box);
}

// fitText puts text into el, shortened at the start with "…" if it is wider than box (el itself by default). The full
// text is kept in el.dataset.label when it was shortened.
export function fitText(el, text, box = el) {
  el.textContent = text;
  if (!overflows(box)) return;
  const i = tailCut(text.length, false, (j) => { el.textContent = ELLIPSIS + text.slice(j); return !overflows(box); });
  el.textContent = ELLIPSIS + text.slice(i);
  el.dataset.label = text;
}
