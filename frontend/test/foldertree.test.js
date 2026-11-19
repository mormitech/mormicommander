// @vitest-environment happy-dom
// The expanding folder tree (features/foldertree.js) behind Alt+F10 with Options > Tree > "Folder tree popup shows the
// whole drive as an expanding tree" (F-171) and the separate tree pane (Layout.SeparateTreePane, F-025): folders are
// read only when opened, the way down to the panel's folder opens, the keys move/open/close/filter, and the panes
// a separate tree level shows. Also the help sections F1 in a dialog opens on (F-145). Go is a stand-in.
import { expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '# Keys' }));

const lib = await import('../src/features/foldertree-lib.js');
const { FolderTree } = await import('../src/features/foldertree.js');

const DISK = {
  '/': ['Users', 'tmp', 'Applications'],
  '/Users': ['ann', 'bob'],
  '/Users/ann': ['Documents', 'Music'],
  '/Users/ann/Documents': [],
  '/Users/ann/Music': [],
  '/Users/bob': ['Projects'],
  '/Users/bob/Projects': [],
  '/tmp': [],
  '/Applications': [],
};
const join = (d, n) => (d.endsWith('/') ? d + n : `${d}/${n}`);
function fakeTree(opts = {}) {
  const reads = [];
  const read = async (p) => { reads.push(p); return (DISK[p] || []).map((n) => ({ name: n, path: join(p, n) })); };
  const el = document.createElement('div');
  document.body.append(el);
  const gone = [];
  const t = new FolderTree({ el, read, onGo: (n) => gone.push(n.path), ...opts });
  return { t, reads, gone, el };
}
const key = (k, extra = {}) => ({ key: k, ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...extra });
const tick = () => new Promise((r) => setTimeout(r, 0));

test('the way from the root down to a folder', () => {
  expect(lib.chainBelow('/', '/Users/ann/Music')).toEqual(['/Users', '/Users/ann', '/Users/ann/Music']);
  expect(lib.chainBelow('/', '/')).toEqual([]);
  expect(lib.chainBelow('C:\\', 'C:\\Work\\a')).toEqual(['C:\\Work', 'C:\\Work\\a']);
  expect(lib.chainBelow('c:\\', 'C:\\Work', true)).toEqual(['C:\\Work']);
  expect(lib.samePath('/a/', '/a')).toBe(true);
  expect(lib.samePath('C:\\', 'C:\\')).toBe(true);
});

test('only open folders show; a filter shows matching folders read so far with the folders above them', () => {
  const root = lib.makeNode('/', '/');
  const [users, tmp] = lib.setKids(root, [{ name: 'Users', path: '/Users' }, { name: 'tmp', path: '/tmp' }]);
  lib.setKids(users, [{ name: 'ann', path: '/Users/ann' }]);
  expect(lib.visibleNodes(root).map((n) => n.path)).toEqual(['/']);
  root.open = true;
  expect(lib.visibleNodes(root).map((n) => n.path)).toEqual(['/', '/Users', '/tmp']);
  expect(lib.visibleNodes(root, 'an').map((n) => n.path)).toEqual(['/', '/Users', '/Users/ann']);
  expect(lib.visibleNodes(root, 'zzz').map((n) => n.path)).toEqual(['/']);
  expect(tmp.depth).toBe(1);
});

test('opening the tree reads only the folders on the way to the panel folder (lazy loading)', async () => {
  const { t, reads } = fakeTree();
  await t.openTo('/', '/Users/ann/Music');
  expect(reads).toEqual(['/', '/Users', '/Users/ann', '/Users/ann/Music']);
  expect(t.cur.path).toBe('/Users/ann/Music');
  expect(t.nodes.map((n) => n.name)).toEqual(['/', 'Users', 'ann', 'Documents', 'Music', 'bob', 'tmp', 'Applications']);
  expect(t.el.querySelectorAll('.mc-ft-row').length).toBe(8);
  expect(t.el.querySelector('.mc-ft-row.on').textContent).toContain('Music');
});

test('keys: move, open, close, go to the folder above, Enter chooses', async () => {
  const { t, reads, gone } = fakeTree();
  await t.openTo('/', '/Users');
  expect(t.cur.path).toBe('/Users');
  t.key(key('ArrowDown')); t.key(key('ArrowDown'));
  expect(t.cur.path).toBe('/Users/bob');
  t.key(key('ArrowRight')); await tick();
  expect(reads).toContain('/Users/bob');
  expect(t.nodes.some((n) => n.path === '/Users/bob/Projects')).toBe(true);
  t.key(key('ArrowRight'));
  expect(t.cur.path).toBe('/Users/bob/Projects');
  t.key(key('ArrowLeft'));
  expect(t.cur.path).toBe('/Users/bob');
  t.key(key('ArrowLeft')); await tick();
  expect(t.nodes.some((n) => n.path === '/Users/bob/Projects')).toBe(false);
  t.key(key('Home'));
  expect(t.cur.path).toBe('/');
  t.key(key('End'));
  t.key(key('Enter'));
  expect(gone).toEqual(['/Applications']);
  expect(t.key(key('Escape'))).toBe(false); // (no filter: the caller closes)
});

test('typing filters, Backspace and Esc take it back', async () => {
  const filters = [];
  const { t } = fakeTree({ onFilter: (q) => filters.push(q) });
  await t.openTo('/', '/Users/ann');
  t.key(key('m')); t.key(key('u'));
  expect(t.query).toBe('mu');
  expect(t.nodes.map((n) => n.name)).toEqual(['/', 'Users', 'ann', 'Music']);
  expect(t.cur.name).toBe('Music');
  t.key(key('Backspace'));
  expect(t.query).toBe('m');
  expect(t.key(key('Escape'))).toBe(true);
  expect(t.query).toBe('');
  expect(filters).toEqual(['m', 'mu', 'm', '']);
});

test('the separate tree panes of each level, and a letter jumps to a folder', async () => {
  const pane = await import('../src/features/treepane.js');
  expect(pane.paneBinds('off')).toEqual([]);
  expect(pane.paneBinds('one')).toEqual(['active']);
  expect(pane.paneBinds('two')).toEqual(['left', 'right']);
  expect(pane.treePaneOn()).toBe(false); // (off by default: Show > Separate tree keeps using the panels' tree view)
  const { t } = fakeTree({ filter: false });
  await t.openTo('/', '/Users');
  expect(pane.jumpTo(t, 'a')).toBe(true);
  expect(t.cur.name).toBe('ann');
  pane.jumpTo(t, 'a');
  expect(t.cur.name).toBe('Applications');
  expect(pane.jumpTo(t, 'q')).toBe(false);
});

test('F1 in a dialog opens its own help section when there is one, else the general page (F-145)', async () => {
  const help = await import('../src/features/config/help.js');
  expect(help.dialogSection('copy')).toEqual({ topic: 'dialogs', anchor: help.helpSlug('Copy and move (F5, F6)') });
  expect(help.helpSlug('Copy and move (F5, F6)')).toBe('dlg-copy-and-move');
  expect(help.dialogSection('config')).toEqual({ topic: 'settings', anchor: '' });
  expect(help.dialogSection('view-mode-edit').anchor).toBe('dlg-view-modes');
  expect(help.dialogSection('no-such-dialog')).toBe(null);
  expect(help.dialogSection('')).toBe(null);
  const page = help.dialogsPage('dlg-mkdir'.replace('mkdir', 'new-folder'));
  expect(page.indexOf('mc-help-here')).toBeGreaterThan(-1);
  expect(page.indexOf('New folder')).toBeLessThan(page.indexOf('Copy and move'));
  expect(help.dialogsPage('')).not.toContain('mc-help-here');
});
