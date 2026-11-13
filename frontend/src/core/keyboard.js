// The keyboard: one keydown listener for the whole window. In order:
//   1. dialogs and popups take their keys first (they listen in the capture phase);
//   2. key handlers of features (quick search on Ctrl+Alt+letters, inline rename...): addKeyHandler(fn) — fn(ev, combo)
//      returns true when it used the key;
//   3. the command line, when it has text or focus: Enter runs it, Backspace/Left/Right/Delete edit it, letters go in;
//   4. the keymap: the combo's cm_* command runs (keymap.js + the user's changes);
//   5. any other printable character goes into the command line.
// Control mode's "key" command dispatches real KeyboardEvents, so it goes through exactly this path.

import { comboFromEvent, printable } from './keys.js';
import { commandForKey } from './registry.js';
import { cmdline } from './cmdline.js';
import { app } from './app.js';

const handlers = [];
export function addKeyHandler(fn) { handlers.push(fn); return () => handlers.splice(handlers.indexOf(fn), 1); }

const mods = new Set();
const modListeners = new Set();
export const heldModifiers = () => new Set(mods);
export function onModifiers(fn) { modListeners.add(fn); }
function setMods(ev) {
  const next = new Set();
  if (ev.ctrlKey) next.add('Ctrl');
  if (ev.altKey) next.add('Alt');
  if (ev.shiftKey) next.add('Shift');
  if (ev.metaKey) next.add('Cmd');
  if ([...next].join() !== [...mods].join()) {
    mods.clear();
    next.forEach((m) => mods.add(m));
    modListeners.forEach((f) => f(heldModifiers()));
  }
}

export function onKeyDown(ev) {
  setMods(ev);
  if (ev.defaultPrevented || ev.isComposing) return;
  const t = ev.target;
  const inCmd = t?.id === 'cmd';
  if (!inCmd && t?.matches?.('input, textarea, select, [contenteditable="true"]')) return; // a field of a feature (path editor, rename...)
  const combo = comboFromEvent(ev);
  for (const h of handlers) {
    if (h(ev, combo)) { ev.preventDefault(); return; }
  }
  if (!combo) return;
  const hasText = cmdline.text.length > 0;
  if (inCmd || hasText) {
    if (combo === 'Enter' || combo === 'Shift+Enter') { ev.preventDefault(); if (hasText) app.run('cm_ExecuteCmdLine', { shift: combo === 'Shift+Enter' }); return; }
    if (combo === 'Esc') { ev.preventDefault(); cmdline.clear(); app.cur?.$.list.focus(); return; }
    if (inCmd) {
      if (combo === 'Tab') { ev.preventDefault(); app.cur?.$.list.focus(); return; }
      if (combo === 'Ctrl+X') { ev.preventDefault(); cmdline.historyStep(1); return; }
      // the field edits itself: characters, Backspace, Delete, Left, Right, Home, End, and the system's text keys
      if (printable(ev) || ['Backspace', 'Delete', 'Left', 'Right', 'Home', 'End', 'Shift+Left', 'Shift+Right', 'Shift+Home', 'Shift+End'].includes(combo)) return;
      if (ev.metaKey && !ev.altKey && /^(Cmd\+[ACVXZ]|Shift\+Cmd\+Z)$/.test(combo)) return;
      if (ev.ctrlKey && app.platform !== 'darwin' && /^Ctrl\+[ACVXZ]$/.test(combo)) return;
    } else {
      if (combo === 'Backspace') { ev.preventDefault(); cmdline.backspace(); return; }
      if (combo === 'Delete') { ev.preventDefault(); cmdline.del(); return; }
      if (combo === 'Left') { ev.preventDefault(); cmdline.caret(-1); return; }
      if (combo === 'Right') { ev.preventDefault(); cmdline.caret(1); return; }
      if (combo === 'Ctrl+X') { ev.preventDefault(); cmdline.historyStep(1); return; }
      if (printable(ev)) { ev.preventDefault(); cmdline.insert(ev.key); return; }
    }
  }
  const name = commandForKey(combo, !hasText);
  if (name) {
    ev.preventDefault();
    app.run(name, { key: combo });
    return;
  }
  if (!inCmd && printable(ev) && ev.key !== ' ') {
    ev.preventDefault();
    cmdline.insert(ev.key);
  }
}

export function initKeyboard() {
  window.addEventListener('keydown', onKeyDown);
  window.addEventListener('keyup', setMods);
  window.addEventListener('blur', () => { if (mods.size) { mods.clear(); modListeners.forEach((f) => f(heldModifiers())); } });
  // mouse buttons 4 and 5 go back and forward (F-073)
  window.addEventListener('mouseup', (ev) => {
    if (ev.button === 3) { ev.preventDefault(); app.run('cm_GoToPreviousDir'); }
    if (ev.button === 4) { ev.preventDefault(); app.run('cm_GoToNextDir'); }
  });
}
