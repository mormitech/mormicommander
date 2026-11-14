// Shift+F5 on a folder with the name unchanged (board #133): the copy dialog must read "dx" as "the same name in the
// same folder", so copyOrMove() gives the copy a free name (dx(2)) instead of refusing a copy of the folder into
// itself. Tests src/features/fileops-target.js.
import { describe, expect, test } from 'vitest';
import { sameNameTarget } from '../src/features/fileops-target.js';

const src = '/tmp/p';
const parsedInto = { dir: '/tmp/p/dx', rename: '', dirExists: true, sameVolume: true }; // what ParseTarget answers for "dx"
const dx = { name: 'dx', base: 'dx', ext: '', dir: true };

describe('sameNameTarget', () => {
  test('one folder, name unchanged: the source folder with the same name', () => {
    expect(sameNameTarget({ same: true, items: [dx], text: 'dx', srcPath: src, t: parsedInto }))
      .toEqual({ dir: src, rename: 'dx', dirExists: true, sameVolume: true });
  });
  test('a separator at the end still means into the folder', () => {
    expect(sameNameTarget({ same: true, items: [dx], text: 'dx/', srcPath: src, t: parsedInto })).toBe(parsedInto);
  });
  test('a typed different name is left to ParseTarget', () => {
    const t = { dir: src, rename: 'dy', dirExists: true, sameVolume: true };
    expect(sameNameTarget({ same: true, items: [dx], text: 'dy', srcPath: src, t })).toBe(t);
  });
  test('F5/F6 to the other panel are untouched', () => {
    expect(sameNameTarget({ same: false, items: [dx], text: 'dx', srcPath: src, t: parsedInto })).toBe(parsedInto);
  });
  test('files and several items are untouched', () => {
    const f = { name: 'dx', base: 'dx', ext: '', dir: false };
    expect(sameNameTarget({ same: true, items: [f], text: 'dx', srcPath: src, t: parsedInto })).toBe(parsedInto);
    expect(sameNameTarget({ same: true, items: [dx, f], text: 'dx', srcPath: src, t: parsedInto })).toBe(parsedInto);
  });
});
