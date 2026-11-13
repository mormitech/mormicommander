// Start-up of a dialog window (dialog.html): the window that shows a dialog of another page as a live copy (see
// core/mirror.js for the page that owns the dialog and feature_dialog_window.go for the windows). Go makes the window
// hidden in advance; this page loads every style sheet of the app, tells Go it is ready, and waits for a dialog
// ("dlg-open", or the answer of DialogWindows.Ready when the dialog was asked for while the page was still loading).
// It builds the copy from the description it gets, asks Go to show the window, and from then on:
//
//   - applies the patches of the owning page ("dlg-patch"): children lists (elements it already has are moved, never
//     rebuilt, so focus, caret, selection and scrolling stay), attributes, texts, field values, focus, selection,
//     scrolling, the look of the page (theme, font) and layers (popups drawn over the dialog);
//   - sends back what the user does (DialogWindows.Input): field values with input/change, clicks and other mouse
//     events at their place in the dialog, keys, focus, scrolling, drags, and its own size when it is resized.
//
// The window's page never runs a feature's code: Enter and Esc, buttons and links do their work in the owning page,
// which answers with patches (and closes this window when the dialog is done).

import './style.css';
import './core/dialogs.css';
import './core/jobs.css';
import './dialog-window.css';
import { go, on } from './core/go.js';
import { Sequencer, copyAttr, now } from './core/mirror-dom.js';

// every feature's style sheet, so a dialog looks the same as in the page (no feature code is loaded)
import.meta.glob('./features/**/*.css', { eager: true });

let me = '';
let cur = null; // {nodes, ids, box, host, layers, t0, drawn}
let seqOut = 0;
const early = []; // patches that came before the dialog was drawn
const inSeq = new Sequencer((m) => { if (!cur?.drawn) early.push(m); else apply(m); });
const lastP = new WeakMap(); // field → the values the owning page gave it
const lastScroll = new WeakMap();
const pongs = new Map();
let echoFocus = null;

const frame = () => new Promise((r) => requestAnimationFrame(() => r()));

function send(m) {
  if (!cur) return;
  m.q = ++seqOut;
  go.DialogWindows.Input(m).catch(() => {});
}

// ---- building the copy ----

function build(d) {
  let n;
  if (d.x !== undefined) n = document.createTextNode(d.x);
  else {
    n = d.ns ? document.createElementNS(d.ns, d.t) : document.createElement(d.t);
    for (const [k, v] of d.a || []) {
      const val = copyAttr(k, v);
      if (val !== null) try { n.setAttribute(k, val); } catch { /* a name the DOM does not take */ }
    }
    for (const c of d.c || []) n.append(build(c));
    if (d.p) setProps(n, d.p);
  }
  cur.nodes.set(d.i, n);
  cur.ids.set(n, d.i);
  return n;
}

function setProps(el, p) {
  lastP.set(el, p);
  if ('v' in p && el.value !== p.v) {
    if (document.activeElement === el && typeof el.selectionStart === 'number') {
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

function propsOf(el) {
  if (el.localName === 'input') return el.type === 'checkbox' || el.type === 'radio' ? { c: el.checked } : { v: el.value };
  if (el.localName === 'textarea') return { v: el.value };
  if (el.localName === 'select') return el.multiple ? { m: [...el.options].map((o) => o.selected) } : { s: el.selectedIndex };
  return null;
}

function applyEnv(e) {
  if (!e) return;
  const root = document.documentElement;
  const keep = new Set(e.html.map(([k]) => k));
  for (const a of [...root.attributes]) if (!keep.has(a.name)) root.removeAttribute(a.name);
  for (const [k, v] of e.html) if (root.getAttribute(k) !== v) root.setAttribute(k, v);
  document.body.className = `mc-dialog-window ${e.body || ''}`.trim();
}

function setLayerBox(el, op) {
  el.style.transform = `translate(${op.x}px, ${op.y}px)`;
  el.style.width = `${op.w}px`;
  el.style.height = `${op.h}px`;
}

// apply carries out one patch of the owning page.
function apply(m) {
  for (const op of m.ops || []) {
    try { applyOp(op); } catch (e) { console.error('dialog window patch', op, e); }
  }
}

function applyOp(op) {
  const el = op.i ? cur.nodes.get(op.i) : null;
  switch (op.o) {
    case 'k': {
      if (!el) return;
      const list = op.c.map((c) => (c.r ? cur.nodes.get(c.r) : build(c))).filter(Boolean);
      list.forEach((n, j) => { const at = el.childNodes[j]; if (at !== n) el.insertBefore(n, at || null); });
      while (el.childNodes.length > list.length) el.lastChild.remove();
      const sel = el.closest?.('select'); // (new options can change a list's choice: the owning page's one is kept)
      if (sel && lastP.has(sel)) setProps(sel, lastP.get(sel));
      return;
    }
    case 'a': {
      if (!el) return;
      const v = op.v === null ? null : copyAttr(op.n, op.v);
      if (v === null) el.removeAttribute(op.n); else el.setAttribute(op.n, v);
      if (el === cur.box && op.n === 'class') el.classList.add('mc-window-box');
      return;
    }
    case 'x': if (el) el.data = op.v; return;
    case 'p': if (el) setProps(el, op.p); return;
    case 'f':
      if (!el || document.activeElement === el) return;
      echoFocus = el;
      el.focus({ preventScroll: false });
      if (op.sel) try { el.setSelectionRange(op.sel[0], op.sel[1]); } catch { /* not a text field */ }
      return;
    case 'sel': if (el) try { el.setSelectionRange(op.a, op.b); } catch { /* not a text field */ } return;
    case 's':
      if (!el) return;
      el.scrollTop = op.t;
      el.scrollLeft = op.l;
      lastScroll.set(el, [el.scrollTop, el.scrollLeft]);
      return;
    case 'L': {
      const layer = document.createElement('div');
      layer.className = 'mc-window-layer';
      setLayerBox(layer, op);
      layer.append(build(op.d));
      document.body.append(layer);
      cur.layers.set(op.key, layer);
      return;
    }
    case 'l-': cur.layers.get(op.key)?.remove(); cur.layers.delete(op.key); return;
    case 'lo': for (const l of cur.layers.values()) setLayerBox(l, op); return;
    case 'e': applyEnv(op.env); return;
    case 'pong': pongs.get(op.ts)?.(); return;
    case 'pingreq': pings(op.n || 20); return;
  }
}

async function pings(n) {
  const rtts = [];
  for (let k = 0; k < n; k++) {
    const ts = now();
    await new Promise((resolve) => {
      const t = setTimeout(resolve, 2000);
      pongs.set(ts, () => { clearTimeout(t); resolve(); });
      send({ t: 'ping', ts });
    });
    pongs.delete(ts);
    rtts.push(now() - ts);
  }
  send({ t: 'pingres', rtts });
}

// ---- drawing a dialog ----

async function render(req) {
  if (cur) return; // (one dialog per window: a window is closed after its dialog)
  const p = req.payload || {};
  cur = { nodes: new Map(), ids: new WeakMap(), layers: new Map(), t0: p.t0 || now(), drawn: false, box: null, got: now() };
  applyEnv(p.env);
  const host = document.createElement('div');
  host.className = 'mc-modal mc-window-host';
  // Go sizes the window while this draws: until it is shown, the dialog is laid out at the size it will get (so a
  // smaller window for a moment does not leave a scroll bar and wrapped lines behind)
  if (req.w > 0 && req.h > 0) Object.assign(host.style, { right: 'auto', bottom: 'auto', width: `${req.w}px`, height: `${req.h}px` });
  const box = build(p.tree);
  box.classList.add('mc-window-box');
  cur.box = box;
  host.append(box);
  document.body.append(host);
  const f = cur.nodes.get(p.focus);
  if (f) {
    echoFocus = f;
    f.focus();
    if (p.sel) try { f.setSelectionRange(p.sel[0], p.sel[1]); } catch { /* not a text field */ }
  }
  cur.drawn = true;
  cur.built = now();
  for (const m of early.splice(0)) apply(m);
  box.getBoundingClientRect(); // (laid out before the window is shown)
  const painted = now();
  await go.DialogWindows.ShowMe().catch(() => {});
  const showed = now();
  host.removeAttribute('style'); // (from now on the dialog fills the window, also when the user resizes it)
  // shown: two frames later it is on the screen, painted
  await frame();
  await frame();
  const ts = now();
  go.DialogWindows.Shown(Math.round((ts - cur.t0) * 10) / 10).catch(() => {});
  send({ t: 'shown', ts, painted, showed, got: cur.got, sent: p.sent || cur.t0, built: cur.built });
  send({ t: 'resize', w: innerWidth, h: innerHeight });
}

// ---- what the user does ----

function idOf(n) {
  for (; n; n = n.parentNode) { const i = cur?.ids.get(n); if (i) return i; }
  return 0;
}

const mods = (ev) => ({ ctrl: ev.ctrlKey, shift: ev.shiftKey, alt: ev.altKey, meta: ev.metaKey });
const mouseMsg = (ev, type) => ({ t: 'mouse', type, i: idOf(ev.target), x: ev.clientX, y: ev.clientY, sx: ev.screenX, sy: ev.screenY, b: ev.button, bs: ev.buttons, d: ev.detail, ...mods(ev) });
const isToggle = (el) => el?.matches?.('input[type=checkbox], input[type=radio]');
const inText = (el) => el?.matches?.('textarea, input:not([type=checkbox]):not([type=radio]):not([type=button]):not([type=submit]), [contenteditable=""], [contenteditable="true"]');

function wire() {
  document.addEventListener('keydown', (ev) => {
    if (!cur) return;
    const k = ev.key;
    const area = ev.target?.matches?.('textarea, [contenteditable=""], [contenteditable="true"]');
    // the owning page does what these keys do (Enter submits there, Esc cancels there); the browser's own (reload,
    // find, print) are not for a dialog
    if ((k === 'Enter' && !area) || k === 'Escape' || /^F\d{1,2}$/.test(k) || ((ev.ctrlKey || ev.metaKey) && /^[rpfgj]$/i.test(k))) ev.preventDefault();
    send({ t: 'key', type: 'keydown', i: idOf(document.activeElement), key: k, code: ev.code, r: ev.repeat, ...mods(ev) });
  }, true);
  document.addEventListener('keyup', (ev) => {
    if (cur) send({ t: 'key', type: 'keyup', i: idOf(document.activeElement), key: ev.key, code: ev.code, ...mods(ev) });
  }, true);
  for (const t of ['input', 'change']) {
    document.addEventListener(t, (ev) => {
      const el = ev.target;
      if (!cur || isToggle(el) || !propsOf(el)) return; // (a checkbox goes as its click: the owning page toggles its own)
      send({ t, i: idOf(el), p: propsOf(el) });
    }, true);
  }
  document.addEventListener('submit', (ev) => ev.preventDefault(), true);
  // A click on a label clicks its field too: only the label's click goes, the owning page's label does the rest.
  let skipClick = null;
  document.addEventListener('click', (ev) => {
    if (!cur) return;
    if (skipClick && ev.target === skipClick) { skipClick = null; return; }
    const lab = ev.target.closest?.('label');
    const ctl = lab?.control;
    if (ctl && ctl !== ev.target && !ctl.contains(ev.target)) { skipClick = ctl; setTimeout(() => { skipClick = null; }, 0); }
    if (ev.target.closest?.('a[href]')) ev.preventDefault(); // (the owning page opens links)
    send(mouseMsg(ev, 'click'));
  }, true);
  for (const t of ['mousedown', 'mouseup', 'dblclick', 'auxclick']) document.addEventListener(t, (ev) => { if (cur) send(mouseMsg(ev, t)); }, true);
  document.addEventListener('contextmenu', (ev) => {
    if (!cur) return;
    if (!inText(ev.target)) ev.preventDefault();
    send(mouseMsg(ev, 'contextmenu'));
  }, true);
  let move = null;
  document.addEventListener('mousemove', (ev) => {
    if (!cur) return;
    const first = !move;
    move = ev;
    if (first) requestAnimationFrame(() => { const e = move; move = null; if (e) send(mouseMsg(e, 'mousemove')); });
  }, true);
  document.addEventListener('focusin', (ev) => {
    if (!cur) return;
    if (echoFocus === ev.target) { echoFocus = null; return; }
    echoFocus = null;
    send({ t: 'focus', i: idOf(ev.target) });
  }, true);
  const scrolled = new Set();
  document.addEventListener('scroll', (ev) => {
    const el = ev.target;
    if (!cur || el?.nodeType !== 1) return;
    if (!scrolled.size) requestAnimationFrame(() => {
      for (const s of scrolled) {
        const last = lastScroll.get(s);
        if (last && last[0] === s.scrollTop && last[1] === s.scrollLeft) continue;
        lastScroll.set(s, [s.scrollTop, s.scrollLeft]);
        send({ t: 'scroll', i: idOf(s), top: s.scrollTop, left: s.scrollLeft });
      }
      scrolled.clear();
    });
    scrolled.add(el);
  }, true);
  // drags between rows (multi-rename's order): replayed as drag events in the owning page
  let dragging = false, over = null;
  const dragMsg = (ev, type) => ({ ...mouseMsg(ev, type), t: 'drag' });
  document.addEventListener('dragstart', (ev) => { if (!cur) return; dragging = true; ev.dataTransfer?.setData('text/plain', 'mc-dialog'); send(dragMsg(ev, 'dragstart')); }, true);
  for (const t of ['dragenter', 'dragover']) {
    document.addEventListener(t, (ev) => {
      if (!cur || !dragging) return;
      ev.preventDefault();
      const first = !over;
      over = ev;
      if (first) requestAnimationFrame(() => { const e = over; over = null; if (e) send(dragMsg(e, 'dragover')); });
    }, true);
  }
  document.addEventListener('drop', (ev) => { if (!cur || !dragging) return; ev.preventDefault(); send(dragMsg(ev, 'drop')); }, true);
  document.addEventListener('dragend', (ev) => { if (!cur || !dragging) return; dragging = false; send(dragMsg(ev, 'dragend')); }, true);
  let sizing = false;
  window.addEventListener('resize', () => {
    if (!cur?.drawn || sizing) return;
    sizing = true;
    requestAnimationFrame(() => { sizing = false; send({ t: 'resize', w: innerWidth, h: innerHeight }); });
  });
}

async function start() {
  wire();
  on('dlg-open', (req) => { me = req?.name || me; render(req).catch(failed); });
  on('dlg-patch', (m) => { if (m) inSeq.push(m.q, m); });
  const r = await go.DialogWindows.Ready(innerWidth, innerHeight).catch(() => null);
  me = r?.name || me;
  document.body.dataset.window = me;
  if (r?.request) render(r.request).catch(failed);
}

// failed reports a dialog the window could not draw to the owning page (control mode shows it in the dialog's
// "window" entry) and shows the window anyway, so the user is not left without the dialog's close button.
function failed(e) {
  console.error('dialog window', e);
  if (cur) send({ t: 'error', msg: String(e?.stack || e) });
  go.DialogWindows.ShowMe().catch(() => {});
}

start();
