// Folder paths in popup lists (src/core/popup.js, src/core/fitpath.js, issue #95): the folder history (Alt+Down) used
// to cut long paths at the right edge, so ".../A/s1" and ".../A/s2" looked the same. A path row is now drawn as its
// start (which CSS shortens with "…") and its last name (kept whole), with the full path as a tooltip.
import { describe, expect, test, vi } from 'vitest';

// popup.js reaches the Wails runtime through go.js, whose load-time timer could outlive this quick file's environment;
// a stand-in bridge keeps the runtime out (see fitpath.test.js).
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});
import { splitTail } from '../src/core/fitpath.js';
import { labelHTML } from '../src/core/popup.js';

describe('splitTail', () => {
  test('keeps the separator with the start', () => {
    expect(splitTail('/Users/me/A/s1')).toEqual(['/Users/me/A/', 's1']);
    expect(splitTail('C:\\Data\\A\\s2')).toEqual(['C:\\Data\\A\\', 's2']);
    expect(splitTail('/a')).toEqual(['/', 'a']);
  });
  test('a trailing separator stays with the last name', () => {
    expect(splitTail('C:\\Data\\x\\')).toEqual(['C:\\Data\\', 'x\\']);
    expect(splitTail('/Users/me/')).toEqual(['/Users/', 'me/']);
  });
  test('roots and bare names are all tail', () => {
    expect(splitTail('/')).toEqual(['', '/']);
    expect(splitTail('C:\\')).toEqual(['', 'C:\\']);
    expect(splitTail('name')).toEqual(['', 'name']);
    expect(splitTail('')).toEqual(['', '']);
    expect(splitTail(undefined)).toEqual(['', '']);
  });
  test('mixed separators and URLs', () => {
    expect(splitTail('\\\\server\\share/dir')).toEqual(['\\\\server\\share/', 'dir']);
    expect(splitTail('ftp://host')).toEqual(['ftp://', 'host']);
  });
  test('the two parts always join back to the path', () => {
    for (const p of ['/x/y/z', 'C:\\a\\b\\', '/', 'q', 'sftp://h/a/b/']) expect(splitTail(p).join('')).toBe(p);
  });
});

describe('labelHTML', () => {
  test('plain items are drawn as before', () => {
    expect(labelHTML({ label: 'Clear history' })).toBe('<span>Clear history</span>');
  });
  test('path items are split, escaped', () => {
    expect(labelHTML({ label: '/a/<b>&c', path: true })).toBe('<span class="pl"><span class="ph">/a/</span><span class="pt">&lt;b&gt;&amp;c</span></span>');
  });
});
