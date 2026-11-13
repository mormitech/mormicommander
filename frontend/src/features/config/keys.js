// Key fields: a text field with data-keycapture="1" takes the next key combo pressed in it ("Ctrl+Shift+F5") instead
// of the typed text, for the hotkey page and the Start menu editor. Dialogs swallow keys in the capture phase of the
// window; this listener is added when the module loads, before any dialog's, so it sees the key first. Esc still
// cancels the dialog, Tab still moves on, and Backspace or Delete alone clear the field.

import { comboFromEvent, normalizeCombo, label } from '../../core/keys.js';
import { app } from '../../core/app.js';

window.addEventListener('keydown', (ev) => {
  const el = document.activeElement;
  if (!el?.dataset?.keycapture || ev.target !== el) return;
  const combo = comboFromEvent(ev);
  if (!combo || combo === 'Esc' || combo === 'Tab' || combo === 'Shift+Tab' || combo === 'Enter') return;
  ev.preventDefault();
  ev.stopImmediatePropagation();
  el.value = combo === 'Backspace' || combo === 'Delete' ? '' : normalizeCombo(combo);
  el.dispatchEvent(new Event('input', { bubbles: true }));
}, true);

// keyText is a combo as the platform shows it (⌃⌥⇧⌘ on macOS).
export const keyText = (combo) => (combo ? label(combo, app.platform) : '');

// hasModifier says whether a combo can serve as a global hotkey without stealing typing (a modifier, or a key that types
// nothing such as F5).
export function hotkeyOK(combo) {
  const c = normalizeCombo(combo);
  if (!c) return false;
  if (/^(Ctrl|Alt|Cmd)\+/.test(c) || /(^|\+)(Ctrl|Alt|Cmd)\+/.test(c)) return true;
  const key = c.split('+').pop();
  return /^F\d{1,2}$/.test(key) || ['Ins', 'Home', 'End', 'PgUp', 'PgDn', 'Apps'].includes(key) || key.startsWith('Num');
}
