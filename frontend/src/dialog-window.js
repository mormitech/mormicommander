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
// Should the window stay empty after it was shown (its page never hears that the dialog is on the screen), Go's
// watchdog wakes it ("dlg-wake"): the page then lays the dialog out and paints it again (wake).
//
// The window is not closed after its dialog: Go hides it and sends "dlg-reset", the page forgets the dialog and empties
// itself (reset), tells Go it is ready again, and the same window shows the next dialog. So the windows made at start
// serve every dialog for as long as the app runs.
//
// The window's page never runs a feature's code: Enter and Esc, buttons and links do their work in the owning page,
// which answers with patches (and closes this window when the dialog is done).

import './style.css';
import './core/dialogs.css';
import './core/jobs.css';
import './dialog-window.css';
import { go, on } from './core/go.js';
import { Sequencer, now } from './core/mirror-dom.js';
import { Copy, windowProps as propsOf } from './core/mirror-copy.js';
import { mouseMsg, keyMsg, keyTakenHere } from './core/mirror-events.js';

// every feature's style sheet, so a dialog looks the same as in the page (no feature code is loaded)
import.meta.glob('./features/**/*.css', { eager: true });

let me = '';
let cur = null; // the Copy being shown (mirror-copy.js), with t0, got, built, drawn
let seqOut = 0;
const early = []; // patches that came before the dialog was drawn
const newSeq = () => new Sequencer((m) => { if (!cur?.drawn) early.push(m); else cur.apply(m); });
let inSeq = newSeq(); // (patches of each dialog are numbered from 1: a new one per dialog)
const pongs = new Map();

const frame = () => new Promise((r) => requestAnimationFrame(() => r()));

function send(m) {
  if (!cur) return;
  m.q = ++seqOut;
  go.DialogWindows.Input(m).catch(() => {});
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
  if (cur) return; // (one dialog at a time: the window is reset before it gets the next one)
  const p = req.payload || {};
  cur = new Copy(document, { pong: (ts) => pongs.get(ts)?.(), pingreq: (n) => pings(n) });
  Object.assign(cur, { t0: p.t0 || now(), drawn: false, got: now() });
  cur.applyEnv(p.env);
  const host = document.createElement('div');
  host.className = 'mc-modal mc-window-host';
  // Go sizes the window while this draws: until it is shown, the dialog is laid out at the size it will get (so a
  // smaller window for a moment does not leave a scroll bar and wrapped lines behind)
  if (req.w > 0 && req.h > 0) Object.assign(host.style, { right: 'auto', bottom: 'auto', width: `${req.w}px`, height: `${req.h}px` });
  const box = cur.root(p.tree);
  host.append(box);
  document.body.append(host);
  const f = cur.nodes.get(p.focus);
  if (f) {
    cur.echoFocus = f;
    f.focus();
    if (p.sel) try { f.setSelectionRange(p.sel[0], p.sel[1]); } catch { /* not a text field */ }
  }
  cur.drawn = true;
  cur.built = now();
  for (const m of early.splice(0)) cur.apply(m);
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

// wake is the answer to Go's watchdog, which found the window shown but not yet drawn: the page is laid out and painted
// again (a class that changes nothing visible forces that) and the owning page learns of it (control mode's "dialog").
function wake(n) {
  const root = document.documentElement;
  root.classList.toggle('mc-wake');
  document.body.getBoundingClientRect();
  if (cur) send({ t: 'wake', n });
}

// ---- what the user does ----

const isToggle = (el) => el?.matches?.('input[type=checkbox], input[type=radio]');
const inText = (el) => el?.matches?.('textarea, input:not([type=checkbox]):not([type=radio]):not([type=button]):not([type=submit]), [contenteditable=""], [contenteditable="true"]');

function wire() {
  document.addEventListener('keydown', (ev) => {
    if (!cur) return;
    // the owning page does what these keys do (Enter submits there, Esc cancels there); the browser's own (reload,
    // find, print) are not for a dialog; a key-capture field gets its text from the owning page only
    if (keyTakenHere(ev)) ev.preventDefault();
    send(keyMsg(cur, ev, 'keydown', document.activeElement));
  }, true);
  document.addEventListener('keyup', (ev) => {
    if (cur) send(keyMsg(cur, ev, 'keyup', document.activeElement));
  }, true);
  for (const t of ['input', 'change']) {
    document.addEventListener(t, (ev) => {
      const el = ev.target;
      if (!cur || isToggle(el) || !propsOf(el)) return; // (a checkbox goes as its click: the owning page toggles its own)
      send({ t, i: cur.idOf(el), p: propsOf(el) });
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
    send(mouseMsg(cur, ev, 'click'));
  }, true);
  for (const t of ['mousedown', 'mouseup', 'dblclick', 'auxclick']) document.addEventListener(t, (ev) => { if (cur) send(mouseMsg(cur, ev, t)); }, true);
  document.addEventListener('contextmenu', (ev) => {
    if (!cur) return;
    if (!inText(ev.target)) ev.preventDefault();
    send(mouseMsg(cur, ev, 'contextmenu'));
  }, true);
  let move = null;
  document.addEventListener('mousemove', (ev) => {
    if (!cur) return;
    const first = !move;
    move = ev;
    if (first) requestAnimationFrame(() => { const e = move; move = null; if (e && cur) send(mouseMsg(cur, e, 'mousemove')); });
  }, true);
  document.addEventListener('focusin', (ev) => {
    if (!cur) return;
    if (cur.echoFocus === ev.target) { cur.echoFocus = null; return; }
    cur.echoFocus = null;
    send({ t: 'focus', i: cur.idOf(ev.target) });
  }, true);
  const scrolled = new Set();
  document.addEventListener('scroll', (ev) => {
    const el = ev.target;
    if (!cur || el?.nodeType !== 1) return;
    if (!scrolled.size) requestAnimationFrame(() => {
      for (const s of scrolled) {
        if (!cur) break;
        const last = cur.lastScroll.get(s);
        if (last && last[0] === s.scrollTop && last[1] === s.scrollLeft) continue;
        cur.lastScroll.set(s, [s.scrollTop, s.scrollLeft]);
        send({ t: 'scroll', i: cur.idOf(s), top: s.scrollTop, left: s.scrollLeft });
      }
      scrolled.clear();
    });
    scrolled.add(el);
  }, true);
  // drags between rows (multi-rename's order): replayed as drag events in the owning page
  let dragging = false, over = null;
  const dragMsg = (ev, type) => ({ ...mouseMsg(cur, ev, type), t: 'drag' });
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

// reset forgets the dialog that was shown (Go hid the window after it closed): the copy, its layers, the numbering of
// the messages both ways and what the user had focused, so the next dialog starts as in a fresh window. The look
// (theme, fonts, colour styles) is set again by the next dialog's env.
async function reset() {
  cur = null;
  seqOut = 0;
  early.length = 0;
  inSeq = newSeq();
  pongs.clear();
  document.activeElement?.blur?.();
  document.body.replaceChildren();
  const r = await go.DialogWindows.Ready(innerWidth, innerHeight).catch(() => null);
  if (r?.request) render(r.request).catch(failed);
}

async function start() {
  wire();
  on('dlg-open', (req) => { me = req?.name || me; render(req).catch(failed); });
  on('dlg-reset', () => { reset().catch((e) => console.error('dialog window reset', e)); });
  on('dlg-patch', (m) => { if (m) inSeq.push(m.q, m); });
  on('dlg-wake', (n) => wake(n));
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
