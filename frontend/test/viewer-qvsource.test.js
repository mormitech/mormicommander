// Quick view on files inside archives (issue #141): which source quick view uses for the entry under the cursor.
import { describe, it, expect } from 'vitest';
import { quickViewSource, ARCHIVE_PREVIEW_LIMIT } from '../src/features/viewer/qvsource.js';

describe('quickViewSource', () => {
  it('opens local files as they are', () => {
    expect(quickViewSource({ kind: 'local' }, '/A', { name: 'a.txt', size: 5 })).toBe('local');
    expect(quickViewSource(null, '/A', { name: 'a.txt', size: 5 })).toBe('local');
  });

  it('unpacks files inside archives up to the limit', () => {
    const info = { kind: 'archive', format: 'tar.gz' };
    expect(quickViewSource(info, '/A/t.tgz', { name: 'nums.txt', size: 12 })).toBe('archive');
    expect(quickViewSource(info, '/A/t.tgz/sub', { name: 'x', size: ARCHIVE_PREVIEW_LIMIT })).toBe('archive');
    expect(quickViewSource(info, '/A/t.tgz', { name: 'big.bin', size: ARCHIVE_PREVIEW_LIMIT + 1 })).toBe('too-big');
  });

  it('leaves server files and URL places alone, as before (not ready yet)', () => {
    expect(quickViewSource({ kind: 'results' }, 'find://1', { name: 'a.txt', size: 5 })).toBe('remote');
    expect(quickViewSource({ kind: 'ftp', remote: true }, 'ftp://h/x', { name: 'a', size: 1 })).toBe('remote');
    expect(quickViewSource({ kind: 'archive', remote: true }, 'sftp://h/t.zip', { name: 'a', size: 1 })).toBe('remote');
  });
});
