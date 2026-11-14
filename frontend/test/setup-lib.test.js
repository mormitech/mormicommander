// Tests of the "Coming from Marta" box of the first-run setup page (features/setup-lib.js, board issue #22): it shows
// only when Marta's folder was found, offers the import only when there are favourites, and says what an import did.
import { expect, test } from 'vitest';
import { martaOffer, martaDone } from '../src/features/setup-lib.js';

test('the box is hidden without Marta', () => {
  expect(martaOffer(null).show).toBe(false);
  expect(martaOffer({ found: false, favorites: [] }).show).toBe(false);
});

test('Marta with favourites offers the import and names a few of them', () => {
  const favorites = [
    { name: 'Code', path: '/Users/me/Code' }, { name: '', path: '/var/log' }, { name: 'A', path: '/a' },
    { name: 'B', path: '/b' }, { name: 'C', path: '/c' }, { name: '', path: '/' },
  ];
  const o = martaOffer({ found: true, favorites });
  expect(o.show).toBe(true);
  expect(o.canImport).toBe(true);
  expect(o.text).toMatch(/6 favourite folders \(Code, log, A, B, and 2 more\)/);
  expect(o.text).toMatch(/never changed/);
  expect(martaOffer({ found: true, favorites: [{ name: '', path: '/' }] }).text).toMatch(/1 favourite folder \(\/\)/);
});

test('Marta without favourites, or unreadable, shows the box without the button', () => {
  const none = martaOffer({ found: true, favorites: [] });
  expect(none.show).toBe(true);
  expect(none.canImport).toBe(false);
  const broken = martaOffer({ found: true, favorites: [], error: 'Marta\'s favourites could not be read: denied' });
  expect(broken.canImport).toBe(false);
  expect(broken.text).toMatch(/denied/);
});

test('the sentence after the import', () => {
  expect(martaDone(3, 3)).toBe('Added 3 Marta favourites to the folder hotlist (Ctrl+D > Marta).');
  expect(martaDone(1, 3)).toBe('Added 1 Marta favourite to the folder hotlist (Ctrl+D > Marta); 2 were already there.');
  expect(martaDone(2, 3)).toMatch(/1 was already there/);
  expect(martaDone(0, 3)).toMatch(/already in the folder hotlist/);
  expect(martaDone(0, 0)).toBe('Nothing to import.');
});
