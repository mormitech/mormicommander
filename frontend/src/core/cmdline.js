// The command line under the panels. As in Total Commander, the keyboard stays with the panel: letters typed there go
// into the command line (F-300), Enter runs it when it has text (F-812), Backspace and Left/Right edit it, Esc clears it.
// Clicking into it gives it real focus, for mouse editing. Typing in a panel also gives it real focus, so its caret shows
// and it is marked as focused; the keys that are not text editing (Up, Down, F-keys...) still go to the panel. Once the
// line is empty again (Backspace, Enter ran it, Esc) the focus goes back to the panel, so Space selects files again.
//
// What Enter does with the text is up to handlers (features/cmdline.js has "cd" and cm_* names; a feature for running
// programs adds its own):  addCmdlineHandler(async (text, {shift}) => handled);

import { setSetting, getSetting } from './settings.js';

const handlers = [];
export function addCmdlineHandler(fn, { first = false } = {}) { if (first) handlers.unshift(fn); else handlers.push(fn); }

const el = () => document.getElementById('cmd');
let histPos = -1;
let typed = false; // the field has the focus because the user typed in a panel (not because it was clicked)

// backToPanel gives the focus back to the active panel's list when typing had taken it to the command line.
function backToPanel() {
  if (!typed) return;
  typed = false;
  if (document.activeElement === el()) document.querySelector('.panel.active .list')?.focus();
}
let wired = false;
function wire() {
  const i = el();
  if (wired || !i) return;
  wired = true;
  i.addEventListener('input', () => { if (!i.value) backToPanel(); });
  i.addEventListener('blur', () => { typed = false; });
}

export const cmdline = {
  get text() { return el()?.value || ''; },
  get focused() { return document.activeElement === el(); },

  set(text) {
    const i = el();
    i.value = text;
    i.setSelectionRange(text.length, text.length);
  },

  // insert puts text at the caret (with a space before it when needed, for Ctrl+Enter names).
  insert(s) {
    const i = el();
    const a = i.selectionStart ?? i.value.length, b = i.selectionEnd ?? a;
    i.value = i.value.slice(0, a) + s + i.value.slice(b);
    i.setSelectionRange(a + s.length, a + s.length);
    histPos = -1;
  },

  // add appends a word, separated by a space (F-813).
  add(word) {
    const t = this.text;
    this.set(t && !t.endsWith(' ') ? `${t} ${word} ` : `${t}${word} `);
  },

  backspace() {
    const i = el();
    let a = i.selectionStart ?? i.value.length, b = i.selectionEnd ?? a;
    if (a === b && a > 0) a--;
    i.value = i.value.slice(0, a) + i.value.slice(b);
    i.setSelectionRange(a, a);
  },

  del() {
    const i = el();
    const a = i.selectionStart ?? i.value.length;
    let b = i.selectionEnd ?? a;
    if (a === b) b++;
    i.value = i.value.slice(0, a) + i.value.slice(b);
    i.setSelectionRange(a, a);
  },

  caret(d) {
    const i = el();
    const p = Math.max(0, Math.min(i.value.length, (i.selectionStart ?? 0) + d));
    i.setSelectionRange(p, p);
  },

  clear() { this.set(''); histPos = -1; backToPanel(); },
  focus() { el().focus(); },

  // focusFromTyping gives the field the focus after a letter typed in a panel went into it.
  focusFromTyping() {
    const i = el();
    if (!i || i.offsetParent === null || document.activeElement === i) return; // hidden (Layout.CmdLine off) or already there
    wire();
    const a = i.selectionStart, b = i.selectionEnd;
    i.focus({ preventScroll: true });
    i.setSelectionRange(a, b);
    typed = true;
  },

  get history() { return getSetting('cmdHistory', []); },

  // historyStep goes back (-1, Ctrl+E) or forward (+1, Ctrl+X) through the commands run before.
  historyStep(d) {
    const h = this.history;
    if (!h.length) return;
    histPos = histPos < 0 ? (d < 0 ? 0 : -1) : histPos - d;
    if (histPos < 0) { histPos = -1; this.set(''); return; }
    histPos = Math.min(h.length - 1, histPos);
    this.set(h[histPos]);
  },

  // submit runs the text: the first handler that takes it wins. The line is cleared afterwards.
  async submit({ shift = false } = {}) {
    const text = this.text.trim();
    if (!text) return false;
    const h = this.history.filter((x) => x !== text);
    h.unshift(text);
    setSetting('cmdHistory', h.slice(0, 50));
    histPos = -1;
    this.clear();
    for (const fn of handlers) {
      if (await fn(text, { shift })) return true;
    }
    return false;
  },
};
