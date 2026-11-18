// Synchronize folders' pure parts (src/features/syncdirs-lib.js): the notes about archive and server sides (F-555).
import { expect, test } from 'vitest';
import { isVirtual, readOnlyNote, dialogNotes, contentTip, fileSide, pathsWithAction } from '../src/features/syncdirs-lib.js';

const local = { kind: 'local', trash: true, times: true };
const rar = { kind: 'archive', readOnly: true, why: 'x.rar can\'t be changed: this archive type (RAR) is read-only here.' };
const sftp = { kind: 'sftp', times: true };
const dav = { kind: 'other', times: false };

test('only archives and servers are virtual sides', () => {
  expect(isVirtual(local)).toBe(false);
  expect(isVirtual(sftp)).toBe(true);
  expect(isVirtual(undefined)).toBe(false);
});

test('a read-only side is named with its reason; local pairs say nothing', () => {
  expect(readOnlyNote({ left: local, right: local })).toBe('');
  expect(readOnlyNote(undefined)).toBe('');
  const n = readOnlyNote({ left: local, right: rar });
  expect(n).toContain('right side can\'t be changed');
  expect(n).toContain('(RAR)');
});

test('the dialog warns about permanent deletes and lost dates only where they happen', () => {
  expect(dialogNotes({ deletesLeft: 3, filesRightToLeft: 2 }, { left: local, right: sftp }, true)).toEqual([]);
  expect(dialogNotes({ deletesRight: 1 }, { left: local, right: sftp }, true)).toEqual(['Deleting on the right side (an SFTP server) is permanent: it has no trash.']);
  expect(dialogNotes({ deletesRight: 1 }, { left: local, right: sftp }, false)).toEqual([]);
  expect(dialogNotes({ filesLeftToRight: 1 }, { left: local, right: dav }, true)[0]).toContain('can\'t set file dates');
  expect(dialogNotes({ filesLeftToRight: 1 }, { left: local, right: sftp }, true)).toEqual([]);
  expect(dialogNotes({ filesRightToLeft: 1 }, { left: local, right: rar }, true)).toEqual([`The right side is read-only: ${rar.why}`]);
  expect(dialogNotes({}, undefined, true)).toEqual([]);
});

test('the by-content tip mentions downloads for server addresses', () => {
  expect(contentTip('/a', '/b')).toBe('Compare files of the same size byte by byte');
  expect(contentTip('/a', 'sftp://me@host/x')).toContain('downloaded');
  expect(contentTip('/a/b.zip/c', '/b')).not.toContain('downloaded');
});

test('F3 / F4 open the asked side, the only file of a single, and never a folder or link', () => {
  const f = { kind: 'file' };
  expect(fileSide({ left: f, right: f }, 'left')).toBe('left');
  expect(fileSide({ left: f, right: f }, 'right')).toBe('right');
  expect(fileSide({ left: f }, 'right')).toBe('left');
  expect(fileSide({ right: f }, 'left')).toBe('right');
  expect(fileSide({ left: { kind: 'dir' }, right: f }, 'left')).toBe('');
  expect(fileSide({ left: { kind: 'link' } }, 'left')).toBe('');
  expect(fileSide({ isDir: true, left: { kind: 'dir' } }, 'left')).toBe('');
  expect(fileSide(null, 'left')).toBe('');
});

test('rows are selected by copy direction', () => {
  const rows = [{ path: 'a', action: 'copyLeftToRight' }, { path: 'b', action: 'copyRightToLeft' }, { path: 'c', action: 'deleteRight' }, { path: 'd', action: 'none' }];
  expect(pathsWithAction(rows, 'lr')).toEqual(['a']);
  expect(pathsWithAction(rows, 'rl')).toEqual(['b']);
  expect(pathsWithAction(rows, 'copy')).toEqual(['a', 'b']);
  expect(pathsWithAction(rows, 'x')).toEqual([]);
  expect(pathsWithAction(undefined, 'lr')).toEqual([]);
});
