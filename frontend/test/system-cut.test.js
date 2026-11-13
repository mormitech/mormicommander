// Tests of the cut state behind the dimmed rows of cut files (features/system-cut.js): how paths are compared on each
// system, replacing and clearing the cut, and recognising the same cut read back from the clipboard.
import { describe, expect, test } from 'vitest';
import { cutKey, createCutState } from '../src/features/system-cut.js';

describe('cutKey', () => {
  test.each([
    ['/Users/me/a.txt', 'darwin', '/Users/me/a.txt'],
    ['/Users/me/dir/', 'darwin', '/Users/me/dir'],
    ['/', 'linux', '/'],
    ['/Users/Me/A.txt', 'linux', '/Users/Me/A.txt'],
    ['C:\\Users\\Me\\A.TXT', 'windows', 'c:\\users\\me\\a.txt'],
    ['C:\\Dir\\\\', 'windows', 'c:\\dir'],
    ['C:\\', 'windows', 'c:\\'],
    ['', 'darwin', ''],
  ])('%s on %s', (p, plat, want) => expect(cutKey(p, plat)).toBe(want));
});

describe('createCutState', () => {
  test('set, has, list, clear', () => {
    const c = createCutState('darwin');
    expect(c.size).toBe(0);
    expect(c.has('/a')).toBe(false);
    c.set(['/x/a.txt', '/x/dir/', '', null]);
    expect(c.size).toBe(2);
    expect(c.has('/x/a.txt')).toBe(true);
    expect(c.has('/x/dir')).toBe(true);
    expect(c.has('/x/A.txt')).toBe(false);
    expect(c.list()).toEqual(['/x/a.txt', '/x/dir/']);
    c.list().push('/y'); // a copy: the state itself does not change
    expect(c.size).toBe(2);
    c.set(['/y']);
    expect(c.has('/x/a.txt')).toBe(false);
    expect(c.has('/y')).toBe(true);
    c.clear();
    expect(c.size).toBe(0);
    expect(c.list()).toEqual([]);
  });

  test('names that differ only in case are the same file on Windows', () => {
    const c = createCutState('windows');
    c.set(['C:\\Data\\Report.DOCX']);
    expect(c.has('c:\\data\\report.docx')).toBe(true);
  });

  test('the platform can be read late', () => {
    let plat = 'darwin';
    const c = createCutState(() => plat);
    plat = 'windows';
    c.set(['C:\\A']);
    expect(c.has('c:\\a')).toBe(true);
  });

  test('same compares the cut with a list read back from the clipboard', () => {
    const c = createCutState('linux');
    c.set(['/a', '/b']);
    expect(c.same(['/b', '/a/'])).toBe(true);
    expect(c.same(['/a'])).toBe(false);
    expect(c.same(['/a', '/c'])).toBe(false);
    expect(c.same([])).toBe(false);
    expect(c.same(null)).toBe(false);
    c.clear();
    expect(c.same([])).toBe(true);
  });
});
