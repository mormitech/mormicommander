// Which keys feed quick search (src/features/find-quick-keys.js): Ctrl+Alt+letter by default (F-066, F-274), Alt+letter
// or plain letters by setting, never with Cmd, and never AltGr characters (F-290) in the Ctrl+Alt and Alt modes.
import { describe, expect, test } from 'vitest';
import { trigger, keyChar } from '../src/features/find-quick-keys.js';
import { comboFromEvent } from '../src/core/keys.js';

const ev = (init, altGr = false) => ({ getModifierState: (m) => altGr && m === 'AltGraph', ctrlKey: false, altKey: false, shiftKey: false, metaKey: false, ...init });
const t = (e, mode) => trigger(e, comboFromEvent(e), mode);

describe('ctrlalt mode (TC default)', () => {
  test('Ctrl+Alt+letter types the letter', () => expect(t(ev({ code: 'KeyD', key: 'd', ctrlKey: true, altKey: true }), 'ctrlalt')).toBe('d'));
  test('the physical key counts (Option+letter on a Mac types a special character)', () => {
    expect(t(ev({ code: 'KeyF', key: 'ƒ', ctrlKey: true, altKey: true }), 'ctrlalt')).toBe('f');
  });
  test('Ctrl+Alt+digit and punctuation', () => {
    expect(t(ev({ code: 'Digit3', key: '3', ctrlKey: true, altKey: true }), 'ctrlalt')).toBe('3');
    expect(t(ev({ code: 'Period', key: '.', ctrlKey: true, altKey: true }), 'ctrlalt')).toBe('.');
  });
  test('plain letters are not for quick search: they go to the command line', () => expect(t(ev({ code: 'KeyD', key: 'd' }), 'ctrlalt')).toBe(''));
  test('Alt+letter alone is not quick search here', () => expect(t(ev({ code: 'KeyD', key: 'd', altKey: true }), 'ctrlalt')).toBe(''));
  test('AltGr (Hungarian AltGr+Q types \\) is no quick search', () => {
    expect(t(ev({ code: 'KeyQ', key: '\\', ctrlKey: true, altKey: true }, true), 'ctrlalt')).toBe('');
    expect(t(ev({ code: 'KeyV', key: '@', altKey: false, ctrlKey: false }, true), 'ctrlalt')).toBe(''); // Linux: AltGr alone
  });
  test('Cmd never', () => expect(t(ev({ code: 'KeyD', key: 'd', ctrlKey: true, altKey: true, metaKey: true }), 'ctrlalt')).toBe(''));
});

describe('other modes', () => {
  test('alt: Alt+letter', () => {
    expect(t(ev({ code: 'KeyD', key: '∂', altKey: true }), 'alt')).toBe('d');
    expect(t(ev({ code: 'KeyD', key: 'd', ctrlKey: true, altKey: true }), 'alt')).toBe('');
    expect(t(ev({ code: 'KeyQ', key: '\\', ctrlKey: true, altKey: true }, true), 'alt')).toBe('');
  });
  test('letter: plain characters, not Space', () => {
    expect(t(ev({ code: 'KeyD', key: 'd' }), 'letter')).toBe('d');
    expect(t(ev({ code: 'KeyD', key: 'D', shiftKey: true }), 'letter')).toBe('D');
    expect(t(ev({ code: 'Space', key: ' ' }), 'letter')).toBe('');
    expect(t(ev({ code: 'KeyD', key: 'd', ctrlKey: true }), 'letter')).toBe('');
  });
  test('off: nothing', () => expect(t(ev({ code: 'KeyD', key: 'd', ctrlKey: true, altKey: true }), 'off')).toBe(''));
});

test('keyChar with Shift', () => {
  expect(keyChar(ev({ code: 'Digit8', key: '8', shiftKey: true }))).toBe('*');
  expect(keyChar(ev({ code: 'Minus', key: '-', shiftKey: true }))).toBe('_');
  expect(keyChar(ev({ code: 'Numpad4', key: '4' }))).toBe('4');
});
