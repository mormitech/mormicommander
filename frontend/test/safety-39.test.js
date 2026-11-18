// @vitest-environment happy-dom
// Section 39 of docs/FEATURES.md (issue #87): the access-denied hint under the error question and the error list
// (F-720: no elevation, a clear message instead), and Configuration > Confirmations… (cm_SafetyConfirmations, F-719)
// in its menu. Go is a stand-in that answers nothing; the timers are frozen so the menus are never sent anywhere.
import { beforeAll, expect, test, vi } from 'vitest';
import { deniedHint, anyDenied } from '../src/core/joberrors.js';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  vi.useRealTimers();
});

test('the access-denied hint says what to do on each system, never offering to raise rights', () => {
  expect(deniedHint('darwin')).toMatch(/Full Disk Access/);
  expect(deniedHint('darwin')).toMatch(/Change attributes/);
  expect(deniedHint('linux')).toMatch(/sudo/);
  expect(deniedHint('windows')).toMatch(/Run as administrator/);
  for (const p of ['darwin', 'linux', 'windows', undefined]) expect(deniedHint(p).length).toBeGreaterThan(40);
});

test('the error list shows the hint only when something was denied', () => {
  expect(anyDenied([{ reason: 'the disk is full' }, { reason: 'permission denied' }])).toBe(true);
  expect(anyDenied([{ reason: 'the disk is full' }])).toBe(false);
  expect(anyDenied(null)).toBe(false);
});

test('Configuration > Confirmations… runs cm_SafetyConfirmations (F-719)', () => {
  const c = reg.getCommand('cm_SafetyConfirmations');
  expect(c).toMatchObject({ menu: 'Configuration', menuAfter: 'cm_ConfigHotkeys' });
  expect(reg.hasCommand('cm_Exit')).toBe(true);
  expect(menus.MENUS.some(([l]) => l === 'Configuration')).toBe(true);
});
