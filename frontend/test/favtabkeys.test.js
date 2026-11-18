// Tests for favtabkeys-lib.js (F-775, board issue #103): a key per favourite tab set, kept as {combo: name}.
import { describe, it, expect } from 'vitest';
import { favKeyFor, favForKey, setFavKey, renameFavKey, dropFavKey } from '../src/features/favtabkeys-lib.js';

describe('favourite tab keys', () => {
  it('gives a favourite one key and a key one favourite', () => {
    let m = setFavKey({}, 'Work', 'Ctrl+Alt+1');
    expect(favKeyFor(m, 'work')).toBe('Ctrl+Alt+1');
    expect(favForKey(m, 'Ctrl+Alt+1')).toBe('Work');
    m = setFavKey(m, 'Work', 'Ctrl+Alt+2');
    expect(m).toEqual({ 'Ctrl+Alt+2': 'Work' });
    m = setFavKey(m, 'Home', 'Ctrl+Alt+2');
    expect(m).toEqual({ 'Ctrl+Alt+2': 'Home' });
  });
  it('removes a key with an empty combo and when the favourite is deleted', () => {
    const m = { 'Ctrl+Alt+1': 'Work', 'Ctrl+Alt+2': 'Home' };
    expect(setFavKey(m, 'Work', '')).toEqual({ 'Ctrl+Alt+2': 'Home' });
    expect(dropFavKey(m, 'home')).toEqual({ 'Ctrl+Alt+1': 'Work' });
    expect(m).toEqual({ 'Ctrl+Alt+1': 'Work', 'Ctrl+Alt+2': 'Home' }); // not changed in place
  });
  it('moves the key along when the favourite is renamed', () => {
    expect(renameFavKey({ 'Ctrl+Alt+1': 'Work' }, 'Work', ' Office ')).toEqual({ 'Ctrl+Alt+1': 'Office' });
    expect(renameFavKey({ 'Ctrl+Alt+1': 'Work' }, 'Other', 'X')).toEqual({ 'Ctrl+Alt+1': 'Work' });
  });
  it('knows no favourite for unknown keys or inherited names', () => {
    expect(favForKey({}, 'toString')).toBe('');
    expect(favForKey(null, 'Ctrl+1')).toBe('');
    expect(favKeyFor(undefined, 'Work')).toBe('');
  });
});
