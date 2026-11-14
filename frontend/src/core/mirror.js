// Dialogs in windows of their own, the side of the page that owns the dialog. modal() (dialogs.js) still builds every
// dialog here, in this page, so each feature's code, its closures and the app's state work as they always did, and
// control mode reads and answers the real dialog. The dialog is kept here invisibly (the page is dimmed behind it) and
// a native window (feature_dialog_window.go, dialog-window.js) shows a live copy of it:
//
//   - the copy starts as a description of the dialog's elements (tag, attributes, children) with the current values of
//     its fields; every element gets a number, which both sides use to name it;
//   - a MutationObserver on the dialog, and hooks on the value/checked/selectedIndex setters (code that fills a field
//     makes no mutation), turn every change into a patch: new children lists (elements the window already has are
//     named, not sent again, so nothing in the window is rebuilt, and focus, caret and scrolling stay), changed
//     attributes and texts, changed field values, focus, selection and scrolling;
//   - what the user does in the window comes back here and is replayed on the real elements: typing (the field's new
//     value and an input/change event), clicks, double clicks, right clicks and mouse moves (with the pointer events
//     before them, at the same place in the dialog), keys (so meta.onKey and Esc work, and Enter submits like a real
//     form), focus changes, scrolling, drags between rows, and the window being resized or closed (= cancel).
//
// A popup list opened from a dialog in a window (showList, a menu drawn as a floating box) is drawn in that window too,
// as a layer over the dialog, at the same place relative to the dialog. Messages are numbered both ways (mirror-dom.js)
// and go through Go (DialogWindows.Push / Input), one call each, batched per change.

import { go, on } from './go.js';
import { copyAttr, cleanClass, Sequencer, isBigDialog, now } from './mirror-dom.js';
import { setPopupArea } from './overlay.js';

const XHTML = 'http://www.w3.org/1999/xhtml';
const OBSERVE = { subtree: true, childList: true, attributes: true, characterData: true };
const sessions = new Map(); // window name → Session
const live = new Set(); // every open Session (also those still waiting for their window)
const stack = []; // the open Sessions, newest last
let quiet = 0; // > 0 while focus or scrolling from a window is replayed (not sent back)
let unavailable = false; // Go has no dialog windows (the browser preview): dialogs stay in the page

// mirrorAvailable tells whether dialogs can go to windows from this page.
export function mirrorAvailable() {
  if (unavailable || !globalThis.document?.body) return false;
  const b = document.body.classList;
  return !b.contains('mc-dialog-window') && !b.contains('about-mode');
}

// ---- field values ----

function propsOf(el) {
  switch (el.localName) {
    case 'input':
      if (el.type === 'checkbox' || el.type === 'radio') return { c: el.checked, n: el.indeterminate };
      if (el.type === 'file') return null;
      return { v: el.value };
    case 'textarea': return { v: el.value };
    case 'select': return el.multiple ? { m: [...el.options].map((o) => o.selected) } : { s: el.selectedIndex };
  }
  return null;
}

function setProps(el, p) {
  if (!p) return;
  if ('v' in p && el.value !== p.v) el.value = p.v;
  if ('c' in p) el.checked = !!p.c;
  if ('n' in p) el.indeterminate = !!p.n;
  if ('s' in p && el.selectedIndex !== p.s) el.selectedIndex = p.s;
  if ('m' in p) [...el.options].forEach((o, i) => { o.selected = !!p.m[i]; });
}

const sameProps = (a, b) => !!a && !!b && a.v === b.v && a.c === b.c && a.n === b.n && a.s === b.s && String(a.m) === String(b.m);

function depth(n) { let d = 0; for (; n; n = n.parentNode) d++; return d; }

// env is what the window needs of this page's look: the attributes of <html> (theme, platform, font settings), the
// classes of <body>, and the style elements made at run time (id "mc-…", such as the colour scheme Options applies
// live), so a change made in Options shows in its own window at once.
export function env() {
  const html = [...document.documentElement.attributes].map((a) => [a.name, a.value]);
  const styles = [...document.head.querySelectorAll('style[id^="mc-"]')].map((st) => [st.id, st.textContent]);
  return { html, body: cleanClass(document.body.className), styles };
}

// titleOf is the window title of a dialog: its heading.
function titleOf(box) {
  const h = box.querySelector(':scope > h2') || box.querySelector('h2');
  return (h?.textContent || '').replace(/\s+/g, ' ').trim();
}

// ---- one dialog in one window ----

class Session {
  constructor(entry, back, box, t0) {
    Object.assign(this, { entry, back, box, t0 });
    this.name = null;
    this.closed = false;
    this.ids = new WeakMap(); // element → number
    this.nodes = new Map(); // number → element
    this.known = new Set(); // numbers the window has
    this.nextId = 1;
    this.props = new WeakMap(); // field → the values the window has
    this.scrolls = new WeakMap(); // element → [top, left] the window has
    this.roots = new Map(); // key → element: "box", and the layers drawn over it
    this.kids = new Set();
    this.attrs = new Map();
    this.texts = new Set();
    this.removed = new Set();
    this.force = new Set(); // fields whose values are sent again (the window may have changed them itself)
    this.scrollPending = new Set();
    this.focusPending = null;
    this.selPending = null;
    this.seq = 0;
    this.queue = [];
    this.inSeq = new Sequencer((m) => { try { this.input(m); } catch (e) { console.error('dialog window input', e); } });
    this.title = '';
    this.asked = null; // the inside size last asked of Go
    this.userSized = false;
    this.layerSeq = 0;
    this.layerAt = null;
    this.shown = false;
    this.openMs = null;
    this.pings = new Map();
    this.pingWait = null;
    this.drag = null;
    this.pendingFlush = false;
    this.mo = new MutationObserver((recs) => { this.take(recs); this.flush(); });
  }

  get alive() { return !this.closed; }

  id(n) {
    let i = this.ids.get(n);
    if (!i) { i = this.nextId++; this.ids.set(n, i); }
    this.nodes.set(i, n);
    return i;
  }

  node(i) { return i ? this.nodes.get(i) || null : null; }

  // desc describes an element (or text) and everything in it, as the window builds it; fresh collects what was described.
  desc(n, fresh) {
    if (n.nodeType === 3) {
      const i = this.id(n);
      this.known.add(i);
      fresh?.add(n);
      return { i, x: n.data };
    }
    const i = this.id(n);
    this.known.add(i);
    fresh?.add(n);
    const d = { i, t: n.localName };
    if (n.namespaceURI !== XHTML) d.ns = n.namespaceURI;
    const a = [];
    for (const at of n.attributes) {
      const v = copyAttr(at.name, at.value);
      if (v !== null) a.push([at.name, v]);
    }
    if (a.length) d.a = a;
    if (n.localName !== 'script') {
      const c = [];
      for (const k of n.childNodes) if (k.nodeType === 1 || k.nodeType === 3) c.push(this.desc(k, fresh));
      if (c.length) d.c = c;
    }
    const p = propsOf(n);
    if (p) { d.p = p; this.props.set(n, p); }
    return d;
  }

  forget(n) {
    const i = this.ids.get(n);
    if (i) this.known.delete(i);
    for (const k of n.childNodes) forgetIn(this, k);
  }

  inTree(n) {
    if (!n || n.nodeType === undefined) return false;
    for (const r of this.roots.values()) if (r === n || r.contains(n)) return true;
    return false;
  }

  observeAll() {
    this.mo.disconnect();
    for (const r of this.roots.values()) this.mo.observe(r, OBSERVE);
  }

  take(recs) {
    for (const r of recs) {
      if (r.type === 'childList') { this.kids.add(r.target); r.removedNodes.forEach((n) => this.removed.add(n)); }
      else if (r.type === 'attributes') {
        let s = this.attrs.get(r.target);
        if (!s) this.attrs.set(r.target, (s = new Set()));
        s.add(r.attributeName);
      } else this.texts.add(r.target);
    }
  }

  schedule() {
    if (this.pendingFlush || this.closed) return;
    this.pendingFlush = true;
    queueMicrotask(() => { this.pendingFlush = false; this.flush(); });
  }

  // flush sends what changed since the last time, as one patch.
  flush() {
    if (this.closed) return;
    this.take(this.mo.takeRecords());
    const ops = [];
    for (const n of this.removed) if (!this.inTree(n)) this.forget(n);
    this.removed.clear();
    const fresh = new Set();
    const knownIn = (n) => this.known.has(this.ids.get(n)) && this.inTree(n);
    const parents = [...this.kids].filter(knownIn).sort((a, b) => depth(a) - depth(b));
    this.kids.clear();
    for (const p of parents) {
      if (fresh.has(p)) continue; // (described in full a moment ago, with its children)
      const c = [];
      for (const k of p.childNodes) {
        if (k.nodeType !== 1 && k.nodeType !== 3) continue;
        const i = this.ids.get(k);
        c.push(i && this.known.has(i) ? { r: i } : this.desc(k, fresh));
      }
      ops.push({ o: 'k', i: this.ids.get(p), c });
    }
    for (const [el, names] of this.attrs) {
      if (fresh.has(el) || !knownIn(el)) continue;
      const i = this.ids.get(el);
      for (const name of names) {
        if (!el.hasAttribute(name)) { ops.push({ o: 'a', i, n: name, v: null }); continue; }
        const v = copyAttr(name, el.getAttribute(name));
        if (v !== null) ops.push({ o: 'a', i, n: name, v });
      }
    }
    this.attrs.clear();
    for (const t of this.texts) if (!fresh.has(t) && knownIn(t)) ops.push({ o: 'x', i: this.ids.get(t), v: t.data });
    this.texts.clear();
    // field values (code that sets .value makes no mutation, so every field is compared with what the window has)
    for (const r of this.roots.values()) {
      for (const el of r.querySelectorAll('input, select, textarea')) {
        if (fresh.has(el)) continue;
        const i = this.ids.get(el);
        if (!i || !this.known.has(i)) continue;
        const p = propsOf(el);
        if (!p) continue;
        if (this.force.has(el) || !sameProps(p, this.props.get(el))) { this.props.set(el, p); ops.push({ o: 'p', i, p }); }
      }
    }
    this.force.clear();
    for (const el of this.scrollPending) {
      if (!knownIn(el)) continue;
      const t = el.scrollTop, l = el.scrollLeft, last = this.scrolls.get(el);
      if (last && last[0] === t && last[1] === l) continue;
      this.scrolls.set(el, [t, l]);
      ops.push({ o: 's', i: this.ids.get(el), t, l });
    }
    this.scrollPending.clear();
    if (this.focusPending) {
      const f = this.focusPending;
      this.focusPending = null;
      if (knownIn(f)) ops.push({ o: 'f', i: this.ids.get(f), sel: selOf(f) });
    }
    if (this.selPending) {
      const f = this.selPending;
      this.selPending = null;
      const sel = selOf(f);
      if (sel && knownIn(f)) ops.push({ o: 'sel', i: this.ids.get(f), a: sel[0], b: sel[1] });
    }
    if (this.roots.size > 1) { // layers follow the dialog when it moves in this page (its size changed)
      const r = this.box.getBoundingClientRect();
      if (!this.layerAt || this.layerAt[0] !== r.left || this.layerAt[1] !== r.top) {
        this.layerAt = [r.left, r.top];
        ops.push({ o: 'lo', x: -r.left, y: -r.top, w: innerWidth, h: innerHeight });
      }
    }
    if (ops.length) this.send(ops);
    if (!this.name) return;
    const t = titleOf(this.box);
    if (t !== this.title) { this.title = t; go.DialogWindows.SetTitle(this.name, t || 'mormiCommander').catch(() => {}); }
    if (!this.userSized) { // the window follows the dialog's size while the user has not resized it
      const m = this.measure();
      if (this.asked && (Math.abs(m.w - this.asked.w) > 1 || Math.abs(m.h - this.asked.h) > 1)) {
        this.asked = m;
        go.DialogWindows.Resize(this.name, m.w, m.h).catch(() => {});
      }
    }
  }

  send(ops) {
    const msg = { q: ++this.seq, ops };
    if (!this.name) { this.queue.push(msg); return; }
    go.DialogWindows.Push(this.name, msg).catch(() => {});
  }

  measure() {
    const r = this.box.getBoundingClientRect();
    return { w: Math.ceil(r.width), h: Math.ceil(r.height) };
  }

  // fix gives the hidden dialog the window's inside size (the user resized the window, or Go chose the size), so what
  // is laid out here matches what the window shows.
  fix(w, h) {
    this.userSized = true;
    this.asked = { w, h };
    this.box.style.setProperty('--mc-mirror-w', `${w}px`);
    this.box.style.setProperty('--mc-mirror-h', `${h}px`);
    this.box.classList.add('mc-mirror-fixed');
  }

  // ---- layers: popups and floating menus over the dialog ----

  addLayer(node) {
    this.flush();
    const key = `L${++this.layerSeq}`;
    node.classList.add('mc-mirror-layer');
    this.roots.set(key, node);
    this.observeAll();
    const r = this.box.getBoundingClientRect();
    this.layerAt = [r.left, r.top];
    this.send([{ o: 'L', key, d: this.desc(node), x: -r.left, y: -r.top, w: innerWidth, h: innerHeight }]);
    return key;
  }

  removeLayer(node) {
    for (const [key, n] of this.roots) {
      if (n !== node || key === 'box') continue;
      this.flush();
      this.roots.delete(key);
      this.observeAll();
      this.forget(node);
      node.classList.remove('mc-mirror-layer');
      this.send([{ o: 'l-', key }]);
      return;
    }
  }

  hasLayer(node) { for (const [key, n] of this.roots) if (key !== 'box' && n === node) return true; return false; }

  // ---- what the user did in the window ----

  input(m) {
    if (this.closed) return;
    switch (m.t) {
      case 'input': case 'change': {
        const el = this.node(m.i);
        if (!el || !this.inTree(el)) return;
        setProps(el, m.p);
        this.props.set(el, propsOf(el)); // (the window has this value: not sent back)
        el.dispatchEvent(new Event(m.t, { bubbles: true }));
        break;
      }
      case 'mouse': this.mouse(m); break;
      case 'key': this.key(m); break;
      case 'focus': {
        const el = this.node(m.i);
        if (!el || !this.inTree(el) || document.activeElement === el) return;
        quiet++;
        try { el.focus({ preventScroll: true }); } finally { quiet--; }
        return;
      }
      case 'scroll': {
        const el = this.node(m.i);
        if (!el || !this.inTree(el)) return;
        quiet++;
        try { el.scrollTop = m.top; el.scrollLeft = m.left; } finally { quiet--; }
        this.scrolls.set(el, [el.scrollTop, el.scrollLeft]);
        return;
      }
      case 'drag': this.dragEvent(m); break;
      case 'resize':
        if (!this.asked || Math.abs(m.w - this.asked.w) > 2 || Math.abs(m.h - this.asked.h) > 2) this.fix(m.w, m.h);
        break;
      case 'shown':
        this.shown = true;
        this.openMs = Math.round((m.ts - this.t0) * 10) / 10;
        this.paintMs = Math.round((m.painted - this.t0) * 10) / 10;
        this.steps = { sent: m.sent - this.t0, got: m.got - this.t0, built: m.built - this.t0, painted: m.painted - this.t0, showed: m.showed - this.t0, onScreen: m.ts - this.t0 };
        for (const k in this.steps) this.steps[k] = Math.round(this.steps[k] * 10) / 10;
        return;
      case 'ping': this.send([{ o: 'pong', ts: m.ts }]); return;
      case 'error': this.error = m.msg; console.error('dialog window:', m.msg); return;
      case 'wake': this.wakes = m.n; return; // (Go's watchdog woke the window: it had not drawn the dialog yet)
      case 'pingres': this.pingWait?.(m.rtts || []); this.pingWait = null; return;
      default: return;
    }
    this.flush();
  }

  // where maps a point of the window to this page (the window's dialog starts at its top left corner).
  where(m) {
    const b = this.box.getBoundingClientRect();
    return { clientX: b.left + (m.x || 0), clientY: b.top + (m.y || 0) };
  }

  mouseInit(m, extra = {}) {
    return {
      bubbles: true, cancelable: true, composed: true, view: window, ...this.where(m), screenX: m.sx || 0, screenY: m.sy || 0,
      button: m.b || 0, buttons: m.bs || 0, detail: m.d || 0, ctrlKey: !!m.ctrl, shiftKey: !!m.shift, altKey: !!m.alt, metaKey: !!m.meta, ...extra,
    };
  }

  mouse(m) {
    const el = (this.node(m.i) && this.inTree(this.node(m.i)) && this.node(m.i)) || this.box;
    const target = el.nodeType === 3 ? el.parentNode : el;
    const init = this.mouseInit(m);
    const ptr = { mousedown: 'pointerdown', mouseup: 'pointerup', mousemove: 'pointermove' }[m.type];
    if (ptr && globalThis.PointerEvent) target.dispatchEvent(new PointerEvent(ptr, { ...init, pointerId: 1, pointerType: 'mouse', isPrimary: true }));
    target.dispatchEvent(new MouseEvent(m.type, init));
    if (m.type === 'click' && target.matches?.('input[type=checkbox], input[type=radio]')) {
      // the window toggled its own copy; what the real one ended with is sent back in any case
      this.force.add(target);
      if (target.type === 'radio' && target.name) this.box.querySelectorAll('input[type=radio]').forEach((r) => { if (r.name === target.name) this.force.add(r); });
    }
  }

  key(m) {
    const t = this.node(m.i);
    const ae = document.activeElement;
    const el = t && this.inTree(t) ? t : this.inTree(ae) ? ae : this.box;
    const ev = new KeyboardEvent(m.type, {
      key: m.key, code: m.code, repeat: !!m.r, ctrlKey: !!m.ctrl, shiftKey: !!m.shift, altKey: !!m.alt, metaKey: !!m.meta,
      bubbles: true, cancelable: true, composed: true, view: window,
    });
    el.dispatchEvent(ev);
    if (m.type !== 'keydown' || ev.defaultPrevented || this.closed) return;
    if (m.key === 'Enter' && !m.ctrl && !m.alt && !m.meta) enterDefault(el);
  }

  dragEvent(m) {
    const el = this.node(m.i);
    if (!el || !this.inTree(el) || typeof DragEvent === 'undefined') return;
    if (m.type === 'dragstart') { try { this.drag = new DataTransfer(); } catch { this.drag = null; } }
    const ev = new DragEvent(m.type, { ...this.mouseInit(m), dataTransfer: this.drag });
    (el.nodeType === 3 ? el.parentNode : el).dispatchEvent(ev);
    if (m.type === 'dragend') this.drag = null;
  }

  // ---- closing ----

  close() {
    if (this.closed) return;
    this.flush();
    this.closed = true;
    this.mo.disconnect();
    live.delete(this);
    const k = stack.indexOf(this);
    if (k >= 0) stack.splice(k, 1);
    if (this.name) { sessions.delete(this.name); go.DialogWindows.Close(this.name).catch(() => {}); }
    this.pingWait?.([]);
    globalsIdle();
  }

  info() {
    return { name: this.name || '', shown: this.shown, openMs: this.openMs, paintMs: this.paintMs ?? null, steps: this.steps || null, error: this.error || '', wakes: this.wakes || 0, title: this.title, resized: this.userSized, layers: this.roots.size - 1 };
  }

  // ping measures round trips window → Go → this page → Go → window (n of them), in milliseconds.
  ping(n = 20) {
    if (!this.name || this.closed) return Promise.resolve([]);
    return new Promise((resolve) => {
      const t = setTimeout(() => { this.pingWait = null; resolve([]); }, 10000);
      this.pingWait = (r) => { clearTimeout(t); resolve(r); };
      this.send([{ o: 'pingreq', n }]);
    });
  }
}

function forgetIn(s, n) {
  const i = s.ids.get(n);
  if (i) s.known.delete(i);
  for (const k of n.childNodes) forgetIn(s, k);
}

function selOf(el) {
  try { return el.selectionStart != null && el.selectionEnd != null ? [el.selectionStart, el.selectionEnd] : null; } catch { return null; }
}

// enterDefault does what Enter does in a real form when nobody took the key: a button is pressed, a field submits the
// form through its default button.
function enterDefault(el) {
  if (!el?.matches || el.matches('textarea, select')) return;
  if (el.matches('button, input[type=button], input[type=submit], input[type=reset], a[href], summary')) { el.click(); return; }
  if (!el.matches('input')) return;
  const form = el.form || el.closest('form');
  if (!form) return;
  const def = [...form.elements].find((x) => x.matches('button:not([type]), button[type=submit], input[type=submit]'));
  if (def) { if (!def.disabled) def.click(); } else form.requestSubmit();
}

const topSession = () => { for (let k = stack.length - 1; k >= 0; k--) if (!stack[k].closed) return stack[k]; return null; };

// ---- the hooks this page needs while a dialog is in a window (installed once) ----

let installed = false;
let timer = 0;
let bodyMo = null;
let envMo = null;

const sessionOf = (n) => { for (const s of live) if (s.inTree(n)) return s; return null; };

function hookSetter(proto, prop) {
  const d = proto && Object.getOwnPropertyDescriptor(proto, prop);
  if (!d?.set) return;
  Object.defineProperty(proto, prop, {
    ...d,
    set(v) { d.set.call(this, v); if (live.size) sessionOf(this)?.schedule(); },
  });
}

function hookSelection(proto) {
  for (const m of ['setSelectionRange', 'select']) {
    const f = proto?.[m];
    if (typeof f !== 'function') continue;
    proto[m] = function (...args) {
      const r = f.apply(this, args);
      if (live.size && !quiet) { const s = sessionOf(this); if (s) { s.selPending = this; s.schedule(); } }
      return r;
    };
  }
}

function install() {
  if (installed) return;
  installed = true;
  hookSetter(HTMLInputElement.prototype, 'value');
  hookSetter(HTMLInputElement.prototype, 'checked');
  hookSetter(HTMLInputElement.prototype, 'indeterminate');
  hookSetter(HTMLTextAreaElement.prototype, 'value');
  hookSetter(HTMLSelectElement.prototype, 'value');
  hookSetter(HTMLSelectElement.prototype, 'selectedIndex');
  hookSetter(HTMLOptionElement.prototype, 'selected');
  hookSelection(HTMLInputElement.prototype);
  hookSelection(HTMLTextAreaElement.prototype);
  // a replayed pointer has no real pointer behind it: capturing it would throw, and the drag then works without
  const spc = Element.prototype.setPointerCapture;
  Element.prototype.setPointerCapture = function (id) { try { return spc.call(this, id); } catch (e) { if (!live.size) throw e; } };
  document.addEventListener('focusin', (ev) => {
    if (!live.size || quiet) return;
    const s = sessionOf(ev.target);
    if (s) { s.focusPending = ev.target; s.schedule(); }
  }, true);
  document.addEventListener('scroll', (ev) => {
    if (!live.size || quiet || ev.target?.nodeType !== 1) return;
    const s = sessionOf(ev.target);
    if (s) { s.scrollPending.add(ev.target); s.schedule(); }
  }, true);
  // a click on the dimmed page gives the dialog window the focus (and does not close a dialog that closes on outside
  // clicks: the window is still there)
  const backClick = (ev) => {
    if (!live.size || !ev.target?.classList?.contains('mc-mirror-back')) return;
    ev.stopImmediatePropagation();
    ev.preventDefault();
    if (ev.type === 'mousedown') go.DialogWindows.FocusTop().catch(() => {});
  };
  for (const t of ['mousedown', 'mouseup', 'click', 'dblclick', 'contextmenu']) document.addEventListener(t, backClick, true);
  on('dlg-input', (name, msg) => { const s = sessions.get(name); if (s && msg) s.inSeq.push(msg.q, msg); });
  on('dlg-closed', (name) => { const s = sessions.get(name); if (s && !s.closed) s.entry.cancel(); });
  bodyMo = new MutationObserver((recs) => {
    for (const r of recs) {
      for (const n of r.addedNodes) {
        if (n.nodeType === 1 && n.classList.contains('mc-popup')) topSession()?.addLayer(n);
      }
      for (const n of r.removedNodes) {
        if (n.nodeType !== 1) continue;
        for (const s of live) if (s.hasLayer(n) && n.classList.contains('mc-popup')) s.removeLayer(n);
      }
    }
  });
  envMo = new MutationObserver(() => { const e = env(); for (const s of live) s.send([{ o: 'e', env: e }]); });
  setPopupArea(() => {
    const s = topSession();
    if (!s) return null;
    const r = s.box.getBoundingClientRect();
    return { left: r.left, top: r.top, right: r.right, bottom: r.bottom };
  });
}

function globalsBusy() {
  if (timer) return;
  bodyMo.observe(document.body, { childList: true });
  envMo.observe(document.documentElement, { attributes: true });
  envMo.observe(document.body, { attributes: true, attributeFilter: ['class'] });
  envMo.observe(document.head, { childList: true, subtree: true, characterData: true });
  // a slow safety net for changes no hook sees (a checkbox toggled by its label in this page, a size change)
  timer = setInterval(() => { for (const s of live) s.flush(); }, 300);
}

function globalsIdle() {
  if (live.size || !timer) return;
  clearInterval(timer);
  timer = 0;
  bodyMo.disconnect();
  envMo.disconnect();
}

// mirrorDialog shows the dialog of a modal() entry in a window. back is its (dimmed) backdrop in this page and box the
// dialog; t0 when modal() was called. onFail runs when no window can be had (the dialog then stays in the page).
export function mirrorDialog(entry, back, box, t0, onFail) {
  install();
  const s = new Session(entry, back, box, t0);
  back.classList.add('mc-mirror-back');
  box.classList.add('mc-mirror-box');
  live.add(s);
  stack.push(s);
  globalsBusy();
  s.roots.set('box', box);
  s.observeAll();
  const size = s.measure();
  s.asked = size;
  const tree = s.desc(box);
  const ae = document.activeElement;
  const focus = box.contains(ae) ? s.ids.get(ae) || 0 : 0;
  s.title = titleOf(box);
  const bg = getComputedStyle(box).backgroundColor;
  const payload = { tree, focus, sel: focus ? selOf(ae) : null, env: env(), t0, sent: now() };
  go.DialogWindows.Open({ kind: entry.meta.kind || '', title: s.title, w: size.w, h: size.h, resizable: isBigDialog(size.w, size.h), bg, payload }).then((r) => {
    if (s.closed) { if (r?.name) go.DialogWindows.Close(r.name).catch(() => {}); return; }
    s.name = r.name;
    sessions.set(r.name, s);
    if (r.sized || Math.abs(r.w - size.w) > 1 || Math.abs(r.h - size.h) > 1) s.fix(r.w, r.h);
    for (const m of s.queue) go.DialogWindows.Push(s.name, m).catch(() => {});
    s.queue = [];
    s.flush();
  }, (e) => {
    if (/not available/i.test(String(e?.message ?? e))) unavailable = true;
    if (s.closed) return;
    s.closed = true;
    s.mo.disconnect();
    live.delete(s);
    stack.splice(stack.indexOf(s), 1);
    back.classList.remove('mc-mirror-back');
    box.classList.remove('mc-mirror-box', 'mc-mirror-fixed');
    globalsIdle();
    entry.mirror = null;
    onFail?.(e);
  });
  return s;
}

// pingDialogWindow measures input round trips with the top dialog window (control mode's "dialog_ping").
export async function pingDialogWindow(n = 20) {
  const s = topSession();
  if (!s) throw new Error('no dialog is in a window');
  const rtts = await s.ping(n);
  const sorted = [...rtts].sort((a, b) => a - b);
  const pick = (q) => (sorted.length ? Math.round(sorted[Math.min(sorted.length - 1, Math.floor(q * sorted.length))] * 100) / 100 : null);
  return { window: s.name, count: rtts.length, min: pick(0), median: pick(0.5), p90: pick(0.9), max: pick(0.999), rtts: rtts.map((x) => Math.round(x * 100) / 100) };
}

export { now };
