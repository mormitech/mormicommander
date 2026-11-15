// @vitest-environment happy-dom
// The commands of features/reserved-more.js (board issue #38, second slice), with the registry, the Go side, the
// dialogs, the popup, the menu tree and the panels' tree view as stand-ins: the separate tree commands put the right
// panels into the tree view, Shift+Tab switches panels, F9 walks the menu and runs the chosen command, Alt+F10 walks
// the folders and goes to the chosen one, and printing asks first and only then calls Go.
import { beforeEach, expect, test, vi } from 'vitest';

const cmds = new Map();
const calls = [];
const lists = []; // the popups shown: {title, items}
let answers = []; // what each popup answers, in order
let dialogAnswer = null;
let formAnswer = null;
const tree = new Map(); // panel -> in the tree view

vi.mock('../src/core/registry.js', () => ({ command: (c) => cmds.set(c.name, c) }));
vi.mock('../src/core/go.js', () => ({
  go: {
    FS: {
      List: (_k, path) => Promise.resolve({
        '/a': { path: '/a', parent: '/', entries: [['b', 0, 0, 1], ['x.txt', 1, 0, 0]] },
        '/a/b': { path: '/a/b', parent: '/a', entries: [['c', 0, 0, 1]] },
        '/': { path: '/', parent: '', entries: [['a', 0, 0, 1]] },
      }[path]),
    },
    SystemLaunch: {
      Print: (p) => { calls.push(['Print', p]); return Promise.resolve(); },
      PrintText: (name, text) => { calls.push(['PrintText', name, text]); return Promise.resolve(); },
    },
  },
  errText: (e) => String(e?.message ?? e),
}));
vi.mock('../src/core/popup.js', () => ({
  showList: (o) => { lists.push(o); const a = answers.shift(); return Promise.resolve(a ? a(o.items) : null); },
}));
vi.mock('../src/core/dialogs.js', () => ({
  esc: (s) => String(s),
  messageDialog: (o) => { calls.push(['ask', o.Message]); return Promise.resolve(dialogAnswer); },
  formDialog: (title, html) => { calls.push(['form', title, html]); return Promise.resolve(formAnswer); },
}));
vi.mock('../src/core/menus.js', () => ({
  menuSpec: () => [
    { label: 'Files', items: [{ label: 'Quit', cmd: 'cm_Exit', disabled: false }] },
    { label: 'Show', items: [{ label: 'Separate tree', items: [{ label: 'Off', cmd: 'cm_SeparateTreeOff', disabled: false, checked: true }] }] },
  ],
}));
vi.mock('../src/features/views.js', () => ({
  inTreeView: (p) => !!tree.get(p),
  setTreeView: (p, on) => { if (!!tree.get(p) === !!on) return Promise.resolve(false); tree.set(p, !!on); calls.push(['tree', p.side, !!on]); return Promise.resolve(true); },
}));

await import('../src/features/reserved-more.js');

function panel(side, path) {
  const p = {
    side, path, entries: [], filterLabel: '', $: { pathbar: null },
    go: (to) => { calls.push(['go', side, to]); p.path = to; return Promise.resolve(true); },
    fullPath: (n) => `${p.path}/${n}`,
  };
  return p;
}

let app;
beforeEach(() => {
  calls.length = 0;
  lists.length = 0;
  answers = [];
  dialogAnswer = null;
  formAnswer = null;
  tree.clear();
  const left = panel('left', '/a'), right = panel('right', '/');
  app = {
    platform: 'darwin', showHidden: false, panels: [left, right], active: 0,
    get cur() { return this.panels[this.active]; },
    get other() { return this.panels[1 - this.active]; },
    setActive(p) { this.active = this.panels.indexOf(p); },
    sepFor: () => '/',
    keyLabel: (c) => (c === 'cm_Exit' ? 'Alt+F4' : ''),
    flash: (m) => calls.push(['flash', m]),
    run: (name) => { calls.push(['run', name]); return Promise.resolve(true); },
  };
});

const run = (name, args = {}) => cmds.get(name).run(app, args);
const pick = (label) => (items) => { const i = items.findIndex((it) => it.label === label); return { item: items[i], index: i, shift: false }; };

test('every command of the slice is registered', () => {
  for (const n of ['cm_CDtree', 'cm_SeparateTreeOff', 'cm_SeparateTree1', 'cm_SeparateTree2', 'cm_SeparateTreeCycle', 'cm_FocusTreeToggle', 'cm_ActivateMenu', 'cm_PrintFile', 'cm_PrintDir']) {
    expect(cmds.has(n), n).toBe(true);
  }
});

test('separate tree: one, two, off and the cycle', async () => {
  const [left, right] = app.panels;
  await run('cm_SeparateTree1');
  expect([tree.get(left), !!tree.get(right)]).toEqual([true, false]);
  expect(cmds.get('cm_SeparateTree1').checked(app)).toBe(true);
  await run('cm_SeparateTree2');
  expect([tree.get(left), tree.get(right)]).toEqual([true, true]);
  expect(cmds.get('cm_SeparateTree2').checked(app)).toBe(true);
  await run('cm_SeparateTreeCycle'); // two -> off
  expect([tree.get(left), tree.get(right)]).toEqual([false, false]);
  expect(cmds.get('cm_SeparateTreeOff').checked(app)).toBe(true);
  await run('cm_SeparateTreeCycle'); // off -> one, in the active panel
  expect([tree.get(left), !!tree.get(right)]).toEqual([true, false]);
  expect(app.cur).toBe(left); // the active panel stays active
});

test('Shift+Tab switches to the other panel', async () => {
  await run('cm_FocusTreeToggle');
  expect(app.cur.side).toBe('right');
  await run('cm_FocusTreeToggle');
  expect(app.cur.side).toBe('left');
});

test('F9 walks the menu and runs the chosen command', async () => {
  answers = [pick('Files ▸'), pick('Quit')];
  await run('cm_ActivateMenu');
  expect(lists.map((l) => l.title)).toEqual(['Menu', 'Files']);
  expect(lists[1].items[0].detail).toBe('Alt+F4');
  expect(calls).toContainEqual(['run', 'cm_Exit']);
});

test('the menu goes back a level on Esc or Left and closes at the top', async () => {
  answers = [pick('Show ▸'), pick('Separate tree ▸'), () => null, () => null, () => null];
  expect(await run('cm_ActivateMenu')).toBe(false);
  expect(lists.map((l) => l.title)).toEqual(['Menu', 'Show', 'Show › Separate tree', 'Show', 'Menu']);
  expect(lists[2].items[0].label).toBe('✓ Off');
  expect(calls.some((c) => c[0] === 'run')).toBe(false);
});

test('Alt+F10 walks down and up the folders and goes to the chosen one', async () => {
  answers = [pick('b'), pick('..'), pick('Go to /a')];
  await run('cm_CDtree');
  expect(lists[0].items.map((it) => it.label)).toEqual(['Go to /a', '..', undefined, 'b']);
  expect(lists[1].items[0].label).toBe('Go to /a/b');
  expect(lists[2].index).toBe(3); // back up: "b" is highlighted again
  expect(calls).toContainEqual(['go', 'left', '/a']);
});

test('Alt+F10: Shift+Enter on a folder goes there at once; Esc changes nothing', async () => {
  answers = [(items) => ({ item: items[3], index: 3, shift: true })];
  await run('cm_CDtree');
  expect(calls).toContainEqual(['go', 'left', '/a/b']);
  calls.length = 0;
  answers = [() => null];
  expect(await run('cm_CDtree')).toBe(false);
  expect(calls.filter((c) => c[0] === 'go')).toEqual([]);
});

test('printing a file asks first and prints only on yes', async () => {
  app.cur.cur = { name: 'x.txt' };
  dialogAnswer = 'Cancel';
  expect(await run('cm_PrintFile')).toBe(false);
  expect(calls.some((c) => c[0] === 'Print')).toBe(false);
  dialogAnswer = 'Print';
  expect(await run('cm_PrintFile')).toBe(true);
  expect(calls).toContainEqual(['Print', '/a/x.txt']);
});

test('printing refuses folders, ".." and files on servers without asking', async () => {
  for (const e of [{ name: 'b', dir: true }, { name: '..', up: true, dir: true }]) {
    app.cur.cur = e;
    expect(await run('cm_PrintFile')).toBe(false);
  }
  app.cur.path = 'ftp://host/dir';
  app.cur.cur = { name: 'x.txt' };
  expect(await run('cm_PrintFile')).toBe(false);
  expect(calls.some((c) => c[0] === 'ask' || c[0] === 'Print')).toBe(false);
});

test('the file list is previewed and printed only after Print', async () => {
  app.cur.entries = [{ name: '..', up: true, dir: true }, { name: 'x.txt', size: 3, mtime: 0 }];
  formAnswer = null;
  expect(await run('cm_PrintDir')).toBe(false);
  expect(calls.find((c) => c[0] === 'form')[2]).toContain('x.txt');
  expect(calls.some((c) => c[0] === 'PrintText')).toBe(false);
  formAnswer = true;
  expect(await run('cm_PrintDir')).toBe(true);
  const printed = calls.find((c) => c[0] === 'PrintText');
  expect(printed[1]).toBe('File list /a');
  expect(printed[2]).toContain('Folder: /a');
  expect(printed[2]).toContain('x.txt');
});
