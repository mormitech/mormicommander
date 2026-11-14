// Tests of the extended-attribute wording (features/tools/xattr-lib.js, board #29): the Properties lines, the
// sentence after "Remove quarantine flag", the one-time copy note and its gate.
import { describe, expect, test } from 'vitest';
import { xattrLines, hasQuarantine, quarantineMessage, xattrLossNote, onceGate, QUARANTINE } from '../src/features/tools/xattr-lib.js';

describe('xattrLines', () => {
  test('known names get words, others stay as they are', () => {
    expect(xattrLines([{ name: QUARANTINE, size: 57 }, { name: 'user.note', size: 1 }])).toEqual([
      'com.apple.quarantine — downloaded from the internet (57 bytes)',
      'user.note (1 byte)',
    ]);
  });
  test('nothing in, nothing out', () => {
    expect(xattrLines(null)).toEqual([]);
    expect(xattrLines([{ size: 3 }])).toEqual([]);
  });
});

test('hasQuarantine', () => {
  expect(hasQuarantine([{ name: 'x' }, { name: QUARANTINE }])).toBe(true);
  expect(hasQuarantine([{ name: 'x' }])).toBe(false);
  expect(hasQuarantine(undefined)).toBe(false);
});

describe('quarantineMessage', () => {
  test('one file', () => {
    expect(quarantineMessage({ checked: 1, removed: 1, failed: [] })).toBe('Removed the quarantine flag.');
    expect(quarantineMessage({ checked: 1, removed: 0, failed: [] })).toBe('It had no quarantine flag.');
  });
  test('a folder', () => {
    expect(quarantineMessage({ checked: 40, removed: 3, failed: [] })).toBe('Removed the quarantine flag from 3 items (of 40 items looked at).');
    expect(quarantineMessage({ checked: 40, removed: 0, failed: [] })).toBe('None of the 40 items had a quarantine flag.');
  });
  test('failures and a stop are said', () => {
    expect(quarantineMessage({ checked: 5, removed: 2, failed: [{}], canceled: true })).toBe('Removed the quarantine flag from 2 items (of 5 items looked at). 1 item kept it. Stopped before the end.');
  });
});

describe('xattrLossNote', () => {
  test('nothing lost, no note', () => {
    expect(xattrLossNote(null)).toBe('');
    expect(xattrLossNote({ loses: false, fileSystem: 'msdos' })).toBe('');
  });
  test('names the file system as people know it and one file', () => {
    const n = xattrLossNote({ loses: true, fileSystem: 'msdos', example: '/Users/a/Photos/tagged.jpg' });
    expect(n).toContain('(FAT)');
    expect(n).toContain('tagged.jpg');
    expect(xattrLossNote({ loses: true, fileSystem: 'exfat' })).toContain('(exFAT)');
    expect(xattrLossNote({ loses: true, fileSystem: 'weirdfs' })).toContain('(weirdfs)');
  });
});

test('onceGate answers true once per key', () => {
  const once = onceGate();
  expect(once('a')).toBe(true);
  expect(once('a')).toBe(false);
  expect(once('b')).toBe(true);
});
