// @vitest-environment happy-dom
// Drives, volumes and disk operations (issue #86, docs/FEATURES.md section 38): the drive list as a folder (F-716,
// cm_OpenDrives), renaming a volume (F-713, cm_RenameVolume, asks twice and is never offered to the AI tools), the drive
// bar's usage bar (F-714, off by default), and the file commands refusing the drive list folder. Go is a stand-in that
// records calls and renames nothing; no real disk is touched.
import { beforeAll, expect, test, vi } from 'vitest';
import { DRIVES, isDrivesPath, canRenameVolume, volumeLabelError, usedPercent } from '../src/features/drives-lib.js';
import { drivesRefusal } from '../src/features/fileops-readonly.js';

const calls = [];
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: (_, obj) => new Proxy({}, { get: (_, m) => (...a) => {
    calls.push([`${obj}.${m}`, ...a]);
    return obj === 'DialogWindows' ? Promise.reject(new Error('DialogWindows is not available')) : Promise.resolve(null);
  } }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus, allow, dialogs;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  allow = await import('../src/features/aitools-allow.js');
  dialogs = await import('../src/core/dialogs.js');
  vi.useRealTimers();
});

test('the drive list folder and volume rename are commands in the Commands menu', () => {
  for (const n of ['cm_OpenDrives', 'cm_RenameVolume', 'cm_EjectVolume', 'cm_VolumeId']) expect(reg.hasCommand(n), n).toBe(true);
  expect(reg.getCommand('cm_OpenDrives')).toMatchObject({ menu: 'Commands', menuAfter: 'cm_GoToRoot' });
  expect(reg.getCommand('cm_RenameVolume')).toMatchObject({ menu: 'Commands', menuAfter: 'cm_EjectVolume' });
  expect(reg.getCommand('cm_EjectVolume')).toMatchObject({ menu: 'Commands', menuAfter: 'cm_VolumeId' });
  // the entries they go after are in the Commands menu, so they land next to them
  const base = JSON.stringify(menus.MENUS.find(([l]) => l === 'Commands'));
  expect(base).toContain('"cm_GoToRoot"');
  expect(base).toContain('"cm_VolumeId"');
});

test('renaming a volume is not reachable from the AI tools', () => {
  expect(JSON.stringify(allow)).not.toContain('cm_RenameVolume');
});

test('helpers: drives:// paths, renamable volumes, label rules, usage', () => {
  expect(isDrivesPath(DRIVES)).toBe(true);
  expect(isDrivesPath('DRIVES://C:')).toBe(true);
  expect(isDrivesPath('/Volumes/x')).toBe(false);
  expect(canRenameVolume('darwin', { kind: 'removable', path: '/Volumes/USB' })).toBe(true);
  expect(canRenameVolume('darwin', { kind: 'fixed', path: '/' })).toBe(false);
  expect(canRenameVolume('darwin', { kind: 'network', path: '/Volumes/share' })).toBe(false);
  expect(canRenameVolume('darwin', { kind: 'home', path: '/Users/me' })).toBe(false);
  expect(canRenameVolume('darwin', { kind: 'removable', path: '/Volumes/USB', stale: true })).toBe(false);
  expect(canRenameVolume('windows', { kind: 'fixed', path: 'C:\\' })).toBe(true);
  expect(canRenameVolume('linux', { kind: 'removable', path: '/media/usb' })).toBe(false);
  expect(volumeLabelError('Photos')).toBe('');
  for (const bad of ['', '  ', 'a/b', 'C:', 'x'.repeat(33), 'a\tb']) expect(volumeLabelError(bad), bad).not.toBe('');
  expect(usedPercent({ total: 100, free: 25 })).toBe(75);
  expect(usedPercent({ total: 0, free: 0 })).toBe(null);
  expect(usedPercent({ total: 100, free: 0, stale: true })).toBe(null);
});

test('file commands refuse the drive list folder', () => {
  expect(drivesRefusal('drives://')).not.toBe('');
  expect(drivesRefusal('/tmp', 'drives://C:')).not.toBe('');
  expect(drivesRefusal('/tmp', '')).toBe('');
});

function fakeApp(platform, vol) {
  const flashes = [];
  return {
    flashes, platform, demo: false, volumes: [vol], cur: { path: vol.path + '/sub' },
    volumeFor: () => vol, flash: (m) => flashes.push(m), loadVolumes: async () => {},
  };
}

test('cm_RenameVolume asks for the name, then asks again, and only then calls Go', async () => {
  const vol = { name: 'USB', path: '/Volumes/USB', kind: 'removable' };
  const app = fakeApp('darwin', vol);
  calls.length = 0;
  const run = reg.getCommand('cm_RenameVolume').run(app);
  await vi.waitFor(() => expect(dialogs.currentDialog()).toBeTruthy());
  dialogs.answerDialog('Rename', { 'mc-input': 'Stick' });
  await vi.waitFor(() => expect(document.body.textContent).toContain('Rename the volume USB, mounted at /Volumes/USB, to Stick?'));
  expect(calls.filter((c) => c[0] === 'SystemFiles.RenameVolume')).toEqual([]);
  dialogs.answerDialog('Rename');
  expect(await run).toBe(true);
  expect(calls.filter((c) => c[0] === 'SystemFiles.RenameVolume')).toEqual([['SystemFiles.RenameVolume', '/Volumes/USB', 'Stick']]);
});

test('cm_RenameVolume does nothing for a network share or on Linux', async () => {
  calls.length = 0;
  const share = fakeApp('darwin', { name: 'share', path: '/Volumes/share', kind: 'network' });
  expect(await reg.getCommand('cm_RenameVolume').run(share)).toBe(false);
  const lin = fakeApp('linux', { name: 'usb', path: '/media/usb', kind: 'removable' });
  expect(await reg.getCommand('cm_RenameVolume').run(lin)).toBe(false);
  expect(calls.filter((c) => c[0] === 'SystemFiles.RenameVolume')).toEqual([]);
});

test('cm_OpenDrives shows drives:// with the cursor on the drive the panel was on', async () => {
  const gone = [];
  const vol = { name: 'USB', path: '/Volumes/USB', kind: 'removable' };
  const app = { demo: false, cur: { path: '/Volumes/USB/a', go: async (p, o) => { gone.push([p, o]); return true; } }, driveFor: () => vol };
  expect(await reg.getCommand('cm_OpenDrives').run(app)).toBe(true);
  expect(gone).toEqual([['drives://', { focus: 'USB' }]]);
});
