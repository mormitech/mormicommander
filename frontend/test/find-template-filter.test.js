// @vitest-environment happy-dom
// Find files (issue #72, FEATURES.md section 24): the dialog is in the Commands menu with its key (Alt+F7), and a saved
// search works as a filter (F-537): ">name" in Select files (Num +) and in the custom filter (Ctrl+F12), unless a saved
// filter of Select files has that name. Go is a stand-in: Find.TemplateNames lists "Big" and Find.TemplateMatch answers
// the indexes of the names ending in ".log".
import { beforeAll, expect, test, vi } from 'vitest';
import { templateName, kindIndexes } from '../src/features/find-select-lib.js';

const calls = [];
vi.mock('../src/core/go.js', () => {
  const find = {
    TemplateNames: async () => ['Big'],
    TemplateMatch: async (name, dir, names) => { calls.push(['match', name, dir, names]); return names.flatMap((n, i) => (n.endsWith('.log') ? [i] : [])); },
  };
  const select = { Group: async (rows, mask) => { calls.push(['group', mask]); return []; }, Check: async () => '' };
  const any = new Proxy({}, { get: () => () => Promise.resolve(null) });
  const go = new Proxy({}, { get: (_, k) => (k === 'Find' ? find : k === 'Select' ? select : any) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus, keymap;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  keymap = await import('../src/core/keymap.js');
  vi.useRealTimers();
});

const flat = (items) => items.flatMap((it) => (Array.isArray(it) ? (Array.isArray(it[1]) ? flat(it[1]) : [it[1]]) : []));

test('Find files is in the Commands menu with its key, and the separate window next to it (F-524, F-543)', () => {
  const cmds = flat(menus.MENUS.find(([l]) => l === 'Commands')[1]);
  expect(cmds).toContain('cm_SearchFor');
  expect(cmds).toContain('cm_SearchStandalone');
  expect(reg.getCommand('cm_SearchFor').menuKey).toBe(true);
  expect(keymap.DEFAULT_KEYS['Alt+F7']).toBe('cm_SearchFor');
  expect(reg.hasCommand('cm_SearchStandalone')).toBe(true);
});

test('templateName names a saved search only when no saved filter has that name', () => {
  expect(templateName('>big', [], ['Big', 'Old'])).toBe('Big');
  expect(templateName('  > Old  ', [], ['Big', 'Old'])).toBe('Old');
  expect(templateName('>Big', ['BIG'], ['Big'])).toBe('');
  expect(templateName('*.txt', [], ['Big'])).toBe('');
  expect(templateName('>none', [], ['Big'])).toBe('');
  expect(templateName('', [], ['Big'])).toBe('');
});

test('kindIndexes keeps the rows of the group kind and never ".."', () => {
  const es = [{ name: '..', up: true }, { name: 'a' }, { name: 'd', dir: true }];
  expect(kindIndexes(es, [0, 1, 2], 'files')).toEqual([1]);
  expect(kindIndexes(es, [0, 1, 2], 'folders')).toEqual([2]);
  expect(kindIndexes(es, [0, 1, 2], 'both')).toEqual([1, 2]);
});

function fakeApp(filters = {}) {
  const entries = [{ name: '..', up: true }, { name: 'a.log' }, { name: 'b.txt' }, { name: 'logs.log', dir: true }, { name: 'c.log' }];
  const settings = { selectFilters: filters };
  const p = {
    path: '/data', entries, all: entries, selected: new Set(), filter: null, filterLabel: '',
    select(e, on) { if (on) this.selected.add(e.name); else this.selected.delete(e.name); },
    selectionChanged() {}, setFilter(fn, label) { this.filter = fn; this.filterLabel = label; },
  };
  return { cur: p, left: p, right: p, other: p, platform: 'darwin', setting: (k, d) => settings[k] ?? d, setSetting: (k, v) => { settings[k] = v; }, flash: () => {} };
}

test('Select files with ">name" of a saved search selects what the search would find (F-537)', async () => {
  calls.length = 0;
  const app = fakeApp();
  expect(await reg.getCommand('cm_SpreadSelection').run(app, { mask: '>big' })).toBe(true);
  expect([...app.cur.selected].sort()).toEqual(['a.log', 'c.log']); // the folder "logs.log" is left alone: files only
  expect(calls[0]).toEqual(['match', 'Big', '/data', ['', 'a.log', 'b.txt', 'logs.log', 'c.log']]);
});

test('a saved filter of the same name keeps its meaning in Select files', async () => {
  calls.length = 0;
  const app = fakeApp({ Big: '*.txt' });
  await reg.getCommand('cm_SpreadSelection').run(app, { mask: '>Big' });
  expect(calls).toEqual([['group', '>Big']]);
});

test('the custom filter with ">name" shows the files the saved search finds, and every folder', async () => {
  const app = fakeApp();
  expect(await reg.getCommand('cm_SrcUserDef').run(app, { mask: '>Big' })).toBe(true);
  const shown = app.cur.entries.filter(app.cur.filter).map((e) => e.name);
  expect(shown).toEqual(['..', 'a.log', 'logs.log', 'c.log']);
  expect(app.cur.filterLabel).toBe('>Big');
  // a plain mask still works as before
  await reg.getCommand('cm_SrcUserDef').run(app, { mask: '*.txt' });
  expect(app.cur.entries.filter(app.cur.filter).map((e) => e.name)).toEqual(['..', 'b.txt', 'logs.log']);
});
