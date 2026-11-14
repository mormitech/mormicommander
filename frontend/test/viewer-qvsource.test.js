// Quick view and F3 on files inside archives (issue #141) and on servers (issue #101): which source quick view uses for
// the entry under the cursor, and when F3 views a temporary copy.
import { describe, it, expect } from 'vitest';
import { quickViewSource, viewsFromCopy, downloads, ARCHIVE_PREVIEW_LIMIT, SERVER_PREVIEW_LIMIT } from '../src/features/viewer/qvsource.js';

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

  it('leaves fed search results alone, as before (not ready yet)', () => {
    expect(quickViewSource({ kind: 'results' }, 'find://1', { name: 'a.txt', size: 5 })).toBe('remote');
    expect(quickViewSource(null, 'find://1', { name: 'a.txt', size: 5 })).toBe('remote');
  });

  it('downloads server files (FTP, SFTP, WebDAV) up to the server limit', () => {
    expect(quickViewSource({ kind: 'ftp', remote: true }, 'ftp://h/x', { name: 'a', size: 1 })).toBe('server');
    expect(quickViewSource({ kind: 'sftp', remote: true }, 'sftp://u@h/x', { name: 'a', size: SERVER_PREVIEW_LIMIT })).toBe('server');
    expect(quickViewSource({ kind: 'other', remote: true }, 'webdav://h/x', { name: 'a', size: SERVER_PREVIEW_LIMIT + 1 })).toBe('server-too-big');
    // an archive on a server counts as the server (its entries come over the network)
    expect(quickViewSource({ kind: 'archive', remote: true }, 'sftp://h/t.zip', { name: 'a', size: 1 })).toBe('server');
    // with no info at all, the URL decides
    expect(quickViewSource(null, 'webdavs://h/pub', { name: 'a', size: 3 })).toBe('server');
    expect(SERVER_PREVIEW_LIMIT).toBeLessThan(ARCHIVE_PREVIEW_LIMIT);
  });
});

describe('viewsFromCopy', () => {
  it('copies files inside archives and on servers', () => {
    expect(viewsFromCopy({ kind: 'archive' }, '/A/t.zip')).toBe(true);
    expect(viewsFromCopy({ kind: 'ftp', remote: true }, 'ftp://h/x')).toBe(true);
    expect(viewsFromCopy({ kind: 'other', remote: true }, 'webdav://h/x')).toBe(true);
    expect(viewsFromCopy(undefined, 'sftp://u@h/x')).toBe(true);
  });

  it('opens local files as before', () => {
    expect(viewsFromCopy({ kind: 'local' }, '/A')).toBe(false);
    expect(viewsFromCopy(undefined, '/A')).toBe(false);
    expect(viewsFromCopy(undefined, 'C:\\A')).toBe(false);
  });

  it('sends fed search results through LocalCopy, like the F3 key, without saying it downloads', () => {
    expect(viewsFromCopy({ kind: 'results' }, 'find://1')).toBe(true);
    expect(downloads({ kind: 'results' }, 'find://1')).toBe(false);
    expect(downloads(undefined, 'find://1')).toBe(false);
    expect(downloads({ kind: 'archive' }, '/A/t.zip')).toBe(false);
    expect(downloads({ kind: 'ftp', remote: true }, 'ftp://h/x')).toBe(true);
    expect(downloads(undefined, 'webdav://h/x')).toBe(true);
  });
});
