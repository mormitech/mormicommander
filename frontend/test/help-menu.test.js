// @vitest-environment happy-dom
// The Help menu (issue #67, docs/FEATURES.md section 19): every entry names a registered command, Help shows its key
// (F1), and Website, Report a problem and Check for updates only
// open a page in the browser when run (F-454, F-455) — nothing goes online by itself. Go is a stand-in that records calls.
import { beforeAll, expect, test, vi } from 'vitest';

const calls = [];
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: (_, svc) => new Proxy({}, { get: (__, m) => (...a) => { calls.push([`${String(svc)}.${String(m)}`, ...a]); return Promise.resolve(null); } }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '# Keys' }));

let reg, menus, keymap;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  await import('../src/features/config/help.js');
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  keymap = await import('../src/core/keymap.js');
  vi.useRealTimers();
});

const helpMenu = () => menus.MENUS.find(([l]) => l === 'Help')[1].filter(Array.isArray).map((it) => it[1]);

test('every entry of the Help menu runs a registered command', () => {
  expect(helpMenu()).toEqual(['cm_HelpIndex', 'cm_Keyboard', 'cm_VisitHomepage', 'cm_ReportProblem', 'cm_CheckForUpdates', 'cm_About']);
  expect(helpMenu().filter((n) => !reg.hasCommand(n))).toEqual([]);
});

test('Help shows its key F1 in the menu (F-451)', () => {
  expect(reg.getCommand('cm_HelpIndex').menuKey).toBe(true);
  expect(keymap.DEFAULT_KEYS.F1).toBe('cm_HelpIndex');
});

test('nothing in the Help menu goes online until it is run (zero network)', () => {
  expect(calls.filter(([m]) => m === 'App.OpenURL')).toEqual([]);
});

test('Website, Report a problem and Check for updates open their page in the browser (F-454, F-455)', async () => {
  calls.length = 0;
  for (const n of ['cm_VisitHomepage', 'cm_ReportProblem', 'cm_CheckForUpdates']) await reg.getCommand(n).run({});
  expect(calls.filter(([m]) => m === 'App.OpenURL').map((c) => c[1])).toEqual([
    'https://github.com/mormitech/mormicommander',
    'https://github.com/mormitech/mormicommander/issues',
    'https://github.com/mormitech/mormicommander/releases/latest',
  ]);
});
