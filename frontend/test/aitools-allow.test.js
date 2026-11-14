// @vitest-environment happy-dom
// The allow-lists of the AI tools (features/aitools-allow.js, issue #146 step 3). Every command on the list must exist
// once all features are loaded (so a rename shows up here, not as a refusal in the real app), and the commands that
// delete, move, rename, copy, edit, pack, run programs or quit must be refused, as must a command nobody reviewed.
// Every option on the settings list must exist in internal/config/settings.go (read as text here) and pass its own
// checks, and the options about programs, paths, confirmations, the trash, the network and the AI must be refused.
// Go is a stand-in that answers nothing; the timers are frozen so the menus are never sent anywhere.
import { beforeAll, expect, test, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});

// (the help page reads docs/keyboard.md, which a worktree's dev server may not serve: the text is not needed here)
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

const { ALLOWED_COMMANDS, ALLOWED_FAMILIES, DENIED_COMMANDS, ALLOWED_SETTINGS, TEXT_FIELDS, commandVerdict, settingVerdict, allowedSettingNames } = await import('../src/features/aitools-allow.js');

let names = new Set();
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  const { commandList } = await import('../src/core/registry.js');
  names = new Set(commandList().map((c) => c.name));
  vi.useRealTimers();
});

test('every allow-listed command exists in the registry, and every family has members', () => {
  expect(names.size).toBeGreaterThan(500); // (the features were really loaded)
  const missing = Object.keys(ALLOWED_COMMANDS).filter((n) => !names.has(n));
  expect(missing).toEqual([]);
  for (const f of ALLOWED_FAMILIES) expect([...names].some((n) => f.re.test(n)), f.name).toBe(true);
  for (const n of Object.keys(ALLOWED_COMMANDS)) expect(commandVerdict(n, true).ok, n).toBe(true);
  expect(commandVerdict('cm_SrcActivateTab3', true).ok).toBe(true);
  expect(commandVerdict('cm_RightSortByCol12', true).ok).toBe(true);
});

test('every allow-listed command has a reason', () => {
  for (const [n, why] of Object.entries(ALLOWED_COMMANDS)) expect(why.length, n).toBeGreaterThan(5);
});

const DANGEROUS = [
  'cm_Delete', 'cm_DeletePermanent', 'cm_Wipe', 'cm_RenMov', 'cm_RenameOnly', 'cm_MultiRenameFiles', 'cm_Copy', 'cm_CopySamepanel',
  'cm_PackFiles', 'cm_PackFilesMove', 'cm_UnpackFiles', 'cm_Edit', 'cm_EditNewFile', 'cm_Enter', 'cm_EnterDir', 'cm_OpenWith',
  'cm_ExecuteDOS', 'cm_ExecuteCmdLine', 'cm_RunKeepOpen', 'cm_Exit', 'cm_Config', 'cm_ConfigChangeIniFiles', 'cm_PasteFromClipboard',
  'cm_CutToClipboard', 'cm_SetAttrib', 'cm_UndoFileOp', 'cm_SyncDirs', 'cm_ClearThumbnailCache', 'cm_FtpSendCommand', 'cm_MkDir',
  'cm_FileSplit', 'cm_FileCombine', 'cm_CreateSymbolicLink', 'cm_CreateHardLink', 'cm_UUDecode', 'cm_CloseAllTabs', 'cm_SrcQuickView',
  'cm_AIAsk', 'cm_Explorer', 'cm_ContextMenu', 'cm_LoadSelectionFromFile', 'cm_SaveSelectionToFile', 'cm_DirectoryHotlist', 'cm_FocusCmdLine',
];

test('dangerous commands are refused, with a reason, whether or not they exist', () => {
  for (const n of DANGEROUS) {
    expect(Object.hasOwn(ALLOWED_COMMANDS, n), n).toBe(false);
    for (const exists of [true, false]) {
      const v = commandVerdict(n, exists);
      expect(v.ok, n).toBe(false);
      expect(v.reason.length, n).toBeGreaterThan(10);
    }
  }
  expect(commandVerdict('cm_Delete', true).reason).toMatch(/deletes files/);
  expect(commandVerdict('cm_EnterDir', true).reason).toMatch(/open_folder/);
  for (const n of Object.keys(DENIED_COMMANDS)) expect(commandVerdict(n, true).ok, n).toBe(false);
});

test('no allow-listed name has a word that means changing files or running something', () => {
  const bad = Object.keys(ALLOWED_COMMANDS).filter((n) => /Delete|Wipe|Rename|RenMov|Exec|Edit|Pack|Paste|Exit|MkDir|Attrib|Undo|Sync|Ftp/.test(n));
  expect(bad).toEqual([]);
});

test('unknown and unreviewed commands are refused', () => {
  expect(commandVerdict('cm_SomethingNew', true)).toMatchObject({ ok: false });
  expect(commandVerdict('cm_SomethingNew', true).reason).toMatch(/not on the list/);
  expect(commandVerdict('cm_DeleteEverything', true).reason).toMatch(/left out/);
  expect(commandVerdict('rm -rf /', false).reason).toMatch(/no command called/);
  expect(commandVerdict('cm_GoToParent', false)).toMatchObject({ ok: false }); // allowed, but not in this build
  // every registered command that is not on the list is refused (the list is the only way in)
  for (const n of names) if (!Object.hasOwn(ALLOWED_COMMANDS, n) && !ALLOWED_FAMILIES.some((f) => f.re.test(n))) expect(commandVerdict(n, true).ok, n).toBe(false);
});

// ---- options ----

// The options as settings.go defines them: {name: {name, type, label, noEffect}}, the page part being the Settings
// field's name ("CopyDelete" for the "Copy/Delete" page).
function goFields() {
  const root = resolve(__dirname, '../..');
  const src = readFileSync(resolve(root, 'internal/config/settings.go'), 'utf8');
  const noEffect = new Set([...readFileSync(resolve(root, 'internal/config/noeffect.go'), 'utf8').matchAll(/"(\w+\.\w+)":\s*true/g)].map((m) => m[1]));
  const pageOf = {};
  const top = src.slice(src.indexOf('type Settings struct'), src.indexOf('}', src.indexOf('type Settings struct')));
  for (const m of top.matchAll(/^\s+(\w+)\s+(\w+Page)\s+`page:/gm)) pageOf[m[2]] = m[1];
  const out = {};
  for (const m of src.matchAll(/type (\w+Page) struct \{([\s\S]*?)\n\}/g)) {
    const page = pageOf[m[1]];
    if (!page) continue;
    for (const f of m[2].matchAll(/^\s+(\w+)\s+(bool|int|string)\s+`[^`]*label:"((?:[^"\\]|\\.)*)"/gm)) {
      const name = `${page}.${f[1]}`;
      out[name] = { name, type: f[2], label: f[3], noEffect: noEffect.has(name) };
    }
  }
  return out;
}
const fields = goFields();

test('every allow-listed option exists, and passes its checks unless it has no effect yet', () => {
  expect(Object.keys(fields).length).toBeGreaterThan(200);
  const missing = allowedSettingNames().filter((n) => !fields[n]);
  expect(missing).toEqual([]);
  for (const n of allowedSettingNames()) {
    const v = settingVerdict(n, fields[n]);
    if (fields[n].noEffect) expect(v, n).toMatchObject({ ok: false, reason: expect.stringMatching(/no effect/) });
    else expect(v.ok, `${n}: ${v.reason}`).toBe(true);
  }
  // every text option on the list has a pattern
  for (const n of allowedSettingNames()) if (fields[n].type === 'string') expect(TEXT_FIELDS[n.replace('.', ':')], n).toBeTruthy();
  expect(Object.keys(ALLOWED_SETTINGS)).toContain('Colors');
});

test('options about programs, paths, confirmations, the trash, the network and the AI are refused', () => {
  const refused = [
    'CopyDelete.UseTrash', 'CopyDelete.ConfirmDelete', 'CopyDelete.ConfirmOverwrite', 'CopyDelete.OverwriteDefault', 'ViewerEditor.Editor',
    'ViewerEditor.Viewer', 'Plugins.SevenZipPath', 'Plugins.UseExternalTools', 'AI.ClaudePath', 'AI.ClaudeModel', 'AI.Provider', 'Log.File',
    'Log.Enabled', 'Proxy.Host', 'Proxy.Type', 'FTP.DownloadDir', 'FTP.AnonymousEmail', 'Misc.ConfirmExit', 'Misc.MainMenuFile',
    'FolderTabs.ConfirmCloseAll', 'FolderTabs.RestoreTabs', 'Icons.IconsOnNet', 'Lister.ExtViewerFor', 'Operation.ArchiveEnter',
    'Packer.MoveToArchive', 'Packer.ExternalZip', 'Thumbnails.CacheDir', 'Tree.ShowNetwork', 'Tooltips.ShowOnNet', 'DragDrop.ConfirmDrop',
    'IgnoreList.File', 'Refresh.ExcludeFolders', 'Zip.TempDir', 'Colors.Scheme', 'Language.LanguageFile',
  ];
  for (const n of refused) {
    expect(fields[n], n).toBeTruthy();
    expect(settingVerdict(n, fields[n]).ok, n).toBe(false);
  }
  expect(settingVerdict('Nope.Nothing', null).reason).toMatch(/no option called/);
  // the deny rules hold even if someone puts a risky option on the list by mistake
  ALLOWED_SETTINGS.Lister.fields.push('ExtViewerFor');
  try { expect(settingVerdict('Lister.ExtViewerFor', fields['Lister.ExtViewerFor']).ok).toBe(false); } finally { ALLOWED_SETTINGS.Lister.fields.pop(); }
});
