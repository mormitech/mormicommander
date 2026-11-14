// Tests of features/config/optbridge.js (board issue #94): the Options dialog's refresh, drag and drop, history size,
// log file and folder tab options show what the features really use (their own defaults when nothing is stored) and
// write where the features read. A stand-in app keeps the settings in a plain object.
import { expect, test } from 'vitest';
import { MORE_BRIDGE, boolKey, intKey, excludeText, tabShowText, tabShowPart, tabWidthText, tabWidthPart } from '../src/features/config/optbridge.js';
import { TAB_DEFAULTS } from '../src/features/tabs-defaults.js';

function fakeApp(start = {}) {
  const data = { ...start };
  return {
    data,
    setting: (k, def) => (data[k] === undefined || data[k] === null ? def : data[k]),
    setSetting: (k, v) => { data[k] = v; return Promise.resolve(); },
  };
}

test('with nothing stored, the dialog shows the features\' own defaults', () => {
  const app = fakeApp();
  const want = {
    'Refresh.AutoRefresh': '1', 'Refresh.OnlyForeground': '0', 'Refresh.RefreshNetwork': '1', 'Refresh.ExcludeFolders': '',
    'DragDrop.ConfirmDrop': '1', 'DragDrop.SpringLoaded': '0', 'DragDrop.SpringDelayMs': '1000', 'DragDrop.NativeDragOut': '0', 'Hotlist.HistorySize': '30', 'Hotlist.TypeToFilter': '0', 'Operation.UndoJournal': '1',
    'Log.Enabled': '0', 'Log.File': '', 'FolderTabs.Show': '1', 'FolderTabs.MultiLine': '0', 'FolderTabs.CloseButton': '1',
    'FolderTabs.DoubleClickOpen': '1', 'FolderTabs.NewNextToCur': '1', 'FolderTabs.ConfirmCloseAll': '1', 'FolderTabs.LockedIcon': '1',
    'FolderTabs.MaxTitleChars': '0',
  };
  for (const [name, v] of Object.entries(want)) expect([name, MORE_BRIDGE[name].get(app)]).toEqual([name, v]);
  expect(Object.keys(MORE_BRIDGE).sort()).toEqual(Object.keys(want).sort());
  expect(app.data).toEqual({}); // reading writes nothing
});

test('yes/no keys read the way the features do', () => {
  const on = boolKey('autoRefresh', true), off = boolKey('springLoaded', false);
  expect(on.get(fakeApp({ autoRefresh: false }))).toBe('0');
  expect(on.get(fakeApp({ autoRefresh: 0 }))).toBe('1'); // (system.js: "!== false")
  expect(off.get(fakeApp({ springLoaded: true }))).toBe('1');
  expect(off.get(fakeApp({ springLoaded: 0 }))).toBe('0');
  const app = fakeApp();
  on.set(app, '0');
  off.set(app, '1');
  expect(app.data).toEqual({ autoRefresh: false, springLoaded: true });
});

test('numbers fall back to the feature default', () => {
  const k = intKey('springDelay', 1000);
  expect(k.get(fakeApp({ springDelay: 250 }))).toBe('250');
  expect(k.get(fakeApp({ springDelay: 'x' }))).toBe('1000');
  const app = fakeApp();
  k.set(app, '400');
  expect(app.data.springDelay).toBe(400);
  k.set(app, '');
  expect(app.data.springDelay).toBe(1000);
});

test('excluded folders are shown as one line and stored as text', () => {
  expect(excludeText(['/a', '/b'])).toBe('/a;/b');
  expect(excludeText('/a\n/b')).toBe('/a\n/b');
  expect(excludeText(undefined)).toBe('');
  const app = fakeApp({ refreshExclude: ['/x'] });
  expect(MORE_BRIDGE['Refresh.ExcludeFolders'].get(app)).toBe('/x');
  MORE_BRIDGE['Refresh.ExcludeFolders'].set(app, '/x;/y');
  expect(app.data.refreshExclude).toBe('/x;/y');
});

test('the tab bar choice maps onto "show" and "showOne"', () => {
  expect(tabShowText({ show: false, showOne: true })).toBe('0');
  expect(tabShowText({ show: true, showOne: false })).toBe('1');
  expect(tabShowText({ show: true, showOne: true })).toBe('2');
  for (const t of ['0', '1', '2']) expect(tabShowText({ ...TAB_DEFAULTS, ...tabShowPart(t) })).toBe(t);
});

test('the tab title length maps onto "limitWidth" and "widthLimit"', () => {
  expect(tabWidthText({ limitWidth: false, widthLimit: 20 })).toBe('0');
  expect(tabWidthText({ limitWidth: true, widthLimit: 20 })).toBe('20');
  expect(tabWidthPart('0')).toEqual({ limitWidth: false });
  expect(tabWidthPart('1')).toEqual({ limitWidth: true, widthLimit: 3 }); // (tabs.js cuts to at least 3)
  expect(tabWidthPart('40')).toEqual({ limitWidth: true, widthLimit: 40 });
});

test('tab options are merged into "folderTabs", keeping the others', () => {
  const app = fakeApp({ folderTabs: { lockedChange: 2, multiLine: true } });
  expect(MORE_BRIDGE['FolderTabs.MultiLine'].get(app)).toBe('1');
  MORE_BRIDGE['FolderTabs.CloseButton'].set(app, '0');
  MORE_BRIDGE['FolderTabs.Show'].set(app, '2');
  MORE_BRIDGE['FolderTabs.MaxTitleChars'].set(app, '25');
  expect(app.data.folderTabs).toEqual({ lockedChange: 2, multiLine: true, closeButton: false, show: true, showOne: true, limitWidth: true, widthLimit: 25 });
  expect(MORE_BRIDGE['FolderTabs.CloseButton'].get(app)).toBe('0');
  expect(MORE_BRIDGE['FolderTabs.Show'].get(app)).toBe('2');
  expect(MORE_BRIDGE['FolderTabs.MaxTitleChars'].get(app)).toBe('25');
});

test('the log file options write the keys the Go log reads', () => {
  const app = fakeApp();
  MORE_BRIDGE['Log.Enabled'].set(app, '1');
  MORE_BRIDGE['Log.File'].set(app, '/tmp/x.log');
  expect(app.data).toEqual({ enableLog: true, logFile: '/tmp/x.log' });
  expect(MORE_BRIDGE['Log.Enabled'].get(app)).toBe('1');
  expect(MORE_BRIDGE['Log.File'].get(app)).toBe('/tmp/x.log');
});

test('the undo journal option is on unless the journal was turned off, and writes where feature_undo.go reads', () => {
  const k = MORE_BRIDGE['Operation.UndoJournal'];
  expect(k.get(fakeApp())).toBe('1');
  expect(k.get(fakeApp({ undoJournal: false }))).toBe('0');
  const app = fakeApp();
  k.set(app, '0');
  expect(app.data).toEqual({ undoJournal: false });
  k.set(app, '1');
  expect(app.data).toEqual({ undoJournal: true });
});
