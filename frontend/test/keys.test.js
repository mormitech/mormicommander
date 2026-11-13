import { describe, expect, test } from 'vitest';
import { normalizeCombo, comboFromEvent, eventInit, splitCombo, label } from '../src/core/keys.js';

const ev = (init) => ({ getModifierState: () => false, ...init });

describe('normalizeCombo', () => {
  test.each([
    ['ctrl+shift+f5', 'Ctrl+Shift+F5'],
    ['Shift+Ctrl+F5', 'Ctrl+Shift+F5'],
    ['alt+left', 'Alt+Left'],
    ['Num +', 'Num+'],
    ['Ctrl+Num+', 'Ctrl+Num+'],
    ['Ctrl+Shift+Num-', 'Ctrl+Shift+Num-'],
    ['Ctrl++', 'Ctrl++'],
    ['Command+Shift+.', 'Shift+Cmd+.'],
    ['Option+F7', 'Alt+F7'],
    ['del', 'Delete'],
    ['PageDown', 'PgDn'],
    ['Ctrl+\\', 'Ctrl+\\'],
    ['a', 'A'],
    ['Shift', 'Shift'],
  ])('%s → %s', (a, b) => expect(normalizeCombo(a)).toBe(b));
});

describe('comboFromEvent', () => {
  test('physical key wins over the typed character (Option+letter on a Mac)', () => {
    expect(comboFromEvent(ev({ code: 'KeyA', key: 'å', altKey: true }))).toBe('Alt+A');
  });
  test('Hungarian Ctrl+í is the IntlBackslash key: Ctrl+<', () => {
    expect(comboFromEvent(ev({ code: 'IntlBackslash', key: 'í', ctrlKey: true }))).toBe('Ctrl+<');
  });
  test('numpad keys', () => {
    expect(comboFromEvent(ev({ code: 'NumpadAdd', key: '+', ctrlKey: true }))).toBe('Ctrl+Num+');
    expect(comboFromEvent(ev({ code: 'NumpadMultiply', key: '*', shiftKey: true }))).toBe('Shift+Num*');
  });
  test('AltGr is no Ctrl+Alt', () => {
    expect(comboFromEvent({ code: 'KeyQ', key: '\\', ctrlKey: true, altKey: true, getModifierState: (m) => m === 'AltGraph' })).toBe('Q');
  });
  test('a lone modifier is no combo', () => {
    expect(comboFromEvent(ev({ code: 'ShiftLeft', key: 'Shift', shiftKey: true }))).toBe('');
  });
  test('no code: the key is used', () => {
    expect(comboFromEvent(ev({ code: '', key: '~' }))).toBe('~');
  });
});

describe('eventInit round trip', () => {
  test.each(['F5', 'Ctrl+Shift+F5', 'Alt+Left', 'Ctrl+Num+', 'Shift+Num*', 'Ctrl+\\', 'Ctrl+<', 'Shift+Cmd+.', 'Ctrl+=', 'Enter', 'Space', 'Ctrl+PgDn', 'Shift+8', 'Alt+Shift+F9'])('%s', (c) => {
    expect(comboFromEvent(ev(eventInit(c)))).toBe(normalizeCombo(c));
  });
  test('lone modifier sets its flag', () => {
    expect(eventInit('Shift').shiftKey).toBe(true);
  });
  test('splitCombo keeps Num+ whole', () => {
    const { mods, name } = splitCombo('Ctrl+Shift+Num+');
    expect([...mods]).toEqual(['Ctrl', 'Shift']);
    expect(name).toBe('Num+');
  });
});

test('label uses the Mac symbols on macOS only', () => {
  expect(label('Ctrl+Alt+Shift+Cmd+F5', 'darwin')).toBe('⌃⌥⇧⌘F5');
  expect(label('Ctrl+F5', 'windows')).toBe('Ctrl+F5');
});
