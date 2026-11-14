// @vitest-environment happy-dom
// The held modifiers behind the function key bar's labels (#35, F-023): they follow key and mouse events, a dialog
// that swallows a key in the capture phase does not hide a modifier from the bar, and losing the window (blur, the
// page hidden, Go's "window-inactive") forgets them, so the bar never stays on "⌃⇧⌘F3" with no key held. The app and
// the settings are stand-ins; the keyboard, the keymap and the bar are the real ones.
import { beforeAll, beforeEach, expect, test, vi } from 'vitest';

vi.mock('../src/core/app.js', () => ({
  app: { platform: 'darwin', run: () => Promise.resolve(), get cur() { return { $: { list: document.getElementById('list') } }; } },
  LABELS: {},
}));
// The real go.js pulls in @wailsio/runtime, whose drag-setup poll timer outlives happy-dom's teardown and then throws
// "window is not defined" after the tests (a flaky CI failure). The bar never talks to Go here, so a stand-in is enough.
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => d, setSetting: () => {} }));

const kb = await import('../src/core/keyboard.js');
const reg = await import('../src/core/registry.js');
const { initFkeys } = await import('../src/core/fkeys.js');
const { eventInit } = await import('../src/core/keys.js');

beforeAll(() => {
  document.body.innerHTML = '<div id="list" tabindex="0"></div><div id="fkeys"></div>';
  reg.setupKeys('darwin', {});
  kb.initKeyboard();
  initFkeys();
});
beforeEach(() => kb.releaseModifiers());

const keys = () => [...document.querySelectorAll('#fkeys kbd')].map((k) => k.textContent);
const down = (combo) => document.getElementById('list').dispatchEvent(new KeyboardEvent('keydown', eventInit(combo)));
const up = (key) => window.dispatchEvent(new KeyboardEvent('keyup', { key, bubbles: true }));

test('the bar shows the plain keys with nothing held', () => {
  expect(keys()).toEqual(['F3', 'F4', 'F5', 'F6', 'F7', 'F8', '⌥F4']);
});

test('holding Shift shows the shifted keys, letting go brings the plain ones back', () => {
  down('Shift');
  expect(keys()[0]).toBe('⇧F3');
  up('Shift');
  expect(keys()[0]).toBe('F3');
});

test('a dialog that swallows keys in the capture phase does not hide a held modifier', () => {
  const swallow = (ev) => ev.stopImmediatePropagation();
  window.addEventListener('keydown', swallow, true); // as core/dialogs.js does while a dialog is open
  try {
    down('Shift');
    expect(kb.heldModifiers().has('Shift')).toBe(true);
    expect(keys()[0]).toBe('⇧F3');
  } finally { window.removeEventListener('keydown', swallow, true); }
});

test('modifiers whose key-up went to another window are forgotten when the window loses focus', () => {
  down('Ctrl+Shift+Cmd');
  expect(keys()[0]).toBe('⌃⇧⌘F3');
  window.dispatchEvent(new Event('blur'));
  expect(keys()[0]).toBe('F3');
  expect(kb.heldModifiers().size).toBe(0);
});

test('Go telling the page the window went inactive forgets them too (releaseModifiers)', () => {
  down('Shift+Cmd');
  expect(kb.heldModifiers().size).toBe(2);
  kb.releaseModifiers();
  expect(keys()[0]).toBe('F3');
});

test('a mouse movement corrects a stuck modifier from its own flags', () => {
  down('Ctrl+Shift');
  window.dispatchEvent(new MouseEvent('mousemove', { bubbles: true }));
  expect(keys()[0]).toBe('F3');
  window.dispatchEvent(new MouseEvent('mousemove', { bubbles: true, shiftKey: true }));
  expect(keys()[0]).toBe('⇧F3');
});

test('the page being hidden forgets them', () => {
  down('Alt');
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' });
  document.dispatchEvent(new Event('visibilitychange'));
  delete document.visibilityState;
  expect(kb.heldModifiers().size).toBe(0);
});
