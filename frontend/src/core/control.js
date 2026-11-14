// Control mode, the page's side (see control.go and docs/control.md). Commands that are not about the window arrive as
// the "control" event; each runs through the same code a person's key press or click runs: "key" dispatches real
// KeyboardEvents to the focused element, "click" real mouse events, "run" the command registry. After acting, a
// command waits until the app is idle (folders read, the screen painted), so the next "state" sees the result.
//
// A feature adds commands with addControlCommand('name', async (m) => result).

import { go, on, errText } from './go.js';
import { app } from './app.js';
import { run, commandList, keysFor, allKeys, hasCommand, recentRuns, clearRecentRuns } from './registry.js';
import { TC_KEY_REFERENCE } from './tckeys.js';
import { eventInit, normalizeCombo } from './keys.js';
import { idle, isBusy } from './busy.js';
import { currentDialog, answerDialog } from './dialogs.js';
import { pingDialogWindow } from './mirror.js';
import { currentPopup, choosePopup } from './popup.js';
import { cmdline } from './cmdline.js';
import { allSettings } from './settings.js';
import { menuSpec } from './menus.js';

const extra = new Map();
export function addControlCommand(name, fn) { extra.set(name, fn); }

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function panelState(p) {
  const s = p.stats();
  return {
    side: p.side, path: p.path, parent: p.parent, cursor: p.cursor, cursorName: p.cur?.name ?? null,
    count: p.entries.filter((e) => !e.up).length, files: s.files, folders: s.dirs, totalBytes: s.total,
    selected: p.selectedEntries().map((e) => e.name), selectedBytes: s.selBytes, sort: { ...p.sort }, filter: p.filterLabel,
    loading: p.loading, error: p.error, back: [...p.back], forward: [...p.fwd], status: p.$.status.textContent,
    driveInfo: p.$.driveinfo.textContent, volume: app.volumeFor(p.path)?.name ?? null,
    scrollTop: p.$.list.scrollTop, renderedRows: p.$.rows.children.length, firstRenderedRow: +(p.$.rows.firstElementChild?.dataset.i ?? -1),
  };
}

function state() {
  const ae = document.activeElement;
  return {
    ok: true, active: app.cur.side, showHidden: app.showHidden, demo: app.demo, platform: app.platform,
    left: panelState(app.left), right: panelState(app.right),
    cmdline: cmdline.text, focus: ae?.id || (ae?.closest?.('.panel') ? `${ae.closest('.panel').dataset.side}-panel` : ae?.tagName?.toLowerCase() || ''),
    dialog: currentDialog(), popup: currentPopup(), message: app.lastMessage, busy: isBusy(),
    title: document.getElementById('prompt').title || document.getElementById('prompt').textContent.replace(/>$/, ''),
  };
}

function entryInfo(p, e, i) {
  return { i, name: e.name, ext: e.ext, size: e.size, mtime: e.mtime, dir: !!e.dir, up: !!e.up, link: !!e.link, hidden: !!e.hidden, selected: p.isSelected(e), dirSize: e.dirSize ?? null };
}

// press dispatches one key combo to the focused element, like a real key press. hold: no keyup (a modifier stays down
// until "release").
function press(combo, hold = false) {
  const target = document.activeElement && document.activeElement !== document.body ? document.activeElement : app.cur.$.list;
  const init = eventInit(combo);
  const ev = new KeyboardEvent('keydown', init);
  target.dispatchEvent(ev);
  // a typed character in a focused text field (the command line, a dialog's field) is inserted like the browser would
  if (!ev.defaultPrevented && init.key.length === 1 && !init.ctrlKey && !init.metaKey && !init.altKey && target.matches?.('input[type=text], input:not([type]), textarea')) {
    const a = target.selectionStart ?? target.value.length, b = target.selectionEnd ?? a;
    target.value = target.value.slice(0, a) + init.key + target.value.slice(b);
    target.setSelectionRange(a + 1, a + 1);
    target.dispatchEvent(new Event('input', { bubbles: true }));
  }
  // the key-up comes with no modifiers down, as when a person lets go of everything
  if (!hold) target.dispatchEvent(new KeyboardEvent('keyup', { ...init, ctrlKey: false, altKey: false, shiftKey: false, metaKey: false }));
}

// mouse sends the events of a click (or double click) at the middle of an element.
function mouse(el, { button = 0, double = false, ctrl = false, shift = false, meta = false, alt = false } = {}) {
  const r = el.getBoundingClientRect();
  const base = { bubbles: true, cancelable: true, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2, button, ctrlKey: ctrl, shiftKey: shift, metaKey: meta, altKey: alt, view: window };
  el.dispatchEvent(new MouseEvent('mousedown', base));
  el.dispatchEvent(new MouseEvent('mouseup', base));
  if (button === 0) el.dispatchEvent(new MouseEvent('click', base));
  if (button === 2) el.dispatchEvent(new MouseEvent('contextmenu', base));
  if (double) {
    // The first click may redraw what was under the pointer (panel rows are), so the second one goes to whatever is
    // there now, as with a real mouse.
    const el2 = document.elementFromPoint(base.clientX, base.clientY) || el;
    el2.dispatchEvent(new MouseEvent('mousedown', { ...base, detail: 2 }));
    el2.dispatchEvent(new MouseEvent('mouseup', { ...base, detail: 2 }));
    el2.dispatchEvent(new MouseEvent('click', { ...base, detail: 2 }));
    el2.dispatchEvent(new MouseEvent('dblclick', { ...base, detail: 2 }));
  }
}

async function settle(ms = 0) {
  await sleep(ms || 30);
  await idle();
}

async function ctlRun(m) {
  switch (m.cmd) {
    case 'state': return state();
    case 'list': { // the entries of a panel: {side, from, count}
      const p = app.panel(m.side);
      const from = m.from ?? 0, count = m.count ?? 500;
      return { ok: true, path: p.path, total: p.entries.length, cursor: p.cursor, entries: p.entries.slice(from, from + count).map((e, k) => entryInfo(p, e, from + k)) };
    }
    case 'key': { // {keys: "Ctrl+Right"} or {keys: ["Down", "Down", "Enter"]}; hold: true keeps the keys down
      const list = Array.isArray(m.keys) ? m.keys : [m.keys ?? m.key];
      for (const k of list) { press(k, !!m.hold); await settle(m.delay); }
      return state();
    }
    case 'release': { // lets go of held keys: a keyup with no modifiers
      window.dispatchEvent(new KeyboardEvent('keyup', { key: 'Shift', code: 'ShiftLeft', bubbles: true }));
      await settle();
      return state();
    }
    case 'drag': { // {selector, dx, dy}: presses the mouse on the element's middle, moves by dx/dy, lets go
      const el = document.querySelector(m.selector);
      if (!el) throw new Error(`nothing matches ${m.selector}`);
      const r = el.getBoundingClientRect();
      const x = r.left + r.width / 2, y = r.top + r.height / 2;
      const opts = (cx, cy) => ({ bubbles: true, cancelable: true, clientX: cx, clientY: cy, button: 0, view: window });
      el.dispatchEvent(new MouseEvent('mousedown', opts(x, y)));
      for (let i = 1; i <= 5; i++) window.dispatchEvent(new MouseEvent('mousemove', opts(x + (m.dx || 0) * i / 5, y + (m.dy || 0) * i / 5)));
      window.dispatchEvent(new MouseEvent('mouseup', opts(x + (m.dx || 0), y + (m.dy || 0))));
      await settle();
      return state();
    }
    case 'menus': return { ok: true, menus: menuSpec() };
    case 'layout': { // sizes of the main parts, for checks of the layout (splitter, horizontal panels, row height)
      const box = (sel) => { const r = document.querySelector(sel)?.getBoundingClientRect(); return r ? { x: r.x, y: r.y, w: r.width, h: r.height } : null; };
      return { ok: true, left: box('.panel[data-side="left"]'), right: box('.panel[data-side="right"]'), splitter: box('#splitter'), horizontal: document.getElementById('panels').classList.contains('horizontal'),
        rowHeight: app.cur.rowH, theme: document.documentElement.dataset.theme || '', split: app.split };
    }
    case 'dialog_fill': { // {fields: {id: value}}: fills fields of the open dialog as typing would, without pressing a button
      for (const [id, v] of Object.entries(m.fields || {})) {
        const el = document.getElementById(id) || document.querySelector(`[name="${CSS.escape(id)}"]`);
        if (!el) throw new Error(`no field "${id}"`);
        if (el.type === 'checkbox' || el.type === 'radio') el.checked = !!v; else el.value = String(v);
        el.dispatchEvent(new Event('input', { bubbles: true }));
        el.dispatchEvent(new Event('change', { bubbles: true }));
      }
      await new Promise((r) => setTimeout(r, 150)); // (a field may ask Go something, as the Options search does)
      await settle();
      return state();
    }
    case 'type': { // types text character by character, through the key handler (letters go to the command line)
      for (const ch of String(m.text ?? '')) { press(ch === ' ' ? 'Space' : ch.length === 1 && /[A-Z]/.test(ch) ? `Shift+${ch}` : ch); }
      await settle();
      return state();
    }
    case 'run': { // {name: "cm_GoToParent", args: {...}}
      if (!hasCommand(m.name)) throw new Error(`${m.name} is not registered`);
      // A command that opens a dialog or a popup only ends once that is answered; the answer to "run" comes as soon as
      // the app waits for the user (waiting: true), so the script can read and answer the dialog.
      let finished = false, result;
      const pr = run(m.name, m.args || {}).then((r) => { finished = true; result = r; return r; });
      await Promise.race([pr, settle()]);
      if (finished) await settle();
      else pr.catch((e) => app.flash(errText(e), 'err')); // a failure after the answer: shown like any other
      return { ...state(), result: result === undefined ? null : result, waiting: !finished };
    }
    case 'cd': { // {side, path}: the panel shows a folder, as if typed in the path bar
      const p = app.panel(m.side);
      const ok = await p.go(m.path);
      await settle();
      if (!ok) throw new Error(app.lastMessage || `can't open ${m.path}`);
      return state();
    }
    case 'focus': { // {side}: that panel becomes the active one
      app.setActive(app.panel(m.side));
      await settle();
      return state();
    }
    case 'cursor': { // {side, index} or {side, name}
      const p = app.panel(m.side);
      if (m.name != null) { if (!p.focusName(m.name)) throw new Error(`no "${m.name}" in ${p.path}`); } else p.setCursor(m.index ?? 0);
      await settle();
      return state();
    }
    case 'click': { // {selector, button, double, ctrl, shift, meta, alt} or {side, index|name} for a row of a panel
      let el;
      if (m.selector) {
        el = document.querySelector(m.selector);
        if (!el) throw new Error(`nothing matches ${m.selector}`);
      } else {
        const p = app.panel(m.side);
        const i = m.name != null ? p.index.get(m.name) : m.index;
        if (i == null || i < 0 || i >= p.entries.length) throw new Error(`no row ${m.name ?? m.index} in the ${p.side} panel`);
        p.$.list.scrollTop = Math.max(0, i * p.rowH - p.$.list.clientHeight / 2);
        p.renderRows();
        el = p.$.rows.querySelector(`.row[data-i="${i}"]`);
      }
      mouse(el, m);
      await settle(m.wait);
      return state();
    }
    case 'scroll': { // {side, top} or {side, row}
      const p = app.panel(m.side);
      p.$.list.scrollTop = m.row != null ? m.row * p.rowH : m.top ?? 0;
      p.$.list.dispatchEvent(new Event('scroll'));
      await sleep(60);
      await settle();
      return state();
    }
    case 'dialog': return { ok: true, dialog: currentDialog() };
    case 'dialog_answer': { // {button, fields: {id: value}}; button "Esc" cancels, none submits
      answerDialog(m.button, m.fields || {});
      await settle(m.wait || 50);
      return state();
    }
    case 'dialog_ping': // {n}: input round trips with the top dialog window, in milliseconds (mirror.js)
      return { ok: true, ...(await pingDialogWindow(m.n || 20)) };
    case 'popup': return { ok: true, popup: currentPopup() };
    case 'popup_choose': { // {index} or {label}, shift: true for Shift+Enter; null closes
      choosePopup(m.index ?? m.label ?? null, !!m.shift);
      await settle(m.wait || 50);
      return state();
    }
    case 'commands': // every registered command with its keys
      return { ok: true, commands: commandList().map((c) => ({ name: c.name, label: c.label, keys: keysFor(c.name), enabled: c.enabled ? !!c.enabled(app) : true, checked: c.checked ? !!c.checked(app) : null })) };
    case 'keymap': return { ok: true, keymap: Object.fromEntries(allKeys()) };
    case 'keyfor': return { ok: true, command: allKeys().get(normalizeCombo(m.keys)) || '' };
    case 'settings': return { ok: true, settings: allSettings() };
    case 'keyref': return { ok: true, keys: TC_KEY_REFERENCE }; // TC's key reference (tckeys.js), for scripts/qa_keys.py
    case 'runs': { // the commands asked for lately, newest last ({clear: true} empties the list afterwards)
      const runs = recentRuns();
      if (m.clear) clearRecentRuns();
      return { ok: true, runs };
    }
    case 'dom': { // {selector}: text and count of matching elements (for checks of what is drawn)
      const els = [...document.querySelectorAll(m.selector)];
      return { ok: true, count: els.length, texts: els.slice(0, 50).map((e) => e.textContent), visible: els.map((e) => !!(e.offsetWidth || e.offsetHeight)).slice(0, 50), classes: els.slice(0, 50).map((e) => e.className) };
    }
    case 'wait': case 'idle': await sleep(m.ms || 0); await idle(m.timeout || 15000); return state();
    case 'eval': throw new Error('"eval" does not exist: use the commands');
  }
  const f = extra.get(m.cmd);
  if (f) { const r = await f(m); await settle(); return r ?? state(); }
  throw new Error(`unknown command "${m.cmd}"`);
}

export function initControl() {
  on('control', async (id, m) => {
    let res;
    try { res = await ctlRun(m); } catch (e) { res = { error: errText(e) }; }
    try { await go.App.ControlReply(id, JSON.stringify(res)); } catch { /* the app is closing */ }
  });
}
