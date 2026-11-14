// More options of the Options dialog wired to the features that already do the work (board issue #94). These options
// were kept in the settings file but nothing read them, while the feature behind each one keeps its own value in
// settings.json under a key of its own (automatic refresh, drag and drop, the folder history, the log file, folder tabs).
// Each entry here joins the two, in the same shape as BRIDGE in util.js (which spreads this list into it): get(app) is
// the value the feature uses now, as the dialog's text ("1"/"0" for yes/no), set(app, text) stores a new value where the
// feature reads it. Unlike the older BRIDGE entries, get() always answers (with the feature's own default when nothing
// is stored), so the dialog shows what the program really does and not a default it never used.
//
// Nothing here changes how a feature behaves: a value is only written when the user changes the option, and it is the
// same value the feature's own dialog would write. The module has no imports but the tab defaults, so the tests can
// run it in Node with a stand-in app.

import { TAB_DEFAULTS } from '../tabs-defaults.js';

const bit = (v) => (v ? '1' : '0');

// boolKey: a yes/no kept in a settings key of its own; def is the feature's default when the key is missing. (The
// features read a default-on key as "!== false" and a default-off key as "!!", which is what this does.)
export const boolKey = (key, def) => ({
  get: (app) => { const v = app.setting(key, undefined); return bit(v === undefined || v === null ? def : (def ? v !== false : !!v)); },
  set: (app, text) => app.setSetting(key, text === '1'),
});

// intKey: a number kept in a settings key of its own (the features read it as "+value || def").
export const intKey = (key, def) => ({
  get: (app) => String(+app.setting(key, def) || def),
  set: (app, text) => app.setSetting(key, parseInt(text, 10) || def),
});

// strKey: a text kept in a settings key of its own ("" = the feature's default).
export const strKey = (key) => ({
  get: (app) => String(app.setting(key, '') ?? ''),
  set: (app, text) => app.setSetting(key, String(text ?? '')),
});

// The folders left out of automatic refresh: the feature takes a list or one text with ";" or new lines between them.
export const excludeText = (v) => (Array.isArray(v) ? v.join(';') : String(v ?? ''));

// ---- folder tabs: the settings key "folderTabs" (tabs.js), an object of options ----
const tabs = (app) => ({ ...TAB_DEFAULTS, ...(app.setting('folderTabs', {}) || {}) });
const mergeTabs = (app, part) => app.setSetting('folderTabs', { ...(app.setting('folderTabs', {}) || {}), ...part });
export const tabBool = (k) => ({ get: (app) => bit(tabs(app)[k]), set: (app, text) => mergeTabs(app, { [k]: text === '1' }) });

// "Show tab bar": 0 never, 1 when there is more than one tab, 2 always (tabs.js: "show" and "showOne").
export const tabShowText = (o) => (!o.show ? '0' : o.showOne ? '2' : '1');
export const tabShowPart = (text) => (text === '0' ? { show: false } : { show: true, showOne: text === '2' });

// "Maximum tab title length (0 = unlimited)": tabs.js cuts captions only with "limitWidth", to "widthLimit" (at least 3).
export const tabWidthText = (o) => (o.limitWidth ? String(Math.max(3, +o.widthLimit || 32)) : '0');
export const tabWidthPart = (text) => { const n = parseInt(text, 10) || 0; return n > 0 ? { limitWidth: true, widthLimit: Math.max(3, n) } : { limitWidth: false }; };

export const MORE_BRIDGE = {
  // automatic refresh (features/system.js, watchOptions; it is set again on the "settings" event the dialog sends)
  'Refresh.AutoRefresh': boolKey('autoRefresh', true),
  'Refresh.OnlyForeground': boolKey('refreshOnlyActive', false),
  'Refresh.RefreshNetwork': boolKey('refreshNetwork', true),
  'Refresh.ExcludeFolders': { get: (app) => excludeText(app.setting('refreshExclude', '')), set: (app, text) => app.setSetting('refreshExclude', String(text ?? '')) },
  // drag and drop (features/system-dnd.js reads these at each drop and hover)
  'DragDrop.ConfirmDrop': boolKey('confirmDrop', true),
  'DragDrop.SpringLoaded': boolKey('springLoaded', false),
  'DragDrop.SpringDelayMs': intKey('springDelay', 1000),
  // native file drag-out (features/system-dnd.js reads it when a drag starts; board issue #108)
  'DragDrop.NativeDragOut': boolKey('nativeDragOut', false),
  // the folder history (features/hotlist.js and tabs.js; the history dialog changes the same key)
  'Hotlist.HistorySize': intKey('historyLength', 30),
  // the log file (feature_system_watch.go, SystemLog.Write; "" = mormicommander.log in the settings folder)
  'Log.Enabled': boolKey('enableLog', false),
  'Log.File': strKey('logFile'),
  // folder tabs (features/tabs.js; the Folder tabs settings dialog changes the same key, and the bar is drawn again
  // on the "settings" event)
  'FolderTabs.Show': { get: (app) => tabShowText(tabs(app)), set: (app, text) => mergeTabs(app, tabShowPart(text)) },
  'FolderTabs.MultiLine': tabBool('multiLine'),
  'FolderTabs.CloseButton': tabBool('closeButton'),
  'FolderTabs.DoubleClickOpen': tabBool('dblClickClose'),
  'FolderTabs.NewNextToCur': tabBool('nextToCurrent'),
  'FolderTabs.ConfirmCloseAll': tabBool('confirmCloseAll'),
  'FolderTabs.LockedIcon': tabBool('starLocked'),
  'FolderTabs.MaxTitleChars': { get: (app) => tabWidthText(tabs(app)), set: (app, text) => mergeTabs(app, tabWidthPart(text)) },
};
