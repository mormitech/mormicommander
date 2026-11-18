// @vitest-environment happy-dom
// The editor commands in the Files menu (issue #76, FEATURES.md section 28): Edit (F4), Edit new file (Shift+F4) and
// Edit with the built-in editor are in the menu, run their cm_* command and show their key; the built-in editor is not
// on the AI allow-list. Go is a stand-in that answers nothing.
import { beforeAll, expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus, keymap, allow;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  keymap = await import('../src/core/keymap.js');
  allow = await import('../src/features/aitools-allow.js');
  vi.useRealTimers();
});

const flat = (items) => items.flatMap((it) => (Array.isArray(it) ? (Array.isArray(it[1]) ? flat(it[1]) : [it[1]]) : []));

test('Files has Edit, Edit new file and the built-in editor, in that order after Properties, with their keys', () => {
  expect(flat(menus.MENUS.find(([l]) => l === 'Files')[1])).toContain('cm_Properties');
  expect(reg.getCommand('cm_Edit')).toMatchObject({ menu: 'Files', menuAfter: 'cm_Properties', menuKey: true });
  expect(reg.getCommand('cm_EditNewFile')).toMatchObject({ menu: 'Files', menuAfter: 'cm_Edit', menuKey: true });
  expect(reg.getCommand('cm_EditInternal')).toMatchObject({ menu: 'Files', menuAfter: 'cm_EditNewFile' });
  // registered in that order, so each one finds the one it follows already in the menu
  const order = reg.commandList().map((c) => c.name);
  expect(order.indexOf('cm_Edit')).toBeLessThan(order.indexOf('cm_EditNewFile'));
  expect(order.indexOf('cm_EditNewFile')).toBeLessThan(order.indexOf('cm_EditInternal'));
  expect(keymap.DEFAULT_KEYS.F4).toBe('cm_Edit');
  expect(keymap.DEFAULT_KEYS['Shift+F4']).toBe('cm_EditNewFile');
});

test('the built-in editor saves files, so the AI can\'t run it', () => {
  expect(allow.DENIED_COMMANDS.cm_EditInternal).toBeTruthy();
  expect(JSON.stringify(allow)).not.toMatch(/ALLOWED[^}]*cm_EditInternal/);
});
