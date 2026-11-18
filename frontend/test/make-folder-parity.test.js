// @vitest-environment happy-dom
// Make folder (issue #71, FEATURES.md section 23): New folder (F7) is in the Files menu with its key, right after
// Rename, and runs the same cm_MkDir as F7 (F-520..F-522); it is not reachable from the AI tools. Go is a stand-in
// that answers nothing; the timers are frozen so the menus are never sent anywhere.
import { beforeAll, expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, keymap, allow;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  keymap = await import('../src/core/keymap.js');
  allow = await import('../src/features/aitools-allow.js');
  vi.useRealTimers();
});

test('New folder sits in the Files menu with its key, after Rename (F-520..F-522)', () => {
  expect(reg.getCommand('cm_MkDir')).toMatchObject({ menu: 'Files', menuAfter: 'cm_RenameOnly', menuKey: true });
  expect(keymap.DEFAULT_KEYS.F7).toBe('cm_MkDir');
  const order = reg.commandList().map((c) => c.name);
  expect(order.indexOf('cm_RenameOnly')).toBeGreaterThan(-1);
  expect(order.indexOf('cm_RenameOnly')).toBeLessThan(order.indexOf('cm_MkDir'));
});

test('New folder is not reachable from the AI tools', () => {
  expect(Object.hasOwn(allow.ALLOWED_COMMANDS, 'cm_MkDir')).toBe(false);
  expect(allow.commandVerdict('cm_MkDir').ok).toBe(false);
});
