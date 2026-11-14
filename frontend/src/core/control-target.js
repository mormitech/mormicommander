// Where control mode sends the next mouse event of a click (core/control.js, issue #42).
//
// A click is a sequence of events (mousedown, mouseup, click or contextmenu). The first of them may redraw what was
// under the pointer: pressing on a panel row moves the cursor there, and the panel redraws its rows, so the element the
// script aimed at is no longer in the page. Events sent to a removed element reach nobody (they bubble up through a
// tree that is no longer attached), which is why a right-click on a row opened no context menu. A real mouse sends each
// event to whatever is under the pointer at that moment, so this does the same: while the element is still in the page
// it keeps getting the events; once it is gone, the replacement found by refind() (the row with the same index), else
// whatever is now at the point, else the old element as a last resort.

// liveTarget returns the element the next event of a click at (x, y) should go to.
export function liveTarget(el, x, y, refind, doc = globalThis.document) {
  if (el && el.isConnected) return el;
  const again = refind ? refind() : null;
  if (again && again.isConnected) return again;
  const there = doc && doc.elementFromPoint ? doc.elementFromPoint(x, y) : null;
  return there || el;
}
