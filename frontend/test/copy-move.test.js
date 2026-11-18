// @vitest-environment happy-dom
// Copy / Move (issue #68, FEATURES.md section 20): Copy in the same folder (F-479) is in the Files menu and shows its
// key, Shift+F5. Go is a stand-in that answers nothing; the timers are frozen so the menus are never sent anywhere.
import { beforeAll, expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, keymap;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  keymap = await import('../src/core/keymap.js');
  vi.useRealTimers();
});

test('Copy in the same folder sits in the Files menu with its key (F-479)', () => {
  expect(reg.getCommand('cm_CopySamepanel')).toMatchObject({ menu: 'Files', menuAfter: 'cm_MultiRenameFiles', menuKey: true });
  expect(keymap.DEFAULT_KEYS['Shift+F5']).toBe('cm_CopySamepanel');
  expect(reg.hasCommand('cm_MultiRenameFiles')).toBe(true);
});
