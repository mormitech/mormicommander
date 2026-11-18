// @vitest-environment happy-dom
// The Show menu (issue #64, docs/FEATURES.md section 16): every entry names a registered command, the entries with a key
// show it (menuKey), Show only selected / Hide selected files (F-408) filter the panel until a reread, Active panel alone
// (F-411) sits after Panels on top of each other, and the saved view modes become a View mode submenu (F-819). Go is a
// stand-in that answers nothing; the timers are frozen so the menus are never sent anywhere.
import { beforeAll, expect, test, vi } from 'vitest';
import { selectionFilter } from '../src/features/reserved-lib.js';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus, keymap, views, Panel;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  keymap = await import('../src/core/keymap.js');
  views = await import('../src/features/views.js');
  ({ Panel } = await import('../src/core/panel.js'));
  vi.useRealTimers();
});

const flat = (items) => items.flatMap((it) => (Array.isArray(it) ? (Array.isArray(it[1]) ? flat(it[1]) : [it[1]]) : []));
const showMenu = () => flat(menus.MENUS.find(([l]) => l === 'Show')[1]);

test('every entry of the Show menu runs a registered command', () => {
  expect(showMenu().filter((n) => !reg.hasCommand(n))).toEqual([]);
});

test('the Show menu entries with a key show it (F-393..F-407)', () => {
  const keys = {
    cm_SrcShort: 'Ctrl+F1', cm_SrcLong: 'Ctrl+F2', cm_SrcComments: 'Ctrl+Shift+F2', cm_SrcThumbs: 'Ctrl+Shift+F1', cm_SrcCustomViewMenu: 'Shift+F1',
    cm_SrcTree: 'Ctrl+F8', cm_SrcQuickView: 'Ctrl+Q', cm_SrcAllFiles: 'Ctrl+F10', cm_SrcExecs: 'Ctrl+F11', cm_SrcUserSpec: 'Ctrl+F12',
    cm_SrcByName: 'Ctrl+F3', cm_SrcByExt: 'Ctrl+F4', cm_SrcByDateTime: 'Ctrl+F5', cm_SrcBySize: 'Ctrl+F6', cm_SrcUnsorted: 'Ctrl+F7',
    cm_RereadSource: 'Ctrl+R', cm_SwitchHidSys: 'Ctrl+H', cm_HorizontalPanels: 'Ctrl+Shift+H',
  };
  for (const [n, k] of Object.entries(keys)) {
    expect(showMenu(), n).toContain(n);
    expect(reg.getCommand(n).menuKey, n).toBe(true);
    expect(keymap.DEFAULT_KEYS[k], k).toBe(n);
  }
  for (const n of ['cm_SrcUserDef', 'cm_SrcNegOrder']) expect(showMenu(), n).toContain(n);
});

test('the commands that add themselves to the Show menu sit where they belong (F-408, F-411)', () => {
  expect(reg.getCommand('cm_ShowOnlySelected')).toMatchObject({ menu: 'Show', menuAfter: 'cm_SrcUserDef' });
  expect(reg.getCommand('cm_HideSelected')).toMatchObject({ menu: 'Show', menuAfter: 'cm_ShowOnlySelected' });
  expect(reg.getCommand('cm_100Percent')).toMatchObject({ menu: 'Show', menuAfter: 'cm_HorizontalPanels' });
});

test('selectionFilter keeps only the names given, or all but them, on top of the earlier filter', () => {
  const es = ['a.txt', 'b.txt', 'c.md', 'sub'].map((name) => ({ name }));
  const names = (f) => es.filter(f).map((e) => e.name);
  expect(names(selectionFilter(['a.txt', 'c.md'], true))).toEqual(['a.txt', 'c.md']);
  expect(names(selectionFilter(['a.txt', 'c.md'], false))).toEqual(['b.txt', 'sub']);
  const txt = (e) => e.name.endsWith('.txt');
  const f = selectionFilter(['a.txt'], false, txt, '*.txt');
  expect(names(f)).toEqual(['b.txt']);
  expect({ ...f }).toEqual({ mcReread: true, mcSrc: 'hidesel', mcPrev: txt, mcPrevLabel: '*.txt' });
});

test('Show only selected filters the active panel; with nothing selected it says so', () => {
  const flash = vi.fn();
  const p = { selected: new Set(), filter: null, filterLabel: '', setFilter: vi.fn() };
  const app = { cur: p, flash };
  expect(reg.getCommand('cm_ShowOnlySelected').run(app)).toBe(false);
  expect(reg.getCommand('cm_HideSelected').run(app)).toBe(false);
  expect(flash).toHaveBeenCalledTimes(2);
  expect(p.setFilter).not.toHaveBeenCalled();
  p.selected = new Set(['a.txt']);
  expect(reg.getCommand('cm_ShowOnlySelected').run(app)).toBe(true);
  const [fn, label] = p.setFilter.mock.calls[0];
  expect(label).toBe('Only selected');
  expect(fn({ name: 'a.txt' })).toBe(true);
  expect(fn({ name: 'b.txt' })).toBe(false);
});

test('a reread drops Show only selected / Hide selected and puts the earlier filter back', async () => {
  const txt = Object.assign((e) => e.name.endsWith('.txt'), {});
  const inner = selectionFilter(['a.txt'], true, txt, '*.txt');
  const outer = selectionFilter(['b.txt'], false, inner, 'Only selected');
  const p = { path: '/d', filter: outer, filterLabel: 'Selected hidden', go: vi.fn(async () => true) };
  await Panel.prototype.reload.call(p);
  expect(p.filter).toBe(txt);
  expect(p.filterLabel).toBe('*.txt');
  p.filter = selectionFilter(['a'], true);
  p.filterLabel = 'Only selected';
  await Panel.prototype.reload.call(p);
  expect(p.filter).toBe(null);
  expect(p.filterLabel).toBe('');
  expect(p.go).toHaveBeenCalledWith('/d', { history: false, keepSelection: true });
});

test('the saved view modes become a View mode submenu of the first nine (F-819)', () => {
  expect(views.viewModeMenu([])).toEqual([]);
  const modes = Array.from({ length: 11 }, (_, i) => ({ name: `m${i + 1}` }));
  const [sep, [label, items]] = views.viewModeMenu(modes);
  expect(sep).toBe('-');
  expect(label).toBe('View mode');
  expect(items.length).toBe(9);
  expect(items[0]).toEqual(['m1', 'cm_SrcViewMode1']);
  for (const [, c] of items) expect(reg.hasCommand(c), c).toBe(true);
});

test('a view mode number that is not saved says so', () => {
  const flash = vi.fn();
  const app = { cur: {}, flash, setActive: vi.fn() };
  expect(reg.getCommand('cm_SrcViewMode3').run(app)).toBe(false);
  expect(flash).toHaveBeenCalledOnce();
  expect(app.setActive).not.toHaveBeenCalled();
});
