// @vitest-environment happy-dom
// The Start menu (issue #66, docs/FEATURES.md section 18): the user's entries come into the Start menu with their
// submenus, separators and keys (F-448, F-449), an entry's icon file goes along for Go to draw, and Change start menu
// (F-450) edits the entries, the icon among them, and saves them. Go is a stand-in with one Start menu.
import { beforeAll, expect, test, vi } from 'vitest';

const saved = [];
vi.mock('../src/core/go.js', () => {
  const start = [
    { kind: 'item', depth: 0, title: '&Notes', cmd: 'open', params: '-a TextEdit', path: '', key: 'Ctrl+Shift+N', icon: '/icons/notes.png' },
    { kind: 'submenu', depth: 0, title: 'Tools', cmd: '', params: '', path: '', key: '', icon: '' },
    { kind: 'item', depth: 1, title: 'Reread', cmd: 'cm_RereadSource', params: '', path: '', key: '', icon: '' },
    { kind: 'sep', depth: 1, title: '-', cmd: '', params: '', path: '', key: '', icon: '' },
    { kind: 'item', depth: 1, title: 'Edit', cmd: 'cm_Edit', params: '', path: '', key: '', icon: '' },
    { kind: 'submenu', depth: 0, title: 'Empty', cmd: '', params: '', path: '', key: '', icon: '' },
  ];
  const special = {
    ConfigStart: { Menu: () => Promise.resolve(start.map((x) => ({ ...x }))), SaveMenu: (l) => { saved.push(l); return Promise.resolve(null); } },
    ConfigBar: { IconData: () => Promise.resolve('data:image/png;base64,AA==') },
    DialogWindows: { Open: () => Promise.reject(new Error('no windows here')) },
  };
  const any = new Proxy({}, { get: () => () => Promise.resolve(null) });
  const go = new Proxy({}, { get: (t, k) => special[k] || any });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus, sm, app;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  sm = await import('../src/features/config/startmenu.js');
  ({ app } = await import('../src/core/app.js'));
  const deep = new Proxy(function () {}, { get: (t, k) => (k === Symbol.toPrimitive ? () => '' : k === Symbol.iterator ? [][Symbol.iterator] : deep), apply: () => deep });
  document.body.innerHTML = '<div id="panels"></div>';
  for (const k of ['cur', 'other', 'left', 'right']) Object.defineProperty(app, k, { value: deep, configurable: true, writable: true });
  await sm.loadStartMenu();
});

const startMenu = () => menus.menuSpec().find((m) => m.label === 'Start');

test('the entries come into the Start menu with submenus, separators, keys and icons (F-448, F-449, F-450)', () => {
  const items = startMenu().items;
  const at = items.findIndex((it) => it.cmd === 'cm_StartMenu_1');
  expect(at).toBeGreaterThan(0);
  expect(items[at - 1].sep).toBe(true); // a line between the built-in entries and the user's
  expect(items[at].label).toMatch(/^Notes /); // & marks the underlined letter, it is not shown
  expect(items[at].label).toMatch(/N/);
  expect(items[at].icon).toBe('/icons/notes.png');
  const tools = items[at + 1];
  expect(tools.label).toBe('Tools');
  expect(tools.items.map((it) => it.cmd || 'sep')).toEqual(['cm_StartMenu_3', 'sep', 'cm_StartMenu_5']);
  expect(tools.items[0].icon).toBeUndefined();
  expect(items.some((it) => it.label === 'Empty')).toBe(false); // an empty submenu is left out
  for (const i of [1, 3, 5]) expect(reg.hasCommand(`cm_StartMenu_${i}`)).toBe(true);
});

test("an entry's own key runs it (F-449)", async () => {
  const { onKeyDown } = await import('../src/core/keyboard.js');
  const run = vi.spyOn(app, 'run').mockImplementation(() => true);
  const ev = new KeyboardEvent('keydown', { key: 'N', code: 'KeyN', ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true });
  Object.defineProperty(ev, 'target', { value: document.body });
  onKeyDown(ev);
  expect(run).toHaveBeenCalledWith('cm_StartMenu_1');
  run.mockRestore();
});

test('Change start menu edits an entry and its icon, adds one and saves the list (F-450)', async () => {
  const p = sm.openStartMenuConfig(app);
  const box = document.querySelector('.mc-modal form');
  const $ = (s) => box.querySelector(s);
  expect($('#sm-list').options.length).toBe(6);
  expect($('#sm-icon').value).toBe('/icons/notes.png');
  await vi.waitFor(() => expect($('#sm-iconview').hidden).toBe(false));
  // a submenu has no icon
  $('#sm-list').value = '1';
  $('#sm-list').dispatchEvent(new Event('change'));
  expect($('#sm-icon').disabled).toBe(true);
  // a new entry after Edit, inside the Tools submenu
  $('#sm-list').value = '4';
  $('#sm-list').dispatchEvent(new Event('change'));
  $('.sm-add').click();
  for (const [f, v] of [['title', 'Find'], ['cmd', 'cm_SearchFor'], ['icon', '/icons/find.png']]) {
    $(`#sm-${f}`).value = v;
    $(`#sm-${f}`).dispatchEvent(new Event('input'));
  }
  box.dispatchEvent(new Event('submit', { cancelable: true }));
  await vi.waitFor(() => expect(saved.length).toBe(1));
  const list = saved[0];
  expect(list[5]).toMatchObject({ kind: 'item', depth: 1, title: 'Find', cmd: 'cm_SearchFor', icon: '/icons/find.png' });
  expect(list[0].icon).toBe('/icons/notes.png');
  await p;
});
