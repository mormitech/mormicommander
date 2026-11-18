// @vitest-environment happy-dom
// The folder hotlist and history in the menus (issue #79, docs/FEATURES.md section 31): the hotlist of the other panel
// (F-659), hotlist search (F-658), frequently used folders (F-661), history clear and settings (F-662), the TC import
// (F-663) and adding the current tabs (F-664) each have a registered command and sit in a menu; the import appends
// with a separator and changes nothing already there. Go is a stand-in that answers nothing.
import { beforeAll, expect, test, vi } from 'vitest';
import { appendImported } from '../src/features/hotlist-lib.js';

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

test('the section 31 commands are registered', () => {
  for (const n of ['cm_DirectoryHotlist', 'cm_OtherDirectoryHotlist', 'cm_SearchDirHotlist', 'cm_AddDirToHotlist', 'cm_AddDirsToHotlist',
    'cm_AddTabsToHotlist', 'cm_ConfigDirHotlist', 'cm_ImportDirHotlist', 'cm_DirectoryHistory', 'cm_FrequentlyUsedDirs',
    'cm_ClearDirHistory', 'cm_DirHistorySettings']) expect(reg.hasCommand(n), n).toBe(true);
});

test('the hotlist and history commands sit in their menus, after the entries they belong to', () => {
  expect(reg.getCommand('cm_OtherDirectoryHotlist')).toMatchObject({ menu: 'Commands', menuAfter: 'cm_DirectoryHotlist', menuKey: true });
  expect(reg.getCommand('cm_SearchDirHotlist')).toMatchObject({ menu: 'Commands', menuAfter: 'cm_OtherDirectoryHotlist' });
  expect(reg.getCommand('cm_FrequentlyUsedDirs')).toMatchObject({ menu: 'Commands', menuAfter: 'cm_DirectoryHistory' });
  expect(reg.getCommand('cm_ClearDirHistory')).toMatchObject({ menu: 'Commands', menuAfter: 'cm_FrequentlyUsedDirs' });
  expect(reg.getCommand('cm_ImportDirHotlist')).toMatchObject({ menu: 'Configuration', menuAfter: 'cm_ConfigDirHotlist' });
  expect(reg.getCommand('cm_DirHistorySettings')).toMatchObject({ menu: 'Configuration', menuAfter: 'cm_ImportDirHotlist' });
  // the entries they follow are in the fixed menus, so each lands right after its entry
  const fixed = JSON.stringify(menus.MENUS);
  for (const n of ['cm_DirectoryHotlist', 'cm_DirectoryHistory', 'cm_ConfigDirHotlist']) expect(fixed, n).toContain(`"${n}"`);
});

test('Ctrl+Shift+D opens the hotlist of the other panel; Ctrl+D and Alt+Down stay as they were', () => {
  reg.setupKeys('windows');
  expect(reg.commandForKey('Ctrl+Shift+D')).toBe('cm_OtherDirectoryHotlist');
  expect(reg.commandForKey('Ctrl+D')).toBe('cm_DirectoryHotlist');
  expect(reg.commandForKey('Alt+Down')).toBe('cm_DirectoryHistory');
});

test('appendImported adds the TC entries at the end with a separator, and keeps the old entries', () => {
  const old = [{ kind: 'cmd', title: 'Home', cmd: 'cd ~' }];
  const add = [{ kind: 'cmd', title: '&Desktop', cmd: 'cd %USERPROFILE%\\Desktop' }, { kind: 'menu', title: 'Sub', items: [] }];
  const got = appendImported(old, add);
  expect(got).toEqual([old[0], { kind: 'sep', title: '' }, ...add]);
  expect(old).toHaveLength(1);
  expect(appendImported([], add)).toEqual(add);
  expect(appendImported(old, [])).toEqual(old);
  expect(appendImported(null, null)).toEqual([]);
});
