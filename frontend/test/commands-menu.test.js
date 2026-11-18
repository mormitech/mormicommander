// @vitest-environment happy-dom
// The Commands menu (issue #62, docs/FEATURES.md section 14). Every entry of the menu names a command that exists once
// all features are loaded, the entries with a key show it (menuKey, F-354..F-371), Refresh folder tree (cm_RescanTree,
// F-369) sits after Folder tree, and every cm_* command docs/FEATURES.md names is in the registry, which is the list
// All commands (cm_CommandBrowser, F-373/F-374) shows. Go is a stand-in that answers nothing; the timers are frozen so
// the menus are never sent anywhere. The tree merge itself is tested in views-tree.test.js.
import { beforeAll, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  vi.useRealTimers();
});

// (the listed entries; a command that adds itself with menu: 'Commands' is checked by its own fields)
const commandsMenu = () => menus.MENUS.find(([l]) => l === 'Commands')[1].filter(Array.isArray).map((it) => it[1]);

test('every entry of the Commands menu runs a registered command', () => {
  const missing = commandsMenu().filter((n) => typeof n === 'string' && !reg.hasCommand(n));
  expect(missing).toEqual([]);
});

test('the Commands menu entries with a key show it', () => {
  for (const n of ['cm_DirectoryHotlist', 'cm_DirectoryHistory', 'cm_CDtree', 'cm_SearchFor', 'cm_Exchange', 'cm_ExchangeWithTabs',
    'cm_TransferLeft', 'cm_TransferRight', 'cm_DirBranch', 'cm_DirBranchSel', 'cm_ExecuteDOS', 'cm_GoToRoot', 'cm_FocusCmdLine']) {
    expect(commandsMenu(), n).toContain(n);
    expect(reg.getCommand(n).menuKey, n).toBe(true);
  }
});

test('Refresh folder tree adds itself to the Commands menu after Folder tree', () => {
  const c = reg.getCommand('cm_RescanTree');
  expect(c.menu).toBe('Commands');
  expect(c.menuAfter).toBe('cm_CDtree');
});

test('Refresh folder tree with no panel in the tree view says so and changes nothing', async () => {
  const flash = vi.fn();
  const app = { panels: [{ vs: { mode: 'full' } }, { vs: { mode: 'brief' } }], flash };
  expect(await reg.getCommand('cm_RescanTree').run(app)).toBe(false);
  expect(flash).toHaveBeenCalledOnce();
});

test('every cm_* command named in docs/FEATURES.md is registered, so All commands lists it (F-374)', () => {
  const doc = readFileSync(resolve(__dirname, '../../docs/FEATURES.md'), 'utf8');
  const names = new Set();
  for (const line of doc.split('\n')) {
    // not applicable on our platforms, or still open (not built yet)
    if (/^- \[(N\/A| )\]/.test(line)) continue;
    // a family ("cm_Vis*", "cm_LeftSortByCol..") is not one command
    for (const m of line.matchAll(/\bcm_[A-Za-z0-9_]+/g)) {
      const after = line.slice(m.index + m[0].length, m.index + m[0].length + 2);
      if (!after.startsWith('*') && after !== '..') names.add(m[0]);
    }
  }
  const missing = [...names].filter((n) => !reg.hasCommand(n)).sort();
  expect(missing).toEqual([]);
});
