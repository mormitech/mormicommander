// @vitest-environment happy-dom
// Delete (issue #69, docs/FEATURES.md section 21): F8/Delete and Shift+F8/Shift+Delete run the delete commands, Wipe
// and the trash commands sit in the Files menu, none of them is reachable from the AI tools, a drive without a trash
// is recognised (F-833) and only changes the question when Options > Copy/Delete "Say when a drive has no trash" is on,
// and a put-back from the trash is summed up (F-501). Go is a stand-in and every question is answered Cancel, so
// nothing is ever deleted.
import { beforeAll, beforeEach, expect, test, vi } from 'vitest';
import { noTrashReason, restoreReport } from '../src/features/trash-lib.js';

const asked = [];
const goState = { trashSupported: true };
vi.mock('../src/core/go.js', () => {
  const svc = (name) => new Proxy({}, {
    get: (_, m) => () => {
      if (name === 'Jobs' && m === 'TrashSupported') return Promise.resolve(goState.trashSupported);
      if (name === 'Jobs' && m === 'Start') return Promise.reject(new Error('no jobs in this test'));
      return Promise.resolve(null);
    },
  });
  const go = new Proxy({}, { get: (_, name) => svc(name) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));
vi.mock('../src/core/dialogs.js', async (orig) => ({
  ...(await orig()),
  messageDialog: async (o) => { asked.push(o); return 'Cancel'; },
}));

let reg, menus, keymap, allow;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  keymap = await import('../src/core/keymap.js');
  allow = await import('../src/features/aitools-allow.js');
  vi.useRealTimers();
});
beforeEach(() => { asked.length = 0; goState.trashSupported = true; });

test('the delete keys run the delete commands (F-493, F-494)', () => {
  expect(keymap.DEFAULT_KEYS.F8).toBe('cm_Delete');
  expect(keymap.DEFAULT_KEYS.Delete).toBe('cm_Delete');
  expect(keymap.DEFAULT_KEYS['Shift+F8']).toBe('cm_DeletePermanent');
  expect(keymap.DEFAULT_KEYS['Shift+Delete']).toBe('cm_DeletePermanent');
});

test('Wipe, undo and the trash commands sit together in the Files menu (F-500, F-501)', () => {
  const files = menus.MENUS.find(([l]) => l === 'Files')[1];
  expect(files.some((it) => Array.isArray(it) && it[1] === 'cm_Wipe')).toBe(true);
  expect(reg.getCommand('cm_GoToTrash')).toMatchObject({ menu: 'Files', menuAfter: 'cm_Wipe' });
  expect(reg.getCommand('cm_RestoreFromTrash')).toMatchObject({ menu: 'Files', menuAfter: 'cm_GoToTrash' });
  // registered before undo.js, so the undo entries (also after Wipe) end up between Wipe and these two
  const order = reg.commandList().map((c) => c.name);
  expect(order.indexOf('cm_GoToTrash')).toBeLessThan(order.indexOf('cm_UndoFileOp'));
  expect(reg.getCommand('cm_UndoFileOp')).toMatchObject({ menu: 'Files', menuAfter: 'cm_Wipe' });
});

test('no delete or trash command is reachable from the AI tools', () => {
  for (const n of ['cm_Delete', 'cm_DeletePermanent', 'cm_Wipe', 'cm_RestoreFromTrash', 'cm_GoToTrash']) {
    expect(allow.ALLOWED_COMMANDS[n], n).toBeUndefined();
    expect(allow.commandVerdict(n, true).ok, n).toBe(false);
  }
});

test('drives without a trash (F-833)', () => {
  expect(noTrashReason('darwin', { kind: 'network' })).toMatch(/network/);
  expect(noTrashReason('linux', { kind: 'network' })).toMatch(/network/);
  expect(noTrashReason('windows', { kind: 'removable' })).toMatch(/removable/);
  expect(noTrashReason('darwin', { kind: 'removable' })).toBe(''); // macOS makes .Trashes on a USB stick
  expect(noTrashReason('linux', { kind: 'removable' })).toBe('');
  expect(noTrashReason('windows', { kind: 'fixed' })).toBe('');
  expect(noTrashReason('windows', null, '\\\\server\\share\\x')).toMatch(/network/);
  expect(noTrashReason('darwin', null, '/Users/me')).toBe('');
});

test('put-back summary (F-501)', () => {
  expect(restoreReport([{ path: '/t/a' }, { path: '/t/b' }], 'Trash')).toEqual({ text: '2 items put back from the Trash.', err: false });
  expect(restoreReport([{ path: '/t/a' }, { path: '/t/b', error: 'something else is at its old place now' }], 'Trash'))
    .toEqual({ text: '1 item put back. b: something else is at its old place now', err: true });
  expect(restoreReport([{ path: '/t/a', error: 'x' }, { path: '/t/b', error: 'y' }]).text).toBe('Nothing was put back. 2 items stayed in the trash.');
});

// a panel on a network volume with one file under the cursor
function fakeApp({ check, kind = 'network', fileops = {} }) {
  const entry = { name: 'a.txt', dir: false };
  const cur = {
    path: '/Volumes/share', entries: [entry], index: new Map([['a.txt', 0]]), targets: () => [entry],
    fullPath: (n) => `/Volumes/share/${n}`, saveSelection() {}, focusName() {},
  };
  return {
    cur, platform: 'darwin', panels: [cur], flash() {},
    setting: (k, d) => (k === 'fileops' ? fileops : d), setSetting() {},
    volumeFor: () => ({ kind, path: '/Volumes/share' }),
    config: { values: check ? { 'CopyDelete.CheckVolumeTrash': '1' } : {} },
  };
}

test('F8 on a network share: today\'s trash question unless the option is on (F-833)', async () => {
  await reg.getCommand('cm_Delete').run(fakeApp({ check: false }));
  expect(asked).toHaveLength(1);
  expect(asked[0].Buttons[0]).toBe('Move to Trash');
  expect(asked[0].Message).toMatch(/Network volumes often have no Trash/);

  asked.length = 0;
  await reg.getCommand('cm_Delete').run(fakeApp({ check: true }));
  expect(asked).toHaveLength(1);
  expect(asked[0].Title).toBe('Delete permanently');
  expect(asked[0].Message).toMatch(/There is no Trash here \(it is a network share\), so "a.txt" will be deleted permanently/);
  expect(asked[0].Danger).toBe(true);
});

test('with the option on, a drive without a trash asks even when delete confirmation is off (F-833)', async () => {
  await reg.getCommand('cm_Delete').run(fakeApp({ check: true, fileops: { confirmDelete: false } }));
  expect(asked).toHaveLength(1);
  expect(asked[0].Title).toBe('Delete permanently');
});

test('a local drive keeps the trash question with the option on', async () => {
  await reg.getCommand('cm_Delete').run(fakeApp({ check: true, kind: 'fixed' }));
  expect(asked[0].Buttons[0]).toBe('Move to Trash');
});
