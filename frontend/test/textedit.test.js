// The built-in text editor (F-594, features/textedit-lib.js): when F4 uses it, the status line, and the questions it
// asks; and server editing's question and note (F-593, features/archive-edit-lib.js).
import { describe, expect, test } from 'vitest';
import { wantInternal, lineCol, statusText, closeQuestion, changedQuestion, isChangedError, SAVE, DONT_SAVE, CANCEL, OVERWRITE } from '../src/features/textedit-lib.js';
import { updateQuestion, editNote, panelsShowing, UPDATE, DONT_UPDATE } from '../src/features/archive-edit-lib.js';

describe('wantInternal', () => {
  const on = { 'ViewerEditor.InternalEditor': '1' };
  test('off by default: F4 keeps running the external editor', () => {
    expect(wantInternal({}, '/a/b.txt')).toBe(false);
    expect(wantInternal(undefined, '/a/b.txt')).toBe(false);
    expect(wantInternal({ 'ViewerEditor.InternalEditor': '0' }, '/a/b.txt')).toBe(false);
  });
  test('on: local files only', () => {
    expect(wantInternal(on, '/a/b.txt')).toBe(true);
    expect(wantInternal(on, 'C:\\a\\b.txt')).toBe(true);
    expect(wantInternal(on, 'sftp://h/b.txt')).toBe(false);
    expect(wantInternal(on, '/a/x.zip/b.txt', true)).toBe(false);
    expect(wantInternal(on, '')).toBe(false);
  });
});

describe('status line', () => {
  test('line and column', () => {
    expect(lineCol('ab\ncd', 0)).toEqual({ line: 1, col: 1 });
    expect(lineCol('ab\ncd', 2)).toEqual({ line: 1, col: 3 });
    expect(lineCol('ab\ncd', 3)).toEqual({ line: 2, col: 1 });
    expect(lineCol('ab\ncd', 5)).toEqual({ line: 2, col: 3 });
  });
  test('line ends, BOM, changed', () => {
    expect(statusText({ text: 'x', pos: 0, crlf: true, bom: true, dirty: true })).toBe('Line 1, column 1 · CRLF · UTF-8 with BOM · changed');
    expect(statusText({ text: 'x', pos: 1, crlf: false, bom: false, dirty: false })).toBe('Line 1, column 2 · LF · UTF-8');
  });
});

describe('questions', () => {
  test('closing with changes: Save first, Esc keeps the editor open', () => {
    const q = closeQuestion('a.txt');
    expect(q.Message).toContain('a.txt');
    expect(q.Buttons).toEqual([SAVE, DONT_SAVE, CANCEL]);
    expect(q.CancelButton).toBe(CANCEL);
  });
  test('a file changed on disk: Enter does not overwrite it', () => {
    const q = changedQuestion('a.txt');
    expect(q.Buttons).toContain(OVERWRITE);
    expect(q.DefaultButton).toBe(CANCEL);
    expect(q.CancelButton).toBe(CANCEL);
  });
  test('Go\'s refusal is recognised', () => {
    expect(isChangedError('changed on disk')).toBe(true);
    expect(isChangedError('Error: changed on disk')).toBe(true);
    expect(isChangedError('Can\'t save a.txt: you don\'t have permission.')).toBe(false);
  });
});

describe('editing on a server (F-593)', () => {
  test('the question asks to upload to the server folder', () => {
    const q = updateQuestion({ id: 1, name: 'readme.txt', archive: 'sftp://me@host/pub', remote: true });
    expect(q.Message).toContain('Upload it to sftp://me@host/pub?');
    expect(q.Buttons).toEqual([UPDATE, DONT_UPDATE]);
    expect(q.Kind).toBe('server-edit');
  });
  test('the note after F4', () => {
    expect(editNote({ watching: true, remote: true }, 'r.txt')).toMatch(/copy of r\.txt.*upload it to the server/);
  });
  test('the panels showing the server folder are read again', () => {
    const panels = [{ side: 'l', path: 'sftp://h/pub' }, { side: 'r', path: 'sftp://h/public' }];
    expect(panelsShowing(panels, 'sftp://h/pub').map((p) => p.side)).toEqual(['l']);
  });
});
