// The embedded terminal pane (issue #31, F-856): a pane below the panels running the user's shell (Go: Terminal, on a
// pseudo terminal) and drawn by xterm.js, bundled with the app (nothing is loaded from the network). It is off by
// default; Commands > Terminal pane (cm_TerminalPane) opens and closes it, Ctrl+` (cm_TerminalFocus) opens it and moves
// the keyboard between it and the panels.
//
// While it is open and "Terminal follows the panel" (cm_TerminalFollow, on by default) is checked, opening a local folder
// in the active panel sends a cd to it, but only while the shell sits at its prompt and the user has nothing typed on
// its line; the app never sends any other command. cm_TerminalInsertNames / cm_TerminalInsertPaths put the selected
// names (or full paths), quoted for the shell, at the prompt without pressing Enter.
//
// None of these commands is open to the AI tools (aitools-allow.js refuses them). Not on Windows yet (no ConPTY).
// Setting "terminal" = {follow, height}.

import './terminal.css';
import { command, keysFor } from '../core/registry.js';
import { go, on, errText } from '../core/go.js';
import { comboFromEvent } from '../core/keys.js';
import { messageDialog } from '../core/dialogs.js';
import { app as theApp } from '../core/app.js';
import { isLocal } from './system-lib.js';
import { termConfig, clampHeight, b64ToBytes, lineDirty, followTarget, MIN_HEIGHT } from './terminal-lib.js';

const cfg = () => termConfig(theApp.setting('terminal', {}));
const save = (patch) => theApp.setSetting('terminal', { ...cfg(), ...patch });

// the pane's state
const st = { el: null, body: null, term: null, fit: null, id: 0, open: false, starting: null, lastSent: '', dirty: false, ended: false };

function css(name, fallback) {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v || fallback;
}

function theme() {
  return { background: css('--bg', '#1b2636'), foreground: css('--text', '#e8edf4'), cursor: css('--accent', '#5aa9ff'),
    selectionBackground: 'rgba(90,169,255,0.35)' };
}

// buildPane makes the pane's elements (once), between the panels and the command line.
function buildPane() {
  if (st.el) return st.el;
  const el = document.createElement('div');
  el.id = 'terminal-pane';
  el.hidden = true;
  el.innerHTML = `<div class="mc-term-grip" title="Drag to change the terminal's height"></div>
    <div class="mc-term-head"><span class="mc-term-title">Terminal</span><button type="button" class="mc-term-close" title="Close the terminal">×</button></div>
    <div class="mc-term-body"></div>`;
  document.getElementById('panels').after(el);
  st.el = el;
  st.body = el.querySelector('.mc-term-body');
  el.querySelector('.mc-term-close').addEventListener('click', () => closePane());
  el.querySelector('.mc-term-grip').addEventListener('mousedown', startDrag);
  new ResizeObserver(() => fitNow()).observe(st.body);
  return el;
}

function setHeight(h) {
  const avail = (document.getElementById('panels')?.offsetHeight || 600) + (st.el.offsetHeight || 0);
  st.el.style.height = `${clampHeight(h, avail)}px`;
}

function startDrag(ev) {
  ev.preventDefault();
  const y0 = ev.clientY, h0 = st.el.offsetHeight;
  const move = (e) => setHeight(h0 + (y0 - e.clientY));
  const up = () => {
    window.removeEventListener('mousemove', move);
    window.removeEventListener('mouseup', up);
    save({ height: Math.max(MIN_HEIGHT, st.el.offsetHeight) });
  };
  window.addEventListener('mousemove', move);
  window.addEventListener('mouseup', up);
}

function fitNow() {
  if (!st.open || !st.fit || !st.term) return;
  try { st.fit.fit(); } catch { return; }
  if (st.id) go.Terminal.Resize(st.id, st.term.cols, st.term.rows).catch(() => {});
}

function focusPanel() { theApp.cur?.$?.list?.focus(); }

// makeTerm loads xterm.js (a chunk of its own, bundled) and makes the terminal view.
async function makeTerm() {
  if (st.term) return st.term;
  const [{ Terminal }, { FitAddon }] = await Promise.all([import('@xterm/xterm'), import('@xterm/addon-fit'), import('@xterm/xterm/css/xterm.css')]);
  const term = new Terminal({
    fontFamily: 'ui-monospace, "SF Mono", Menlo, Consolas, monospace', fontSize: 12, cursorBlink: true,
    scrollback: 5000, theme: theme(), allowProposedApi: false,
  });
  const fit = new FitAddon();
  term.loadAddon(fit);
  term.open(st.body);
  // the key that moves the keyboard back to the panels works inside the terminal too (the page's keyboard handler
  // leaves text fields alone, and xterm.js types into one)
  term.attachCustomKeyEventHandler((ev) => {
    if (ev.type === 'keydown' && keysFor('cm_TerminalFocus').includes(comboFromEvent(ev))) { ev.preventDefault(); focusPanel(); return false; }
    return true;
  });
  term.onData((d) => {
    if (!st.id) {
      if (st.ended && d.includes('\r')) startShell();
      return;
    }
    st.dirty = lineDirty(st.dirty, d);
    go.Terminal.Write(st.id, d).catch(() => {});
  });
  st.term = term;
  st.fit = fit;
  return term;
}

// startShell starts the user's shell in the active panel's folder.
async function startShell() {
  if (st.starting) return st.starting;
  st.starting = (async () => {
    const p = theApp.cur, dir = p && isLocal(p.path) ? p.path : '';
    try { st.fit.fit(); } catch { /* not laid out yet */ }
    st.ended = false;
    st.dirty = false;
    st.id = await go.Terminal.Start(dir, st.term.cols || 80, st.term.rows || 24);
    st.lastSent = dir;
  })();
  try { await st.starting; } finally { st.starting = null; }
}

async function openPane({ focus = true } = {}) {
  if (!(await go.Terminal.Available().catch(() => false))) {
    theApp.flash('The embedded terminal is not available on this system yet. Commands > Open terminal here opens the system terminal.', 'err');
    return false;
  }
  buildPane();
  st.el.hidden = false;
  st.open = true;
  setHeight(cfg().height);
  try {
    await makeTerm();
    if (!st.id) await startShell();
  } catch (e) {
    st.el.hidden = true;
    st.open = false;
    theApp.flash(errText(e), 'err');
    return false;
  }
  fitNow();
  if (focus) st.term.focus();
  return true;
}

async function closePane({ ask = true } = {}) {
  if (!st.open) return true;
  if (ask && st.id && await go.Terminal.Busy(st.id).catch(() => false)) {
    const b = await messageDialog({ Title: 'Close the terminal', Message: 'A program is still running in the terminal.\n\nClose the terminal and stop it?',
      Buttons: ['Close', 'Cancel'], Danger: true, Kind: 'terminal-close' });
    if (b !== 'Close') return false;
  }
  const id = st.id;
  st.id = 0;
  st.open = false;
  st.el.hidden = true;
  if (id) await go.Terminal.Close(id).catch(() => {});
  st.term?.reset();
  focusPanel();
  return true;
}

// follow sends a cd to the active panel's folder when it should (followTarget says when).
async function follow() {
  const p = theApp.cur;
  const path = followTarget({ open: st.open, id: st.id, follow: cfg().follow, path: p?.path, lastSent: st.lastSent, dirty: st.dirty, isLocal });
  if (!path) return;
  const id = st.id;
  try { if (await go.Terminal.Cd(id, path) && id === st.id) st.lastSent = path; } catch { /* the shell ended */ }
}

on('terminal-data', (id, b64) => { if (id === st.id && st.term) st.term.write(b64ToBytes(b64)); });
on('terminal-exit', (id, code) => {
  if (id !== st.id) return;
  st.id = 0;
  st.ended = true;
  st.term?.write(`\r\n[The shell ended (exit code ${code}). Press Enter to start a new one.]\r\n`);
});

command({
  name: 'cm_TerminalPane', menuKey: true, label: 'Terminal pane', menu: 'Commands', menuAfter: 'cm_ExecuteDOS',
  checked: () => st.open,
  run: async (app, args = {}) => {
    const want = typeof args.on === 'boolean' ? args.on : !st.open;
    return want ? openPane() : closePane();
  },
});

command({
  name: 'cm_TerminalFocus', menuKey: true, label: 'Terminal / panels', keys: ['Ctrl+`'],
  run: async () => {
    if (!st.open) return openPane();
    if (st.el.contains(document.activeElement)) focusPanel();
    else st.term?.focus();
    return true;
  },
});

command({
  name: 'cm_TerminalFollow', menuKey: true, label: 'Terminal follows the panel', menu: 'Commands', menuAfter: 'cm_TerminalPane',
  checked: () => cfg().follow,
  run: (app, args = {}) => {
    const v = typeof args.on === 'boolean' ? args.on : !cfg().follow;
    save({ follow: v });
    app.flash(v ? 'The terminal changes to the folder of the active panel.' : 'The terminal stays in its folder.');
    if (v) follow();
    return true;
  },
});

// insert puts the panel's targets (selected names, or the one under the cursor) at the terminal's prompt.
async function insert(app, full) {
  const p = app.cur;
  const names = (p?.targets() || []).map((e) => (full ? p.fullPath(e.name) : e.name));
  if (!names.length) return false;
  if (!st.open && !(await openPane())) return false;
  if (!st.id) await startShell();
  try {
    await go.Terminal.Insert(st.id, names);
    st.dirty = true;
  } catch (e) { app.flash(errText(e), 'err'); return false; }
  st.term.focus();
  return true;
}

command({ name: 'cm_TerminalInsertNames', menuKey: true, label: 'Insert names in terminal', menu: 'Commands', menuAfter: 'cm_TerminalFollow', run: (app) => insert(app, false) });
command({ name: 'cm_TerminalInsertPaths', menuKey: true, label: 'Insert full paths in terminal', menu: 'Commands', menuAfter: 'cm_TerminalInsertNames', run: (app) => insert(app, true) });

export function init(app) {
  app.on('panel', (p) => { if (p === app.cur) follow(); });
  app.on('active', () => follow());
}
