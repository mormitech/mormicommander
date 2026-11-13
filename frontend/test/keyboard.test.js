// @vitest-environment happy-dom
// The keyboard's routing (src/core/keyboard.js) as Total Commander has it: printable keys typed in a panel go to the
// command line (F-275, F-300), so "cd .." + Enter works; Space selects while the command line is empty and is a space
// once it has text; Esc clears it; AltGr characters are typed, not taken as Ctrl+Alt keys (F-290). The app and the
// settings are stand-ins; the keymap and the registry are the real ones.
import { beforeEach, expect, test, vi } from 'vitest';

globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const ran = [];
vi.mock('../src/core/app.js', () => ({
  app: {
    platform: 'linux',
    run: (name, args) => { ran.push({ name, args }); return Promise.resolve(); },
    get cur() { return { $: { list: document.getElementById('list') } }; },
  },
}));
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => d, setSetting: () => {} }));

const { onKeyDown } = await import('../src/core/keyboard.js');
const { cmdline } = await import('../src/core/cmdline.js');
const reg = await import('../src/core/registry.js');
const { eventInit } = await import('../src/core/keys.js');

beforeEach(() => {
  document.body.innerHTML = '<div id="list" tabindex="0"></div><input id="cmd" type="text">';
  document.getElementById('list').focus();
  ran.length = 0;
  reg.setupKeys('linux', {});
});

// press sends one key to the panel's file list the way the browser would.
function press(combo, extra = {}) {
  const init = { ...eventInit(combo), ...extra };
  const ev = new KeyboardEvent('keydown', init);
  if (extra.altGr) Object.defineProperty(ev, 'getModifierState', { value: (m) => m === 'AltGraph' });
  document.getElementById('list').dispatchEvent(ev);
  onKeyDown(ev);
  return ev;
}
const type = (s) => { for (const ch of s) press(ch === ' ' ? 'Space' : ch === '.' ? '.' : /[A-Z]/.test(ch) ? `Shift+${ch}` : ch.toUpperCase(), { key: ch }); };

test('letters typed in the panel go to the command line', () => {
  type('cd');
  expect(cmdline.text).toBe('cd');
  expect(ran).toEqual([]);
});

test('"cd .." with a space, then Enter runs the command line', () => {
  type('cd ..');
  expect(cmdline.text).toBe('cd ..');
  expect(ran.map((r) => r.name)).toEqual([]); // the space was typed, not Space's select
  press('Enter');
  expect(ran.map((r) => r.name)).toEqual(['cm_ExecuteCmdLine']);
});

test('Space with an empty command line selects', () => {
  press('Space');
  expect(ran.map((r) => r.name)).toEqual(['cm_SelectToggle']);
  expect(cmdline.text).toBe('');
});

test('Backspace edits the command line while it has text, else goes up', () => {
  type('ab');
  press('Backspace');
  expect(cmdline.text).toBe('a');
  press('Esc');
  expect(cmdline.text).toBe('');
  press('Backspace');
  expect(ran.map((r) => r.name)).toEqual(['cm_GoToParent']);
});

test('Esc with an empty command line runs cm_ClearCmdLine', () => {
  press('Esc');
  expect(ran.map((r) => r.name)).toEqual(['cm_ClearCmdLine']);
});

test('AltGr characters are typed (Hungarian AltGr+Q is \\), not taken as Ctrl+Alt+Q', () => {
  press('Ctrl+Alt+Q', { key: '\\', altGr: true });
  expect(cmdline.text).toBe('\\');
  expect(ran).toEqual([]);
});

test('a TC key still runs its command while the command line is empty', () => {
  press('F7');
  expect(ran.map((r) => r.name)).toEqual(['cm_MkDir']);
});
