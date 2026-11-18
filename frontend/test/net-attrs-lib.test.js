// Server attributes and web downloads (src/features/net-attrs-lib.js, issue #78: F-633, F-649, F-644): permission
// numbers, the mode of a panel entry, the ls-style text, and what quick connect treats as a web file.
import { expect, test } from 'vitest';
import { parseOctal, octalText, bitsOfEntry, permString, isWebURL, webFileName, PERM_ROWS, SPECIAL_BITS } from '../src/features/net-attrs-lib.js';

test('octal numbers are read and written', () => {
  expect(parseOctal('644')).toBe(0o644);
  expect(parseOctal(' 0755 ')).toBe(0o755);
  expect(parseOctal('4755')).toBe(0o4755);
  for (const bad of ['', '8', '12345', 'rwx', null, undefined]) expect(parseOctal(bad)).toBe(null);
  expect(octalText(0o644)).toBe('0644');
  expect(octalText(0o4755)).toBe('4755');
});

test('the mode of a panel entry (Go fs.FileMode) gives the Unix bits', () => {
  expect(bitsOfEntry(0)).toBe(null);
  expect(bitsOfEntry(0o644)).toBe(0o644);
  expect(bitsOfEntry((0x80000000 | 0o755) >>> 0)).toBe(0o755); // a folder
  expect(bitsOfEntry(0x800000 | 0o755)).toBe(0o4755); // set-user-ID
  expect(bitsOfEntry(0x400000 | 0x100000 | 0o775)).toBe(0o3775); // set-group-ID and sticky
});

test('permissions read like ls', () => {
  expect(permString(0o644)).toBe('rw-r--r--');
  expect(permString(0o755)).toBe('rwxr-xr-x');
  expect(permString(0o4755)).toBe('rwsr-xr-x');
  expect(permString(0o1777)).toBe('rwxrwxrwt');
  expect(permString(0o2640)).toBe('rw-r-S---');
  const all = [...PERM_ROWS.flatMap(([, b]) => b), ...SPECIAL_BITS.map(([b]) => b)].reduce((a, b) => a | b, 0);
  expect(all).toBe(0o7777);
});

test('web addresses are downloaded under the last part of their path', () => {
  expect(isWebURL('https://example.com/a.zip')).toBe(true);
  expect(isWebURL(' HTTP://host/')).toBe(true);
  expect(isWebURL('ftp://host/a')).toBe(false);
  expect(isWebURL('http://')).toBe(false);
  expect(webFileName('https://example.com/dl/file.tar.gz?x=1')).toBe('file.tar.gz');
  expect(webFileName('https://example.com/')).toBe('index.html');
  expect(webFileName('https://example.com/%E2%82%AC.txt')).toBe('€.txt');
  expect(webFileName('not a url')).toBe('index.html');
});
