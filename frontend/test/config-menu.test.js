// @vitest-environment happy-dom
// The Configuration menu (issue #65, docs/FEATURES.md section 17): every entry names a registered command, Options…
// shows its key (Cmd+, on macOS), the settings dialogs that add themselves to the menu are there (favourite tabs among
// them), a custom main menu file replaces the built-in menu bar and an empty one brings it back (F-828), and the view
// modes can be put in order (F-820). Go is a stand-in that answers nothing; the timers are frozen so no menu is sent.
import { beforeAll, expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus, keymap, views;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  keymap = await import('../src/core/keymap.js');
  views = await import('../src/features/views.js');
  // a stand-in for the panels, so that the menu items' enabled/checked tests can run: every property and call gives it again
  const deep = new Proxy(function () {}, { get: (t, k) => (k === Symbol.toPrimitive ? () => '' : k === Symbol.iterator ? [][Symbol.iterator] : deep), apply: () => deep });
  document.body.innerHTML = '<div id="panels"></div>';
  const { app } = await import('../src/core/app.js');
  for (const k of ['cur', 'other', 'left', 'right']) Object.defineProperty(app, k, { value: deep, configurable: true, writable: true });
});

const flat = (items) => items.flatMap((it) => (Array.isArray(it) ? (Array.isArray(it[1]) ? flat(it[1]) : [it[1]]) : []));
const configMenu = () => flat(menus.MENUS.find(([l]) => l === 'Configuration')[1]);
const specMenu = (label) => menus.menuSpec().find((m) => m.label === label);

test('every entry of the Configuration menu runs a registered command', () => {
  expect(configMenu().filter((n) => !reg.hasCommand(n))).toEqual([]);
  for (const n of ['cm_Config', 'cm_ConfigHotkeys', 'cm_ButtonConfig', 'cm_ConfigStartMenu', 'cm_ConfigDirHotlist', 'cm_ConfigSavePos', 'cm_ConfigSaveSettings', 'cm_ConfigChangeIniFiles']) {
    expect(configMenu(), n).toContain(n);
  }
});

test('Options… shows its key (Cmd+, on macOS, F-412)', () => {
  expect(reg.getCommand('cm_Config').menuKey).toBe(true);
  expect(keymap.MAC_KEYS['Cmd+,']).toBe('cm_Config');
});

test('the settings dialogs that add themselves to the Configuration menu are in it (F-426, F-433, F-434)', () => {
  expect(reg.getCommand('cm_ConfigFavoriteTabs')).toMatchObject({ menu: 'Configuration', menuAfter: 'cm_ConfigDirHotlist' });
  const cmds = specMenu('Configuration').items.map((it) => it.cmd);
  for (const n of ['cm_ConfigFavoriteTabs', 'cm_CustomColumnConfig', 'cm_ThumbnailsConfig', 'cm_ListerConfig']) expect(cmds, n).toContain(n);
});

test('a custom main menu replaces the built-in menus; none brings them back (F-828)', () => {
  menus.setCustomMenus([['Mine', [['Options…', 'cm_Config'], '-', ['Quit', 'cm_Exit']]]]);
  expect(specMenu('Files')).toBeUndefined();
  expect(specMenu('Mine').items.map((it) => it.cmd || 'sep')).toEqual(['cm_Config', 'sep', 'cm_Exit']);
  menus.setCustomMenus(null);
  expect(specMenu('Files')).toBeDefined();
  expect(specMenu('Mine')).toBeUndefined();
  menus.setCustomMenus([]);
  expect(specMenu('Configuration')).toBeDefined();
});

test('view modes can be moved up and edited in place (F-820)', () => {
  const ms = [{ name: 'a' }, { name: 'b' }, { name: 'c' }];
  expect(views.moveViewModeUp(ms, 'c').map((m) => m.name)).toEqual(['a', 'c', 'b']);
  expect(views.moveViewModeUp(ms, 'a').map((m) => m.name)).toEqual(['a', 'b', 'c']);
  expect(views.moveViewModeUp(ms, 'x').map((m) => m.name)).toEqual(['a', 'b', 'c']);
  expect(ms.map((m) => m.name)).toEqual(['a', 'b', 'c']);
  const b2 = { name: 'b', mode: 'brief' };
  expect(views.putViewMode(ms, b2)).toEqual([{ name: 'a' }, b2, { name: 'c' }]);
  expect(views.putViewMode(ms, { name: 'd' }).map((m) => m.name)).toEqual(['a', 'b', 'c', 'd']);
});
