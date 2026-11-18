// @vitest-environment happy-dom
// The Files menu (issue #60, FEATURES.md section 12): every entry runs its cm_* command, the ones with a key show it
// (F-320, F-324, F-326, F-336 ...), and Internal associations (features/assoc.js, F-323) is used by Enter before the
// system's app only when the user set some. The registry, the Go side and the dialogs are stand-ins.
import { beforeEach, expect, test, vi } from 'vitest';

const cmds = {};
const goCalls = [];
let goAnswer = null;
let dialogValue = null;
let settings = {};
const ran = [];
const app = {
  setting: (k, d) => (settings[k] === undefined ? d : settings[k]),
  setSetting: (k, v) => { settings[k] = v; return Promise.resolve(); },
  run: (name, args) => { ran.push([name, args]); return Promise.resolve(); },
  flash: () => {},
};
vi.mock('../src/core/registry.js', () => ({
  command: (c) => { cmds[c.name] = c; }, getCommand: (n) => cmds[n], hasCommand: () => false, commandList: () => [], onChange: () => {},
}));
vi.mock('../src/core/app.js', () => ({ app, LABELS: {} }));
vi.mock('../src/core/go.js', () => ({
  on: () => {},
  go: { SystemLaunch: { OpenInternal: (...a) => { goCalls.push(a); return Promise.resolve(goAnswer); } } },
  errText: String,
}));
vi.mock('../src/core/dialogs.js', () => ({
  esc: (s) => String(s),
  inputDialog: () => Promise.resolve(null),
  formDialog: (title, html, read) => {
    const box = document.createElement('form');
    box.innerHTML = html;
    if (dialogValue === null) return Promise.resolve(null);
    box.querySelector('#assoc').value = dialogValue;
    return Promise.resolve(read(box));
  },
}));
vi.mock('../src/core/popup.js', () => ({ showList: () => Promise.resolve(null) }));

const { openAssociated, internalAssociations } = await import('../src/features/assoc.js');
const { MENUS } = await import('../src/core/menus.js');
const { readFileSync } = await import('node:fs');
const { resolve } = await import('node:path');

beforeEach(() => { goCalls.length = 0; ran.length = 0; settings = {}; goAnswer = null; dialogValue = null; });

test('without internal associations Enter does not ask Go and opens the file as before', async () => {
  expect(await openAssociated(app, { name: 'a.txt' }, '/d/a.txt', '/d')).toBe(false);
  settings.internalAssociations = '   \n';
  expect(await openAssociated(app, { name: 'a.txt' }, '/d/a.txt', '/d')).toBe(false);
  expect(goCalls).toEqual([]);
});

test('a matching association is carried out by Go; an internal command runs here with its arguments', async () => {
  settings.internalAssociations = '*.txt = myeditor "%1"';
  goAnswer = { handled: true };
  expect(await openAssociated(app, { name: 'a.txt' }, '/d/a.txt', '/d')).toBe(true);
  expect(goCalls).toEqual([['/d/a.txt', '/d']]);
  goAnswer = { handled: true, internal: 'cm_MultiRename', args: '/T' };
  expect(await openAssociated(app, { name: 'a.txt' }, '/d/a.txt', '/d')).toBe(true);
  expect(ran).toEqual([['cm_MultiRenameFiles', { params: '/T' }]]);
  goAnswer = { handled: false };
  expect(await openAssociated(app, { name: 'b.jpg' }, '/d/b.jpg', '/d')).toBe(false);
});

test('the dialog saves the lines, blank text clears them, Cancel keeps them', async () => {
  dialogValue = '*.log = cm_List\n';
  expect(await cmds.cm_InternalAssociate.run(app)).toBe(true);
  expect(settings.internalAssociations).toBe('*.log = cm_List\n');
  dialogValue = null;
  expect(await internalAssociations(app)).toBe(false);
  expect(settings.internalAssociations).toBe('*.log = cm_List\n');
  dialogValue = '  ';
  await internalAssociations(app);
  expect(settings.internalAssociations).toBe('');
});

test('the Files menu lists its commands, and the ones with a key show it', () => {
  const files = MENUS.find(([l]) => l === 'Files')[1].filter(Array.isArray).map((it) => it[1]);
  for (const n of ['cm_SetAttrib', 'cm_PackFiles', 'cm_UnpackFiles', 'cm_TestArchive', 'cm_CompareFilesByContent', 'cm_InternalAssociate',
    'cm_Properties', 'cm_GetFileSpace', 'cm_MultiRenameFiles', 'cm_EditComment', 'cm_FileSplit', 'cm_FileCombine', 'cm_UUEncode',
    'cm_UUDecode', 'cm_CRCCreate', 'cm_CRCCheck', 'cm_CreateSymbolicLink', 'cm_CreateHardLink', 'cm_Wipe', 'cm_PrintDir', 'cm_PrintDirSub', 'cm_Exit']) {
    expect(files).toContain(n);
  }
  // (the features that register these are big; their command({...}) lines are checked in the source)
  const src = (f) => readFileSync(resolve(__dirname, '../src/features', f), 'utf8');
  const keyed = { 'vfs.js': ['cm_PackFiles', 'cm_UnpackFiles', 'cm_TestArchive'], 'tools/props.js': ['cm_Properties'], 'tools/space.js': ['cm_GetFileSpace'],
    'multirename.js': ['cm_MultiRenameFiles'], 'views.js': ['cm_EditComment'], 'window.js': ['cm_Exit'] };
  for (const [f, names] of Object.entries(keyed)) for (const n of names) expect(src(f)).toContain(`name: '${n}', menuKey: true`);
});
