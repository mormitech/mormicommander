// The copy of a dialog in a dialog window (dialog-window.js): builds it from the description core/mirror.js sends and
// applies that page's patches to it. Elements the copy already has are moved, never rebuilt, so focus, caret, selection
// and scrolling survive every patch (a morph, not a re-render). It only needs a document, so the tests run it in
// happy-dom together with mirror.js (test/mirror.test.js).

import { copyAttr } from './mirror-dom.js';

// windowProps is what a field of the copy holds (sent back to the owning page with input/change).
export function windowProps(el) {
  if (el.localName === 'input') return el.type === 'checkbox' || el.type === 'radio' ? { c: el.checked } : el.type === 'file' ? null : { v: el.value };
  if (el.localName === 'textarea') return { v: el.value };
  if (el.localName === 'select') return el.multiple ? { m: [...el.options].map((o) => o.selected) } : { s: el.selectedIndex };
  return null;
}

export class Copy {
  // hooks: {pong(ts), pingreq(n)} for the round-trip measurements
  constructor(doc, hooks = {}) {
    this.doc = doc;
    this.hooks = hooks;
    this.nodes = new Map(); // number → node
    this.ids = new WeakMap(); // node → number
    this.layers = new Map(); // key → layer element
    this.lastP = new WeakMap(); // field → the values the owning page gave it
    this.lastScroll = new WeakMap();
    this.box = null;
    this.echoFocus = null; // a focus this copy made itself (not sent back)
  }

  // idOf is the number of a node, or of the nearest element around it that has one (0: none).
  idOf(n) {
    for (; n; n = n.parentNode) { const i = this.ids.get(n); if (i) return i; }
    return 0;
  }

  build(d) {
    const doc = this.doc;
    let n;
    if (d.x !== undefined) n = doc.createTextNode(d.x);
    else {
      n = d.ns ? doc.createElementNS(d.ns, d.t) : doc.createElement(d.t);
      for (const [k, v] of d.a || []) {
        const val = copyAttr(k, v);
        if (val !== null) try { n.setAttribute(k, val); } catch { /* a name the DOM does not take */ }
      }
      for (const c of d.c || []) n.append(this.build(c));
      if (d.p) this.setProps(n, d.p);
    }
    this.nodes.set(d.i, n);
    this.ids.set(n, d.i);
    return n;
  }

  // root builds the dialog itself (the box), marked as the window's box.
  root(tree) {
    this.box = this.build(tree);
    this.box.classList.add('mc-window-box');
    return this.box;
  }

  setProps(el, p) {
    this.lastP.set(el, p);
    if ('v' in p && el.value !== p.v) {
      if (this.doc.activeElement === el && typeof el.selectionStart === 'number') {
        const a = el.selectionStart, b = el.selectionEnd;
        el.value = p.v;
        try { el.setSelectionRange(Math.min(a, p.v.length), Math.min(b, p.v.length)); } catch { /* not a text field */ }
      } else el.value = p.v;
    }
    if ('c' in p) el.checked = !!p.c;
    if ('n' in p) el.indeterminate = !!p.n;
    if ('s' in p && el.selectedIndex !== p.s) el.selectedIndex = p.s;
    if ('m' in p) [...el.options].forEach((o, i) => { o.selected = !!p.m[i]; });
  }

  // applyEnv copies the owning page's look: the attributes of <html> (theme, font sizes as inline variables), the
  // classes of <body>, and the style elements it made at run time (the colour scheme of Options, "mc-…" ids).
  applyEnv(e) {
    if (!e) return;
    const root = this.doc.documentElement;
    const keep = new Set(e.html.map(([k]) => k));
    for (const a of [...root.attributes]) if (!keep.has(a.name)) root.removeAttribute(a.name);
    for (const [k, v] of e.html) if (root.getAttribute(k) !== v) root.setAttribute(k, v);
    this.doc.body.className = `mc-dialog-window ${e.body || ''}`.trim();
    if (e.styles) {
      const want = new Map(e.styles);
      for (const st of [...this.doc.head.querySelectorAll('style[data-mirror]')]) if (!want.has(st.dataset.mirror)) st.remove();
      for (const [id, css] of want) {
        let st = this.doc.head.querySelector(`style[data-mirror="${id.replace(/["\\]/g, '')}"]`);
        if (!st) { st = this.doc.createElement('style'); st.dataset.mirror = id; this.doc.head.append(st); }
        if (st.textContent !== css) st.textContent = css;
      }
    }
  }

  // apply carries out one patch of the owning page.
  apply(m) {
    for (const op of m.ops || []) {
      try { this.applyOp(op); } catch (e) { console.error('dialog window patch', op, e); }
    }
  }

  applyOp(op) {
    const el = op.i ? this.nodes.get(op.i) : null;
    switch (op.o) {
      case 'k': { // the new children list of el: known nodes by number (moved), new ones described
        if (!el) return;
        const list = op.c.map((c) => (c.r ? this.nodes.get(c.r) : this.build(c))).filter(Boolean);
        list.forEach((n, j) => { const at = el.childNodes[j]; if (at !== n) el.insertBefore(n, at || null); });
        while (el.childNodes.length > list.length) el.lastChild.remove();
        const sel = el.closest?.('select'); // (new options can change a list's choice: the owning page's one is kept)
        if (sel && this.lastP.has(sel)) this.setProps(sel, this.lastP.get(sel));
        return;
      }
      case 'a': {
        if (!el) return;
        const v = op.v === null ? null : copyAttr(op.n, op.v);
        if (v === null) el.removeAttribute(op.n); else el.setAttribute(op.n, v);
        if (el === this.box && op.n === 'class') el.classList.add('mc-window-box');
        return;
      }
      case 'x': if (el) el.data = op.v; return;
      case 'p': if (el) this.setProps(el, op.p); return;
      case 'f':
        if (!el || this.doc.activeElement === el) return;
        this.echoFocus = el;
        el.focus({ preventScroll: false });
        if (op.sel) try { el.setSelectionRange(op.sel[0], op.sel[1]); } catch { /* not a text field */ }
        return;
      case 'sel': if (el) try { el.setSelectionRange(op.a, op.b); } catch { /* not a text field */ } return;
      case 's':
        if (!el) return;
        el.scrollTop = op.t;
        el.scrollLeft = op.l;
        this.lastScroll.set(el, [el.scrollTop, el.scrollLeft]);
        return;
      case 'L': { // a popup or floating menu over the dialog, placed as in the owning page relative to the dialog
        const layer = this.doc.createElement('div');
        layer.className = 'mc-window-layer';
        setLayerBox(layer, op);
        layer.append(this.build(op.d));
        this.doc.body.append(layer);
        this.layers.set(op.key, layer);
        return;
      }
      case 'l-': this.layers.get(op.key)?.remove(); this.layers.delete(op.key); return;
      case 'lo': for (const l of this.layers.values()) setLayerBox(l, op); return;
      case 'e': this.applyEnv(op.env); return;
      case 'pong': this.hooks.pong?.(op.ts); return;
      case 'pingreq': this.hooks.pingreq?.(op.n || 20); return;
    }
  }
}

function setLayerBox(el, op) {
  el.style.transform = `translate(${op.x}px, ${op.y}px)`;
  el.style.width = `${op.w}px`;
  el.style.height = `${op.h}px`;
}
