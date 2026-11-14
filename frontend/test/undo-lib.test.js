// The words of the undo of file operations (board issue #13): the flash line after an undo and the undo list.
import { expect, test } from 'vitest';
import { undoReport, undoListItems, overwriteNote } from '../src/features/undo-lib.js';

test('a clean undo says what it undid', () => {
  expect(undoReport({ what: 'the rename of a.txt to b.txt', done: 1, jobs: [], problems: [] })).toEqual({ text: 'Undid the rename of a.txt to b.txt.', err: false });
});

test('an undo finished by a job counts the job', () => {
  const r = undoReport({ what: 'the copy of 2 items to /b', done: 0, jobs: [{}], problems: [] }, [{ state: 'done' }]);
  expect(r).toEqual({ text: 'Undid the copy of 2 items to /b.', err: false });
});

test('items left alone and failed jobs are listed, at most three', () => {
  const r = undoReport({ what: 'the move of 5 items to /c', done: 1, problems: ['a is not there any more.', 'b can\'t go back.'] },
    [{ state: 'failed', errors: ['Can\'t move c.', 'Can\'t move d.'] }]);
  expect(r.err).toBe(true);
  expect(r.text).toBe('Undid the move of 5 items to /c.\na is not there any more.\nb can\'t go back.\nCan\'t move c.\n…and 1 more.');
});

test('nothing done and a stopped job', () => {
  const r = undoReport({ what: 'the copy of x to /b', done: 0, problems: [] }, [{ state: 'cancelled' }]);
  expect(r.err).toBe(true);
  expect(r.text).toBe('Nothing of the copy of x to /b could be undone.\nThe undo was stopped before it was finished.');
});

test('the undo list: capitalised phrases, the time as detail, the id as value', () => {
  const items = undoListItems([{ id: 7, what: 'the new folder x', time: 1000 }], (ms) => `t${ms}`);
  expect(items).toEqual([{ label: 'The new folder x', detail: 't1000', value: 7 }]);
  expect(undoListItems(null)).toEqual([]);
});

test('Cmd+Z undoes on macOS only; Ctrl+Z stays TC\'s comment key', async () => {
  const { DEFAULT_KEYS, MAC_KEYS } = await import('../src/core/keymap.js');
  expect(MAC_KEYS['Cmd+Z']).toBe('cm_UndoFileOp');
  expect(DEFAULT_KEYS['Cmd+Z']).toBeUndefined();
  expect(DEFAULT_KEYS['Ctrl+Z']).toBe('cm_EditComment');
});

test('the copy dialog marks the choices that replace files as not undoable, while the journal is on', () => {
  expect(overwriteNote('overwrite')).toBe('Files replaced this way can\'t be undone.');
  expect(overwriteNote('older', true)).toBe('Files replaced this way can\'t be undone.');
  for (const p of ['', 'skip', 'rename', 'rename_target', undefined]) expect(overwriteNote(p)).toBe('');
  expect(overwriteNote('overwrite', false)).toBe('');
});
