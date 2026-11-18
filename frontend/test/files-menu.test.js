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
  go: {
    SystemLaunch: {
      OpenInternal: (...a) => { goCalls.push(a); return Promise.resolve(goAnswer); },
      OpenInternalVerb: (...a) => { goCalls.push(a); return Promise.resolve(goAnswer); },
    },
    Icons: { FromFiles: (paths) => { goCalls.push(['icons', paths]); return Promise.resolve({ '/i/t.png': 'data:image/png;base64,AA' }); } },
  },
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

const { openAssociated, internalAssociations, parseAssoc, serializeAssoc, viewEditAssociated, assocIcon, wireDialog } = await import('../src/features/assoc.js');
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

test('association text: old one-line entries keep working, indented view/edit/icon lines add to the entry above (F-323)', () => {
  const old = '# notes\n*.txt *.md = code "%1"\n*.log = cm_List\n';
  expect(parseAssoc(old)).toEqual([
    { masks: '*.txt *.md', open: 'code "%1"', view: '', edit: '', icon: '', desc: '' },
    { masks: '*.log', open: 'cm_List', view: '', edit: '', icon: '', desc: '' },
  ]);
  expect(serializeAssoc(parseAssoc(old))).toBe('*.txt *.md = code "%1"\n*.log = cm_List\n');
  const text = '*.txt = code "%1"\n  view = cm_List\n  edit = myedit %1\n  icon = 📝\n*.png =\n  icon = /i/t.png\nview = x\n';
  const e = parseAssoc(text);
  expect(e).toHaveLength(3);
  expect(e[0]).toMatchObject({ masks: '*.txt', open: 'code "%1"', view: 'cm_List', edit: 'myedit %1', icon: '📝' });
  expect(e[1]).toMatchObject({ masks: '*.png', open: '', icon: '/i/t.png' });
  expect(e[2]).toMatchObject({ masks: 'view', open: 'x' }); // (not indented: an entry of its own)
  expect(serializeAssoc(e)).toBe('*.txt = code "%1"\n  view = cm_List\n  edit = myedit %1\n  icon = 📝\n*.png =\n  icon = /i/t.png\nview = x\n');
  expect(serializeAssoc([])).toBe('');
});

test('F3 and F4 use an entry with a View or Edit command; none with that verb leaves them as they are (F-323)', async () => {
  expect(await viewEditAssociated(app, 'view', '/d/a.txt', '/d')).toBe(false);
  settings.internalAssociations = '*.txt = code "%1"';
  expect(await viewEditAssociated(app, 'view', '/d/a.txt', '/d')).toBe(false);
  expect(goCalls).toEqual([]); // (no entry has a view command: Go is not asked)
  settings.internalAssociations = '*.txt = code "%1"\n  view = cm_List\n  edit = myedit "%1"';
  goAnswer = { handled: true, internal: 'cm_List' };
  expect(await viewEditAssociated(app, 'view', '/d/a.txt', '/d')).toBe(true);
  expect(goCalls).toEqual([['view', '/d/a.txt', '/d']]);
  expect(ran).toEqual([['cm_List', { noAssoc: true }]]); // (the built-in viewer then, not the association again)
  expect(await viewEditAssociated(app, 'view', '/d/a.txt', '/d', { noAssoc: true })).toBe(false);
  expect(await viewEditAssociated(app, 'edit', 'sftp://h/a.txt', '/d')).toBe(false);
  goAnswer = { handled: false };
  expect(await viewEditAssociated(app, 'edit', '/d/b.jpg', '/d')).toBe(false);
});

test('an entry with an icon gives matching files that icon; other files keep theirs (F-323)', async () => {
  vi.useFakeTimers();
  const rendered = [];
  app.panels = [{ renderRows: () => rendered.push(1) }];
  settings.internalAssociations = '';
  expect(assocIcon(app, {}, { name: 'a.txt' })).toBe('');
  settings.internalAssociations = '*.txt = x\n  icon = 📝\n*.png =\n  icon = /i/t.png';
  expect(assocIcon(app, {}, { name: 'a.txt' })).toContain('📝');
  expect(assocIcon(app, {}, { name: 'dir.txt', dir: true })).toBe('');
  expect(assocIcon(app, {}, { name: 'a.jpg' })).toBe('');
  expect(assocIcon(app, {}, { name: 'b.png' })).toBe(''); // (asked for; the usual icon until it comes)
  await vi.runAllTimersAsync();
  expect(goCalls).toEqual([['icons', ['/i/t.png']]]);
  expect(rendered.length).toBe(1);
  expect(assocIcon(app, {}, { name: 'b.png' })).toContain('data:image/png;base64,AA');
  vi.useRealTimers();
  delete app.panels;
});

test('the dialog list edits the entries and writes the text that is saved (F-323)', () => {
  const box = document.createElement('form');
  box.innerHTML = `<select id="as-list"></select>${['masks', 'open', 'view', 'edit', 'icon', 'desc'].map((k) => `<input id="as-${k}">`).join('')}
    <button class="as-add"></button><button class="as-del"></button><button class="as-up"></button><button class="as-down"></button><textarea id="assoc">*.log = cm_List\n</textarea>`;
  wireDialog(box);
  const $ = (s) => box.querySelector(s);
  expect($('#as-masks').value).toBe('*.log');
  $('.as-add').click();
  $('#as-masks').value = '*.txt'; $('#as-masks').dispatchEvent(new Event('input'));
  $('#as-view').value = 'cm_List'; $('#as-view').dispatchEvent(new Event('input'));
  expect($('#assoc').value).toBe('*.log = cm_List\n*.txt =\n  view = cm_List\n');
  $('.as-up').click();
  expect($('#assoc').value).toBe('*.txt =\n  view = cm_List\n*.log = cm_List\n');
  $('.as-del').click();
  expect($('#assoc').value).toBe('*.log = cm_List\n');
  $('#assoc').value = '*.a = x\n*.b = y\n'; $('#assoc').dispatchEvent(new Event('input'));
  expect([...$('#as-list').options].map((o) => o.textContent)).toEqual(['*.a', '*.b']);
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
