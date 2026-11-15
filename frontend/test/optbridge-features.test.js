// Tests of features/config/optbridge-features.js (board issue #94, second round): the Options dialog shows what the
// features really do (their own defaults when nothing is stored, also where the settings file's default differs) and
// writes where the features read, leaving the rest of each settings object alone. A stand-in app keeps the settings in
// a plain object.
import { expect, test, vi } from 'vitest';

// util.js (for the dialog's BRIDGE) reaches the Wails runtime through go.js; the runtime starts a timer on load that
// could fire after this quick file's environment is gone, so the bridge is a stand-in here, as in fitpath.test.js.
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});
import {
  FEATURE_BRIDGE, countKey, confirmOverwrite, renameSelectsName, listerInt, imageFitText, imageFitPart, packerName, packerText,
} from '../src/features/config/optbridge-features.js';
import { BRIDGE } from '../src/features/config/util.js';

function fakeApp(start = {}) {
  const data = JSON.parse(JSON.stringify(start));
  return {
    data,
    setting: (k, def) => (data[k] === undefined || data[k] === null ? def : data[k]),
    setSetting: (k, v) => { data[k] = v; return Promise.resolve(); },
  };
}

test('with nothing stored, the dialog shows what the program does today', () => {
  const app = fakeApp();
  const want = {
    'Operation.RenameSelectsName': '1', 'CopyDelete.ConfirmOverwrite': '1', 'CopyDelete.ConfirmDragDrop': '1', 'DragDrop.DragToOtherApp': '1',
    'Lister.TabWidth': '8', 'Lister.BinaryWidth': '75', 'Font.ListerSize': '12', 'Lister.ImageFit': '1', 'ViewerEditor.Viewer': '',
    'Hotlist.CmdHistSize': '50', 'Hotlist.FrequentSize': '20', 'Hotlist.ShowPaths': '1', 'Packer.DefaultPacker': 'zip',
    'FolderTabs.Bottom': '0', 'FolderTabs.RestoreTabs': '1', 'FolderTabs.ActivateNew': '0', // (Ctrl+Up opens its tab in the background)
    'FavoriteTabs.RestoreBoth': '1', 'FavoriteTabs.CloseOldTabs': '1', 'FavoriteTabs.ConfirmReplac': '0',
    'CustomColumns.AutoSwitch': '1', 'Thumbnails.Width': '96', 'Log.MaxSizeKB': '0',
  };
  for (const [name, v] of Object.entries(want)) expect([name, FEATURE_BRIDGE[name].get(app)]).toEqual([name, v]);
  expect(Object.keys(FEATURE_BRIDGE).sort()).toEqual(Object.keys(want).sort());
  expect(app.data).toEqual({}); // reading writes nothing
  for (const name of Object.keys(want)) expect(BRIDGE[name]).toBe(FEATURE_BRIDGE[name]); // (and the dialog uses them)
});

test('Shift+F6 selecting the name is the opposite of fileops.renameSelectExt', () => {
  expect(renameSelectsName.get(fakeApp({ fileops: { renameSelectExt: true } }))).toBe('0');
  const app = fakeApp({ fileops: { useTrash: false } });
  renameSelectsName.set(app, '0');
  expect(app.data.fileops).toEqual({ useTrash: false, renameSelectExt: true });
  renameSelectsName.set(app, '1');
  expect(app.data.fileops).toEqual({ useTrash: false, renameSelectExt: false });
});

test('confirm overwriting is the "ask" policy, and other policies are left alone', () => {
  expect(confirmOverwrite.get(fakeApp({ fileops: { overwriteDefault: 'skip' } }))).toBe('0');
  const app = fakeApp();
  confirmOverwrite.set(app, '0');
  expect(app.data.fileops).toEqual({ overwriteDefault: 'overwrite' });
  confirmOverwrite.set(app, '1');
  expect(app.data.fileops).toEqual({ overwriteDefault: '' });
  const skip = fakeApp({ fileops: { overwriteDefault: 'older' } });
  confirmOverwrite.set(skip, '0'); // already doesn't ask: kept
  expect(skip.data.fileops.overwriteDefault).toBe('older');
});

test('the drop and drag-out options write the keys system-dnd.js reads', () => {
  const app = fakeApp();
  FEATURE_BRIDGE['CopyDelete.ConfirmDragDrop'].set(app, '0');
  FEATURE_BRIDGE['DragDrop.DragToOtherApp'].set(app, '0');
  expect(app.data).toEqual({ confirmDrop: false, dragToOtherApp: false });
  expect(BRIDGE['DragDrop.ConfirmDrop'].get(app)).toBe('0'); // the same question on the Drag & drop page
});

test('Lister numbers stay in the ranges the Lister accepts and keep its other settings', () => {
  const tab = listerInt('tabWidth', 1, 32);
  const app = fakeApp({ lister: { wrap: false, tabWidth: 4 } });
  expect(tab.get(app)).toBe('4');
  tab.set(app, '99');
  expect(app.data.lister).toEqual({ wrap: false, tabWidth: 32 });
  tab.set(app, 'x');
  expect(app.data.lister.tabWidth).toBe(8);
  FEATURE_BRIDGE['Font.ListerSize'].set(app, '6');
  expect(app.data.lister.fontSize).toBe(8);
  FEATURE_BRIDGE['ViewerEditor.Viewer'].set(app, '  /usr/bin/less %1 ');
  expect(app.data.lister.viewer).toBe('/usr/bin/less %1');
});

test('the image size maps onto fitImage and fitGrow', () => {
  expect(imageFitText({ fitImage: false, fitGrow: true })).toBe('0');
  expect(imageFitText({ fitImage: true })).toBe('1');
  expect(imageFitText({ fitImage: true, fitGrow: true })).toBe('2');
  for (const t of ['0', '1', '2']) expect(imageFitText({ fitImage: true, ...imageFitPart(t) })).toBe(t);
});

test('history sizes take 0 where it is a real value and fall back to the old fixed numbers', () => {
  const freq = countKey('frequentLength', 20, 0, 200);
  expect(freq.get(fakeApp({ frequentLength: 0 }))).toBe('0');
  expect(freq.get(fakeApp({ frequentLength: 'x' }))).toBe('20');
  const app = fakeApp();
  freq.set(app, '500');
  FEATURE_BRIDGE['Hotlist.CmdHistSize'].set(app, '0');
  expect(app.data).toEqual({ frequentLength: 200, cmdHistoryLength: 1 });
});

test('the default packer is stored as the Pack dialog names it', () => {
  expect(packerName('TGZ')).toBe('tar.gz');
  expect(packerName('.7z')).toBe('7z');
  expect(packerName('')).toBe('zip');
  expect(packerText('tar.bz2')).toBe('tbz');
  expect(packerText(undefined)).toBe('zip');
  const app = fakeApp({ packer: { format: '7z', level: 5 } });
  expect(FEATURE_BRIDGE['Packer.DefaultPacker'].get(app)).toBe('7z');
  FEATURE_BRIDGE['Packer.DefaultPacker'].set(app, 'txz');
  expect(app.data.packer).toEqual({ format: 'tar.xz', level: 5 });
});

test('tab, favourite, column and thumbnail options merge into their objects', () => {
  const app = fakeApp({ folderTabs: { multiLine: true }, views: { sizeFormat: 'k' } });
  FEATURE_BRIDGE['FolderTabs.Bottom'].set(app, '1');
  FEATURE_BRIDGE['FolderTabs.ActivateNew'].set(app, '1');
  FEATURE_BRIDGE['FavoriteTabs.ConfirmReplac'].set(app, '1');
  expect(app.data.folderTabs).toEqual({ multiLine: true, bottom: true, ctrlUpForeground: true, favConfirm: true });
  FEATURE_BRIDGE['Thumbnails.Width'].set(app, '16');
  expect(app.data.views).toEqual({ sizeFormat: 'k', thumbSize: 32 }); // (views.js draws them from 32 pixels)
  FEATURE_BRIDGE['CustomColumns.AutoSwitch'].set(app, '0');
  expect(app.data.columnsAutoSwitch).toBe(false);
});
