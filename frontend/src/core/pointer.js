// Pointer gestures for control mode (core/control.js, docs/control.md "Pointer gestures"). Control mode never moves the
// system's pointer, so features that only a mouse reaches (dragging a dialog by its title, resizing a column, dragging
// rows to reorder them, hover-only highlights and tooltips) had no way to be driven by a script. This module plays such
// a gesture inside the page the way a browser would: pointer events together with the matching mouse events, pointer
// capture honoured (a handler that captures the pointer gets the moves and the release, wherever they are), and the
// browser's own drag and drop (dragstart, dragenter, dragover, drop, dragend with a DataTransfer) when the pressed
// element is draggable. Each step goes to the element under the point, found with elementFromPoint.
//
// It only dispatches events: nothing here knows the app, so the tests (test/pointer.test.js) run it on a plain page.

const POINTER_ID = 1; // the id a real mouse has in WebKit and Chromium

// box gives an element's rectangle, or null when it has none (detached).
function box(el) {
  const r = el?.getBoundingClientRect?.();
  return r ? { left: r.left, top: r.top, width: r.width, height: r.height } : null;
}

// pointOf turns an element and an optional offset from its top-left corner into page coordinates; without an offset the
// middle of the element is meant. Offsets may be negative or larger than the element (a column border lies between two
// titles, a few pixels left of the second).
export function pointOf(el, x, y) {
  const r = box(el) || { left: 0, top: 0, width: 0, height: 0 };
  return { x: r.left + (x ?? r.width / 2), y: r.top + (y ?? r.height / 2) };
}

// path gives the points of a straight move from a to b in n steps (b included, a not).
export function path(a, b, n) {
  const steps = Math.max(1, Math.round(n || 1));
  const out = [];
  for (let i = 1; i <= steps; i++) out.push({ x: a.x + (b.x - a.x) * i / steps, y: a.y + (b.y - a.y) * i / steps });
  return out;
}

// A minimal DataTransfer for engines that can't make one (the test page): what the app's drag handlers use.
class FakeDataTransfer {
  constructor() { this.data = new Map(); this.dropEffect = 'none'; this.effectAllowed = 'all'; this.files = []; this.items = []; }
  get types() { return [...this.data.keys()]; }
  setData(type, v) { this.data.set(String(type), String(v)); }
  getData(type) { return this.data.get(String(type)) ?? ''; }
  clearData(type) { if (type == null) this.data.clear(); else this.data.delete(String(type)); }
  setDragImage() {}
}

// innermostAt gives the deepest element inside root (root included) whose box holds the point, or null.
export function innermostAt(root, p) {
  const holds = (el) => { const r = box(el); return r && r.width > 0 && r.height > 0 && p.x >= r.left && p.y >= r.top && p.x < r.left + r.width && p.y < r.top + r.height; };
  if (!holds(root)) return null;
  let cur = root;
  for (;;) {
    let next = null;
    for (const c of cur.children) if (holds(c)) next = c; // (the last one drawn wins, as on screen)
    if (!next) return cur;
    cur = next;
  }
}

function newDataTransfer(win) {
  try { if (win.DataTransfer) return new win.DataTransfer(); } catch { /* not constructible here */ }
  return new FakeDataTransfer();
}

// Gesture keeps what a gesture needs between its steps: the document, the modifier keys, which element captured the
// pointer, the element last under it (for over/out and enter/leave), and the drag and drop in progress.
class Gesture {
  constructor(doc, { button = 0, ctrl = false, shift = false, alt = false, meta = false, scope = null } = {}) {
    this.doc = doc;
    this.scope = scope;
    this.win = doc.defaultView || globalThis;
    this.button = button;
    this.mods = { ctrlKey: !!ctrl, shiftKey: !!shift, altKey: !!alt, metaKey: !!meta };
    this.captured = null;
    this.trace = [];
  }

  // at finds the element under a point. With a scope (a dialog box whose page copy is hidden because a dialog window
  // shows it, so the browser's hit test can't see it), a point outside what the hit test finds inside the scope is
  // looked up by the boxes of the scope's elements instead: the innermost one holding the point.
  at(p, fallback) {
    let el = this.doc.elementFromPoint?.(p.x, p.y);
    if (el && el.nodeType !== 1) el = null;
    if (this.scope && !(el && this.scope.contains(el))) el = innermostAt(this.scope, p) || el;
    return el || fallback || this.doc.body;
  }

  init(p, extra = {}) {
    return { bubbles: true, cancelable: true, composed: true, view: this.win, clientX: p.x, clientY: p.y, screenX: p.x, screenY: p.y, button: this.button, buttons: 0, ...this.mods, ...extra };
  }

  pointer(type, el, p, extra = {}) {
    const Ctor = this.win.PointerEvent || this.win.MouseEvent;
    const ev = new Ctor(type, this.init(p, { pointerId: POINTER_ID, pointerType: 'mouse', isPrimary: true, ...extra }));
    el.dispatchEvent(ev);
    return ev;
  }

  mouse(type, el, p, extra = {}) {
    const ev = new this.win.MouseEvent(type, this.init(p, extra));
    el.dispatchEvent(ev);
    return ev;
  }

  drag(type, el, p, dt, extra = {}) {
    const Ctor = this.win.DragEvent || this.win.MouseEvent;
    const ev = new Ctor(type, this.init(p, { dataTransfer: dt, ...extra }));
    // Some engines leave dataTransfer out of a constructed event; the handlers need it.
    if (ev.dataTransfer !== dt) Object.defineProperty(ev, 'dataTransfer', { value: dt });
    el.dispatchEvent(ev);
    return ev;
  }

  // hookCapture makes setPointerCapture work for the played pointer while the gesture lasts: a real one would throw
  // (no such pointer is down), and what it captures is where the moves and the release then go. unhook puts the
  // original methods back.
  hookCapture() {
    const proto = this.win.Element?.prototype;
    if (!proto) return;
    const g = this;
    this.saved = ['setPointerCapture', 'releasePointerCapture', 'hasPointerCapture'].map((k) => [k, Object.getOwnPropertyDescriptor(proto, k)]);
    const orig = Object.fromEntries(this.saved.map(([k, d]) => [k, d?.value]));
    Object.defineProperty(proto, 'setPointerCapture', { configurable: true, writable: true, value(id) { if (id === POINTER_ID) { g.captured = this; return undefined; } return orig.setPointerCapture?.call(this, id); } });
    Object.defineProperty(proto, 'releasePointerCapture', { configurable: true, writable: true, value(id) { if (id === POINTER_ID) { if (g.captured === this) g.captured = null; return undefined; } return orig.releasePointerCapture?.call(this, id); } });
    Object.defineProperty(proto, 'hasPointerCapture', { configurable: true, writable: true, value(id) { return id === POINTER_ID ? g.captured === this : !!orig.hasPointerCapture?.call(this, id); } });
  }

  unhook() {
    const proto = this.win.Element?.prototype;
    if (!proto || !this.saved) return;
    for (const [k, d] of this.saved) { if (d) Object.defineProperty(proto, k, d); else delete proto[k]; }
    this.saved = null;
  }
}

// Hover state lives across commands: the element the played pointer is over now, so the next hover sends it out/leave.
const hovered = new WeakMap(); // document → element

function chain(el) {
  const out = [];
  for (let n = el; n && n.nodeType === 1; n = n.parentElement) out.push(n);
  return out;
}

// moveOver sends what a browser sends when the pointer goes from the element it was over to el: pointerout/mouseout on
// the old one, pointerleave/mouseleave on the old ancestors el is not inside, pointerover/mouseover on el, and
// pointerenter/mouseenter on the new ancestors (outermost first). Nothing is sent when el is the same element.
function moveOver(g, el, p) {
  const old = hovered.get(g.doc) || null;
  if (old === el) return;
  const oldChain = old && old.isConnected !== false ? chain(old) : [];
  const newChain = el ? chain(el) : [];
  if (oldChain.length) {
    g.pointer('pointerout', old, p, { relatedTarget: el });
    g.mouse('mouseout', old, p, { relatedTarget: el });
    for (const n of oldChain) if (!newChain.includes(n)) { g.pointer('pointerleave', n, p, { bubbles: false, relatedTarget: el }); g.mouse('mouseleave', n, p, { bubbles: false, relatedTarget: el }); }
  }
  if (el) {
    g.pointer('pointerover', el, p, { relatedTarget: old });
    g.mouse('mouseover', el, p, { relatedTarget: old });
    for (const n of [...newChain].reverse()) if (!oldChain.includes(n)) { g.pointer('pointerenter', n, p, { bubbles: false, relatedTarget: old }); g.mouse('mouseenter', n, p, { bubbles: false, relatedTarget: old }); }
  }
  if (el) hovered.set(g.doc, el); else hovered.delete(g.doc);
}

function commonAncestor(a, b) {
  for (let n = a; n && n.nodeType === 1; n = n.parentElement) if (n === b || n.contains?.(b)) return n;
  return null;
}

function describe(el) {
  if (!el || el.nodeType !== 1) return '';
  const cls = typeof el.className === 'string' && el.className.trim() ? `.${el.className.trim().split(/\s+/).join('.')}` : '';
  return `${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ''}${cls}`;
}

// hover puts the played pointer over a point (over/enter events, then a move), with no button down: over `target`
// when given, else over what is under the point. Answers the element it is over.
export function hover(doc, p, target = null, mods = {}) {
  const g = new Gesture(doc, mods);
  const el = target || g.at(p);
  moveOver(g, el, p);
  g.pointer('pointermove', el, p);
  g.mouse('mousemove', el, p);
  return { over: describe(el) };
}

// unhover takes the played pointer off the page: out/leave events for what it was over.
export function unhover(doc) {
  const g = new Gesture(doc);
  const had = hovered.get(doc);
  moveOver(g, null, { x: -1, y: -1 });
  return { left: describe(had) };
}

// dragGesture presses the button at `from` (over the element `source`, or what is under the point), moves to `to` in
// `steps` steps (awaiting `wait()` between them, so the page can redraw), and lets go. When the pressed element is
// inside a draggable one and `dnd` is not false, the moves become the browser's drag and drop instead, as they do with
// a real mouse: dragstart on the draggable element, dragenter/dragleave/dragover on what is under each point, drop
// there when the last dragover was accepted (its default prevented), then dragend. Answers what happened.
export async function dragGesture(doc, { from, to, source = null, steps = 8, dnd = 'auto', wait = null, ...mods }) {
  const g = new Gesture(doc, mods);
  const pressed = source || g.at(from);
  const res = { from, to, pressed: describe(pressed), captured: '', dnd: false, dragstartCancelled: false, dropped: false, dropTarget: '', released: '' };
  g.hookCapture();
  try {
    moveOver(g, pressed, from);
    const down = g.pointer('pointerdown', pressed, from, { buttons: 1 });
    // a cancelled pointerdown still sends mousedown in WebKit and Chromium for a mouse; only the default is skipped
    g.mouse('mousedown', pressed, from, { buttons: 1 });
    res.captured = describe(g.captured);
    const dragEl = dnd === false ? null : pressed.closest?.('[draggable="true"]');
    let dt = null, over = null, accepted = false;
    for (const p of path(from, to, steps)) {
      if (wait) await wait();
      if (dragEl && !res.dnd && !res.dragstartCancelled) {
        dt = newDataTransfer(g.win);
        const ev = g.drag('dragstart', dragEl, from, dt, { buttons: 1 });
        if (ev.defaultPrevented) res.dragstartCancelled = true; else res.dnd = true;
        // the browser lets go of the pointer when a drag and drop starts
        if (res.dnd) g.pointer('pointercancel', g.captured || pressed, from);
      }
      if (res.dnd) {
        const el = g.at(p, dragEl);
        g.drag('drag', dragEl, p, dt, { buttons: 1 });
        if (el !== over) {
          g.drag('dragenter', el, p, dt, { buttons: 1, relatedTarget: over });
          if (over) g.drag('dragleave', over, p, dt, { buttons: 1, relatedTarget: el });
          over = el;
        }
        accepted = g.drag('dragover', el, p, dt, { buttons: 1 }).defaultPrevented;
        continue;
      }
      const target = g.captured || g.at(p, pressed);
      if (!g.captured) moveOver(g, target, p);
      g.pointer('pointermove', target, p, { buttons: 1 });
      g.mouse('mousemove', target, p, { buttons: 1 });
    }
    if (wait) await wait();
    if (res.dnd) {
      if (over && accepted) { g.drag('drop', over, to, dt); res.dropped = true; res.dropTarget = describe(over); } else if (over) g.drag('dragleave', over, to, dt);
      g.drag('dragend', dragEl, to, dt);
      moveOver(g, g.at(to, dragEl), to);
      return res;
    }
    const target = g.captured || g.at(to, pressed);
    res.released = describe(target);
    g.pointer('pointerup', target, to);
    g.mouse('mouseup', target, to);
    // A left button release is followed by a click on the nearest element that holds both where the button went down
    // and where it came up, as in a browser.
    const both = commonAncestor(pressed, target);
    if (g.button === 0 && both) g.mouse('click', both, to, { detail: 1 });
    if (g.captured) { const c = g.captured; g.captured = null; g.pointer('lostpointercapture', c, to); }
    const under = g.at(to, pressed);
    moveOver(g, under, to);
    if (down.defaultPrevented) res.pointerdownPrevented = true;
    return res;
  } finally {
    g.unhook();
  }
}
