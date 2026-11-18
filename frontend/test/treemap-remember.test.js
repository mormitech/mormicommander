// @vitest-environment happy-dom
// The folder size treemap's remembered sizes (features/treemap.js, board #24): with Operation.TreemapRemember on, the
// folders not counted yet are drawn at once with the size Go remembers (marked old), the new count replaces them and is
// handed to Treemap.Remember; with the option off Go is not asked and nothing is kept. Go and the panel are stand-ins.
import { afterEach, expect, test, vi } from 'vitest';

const calls = [];
let release = null;
const answers = {
  'Treemap.Remembered': (paths) => Object.fromEntries(paths.filter((p) => p.endsWith('/big')).map((p) => [p, { path: p, bytes: 5000, when: Math.floor(Date.now() / 1000) }])),
  'Treemap.Remember': (m) => Object.keys(m).length,
  'Tools.Space': (id, paths) => new Promise((res) => { release = () => res({ items: paths.map(() => ({ complete: true, bytes: 999 })) }); }),
};
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, {
    get: (_, obj) => new Proxy({}, {
      get: (_, m) => (...args) => {
        calls.push({ k: `${obj}.${m}`, args });
        const f = answers[`${obj}.${m}`];
        try { return Promise.resolve(f ? f(...args) : null); } catch (e) { return Promise.reject(e); }
      },
    }),
  });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => d, setSetting: () => Promise.resolve(), onSettingChange: () => () => {}, allSettings: () => ({}) }));
globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);
globalThis.ResizeObserver = class { observe() {} disconnect() {} };
Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get() { return 400; } });
Object.defineProperty(HTMLElement.prototype, 'clientHeight', { configurable: true, get() { return 300; } });

const { openTreemap, closeTreemap } = await import('../src/features/treemap.js');

const tick = (ms = 10) => new Promise((r) => setTimeout(r, ms));
const called = (k) => calls.filter((c) => c.k === k);

function setup(remember) {
  const el = document.createElement('div');
  const status = document.createElement('div');
  el.appendChild(status);
  document.body.appendChild(el);
  const entries = [{ name: '..', up: true, dir: true }, { name: 'big', dir: true, size: 0 }, { name: 'f.txt', dir: false, size: 100, ext: 'txt' }];
  const p = {
    side: 'left', el, $: { status, list: { focus() {} } }, path: '/home/me', parent: '/home', entries, dirSizes: new Map(),
    index: new Map(entries.map((e, i) => [e.name, i])), fullPath: (n) => `/home/me/${n}`, renderRows() {}, renderStatus() {}, go: async () => true, focusName() {},
  };
  const app = { config: { values: { 'Operation.TreemapRemember': remember ? '1' : '0' } }, cur: p, setActive() {}, on() {}, panel: () => p, flash() {} };
  return { app, p };
}

afterEach(() => { calls.length = 0; release = null; document.body.innerHTML = ''; });

test('remembered sizes show at once, marked old, until the new count replaces them and is remembered', async () => {
  const { app, p } = setup(true);
  const v = openTreemap(app, p);
  await tick();
  expect(called('Treemap.Remembered')[0].args[0]).toEqual(['/home/me/big']);
  const big = () => v.blocks.find((b) => b.name === 'big');
  expect(big()).toMatchObject({ value: 5000, old: true, counting: true });
  expect(v.area.querySelector('.mc-tm-b.old').title).toMatch(/remembered today, counting again/);
  expect(p.dirSizes.has('big')).toBe(false); // never put into the size column
  release();
  await tick();
  expect(p.dirSizes.get('big')).toBe(999);
  expect(big()).toMatchObject({ value: 999, counting: false });
  expect(big().old).toBeUndefined();
  expect(called('Treemap.Remember').map((c) => c.args[0])).toEqual([{ '/home/me/big': 999 }]);
  closeTreemap(app, p);
});

test('with the option off Go is neither asked nor told', async () => {
  const { app, p } = setup(false);
  const v = openTreemap(app, p);
  await tick();
  expect(v.blocks.find((b) => b.name === 'big')).toBeUndefined(); // not counted yet: pending
  release();
  await tick();
  expect(v.blocks.find((b) => b.name === 'big')).toMatchObject({ value: 999 });
  expect(called('Treemap.Remembered')).toEqual([]);
  expect(called('Treemap.Remember')).toEqual([]);
  closeTreemap(app, p);
});
