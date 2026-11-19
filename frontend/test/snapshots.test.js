// @vitest-environment happy-dom
// Versions from snapshots (board #34, F-874): which paths are inside a snapshot (refused by the file commands), the
// rows of the version list, how a chosen version opens in the other panel, and that the command is in the Files menu
// but not reachable from the AI tools. Go is a stand-in that answers nothing.
import { beforeAll, expect, test, vi } from 'vitest';
import { inSnapshot, snapshotRefusal, versionRows, openTarget, kindLabel } from '../src/features/snapshots-lib.js';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});

let reg;
beforeAll(async () => {
  await import('../src/features/snapshots.js');
  reg = await import('../src/core/registry.js');
});

test('paths inside a snapshot', () => {
  for (const p of ['/tank/.zfs/snapshot/daily', '/tank/.zfs/snapshot/daily/a.txt', '/.snapshots/12/snapshot', '/home/.snapshots/3/snapshot/me',
    '\\\\?\\GLOBALROOT\\Device\\HarddiskVolumeShadowCopy3\\Users', '/Volumes/.timemachine/X/2024.backup/a',
    '/Volumes/com.apple.TimeMachine.localsnapshots/Backups.backupdb/Mac']) expect(inSnapshot(p), p).toBe(true);
  for (const p of ['/tank/.zfs/snapshot', '/.snapshots/12', '/.snapshots/x/snapshot', '/home/me/snapshot', 'C:\\Users', '', null, '/Volumes/Data'])
    expect(inSnapshot(p), String(p)).toBe(false);
});

test('snapshotRefusal says why, and nothing outside snapshots', () => {
  expect(snapshotRefusal('/home/me', '/tank/.zfs/snapshot/d/x')).toMatch(/can't be changed/);
  expect(snapshotRefusal('/home/me', '')).toBe('');
});

test('version rows: date, kind, state; only openable ones are enabled', () => {
  const rows = versionRows([
    { kind: 'zfs', name: 'daily', when: 1000, path: '/t/.zfs/snapshot/daily/a', size: 2048 },
    { kind: 'btrfs', name: '4', when: 900, path: '/.snapshots/4/snapshot/a', missing: true, info: 'timeline' },
    { kind: 'apfs', name: 'com.apple.TimeMachine.x.local', when: 0, path: '', why: 'Not mounted.' },
    { kind: 'vss', name: 'HarddiskVolumeShadowCopy1', when: 5, path: 'p', dir: true },
  ], (ms) => `D${ms}`, (n) => `${n}B`);
  expect(rows.map((r) => r.disabled)).toEqual([false, true, true, false]);
  expect(rows[0].label).toBe('D1000  ·  ZFS  ·  daily  ·  2048B');
  expect(rows[1].label).toContain('Btrfs  ·  4 (timeline)  ·  not in this snapshot');
  expect(rows[2].label).toContain('date not known');
  expect(rows[2].label).toContain('not mounted');
  expect(rows[2].title).toBe('Not mounted.');
  expect(rows[3].label).toContain('Shadow copy');
  expect(rows[3].label).toContain('folder');
  expect(kindLabel('timemachine')).toBe('Time Machine');
});

test('a folder opens itself, a file its folder with the cursor on it', () => {
  expect(openTarget({ dir: true, path: '/t/.zfs/snapshot/d/me' })).toEqual({ dir: '/t/.zfs/snapshot/d/me', focus: null });
  expect(openTarget({ path: '/t/.zfs/snapshot/d/me/a.txt' })).toEqual({ dir: '/t/.zfs/snapshot/d/me', focus: 'a.txt' });
  expect(openTarget({ path: '\\\\?\\GLOBALROOT\\Device\\HarddiskVolumeShadowCopy1\\Users\\a.txt' }, '\\'))
    .toEqual({ dir: '\\\\?\\GLOBALROOT\\Device\\HarddiskVolumeShadowCopy1\\Users', focus: 'a.txt' });
});

test('cm_SnapshotVersions is in the Files menu and not in the AI tools', async () => {
  const c = reg.getCommand('cm_SnapshotVersions');
  expect(c?.menu).toBe('Files');
  const allow = await import('../src/features/aitools-allow.js');
  expect(allow.commandVerdict('cm_SnapshotVersions').ok).toBe(false);
});
