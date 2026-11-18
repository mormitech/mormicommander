// @vitest-environment happy-dom
// Rename and the Multi-Rename tool (issue #70, FEATURES.md section 22): Move or rename (F6) and Rename (Shift+F6) are
// in the Files menu with their keys, Shift+F6 again cycles the selection of the inline edit box (F-502), Ctrl+M opens
// the Multi-Rename tool (F-504), and none of the rename commands is reachable from the AI tools. Go is a stand-in that
// answers nothing; the timers are frozen so the menus are never sent anywhere.
import { beforeAll, expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, keymap, menus, fileops, allow;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  keymap = await import('../src/core/keymap.js');
  menus = await import('../src/core/menus.js');
  fileops = await import('../src/features/fileops.js');
  allow = await import('../src/features/aitools-allow.js');
  vi.useRealTimers();
});

test('Move or rename and Rename sit in the Files menu with their keys, after Copy in the same folder (F-502, F-503)', () => {
  expect(reg.getCommand('cm_RenMov')).toMatchObject({ menu: 'Files', menuAfter: 'cm_CopySamepanel', menuKey: true });
  expect(reg.getCommand('cm_RenameOnly')).toMatchObject({ menu: 'Files', menuAfter: 'cm_RenMov', menuKey: true });
  expect(keymap.DEFAULT_KEYS.F6).toBe('cm_RenMov');
  expect(keymap.DEFAULT_KEYS['Shift+F6']).toBe('cm_RenameOnly');
  // menuSpec puts a command right after its menuAfter entry, in registration order: the anchors come first
  const order = reg.commandList().map((c) => c.name);
  expect(order.indexOf('cm_MultiRenameFiles')).toBeGreaterThan(-1);
  expect(order.indexOf('cm_CopySamepanel')).toBeLessThan(order.indexOf('cm_RenMov'));
  expect(order.indexOf('cm_RenMov')).toBeLessThan(order.indexOf('cm_RenameOnly'));
});

test('Ctrl+M opens the Multi-Rename tool, which is in the Files menu with its key (F-504)', () => {
  expect(keymap.DEFAULT_KEYS['Ctrl+M']).toBe('cm_MultiRenameFiles');
  expect(reg.getCommand('cm_MultiRenameFiles')).toMatchObject({ menuKey: true });
  expect(menus.MENUS.find(([l]) => l === 'Files')[1].some((it) => Array.isArray(it) && it[1] === 'cm_MultiRenameFiles')).toBe(true);
});

test('Shift+F6 again cycles the selection: name only, extension only, the whole name (F-502)', () => {
  const file = { name: 'report.final.txt', base: 'report.final', ext: 'txt' };
  expect(fileops.renameSelection(file, 0)).toEqual([0, 12]);
  expect(fileops.renameSelection(file, 1)).toEqual([13, 16]);
  expect(fileops.renameSelection(file, 2)).toEqual([0, 16]);
  // folders and files without an extension always select the whole name
  expect(fileops.renameSelection({ name: 'photos.2024', base: 'photos.2024', ext: '', dir: true }, 1)).toEqual([0, 11]);
  expect(fileops.renameSelection({ name: 'Makefile', base: 'Makefile', ext: '' }, 0)).toEqual([0, 8]);
});

test('no rename command is reachable from the AI tools', () => {
  for (const c of ['cm_RenMov', 'cm_RenameOnly', 'cm_MultiRenameFiles']) {
    expect(Object.hasOwn(allow.ALLOWED_COMMANDS, c)).toBe(false);
    expect(allow.commandVerdict(c).ok).toBe(false);
  }
});
