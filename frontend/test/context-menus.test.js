// @vitest-environment happy-dom
// Context menus (issue #85, docs/FEATURES.md section 37) in features/system-menus.js: the file menu (F-703, F-708),
// the menu of the empty part of the list (F-704) and the drive button menu (F-705) list what TC's menus have. The
// registry, the Go side, the popup and the app are stand-ins; the popup records what it was asked to show and chooses
// nothing.
import { beforeEach, expect, test, vi } from 'vitest';

const cmds = new Map();
let shown = null; // the last showList call

vi.mock('../src/core/registry.js', () => ({ command: (c) => cmds.set(c.name, c), hasCommand: () => true, getCommand: () => null }));
vi.mock('../src/core/go.js', () => ({
  go: new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) }),
  errText: (e) => String(e),
}));
vi.mock('../src/core/popup.js', () => ({ showList: (o) => { shown = o; return Promise.resolve(null); } }));
vi.mock('../src/core/dialogs.js', () => ({ esc: (s) => String(s), modal: () => Promise.resolve(null) }));
vi.mock('../src/core/control.js', () => ({ addControlCommand: () => {} }));
vi.mock('../src/core/app.js', () => ({ app: {} }));
vi.mock('../src/features/system.js', () => ({ anchorFor: () => null, revealLabel: () => 'Show in Finder', openWith: () => {} }));
vi.mock('../src/features/drivemaps.js', () => ({ mapFolder: () => {}, unmapFolder: () => {}, moveMap: () => {}, isMapped: () => false, mapIndex: () => -1, mapCount: () => 0 }));

const menus = await import('../src/features/system-menus.js');

function panel(entries, cur) {
  const handlers = {}; // the listeners init adds to the drive bar
  const drives = { addEventListener: (t, f) => { handlers[t] = f; } }, pathbar = { addEventListener: () => {} };
  return {
    path: '/Users/me', cur, handlers,
    $: { drives, pathbar, list: null, rows: null },
    targets: () => entries,
    fullPath: (n) => `/Users/me/${n}`,
    selectedEntries: () => [],
    isSelected: () => false,
    stats: () => ({ files: 0, dirs: 0, total: 0 }),
  };
}

const labels = () => shown.items.filter((x) => !x.sep).map((x) => x.label);
const disabled = (l) => shown.items.find((x) => x.label === l)?.disabled;

function makeApp(p) {
  return { platform: 'darwin', demo: false, cur: p, panels: [p], volumes: [], config: { values: {} }, keyLabel: () => '', flash: () => {} };
}

beforeEach(() => { shown = null; });

test('the file menu has the items of F-703 (Open, Open with, View, Edit, Copy, Cut, Paste, Rename, Delete, Pack, Properties, Copy path, reveal)', async () => {
  const f = { name: 'a.txt' };
  const p = panel([f], f);
  await cmds.get('cm_ContextMenu').run(makeApp(p), {});
  expect(shown.kind).toBe('context-file');
  expect(shown.title).toBe('a.txt');
  for (const l of ['Open', 'Open with…', 'View', 'Edit', 'Cut', 'Copy', 'Paste', 'Rename', 'Delete', 'Pack…', 'Properties', 'Copy path', 'Show in Finder'])
    expect(labels(), l).toContain(l);
  expect(disabled('Open with…')).toBe(false); // (F-708: the own menu's Open with on macOS/Linux)
});

test('the file menu has Split, Encode and Decode for one file, and none of them for a folder (F-763)', async () => {
  const f = { name: 'a.bin' };
  await cmds.get('cm_ContextMenu').run(makeApp(panel([f], f)), {});
  for (const l of ['Split…', 'Encode…', 'Decode…']) expect(labels(), l).toContain(l);
  const d = { name: 'dir', dir: true };
  await cmds.get('cm_ContextMenu').run(makeApp(panel([d], d)), {});
  for (const l of ['Split…', 'Encode…', 'Decode…']) expect(labels(), l).not.toContain(l);
  const two = [{ name: 'a' }, { name: 'b' }];
  await cmds.get('cm_ContextMenu').run(makeApp(panel(two, two[0])), {});
  expect(labels()).toContain('Encode…');
  expect(labels()).not.toContain('Decode…');
});

test('the file menu of several files copies their paths', async () => {
  const es = [{ name: 'a' }, { name: 'b' }];
  await cmds.get('cm_ContextMenu').run(makeApp(panel(es, es[0])), {});
  expect(shown.title).toBe('2 items');
  expect(labels()).toContain('Copy paths');
});

test('on ".." the background menu has the items of F-704 (New folder, New file, Paste, Refresh, folder properties)', async () => {
  const up = { name: '..', up: true, dir: true };
  await cmds.get('cm_ContextMenu').run(makeApp(panel([], up)), {});
  expect(shown.kind).toBe('context-folder');
  for (const l of ['New folder…', 'New file…', 'Paste', 'Reread', 'Folder properties…']) expect(labels(), l).toContain(l);
});

test('the drive button menu offers Eject for removable volumes only, and Properties (F-705)', async () => {
  const p = panel([], null);
  const app = makeApp(p);
  menus.init(app);
  const fire = (v) => {
    app.volumes = [v];
    const b = { dataset: { i: '0' } };
    p.handlers.contextmenu({ target: { closest: () => b }, preventDefault: () => {}, clientX: 1, clientY: 2 });
  };
  fire({ name: 'USB', path: '/Volumes/USB', kind: 'removable' });
  expect(shown.kind).toBe('context-drive');
  expect(labels()).toEqual(['Open', 'Open in the other panel', 'Eject', 'Properties…']);
  expect(disabled('Eject')).toBe(false);
  fire({ name: 'Macintosh HD', path: '/', kind: 'fixed' });
  expect(disabled('Eject')).toBe(true);
  expect(disabled('Properties…')).toBe(false);
});
