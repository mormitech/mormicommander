// Editing a file inside an archive (F-592): the question after a save, the note after F4, and which panels are read
// again after the archive was updated. Tests src/features/archive-edit-lib.js.
import { describe, expect, test } from 'vitest';
import { updateQuestion, editNote, panelsShowing, UPDATE, DONT_UPDATE } from '../src/features/archive-edit-lib.js';

describe('updateQuestion', () => {
  test('names the file and the archive, Update first, Esc declines', () => {
    const q = updateQuestion({ id: 3, name: 'notes.txt', archive: '/Users/me/a.zip', entry: '/Users/me/a.zip/docs/notes.txt' });
    expect(q.Message).toContain('notes.txt');
    expect(q.Message).toContain('a.zip');
    expect(q.Message).not.toContain('/Users/me');
    expect(q.Buttons).toEqual([UPDATE, DONT_UPDATE]);
    expect(q.DefaultButton).toBe(UPDATE);
    expect(q.CancelButton).toBe(DONT_UPDATE);
    expect(q.Kind).toBe('archive-edit');
  });
  test('Windows paths', () => {
    expect(updateQuestion({ name: 'x.ini', archive: 'C:\\Data\\b.zip' }).Message).toContain('Update it in b.zip?');
  });
});

describe('editNote', () => {
  test('a read-only archive gives Go\'s reason', () => {
    expect(editNote({ watching: false, note: 'a.7z can\'t be changed.' }, 'f.txt')).toBe('a.7z can\'t be changed.');
  });
  test('a watched copy says saves ask', () => {
    expect(editNote({ watching: true, note: '' }, 'f.txt')).toMatch(/copy of f\.txt.*update the archive/);
  });
  test('a local file (or an old Go answer) says nothing', () => {
    expect(editNote({ watching: false, note: '' }, 'f.txt')).toBe('');
    expect(editNote(null, 'f.txt')).toBe('');
    expect(editNote(undefined, 'f.txt')).toBe('');
  });
});

describe('panelsShowing', () => {
  const panels = [{ side: 'left', path: '/d/a.zip/sub' }, { side: 'right', path: '/d/a.zipper' }, { side: 'x', path: '/d/a.zip' }, { side: 'y', path: '' }];
  test('the archive and folders inside it, not look-alike names', () => {
    expect(panelsShowing(panels, '/d/a.zip').map((p) => p.side)).toEqual(['left', 'x']);
    expect(panelsShowing(panels, '/d/a.zip/').map((p) => p.side)).toEqual(['left', 'x']);
  });
  test('Windows separators and empty input', () => {
    expect(panelsShowing([{ path: 'C:\\d\\a.zip\\in' }], 'C:\\d\\a.zip')).toHaveLength(1);
    expect(panelsShowing(panels, '')).toEqual([]);
    expect(panelsShowing(null, '/d/a.zip')).toEqual([]);
  });
});
