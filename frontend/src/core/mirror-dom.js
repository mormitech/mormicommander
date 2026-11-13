// The pieces both sides of a mirrored dialog window share (see core/mirror.js for the page that owns the dialog and
// dialog-window.js for the window that shows it): which classes belong to the owning page only, which attributes are
// never copied, the dialog kinds that stay in the page, and the numbering that keeps messages in order. Nothing here
// needs a window, so the tests run it in Node.

// Classes starting with "mc-mirror" are the owning page's own marks (the hidden dialog, the dim backdrop); the copy in
// the window never gets them.
const MAIN_ONLY = /^mc-mirror/;

// cleanClass drops the owning page's own marks from a class attribute.
export function cleanClass(v) {
  return String(v ?? '').split(/\s+/).filter((c) => c && !MAIN_ONLY.test(c)).join(' ');
}

// copyAttr is the value an attribute gets in the copy, or null when it is not copied: inline event handlers (they would
// run in the window, away from the code they belong to), "autofocus" (the window's focus follows the real dialog) and
// javascript: links.
export function copyAttr(name, value) {
  const n = name.toLowerCase();
  if (n.startsWith('on') || n === 'autofocus') return null;
  if (n === 'class') return cleanClass(value);
  if ((n === 'href' || n === 'src' || n === 'action') && /^\s*javascript:/i.test(value)) return null;
  return value;
}

// Dialog kinds that always stay in the page, and why:
//   lister  – the Lister over the panels (viewerInWindow off): its view shows pictures from blob: URLs, which only
//             the page that made them can load, and it measures and scrolls its text itself; with Lister windows on
//             (the default) F3 has a real window of its own anyway.
// Besides these, a dialog drawn as a floating box (position: fixed, such as the right-click menu of the Synchronize
// window) is not a window: over a dialog window it is drawn inside that window, otherwise in the page.
export const PAGE_ONLY_KINDS = Object.freeze(['lister']);

// Sequencer applies numbered messages in order (1, 2, 3…), holding back any that arrive early; a message calls cannot
// overtake one another this way, whatever order Go delivers them in.
export class Sequencer {
  constructor(apply) { this.next = 1; this.held = new Map(); this.apply = apply; }
  push(seq, msg) {
    if (!(seq >= this.next)) return;
    this.held.set(seq, msg);
    while (this.held.has(this.next)) {
      const m = this.held.get(this.next);
      this.held.delete(this.next);
      this.next++;
      this.apply(m);
    }
  }
}

// isBigDialog: a dialog this big is a tool window (Options, Find, Synchronize…), which can be resized.
export const isBigDialog = (w, h) => w > 640 || h > 520;

// now is a time both windows agree on (milliseconds since 1970, with fractions), for the timings.
export const now = () => (globalThis.performance ? performance.timeOrigin + performance.now() : Date.now());
