// Which keys start or feed quick search (part of find-quick.js, kept apart so the unit tests can load it without a
// window): with the "ctrlalt" mode Ctrl+Alt+letter types that letter into the search box, with "alt" Alt+letter does,
// with "letter" plain letters do, and "off" never. The letter comes from the physical key, because Option+letter on a
// Mac types a special character (F-286). AltGr on Windows and Linux is no Ctrl+Alt (F-290): keys.js leaves Ctrl and
// Alt out of the combo then, so an AltGr character never starts a search in the "ctrlalt" or "alt" mode and goes to
// the command line like any typed character.

import { printable } from '../core/keys.js';

const PUNCT = { Minus: ['-', '_'], Period: ['.', '>'], Comma: [',', '<'], Equal: ['=', '+'], Semicolon: [';', ':'], Quote: ["'", '"'],
  BracketLeft: ['[', '{'], BracketRight: [']', '}'], Backquote: ['`', '~'], Slash: ['/', '?'], Backslash: ['\\', '|'], Space: [' ', ' '] };
const SHIFT_DIGITS = { 8: '*', 1: '!', 2: '@', 3: '#', 4: '$', 5: '%', 6: '^', 7: '&', 9: '(', 0: ')' };

// keyChar is the character of a key pressed with Ctrl+Alt or Alt, from the physical key (F-286: Option+letter on a Mac
// types a special character, so ev.key can't be used).
export function keyChar(ev) {
  const c = ev.code || '';
  if (/^Key[A-Z]$/.test(c)) return c.slice(3).toLowerCase();
  if (/^Digit\d$/.test(c)) return ev.shiftKey ? SHIFT_DIGITS[c.slice(5)] || '' : c.slice(5);
  if (/^Numpad\d$/.test(c)) return c.slice(6);
  if (PUNCT[c]) return PUNCT[c][ev.shiftKey ? 1 : 0];
  return ev.key?.length === 1 ? ev.key : '';
}

// trigger is the character a key types into quick search in this mode ('' when the key is not for quick search).
export function trigger(ev, combo, mode) {
  if (ev.metaKey) return '';
  if (mode === 'ctrlalt' && /^Ctrl\+Alt\+/.test(combo)) return keyChar(ev);
  if (mode === 'alt' && /^Alt\+/.test(combo)) return keyChar(ev);
  if (mode === 'letter' && printable(ev) && ev.key !== ' ' && ev.key.length === 1) return ev.key;
  return '';
}
