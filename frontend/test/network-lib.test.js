import { expect, test } from 'vitest';
import { networkVolumes, typedAddress, hasShare, isNetworkRoot, typedPath, bannerText } from '../src/features/network-lib.js';

test('network volumes for Disconnect (#148)', () => {
  expect(networkVolumes([{ kind: 'fixed' }, { kind: 'network', name: 'media' }, { kind: 'map' }])).toEqual([{ kind: 'network', name: 'media' }]);
  expect(networkVolumes(null)).toEqual([]);
});

test('typed servers', () => {
  expect(typedAddress('  nas ')).toEqual({ address: 'nas' });
  expect(typedAddress('').error).toBeTruthy();
  expect(typedAddress('ftp://x').error).toMatch(/smb/);
  expect(typedAddress('NFS://srv/export')).toEqual({ address: 'NFS://srv/export' });
  for (const s of ['smb://nas/media', '\\\\nas\\media', '//nas/media', 'nas/media', 'nfs://srv/export/x']) expect(hasShare(s)).toBe(true);
  for (const s of ['smb://nas', 'smb://nas/', '\\\\nas', 'nas', 'nfs://srv']) expect(hasShare(s)).toBe(false);
  expect(typedPath('\\\\nas')).toBe('smb://nas');
  expect(typedPath('//nas')).toBe('smb://nas');
  expect(typedPath('nas')).toBe('smb://nas');
  expect(typedPath('nfs://srv')).toBe('nfs://srv');
});

test('the network folder and its banner', () => {
  expect(isNetworkRoot('network://')).toBe(true);
  expect(isNetworkRoot('network://NAS')).toBe(false);
  expect(bannerText({ searching: true }, 0)).toBe('Searching the local network…');
  expect(bannerText({ searching: true }, 2)).toBe('Searching the local network… 2 found so far.');
  expect(bannerText({ searching: false }, 2)).toMatch(/^Still listening/);
  expect(bannerText({ searching: false }, 0)).toMatch(/^No servers found yet/);
  expect(bannerText({ note: 'avahi-browse is missing' }, 0)).toBe('avahi-browse is missing');
});
