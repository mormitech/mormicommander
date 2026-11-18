// The Mark menu (issue #61, FEATURES.md section 13): every entry is in the menu, the ones with a key show it
// (F-342..F-348, F-351), and Select search results (F-353) picks the panel entries the last search found.
import { expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { searchMatches } from '../src/features/find-select-lib.js';

vi.mock('../src/core/registry.js', () => ({ command: () => {}, getCommand: () => null, hasCommand: () => false, commandList: () => [], onChange: () => {} }));
vi.mock('../src/core/app.js', () => ({ app: {}, LABELS: {} }));
vi.mock('../src/core/go.js', () => ({ on: () => {}, go: {} }));

const { MENUS } = await import('../src/core/menus.js');
const src = (f) => readFileSync(resolve(__dirname, '../src/features', f), 'utf8');

test('the Mark menu lists its commands, and the ones with a key show it', () => {
  const mark = MENUS.find(([l]) => l === 'Mark')[1].filter(Array.isArray).map((it) => it[1]);
  for (const n of ['cm_SpreadSelection', 'cm_ShrinkSelection', 'cm_SelectAll', 'cm_ClearAll', 'cm_ExchangeSelection',
    'cm_SelectCurrentExtension', 'cm_RestoreSelection', 'cm_SaveSelection', 'cm_LoadSelection', 'cm_SaveSelectionToFile',
    'cm_LoadSelectionFromFile', 'cm_LoadSelectionFromClip', 'cm_CopyNamesToClip', 'cm_CopyFullNamesToClip', 'cm_CopyDetailsToClip',
    'cm_CompareDirs']) {
    expect(mark).toContain(n);
  }
  const keyed = { 'selection.js': ['cm_SpreadSelection', 'cm_ShrinkSelection', 'cm_SelectAll', 'cm_ClearAll', 'cm_ExchangeSelection',
    'cm_SelectCurrentExtension', 'cm_UnselectCurrentExtension', 'cm_RestoreSelection'], 'comparedirs.js': ['cm_CompareDirs'] };
  for (const [f, names] of Object.entries(keyed)) for (const n of names) expect(src(f)).toContain(`name: '${n}', menuKey: true`);
  expect(src('find.js')).toContain("name: 'cm_SelectSearchResults', label: 'Select search results', menu: 'Mark'");
});

test('searchMatches picks the entries whose full path the search found', () => {
  const entries = [{ name: '..', up: true }, { name: 'a.txt' }, { name: 'B.jpg' }, { name: 'sub', dir: true }, { name: 'c.txt' }];
  const full = (n) => `/d/${n}`;
  const names = (l) => l.map((e) => e.name);
  expect(names(searchMatches(entries, full, ['/d/a.txt', '/d/sub/', '/other/c.txt']))).toEqual(['a.txt', 'sub']);
  expect(names(searchMatches(entries, full, ['/d/b.jpg']))).toEqual([]);
  expect(names(searchMatches(entries, full, ['/d/b.jpg'], true))).toEqual(['B.jpg']);
  expect(searchMatches(entries, full, [])).toEqual([]);
  expect(names(searchMatches([{ name: 'x' }], (n) => `C:\\w\\${n}`, ['c:\\W\\X'], true))).toEqual(['x']);
});
