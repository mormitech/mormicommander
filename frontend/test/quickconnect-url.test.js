// Quick connect by URL (board issue #10, F-636): a server URL typed in the path bar or after "cd" on the command line
// reaches the panel unchanged, so the Go side (FS.List → vfs router → the network feature's connection layer) connects
// to it, asking for the host key and the password as needed. These tests use the real app.resolvePath (what both the
// path bar and "cd" call) with the Go side stubbed out.
import { describe, expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});

const { app } = await import('../src/core/app.js');
const { cdArg } = await import('../src/features/cmdline.js');

const urls = [
  'ftp://example.com/pub',
  'ftp://peter@example.com:2121/pub/x',
  'ftpes://peter@example.com/',
  'sftp://bob@host:2222/home/bob',
  'sftp://bob@host/~/docs',
  'scp://bob@host/srv',
  'webdavs://host/remote.php/dav',
];

describe('server URLs in the path bar and after cd', () => {
  for (const platform of ['darwin', 'linux', 'windows']) {
    test(`resolvePath keeps a URL as typed (${platform})`, () => {
      app.platform = platform;
      const from = platform === 'windows' ? 'C:\\Users\\me' : '/Users/me';
      for (const u of urls) expect(app.resolvePath(u, from)).toBe(u);
      expect(app.resolvePath('  sftp://bob@host/x  ', from)).toBe('sftp://bob@host/x');
    });
  }

  test('cd takes the whole URL as its folder', () => {
    for (const u of urls) expect(cdArg(`cd ${u}`)).toBe(u);
  });

  test('a relative name inside a server folder stays on the server', () => {
    app.platform = 'darwin';
    expect(app.resolvePath('docs', 'sftp://bob@host/home/bob')).toBe('sftp://bob@host/home/bob/docs');
    expect(app.resolvePath('pub', 'ftp://example.com/')).toBe('ftp://example.com/pub');
  });
});
