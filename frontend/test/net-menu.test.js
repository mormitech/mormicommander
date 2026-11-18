// @vitest-environment happy-dom
// The Net menu (issue #63, docs/FEATURES.md section 15): every entry names a registered command, the entries with a key
// show it (menuKey: FTP connect Ctrl+F, New FTP connection Ctrl+N, Disconnect Ctrl+Shift+F, Transfer mode Ctrl+Shift+M),
// and Disconnect / Send FTP command with no server panel say so without calling Go. Go is a stand-in that records calls
// and answers nothing, so nothing ever connects anywhere.
import { beforeAll, expect, test, vi } from 'vitest';

const calls = [];
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: (_, svc) => new Proxy({}, { get: (__, m) => (...a) => { calls.push(`${String(svc)}.${String(m)}`); return Promise.resolve(null); } }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus, keymap;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  keymap = await import('../src/core/keymap.js');
  vi.useRealTimers();
});

const netMenu = () => menus.MENUS.find(([l]) => l === 'Net')[1].filter(Array.isArray).map((it) => it[1]);

test('every entry of the Net menu runs a registered command', () => {
  expect(netMenu().filter((n) => typeof n === 'string' && !reg.hasCommand(n))).toEqual([]);
});

test('the Net menu entries with a key show it (F-377, F-378, F-379, F-816)', () => {
  const keys = { cm_FtpConnect: 'Ctrl+F', cm_FtpNew: 'Ctrl+N', cm_FtpDisconnect: 'Ctrl+Shift+F', cm_FtpTransferMode: 'Ctrl+Shift+M' };
  for (const [n, k] of Object.entries(keys)) {
    expect(netMenu(), n).toContain(n);
    expect(reg.getCommand(n).menuKey, n).toBe(true);
    expect(keymap.DEFAULT_KEYS[k], k).toBe(n);
  }
});

test('Disconnect and Send FTP command with no server panel say so and call nothing', async () => {
  const flash = vi.fn();
  const app = { cur: { path: '/home' }, other: { path: '/tmp' }, panels: [], flash, setting: (_, d) => d };
  calls.length = 0;
  expect(await reg.getCommand('cm_FtpDisconnect').run(app)).toBe(false);
  expect(await reg.getCommand('cm_FtpSendCommand').run(app)).toBe(false);
  expect(flash).toHaveBeenCalledTimes(2);
  expect(calls).toEqual([]);
});
