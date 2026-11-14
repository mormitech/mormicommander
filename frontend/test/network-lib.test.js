import { expect, test } from 'vitest';
import { serverItems, shareItems, networkVolumes, typedAddress, hasShare, shareAddress } from '../src/features/network-lib.js';

test('server and share lines (#148)', () => {
  const [a, b] = serverItems([
    { name: 'NAS', host: 'nas.local', kind: 'smb', address: 'smb://nas.local', source: 'recent', user: 'pat', saved: true },
    { name: '', host: 'srv', kind: 'nfs', address: 'nfs://srv', source: 'avahi' },
  ]);
  expect(a.label).toBe('NAS');
  expect(a.detail).toBe('SMB · nas.local · as pat (password saved) · used before');
  expect(b.label).toBe('srv');
  expect(b.detail).toBe('NFS · srv · found');
  expect(shareItems([{ name: '/export', comment: '(everyone)' }])[0]).toMatchObject({ label: '/export', detail: '(everyone)' });
});

test('network volumes for Disconnect', () => {
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
});

test('share addresses', () => {
  expect(shareAddress({ address: 'smb://nas.local' }, 'my media')).toBe('smb://nas.local/my%20media');
  expect(shareAddress({ address: 'nfs://srv/' }, '/volume1/media')).toBe('nfs://srv/volume1/media');
});
