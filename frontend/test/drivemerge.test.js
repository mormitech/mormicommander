// core/drivemerge.js: a share mapped to the drive bar as a network:// button stands for its mounted volume, so the
// share shows one button, not two.
import { expect, test } from 'vitest';
import { mergeMaps, shareOfPath, sameServer } from '../src/core/drivemerge.js';

test('network paths name their server and share', () => {
  expect(shareOfPath('network://DiskStation.local/z')).toEqual({ host: 'DiskStation.local', share: 'z' });
  expect(shareOfPath('smb://pat@nas:445/media/')).toEqual({ host: 'nas', share: 'media' });
  expect(shareOfPath('network://DiskStation')).toBe(null);
  expect(shareOfPath('network://DiskStation/z/sub')).toBe(null);
  expect(shareOfPath('/Users/pat/Code')).toBe(null);
  expect(sameServer('DiskStation', 'diskstation.local')).toBe(true);
  expect(sameServer('DiskStation', 'Other')).toBe(false);
});

test('a mapped share hides its mounted volume and takes over its folder', () => {
  const vols = [
    { name: 'Macintosh HD', path: '/', kind: 'fixed' },
    { name: 'z', path: '/Volumes/DiskStation.local', kind: 'network', host: 'DiskStation.local', share: 'z' },
    { name: 'video', path: '/Volumes/video', kind: 'network', host: 'DiskStation.local', share: 'video' },
  ];
  const maps = [{ name: 'Code', path: '/Users/pat/Code' }, { name: 'z', path: 'network://DiskStation/z' }];
  const out = mergeMaps(vols, maps);
  // the mapped share stands with the network drives, before the local bookmark
  expect(out.filter((v) => !v.hidden).map((v) => `${v.kind}:${v.name}${v.net ? ':net' : ''}`)).toEqual(['fixed:Macintosh HD', 'network:video', 'map:z:net', 'map:Code']);
  expect(out[1]).toMatchObject({ path: '/Volumes/DiskStation.local', hidden: true }); // (still known, for disconnecting)
  expect(out[3].mount).toBe('/Volumes/DiskStation.local');
  expect(out[4].mount).toBeUndefined();
  // a share of another server with the same name stays
  expect(mergeMaps([{ ...vols[1], host: 'other.local' }], maps).map((v) => `${v.kind}${v.hidden ? '-hidden' : ''}`)).toEqual(['network', 'map', 'map']);
  // a mapped share that is not connected is still a network drive button
  expect(mergeMaps([], maps).map((v) => `${v.name}${v.net ? ':net' : ''}${v.mount ? ':mounted' : ''}`)).toEqual(['z:net', 'Code']);
  // without maps nothing changes
  expect(mergeMaps(vols, [])).toEqual(vols);
});

test('the drive bar comes in groups: disks, network drives, mapped folders', () => {
  const vols = [
    { name: 'Macintosh HD', path: '/', kind: 'fixed' }, { name: 'Home', path: '/Users/pat', kind: 'home' },
    { name: 'music', path: '/Volumes/music', kind: 'network', host: 'nas', share: 'music' },
    { name: 'USB', path: '/Volumes/USB', kind: 'removable' },
  ];
  const out = mergeMaps(vols, [{ name: 'Code', path: '/Users/pat/Code' }, { name: 'z', path: 'network://nas/z' }]);
  expect(out.map((v) => v.name)).toEqual(['Macintosh HD', 'Home', 'USB', 'music', 'z', 'Code']);
  // Windows keeps the drive letters in their order
  expect(mergeMaps(vols, [], { keepOrder: true }).map((v) => v.name)).toEqual(['Macintosh HD', 'Home', 'music', 'USB']);
});
