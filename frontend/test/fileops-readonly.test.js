// Archives that can't be changed (board #143): F5/F6 into them, F7 and F8 in them are refused up front with the
// reason Go gives. Tests src/features/fileops-readonly.js.
import { describe, expect, test } from 'vitest';
import { archiveRefusal } from '../src/features/fileops-readonly.js';

describe('archiveRefusal', () => {
  test('a read-only archive gives the reason from Go', () => {
    const why = "s.7z can't be changed: this archive type (7Z) is read-only here.";
    expect(archiveRefusal({ kind: 'archive', format: '7z', readOnly: true, readOnlyWhy: why })).toBe(why);
  });
  test('a read-only archive without a reason still says it', () => {
    expect(archiveRefusal({ kind: 'archive', readOnly: true, nested: true })).toBe("This archive can't be changed.");
  });
  test('writable archives, local folders, servers and no answer are not refused', () => {
    expect(archiveRefusal({ kind: 'archive', format: 'zip', readOnly: false })).toBe('');
    expect(archiveRefusal({ kind: 'local' })).toBe('');
    expect(archiveRefusal({ kind: 'ftp', readOnly: true })).toBe('');
    expect(archiveRefusal(null)).toBe('');
  });
});
