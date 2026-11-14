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

// The held modifiers (Ctrl, Alt, Shift, Cmd), which the function key bar shows (F-023). They are read from every key
// and mouse event, in the capture phase at the window, so a dialog that takes the keys for itself cannot hide them.
// A modifier let go while another window had the keyboard (a dialog window, Cmd+Tab to another app) sends its key-up
// to that window, so the set is emptied whenever this window stops being the active one (releaseModifiers), and
// any later mouse movement over the window corrects it from the event's own flags.
const mods = new Set();
const modListeners = new Set();
export const heldModifiers = () => new Set(mods);
export function onModifiers(fn) { modListeners.add(fn); return () => modListeners.delete(fn); }
function changeMods(next) {
  if ([...next].join() === [...mods].join()) return;
  mods.clear();
  next.forEach((m) => mods.add(m));
  modListeners.forEach((f) => f(heldModifiers()));
}
// syncModifiers takes the modifiers from a keyboard or mouse event's flags.
export function syncModifiers(ev) {
  const next = new Set();
  if (ev.ctrlKey) next.add('Ctrl');
  if (ev.altKey) next.add('Alt');
  if (ev.shiftKey) next.add('Shift');
  if (ev.metaKey) next.add('Cmd');
  changeMods(next);
}
// releaseModifiers forgets the held modifiers (the window lost the keyboard; a key still held shows up again with its
// next event).
export function releaseModifiers() { changeMods(new Set()); }
const setMods = syncModifiers;

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
  // a command line shown only because the user typed (it is turned off) goes away again with Esc, even when empty
  if (combo === 'Esc' && cmdline.shownForTyping) { ev.preventDefault(); cmdline.clear(); app.cur?.$.list.focus(); return; }
  if (inCmd || hasText) {
    if (combo === 'Enter' || combo === 'Shift+Enter') { ev.preventDefault(); if (hasText) app.run('cm_ExecuteCmdLine', { shift: combo === 'Shift+Enter' }); return; }
    if (combo === 'Esc') { ev.preventDefault(); cmdline.clear(); app.cur?.$.list.focus(); return; }
    if (inCmd) {
      // Tab completes the file or folder name at the caret; more Tabs go through the matches (Shift+Tab back). With
      // nothing to complete, Tab gives the keyboard back to the panel as before.
      if (combo === 'Tab' || combo === 'Shift+Tab') {
        ev.preventDefault();
        const back = combo === 'Shift+Tab';
        cmdline.complete(back).then((done) => { if (!done && !back) app.cur?.$.list.focus(); });
        return;
      }
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
      if (printable(ev)) { ev.preventDefault(); cmdline.insert(ev.key); cmdline.focusFromTyping(); return; }
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
    cmdline.focusFromTyping(); // the field takes the focus: its caret shows and it is marked as focused
  }
}

export function initKeyboard() {
  window.addEventListener('keydown', setMods, true);
  window.addEventListener('keyup', setMods, true);
  window.addEventListener('keydown', onKeyDown);
  for (const t of ['mousemove', 'mousedown', 'mouseup', 'wheel']) window.addEventListener(t, setMods, { capture: true, passive: true });
  // the window stops being the active one: nothing it saw pressed can be trusted to be still down
  window.addEventListener('blur', releaseModifiers);
  window.addEventListener('focus', releaseModifiers);
  document.addEventListener('visibilitychange', () => { if (document.visibilityState !== 'visible') releaseModifiers(); });
  // mouse buttons 4 and 5 go back and forward (F-073)
  window.addEventListener('mouseup', (ev) => {
    if (ev.button === 3) { ev.preventDefault(); app.run('cm_GoToPreviousDir'); }
    if (ev.button === 4) { ev.preventDefault(); app.run('cm_GoToNextDir'); }
  });
}
