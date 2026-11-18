// @vitest-environment happy-dom
// Synchronize folders (issue #73, FEATURES.md section 25): the Commands menu entry runs cm_SyncDirs and shows its key
// when one is set (F-546), the window's F3 / F4 and menu entries are wired (F-550, F-553), and synchronizing is never
// reachable from the AI tools, because it copies over and deletes files. Go is a stand-in that answers nothing; the
// timers are frozen so the menus are never sent anywhere.
import { beforeAll, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus, allow;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  allow = await import('../src/features/aitools-allow.js');
  vi.useRealTimers();
});

test('Synchronize folders is in the Commands menu and shows its key', () => {
  const cmds = menus.MENUS.find(([l]) => l === 'Commands')[1].filter(Array.isArray).map((it) => it[1]);
  expect(cmds).toContain('cm_SyncDirs');
  expect(reg.getCommand('cm_SyncDirs').menuKey).toBe(true);
});

test('Synchronize folders is not reachable from the AI tools', () => {
  expect(Object.hasOwn(allow.ALLOWED_COMMANDS, 'cm_SyncDirs')).toBe(false);
  expect(allow.commandVerdict('cm_SyncDirs').ok).toBe(false);
});

test('the result list views and edits a side with F3 / Shift+F3 / F4 / Shift+F4 and from its menu', () => {
  const src = readFileSync(resolve(__dirname, '../src/features/syncdirs.js'), 'utf8');
  for (const k of ["case 'F3': viewEdit('cm_List', 'left')", "case 'Shift+F3': viewEdit('cm_List', 'right')",
    "case 'F4': viewEdit('cm_Edit', 'left')", "case 'Shift+F4': viewEdit('cm_Edit', 'right')",
    "case 'selLR': selectWithAction('lr')", "case 'selRL': selectWithAction('rl')"]) expect(src).toContain(k);
});
