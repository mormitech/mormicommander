// Further options of the Options dialog wired to features that already keep a setting of their own (board issue #94,
// second round). Each entry has the shape of BRIDGE in util.js, which spreads this list into it: get(app) is the value
// the feature uses now, as the dialog's text ("1"/"0" for yes/no), and set(app, text) stores a new value where the
// feature reads it. Like the entries of optbridge.js, get() always answers, with the feature's own default when nothing
// is stored, because several of these options have a default in the settings file that differs from what the program
// has always done (Lister > Binary mode line width says 80 where the Lister uses 75, Folder tabs > Activate newly
// opened tabs says yes where Ctrl+Up opens its tab in the background). The dialog then shows what the program really
// does, and nothing changes for anyone until they change an option.
//
// The module imports only plain default tables, so the tests can run it in Node with a stand-in app.

import { TAB_DEFAULTS } from '../tabs-defaults.js';
import { LISTER_DEFAULTS } from '../viewer/lister-defaults.js';
import { boolKey } from './optbridge.js';

const bit = (v) => (v ? '1' : '0');
const obj = (app, key) => app.setting(key, {}) || {};
const mergeKey = (app, key, part) => app.setSetting(key, { ...obj(app, key), ...part });
const clamp = (n, lo, hi) => Math.max(lo, Math.min(hi, n));

// countKey: a number kept in a settings key of its own where 0 is a real value ("show none"), unlike intKey, which
// takes 0 as "not set". Numbers outside lo..hi are brought into range, a missing or broken value is def.
export const countKey = (key, def, lo, hi) => {
  const read = (v) => { const n = parseInt(v, 10); return Number.isFinite(n) ? clamp(n, lo, hi) : def; };
  return {
    get: (app) => String(read(app.setting(key, def))),
    set: (app, text) => app.setSetting(key, read(text)),
  };
};

// ---- copy and delete (features/fileops.js keeps its options in the settings key "fileops") ----

// "Shift+F6 selects the name without extension" is the opposite of fileops.renameSelectExt (off by default: the
// inline rename selects the name without the extension).
export const renameSelectsName = {
  get: (app) => bit(!obj(app, 'fileops').renameSelectExt),
  set: (app, text) => mergeKey(app, 'fileops', { renameSelectExt: text !== '1' }),
};

// "Confirm overwriting files" is the "ask" policy of fileops.overwriteDefault ('' = ask), the same pairing the
// Confirmations dialog (features/system.js) uses. Turning it off asks no more and overwrites, as that dialog does; a
// policy chosen elsewhere (skip, overwrite older, rename...) already means "don't ask" and is left alone.
export const confirmOverwrite = {
  get: (app) => bit(!obj(app, 'fileops').overwriteDefault),
  set: (app, text) => {
    const cur = obj(app, 'fileops').overwriteDefault || '';
    if (text === '1') return cur ? mergeKey(app, 'fileops', { overwriteDefault: '' }) : null;
    return cur ? null : mergeKey(app, 'fileops', { overwriteDefault: 'overwrite' });
  },
};

// ---- the Lister (features/viewer/view.js keeps its settings in the key "lister") ----

const lister = (app) => ({ ...LISTER_DEFAULTS, ...obj(app, 'lister') });

// listerInt: a number of the Lister's settings, in the range its own options dialog accepts (features/viewer.js).
export const listerInt = (k, lo, hi) => ({
  get: (app) => String(clamp(+lister(app)[k] || LISTER_DEFAULTS[k], lo, hi)),
  set: (app, text) => mergeKey(app, 'lister', { [k]: clamp(parseInt(text, 10) || LISTER_DEFAULTS[k], lo, hi) }),
});

// "Image size": 0 actual size (fitImage off), 1 shrink large images (fitImage, the default), 2 fit to window (fitImage
// with fitGrow, which enlarges small images too).
export const imageFitText = (s) => (s.fitImage === false ? '0' : s.fitGrow ? '2' : '1');
export const imageFitPart = (text) => (text === '0' ? { fitImage: false } : { fitImage: true, fitGrow: text === '2' });

// ---- packing (features/vfs.js: the Pack dialog starts with the packer kept in the key "packer") ----

// The option names packers as Total Commander's dialog does (tgz, tbz, txz); the Pack dialog knows them by their
// extensions (tar.gz, tar.bz2, tar.xz).
const PACKER_ALIASES = { tgz: 'tar.gz', tbz: 'tar.bz2', tbz2: 'tar.bz2', txz: 'tar.xz', tzst: 'tar.zst' };
const PACKER_SHORT = { 'tar.gz': 'tgz', 'tar.bz2': 'tbz', 'tar.xz': 'txz', 'tar.zst': 'tzst' };
export const packerName = (text) => { const t = String(text ?? '').trim().toLowerCase().replace(/^\./, ''); return PACKER_ALIASES[t] || t || 'zip'; };
export const packerText = (format) => { const f = String(format || 'zip'); return PACKER_SHORT[f] || f; };

// ---- folder tabs (features/tabs.js keeps its options in the key "folderTabs") ----

const tabs = (app) => ({ ...TAB_DEFAULTS, ...obj(app, 'folderTabs') });
const tabBool = (k) => ({ get: (app) => bit(tabs(app)[k]), set: (app, text) => mergeKey(app, 'folderTabs', { [k]: text === '1' }) });

// ---- views (features/views.js keeps its options in the key "views") ----

// The thumbnails are square: "thumbSize" is their width and height, from 32 to 512 pixels (views.js), 96 by default.
const thumbSize = (v) => clamp(parseInt(v, 10) || 96, 32, 512);

// thumbLinesOf splits the option's text ("|" between lines) into at most four non-empty lines.
export const thumbLinesOf = (text) => String(text ?? '').split('|').map((x) => x.trim()).filter(Boolean).slice(0, 4);

export const FEATURE_BRIDGE = {
  'Operation.RenameSelectsName': renameSelectsName,
  'CopyDelete.ConfirmOverwrite': confirmOverwrite,
  // the same question as Drag & drop > Confirm before dropping (features/system-dnd.js reads "confirmDrop")
  'CopyDelete.ConfirmDragDrop': boolKey('confirmDrop', true),
  // drags from a panel carry the file list to other programs unless this is off (features/system-dnd.js)
  'DragDrop.DragToOtherApp': boolKey('dragToOtherApp', true),
  'Lister.TabWidth': listerInt('tabWidth', 1, 32),
  'Lister.BinaryWidth': listerInt('binaryWidth', 8, 1024),
  'Font.ListerSize': listerInt('fontSize', 8, 32),
  'Lister.ImageFit': { get: (app) => imageFitText(lister(app)), set: (app, text) => mergeKey(app, 'lister', imageFitPart(text)) },
  'ViewerEditor.Viewer': { get: (app) => String(lister(app).viewer || ''), set: (app, text) => mergeKey(app, 'lister', { viewer: String(text ?? '').trim() }) },
  // the command line history (core/cmdline.js) and the frequently used folders popup (features/hotlist.js)
  'Hotlist.CmdHistSize': countKey('cmdHistoryLength', 50, 1, 500),
  'Hotlist.FrequentSize': countKey('frequentLength', 20, 0, 200),
  // the hotlist popup shows each folder entry's folder next to its name (features/hotlist.js; it always did)
  'Hotlist.ShowPaths': boolKey('hotlistShowPaths', true),
  'Packer.DefaultPacker': { get: (app) => packerText(obj(app, 'packer').format), set: (app, text) => mergeKey(app, 'packer', { format: packerName(text) }) },
  'FolderTabs.Bottom': tabBool('bottom'),
  'FolderTabs.RestoreTabs': tabBool('restoreTabs'),
  'FolderTabs.ActivateNew': tabBool('ctrlUpForeground'),
  'FavoriteTabs.RestoreBoth': tabBool('favBothPanels'),
  'FavoriteTabs.CloseOldTabs': tabBool('favCloseOld'),
  'FavoriteTabs.ConfirmReplac': tabBool('favConfirm'),
  // column sets with a folder mask are switched to by folder (features/views.js)
  'CustomColumns.AutoSwitch': boolKey('columnsAutoSwitch', true),
  'Thumbnails.Width': { get: (app) => String(thumbSize(obj(app, 'views').thumbSize)), set: (app, text) => mergeKey(app, 'views', { thumbSize: thumbSize(text) }) },
  // the lines under each thumbnail (features/views.js "thumbLines": column templates such as [=tc.size.k], F-679)
  'Thumbnails.FieldLines': {
    get: (app) => { const l = obj(app, 'views').thumbLines; return Array.isArray(l) ? l.join('|') : ''; },
    set: (app, text) => mergeKey(app, 'views', { thumbLines: thumbLinesOf(text) }),
  },
  // the log file starts again above this size (feature_system_watch.go, SystemLog.Write; 0 = never)
  'Log.MaxSizeKB': countKey('logMaxKB', 0, 0, 10485760),
};
