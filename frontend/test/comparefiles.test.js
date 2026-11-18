// @vitest-environment happy-dom
// Compare by content (issue #74, docs/FEATURES.md section 26): the menu entries (Files > Compare by content with its key,
// Commands > Synchronize folders, Configuration > Compare program), the pair picked from the panels (two selected, one
// in each panel, the file of the same name in the other panel: F-561), the external program (F-563), and the window
// itself with a stand-in Go: line numbers, next/previous difference (F-558), the ignore options sent to Go (F-560),
// one scrolling body for both sides and the encoding choice per side (F-839).
import { beforeAll, expect, test, vi } from 'vitest';

const calls = [];
const view = (patch = {}) => ({
  id: 7, left: '/l/a.txt', right: '/r/a.txt', binary: false, identical: false, note: '',
  rows: [
    { k: 0, l: 0, r: 0, a: 'same', b: 'same' },
    { k: 3, l: 1, r: 1, a: 'old line', b: 'new line', sa: [[0, 3]], sb: [[0, 3]] },
    { k: 0, l: 2, r: 2, a: 'same 2', b: 'same 2' },
    { k: 1, l: 3, r: -1, a: 'only left', b: '' },
  ],
  blocks: [[1, 1], [3, 3]], stats: { blocks: 2 }, lineCount: [4, 3],
  options: { ignoreCase: false, whitespace: 'exact', ignoreRepeated: false }, modified: [false, false],
  canUndo: false, canRedo: false, encoding: ['UTF-8', 'Latin-1'], sizeA: 10, sizeB: 10, ...patch,
});
const stub = {
  Same: async () => false,
  Open: async (l, r, o) => { calls.push(['Open', l, r, o]); return view(); },
  Encodings: async () => ['Auto', 'UTF-8', 'Latin-1'],
  SetOptions: async (id, o) => { calls.push(['SetOptions', id, o]); return view({ options: { ...view().options, ...o } }); },
  SetEncoding: async (id, side, enc) => { calls.push(['SetEncoding', id, side, enc]); return view(); },
  ExternalStart: async (t, l, r) => { calls.push(['ExternalStart', t, l, r]); },
  Close: async () => {},
};
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: (_, svc) => (svc === 'CompareFiles' ? stub : new Proxy({}, { get: () => () => Promise.resolve(svc === 'DialogWindows' ? {} : null) })) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

let reg, menus, cf;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  globalThis.ResizeObserver ||= class { observe() {} disconnect() {} };
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  cf = await import('../src/features/comparefiles.js');
  vi.useRealTimers();
});

const flat = (items) => items.flatMap((it) => (Array.isArray(it) ? (Array.isArray(it[1]) ? flat(it[1]) : [it[1]]) : []));
const menu = (name) => flat(menus.MENUS.find(([l]) => l === name)[1]);

test('the compare entries are in their menus and run registered commands (F-556, F-562, F-563)', () => {
  expect(menu('Files')).toContain('cm_CompareFilesByContent');
  expect(reg.getCommand('cm_CompareFilesByContent').menuKey).toBe(true);
  expect(menu('Commands')).toContain('cm_SyncDirs');
  expect(reg.getCommand('cm_SyncDirs').menuKey).toBe(true);
  expect(reg.getCommand('cm_CompareToolSetup').menu).toBe('Configuration');
});

const panel = (side, path, names, { sel = [], cur } = {}) => {
  const entries = names.map((n) => ({ name: n.replace(/\/$/, ''), dir: n.endsWith('/') }));
  return {
    side, entries,
    cur: entries.find((e) => e.name === cur) || null,
    selectedEntries: () => entries.filter((e) => sel.includes(e.name)),
    fullPath: (n) => `${path}/${n}`,
  };
};
const appWith = (cur, other) => ({ cur, other, platform: 'darwin' });

test('pickPair: the file under the cursor and the file of the same name in the other panel (F-561)', () => {
  const l = panel('left', '/l', ['a.txt', 'b.txt'], { cur: 'b.txt' });
  const r = panel('right', '/r', ['x.txt', 'B.TXT', 'b.txt.bak'], { cur: 'x.txt' });
  expect(cf.pickPair(appWith(l, r))).toEqual({ left: '/l/b.txt', right: '/r/B.TXT' });
  // from the right panel the left file still goes left
  const rb = panel('right', '/r', ['x.txt', 'B.TXT'], { cur: 'B.TXT' });
  expect(cf.pickPair(appWith(rb, l))).toEqual({ left: '/l/b.txt', right: '/r/B.TXT' });
  // no twin: the file under the cursor in the other panel
  const r2 = panel('right', '/r', ['x.txt'], { cur: 'x.txt' });
  expect(cf.pickPair(appWith(l, r2))).toEqual({ left: '/l/b.txt', right: '/r/x.txt' });
});

test('pickPair: two selected in one panel, one in each, folders refused', () => {
  const l = panel('left', '/l', ['a', 'b', 'd/'], { sel: ['a', 'b'] });
  expect(cf.pickPair(appWith(l, panel('right', '/r', [])))).toEqual({ left: '/l/a', right: '/l/b' });
  const l1 = panel('left', '/l', ['a'], { sel: ['a'] }), r1 = panel('right', '/r', ['z'], { sel: ['z'] });
  expect(cf.pickPair(appWith(l1, r1))).toEqual({ left: '/l/a', right: '/r/z' });
  const ld = panel('left', '/l', ['a', 'd/'], { sel: ['a', 'd'] });
  expect(cf.pickPair(appWith(ld, panel('right', '/r', [])))).toHaveProperty('error');
});

const fakeApp = (compare = {}) => {
  const store = { compare };
  return { setting: (k, d) => store[k] ?? d, setSetting: (k, v) => { store[k] = v; }, flash: vi.fn(), store };
};

test('an external compare program is started instead of the window (F-563)', async () => {
  calls.length = 0;
  const r = await cf.compareFiles(fakeApp({ externalTool: 'meld %1 %2' }), '/l/a', '/r/a');
  expect(r.external).toBe(true);
  expect(calls).toEqual([['ExternalStart', 'meld %1 %2', '/l/a', '/r/a']]);
});

const key = (k, mods = {}) => window.dispatchEvent(new KeyboardEvent('keydown', { key: k, code: k, bubbles: true, ...mods }));
const tick = () => new Promise((r) => setTimeout(r, 0));

test('the window: line numbers, differences, ignore options, one body, encodings (F-556, F-558, F-560, F-839)', async () => {
  calls.length = 0;
  globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);
  const app = fakeApp();
  const done = cf.compareFiles(app, '/l/a.txt', '/r/a.txt');
  await tick(); await tick();
  const box = document.querySelector('.mc-cmp-win');
  expect(box).toBeTruthy();
  // one scrolling body holds both sides, so they always scroll together
  expect(box.querySelectorAll('.mc-cmp-body').length).toBe(1);
  // the window opens on the first difference
  const rows = [...box.querySelectorAll('.mc-cmp-row')];
  expect(rows.length).toBe(4);
  expect([...rows[3].querySelectorAll('.mc-cmp-n')].map((n) => n.textContent)).toEqual(['4', '']);
  expect(rows[1].querySelector('.s0 mark').textContent).toBe('old');
  expect(box.querySelector('.mc-cmp-status').textContent).toMatch(/^Difference 1 of 2/);
  key('F8');
  expect(box.querySelector('.mc-cmp-status').textContent).toMatch(/^Difference 2 of 2/);
  key('F8', { shiftKey: true });
  expect(box.querySelector('.mc-cmp-status').textContent).toMatch(/^Difference 1 of 2/);
  // ignore case goes to Go and is remembered (binary never is)
  const c = box.querySelector('#cmp-case');
  c.checked = true; c.dispatchEvent(new Event('change'));
  await tick();
  expect(calls.find((x) => x[0] === 'SetOptions')[2]).toMatchObject({ ignoreCase: true });
  expect(app.store.compare.options).toMatchObject({ ignoreCase: true });
  expect(app.store.compare.options).not.toHaveProperty('binary');
  // the encoding per side is shown and a change reads the side again
  expect(box.querySelector('#cmp-enc-right').value).toBe('Latin-1');
  const enc = box.querySelector('#cmp-enc-left');
  enc.value = 'Latin-1'; enc.dispatchEvent(new Event('change'));
  await tick();
  expect(calls.find((x) => x[0] === 'SetEncoding')).toEqual(['SetEncoding', 7, 0, 'Latin-1']);
  box.querySelector('.b-close').click();
  expect(await done).toEqual({ saved: [false, false] });
});
