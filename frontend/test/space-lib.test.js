// Whether Space counts a folder's size (features/tools/space-lib.js, board #2, F-797): a choice stored in
// settings.json wins; without one, the settings file's SpaceCalcSize (a copied wincmd.ini) decides; with neither,
// Space counts as before.
import { describe, expect, test } from 'vitest';
import { countSpaceOn } from '../src/features/tools/space-lib.js';

describe('countSpaceOn', () => {
  test('default: counts when nothing is stored anywhere', () => {
    expect(countSpaceOn(undefined, undefined)).toBe(true);
    expect(countSpaceOn(undefined, '')).toBe(true);
    expect(countSpaceOn(null, '1')).toBe(true);
  });
  test('a copied wincmd.ini with SpaceCalcSize=0 turns counting off from the start', () => {
    expect(countSpaceOn(undefined, '0')).toBe(false);
  });
  test('the choice stored in settings.json wins over the file', () => {
    expect(countSpaceOn(true, '0')).toBe(true);
    expect(countSpaceOn(false, '1')).toBe(false);
    expect(countSpaceOn(false, undefined)).toBe(false);
  });
});
