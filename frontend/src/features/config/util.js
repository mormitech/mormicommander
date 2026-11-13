// Shared pieces of the configuration feature: the option values the page keeps (loaded from Go's internal/config), the
// bridge between those options and the settings.json keys other features read, small HTML helpers and TC file masks.

import { go } from '../../core/go.js';
import { esc } from '../../core/dialogs.js';

// The options as text by "Page.Field" name ("1"/"0" for yes/no), as Go's Configuration.Values returns them.
export const state = { values: {}, schema: null, loaded: false };

// loadValues reads the options from Go. With app, options that another feature also keeps (BRIDGE below) take the
// app's live value, so a change made there (Ctrl+= for the font, Ctrl+Shift+H for the panels) is not undone.
export async function loadValues(app) {
  try { state.values = (await go.Configuration.Values()) || {}; state.loaded = true; } catch { state.values = {}; }
  if (app) for (const n of Object.keys(BRIDGE)) state.values[n] = currentValue(app, n);
  return state.values;
}

export async function loadSchema() {
  if (!state.schema) state.schema = await go.Configuration.Schema();
  return state.schema;
}

export const opt = (name, def = '') => (state.values[name] ?? def);
export const on = (name) => opt(name) === '1';
export const num = (name, def = 0) => { const n = parseInt(opt(name), 10); return Number.isFinite(n) ? n : def; };

// apply sends changed options to Go (which saves mormicommander.ini) and keeps the local copy in step.
export async function applyOptions(changes) {
  if (!Object.keys(changes).length) return [];
  const changed = (await go.Configuration.Apply(changes)) || [];
  Object.assign(state.values, changes);
  return changed;
}

const merge = (app, key, part) => app.setSetting(key, { ...(app.setting(key, {}) || {}), ...part });
const bit = (v) => (v ? '1' : '0');
const OVERWRITE = ['', 'overwrite', 'skip', 'older', 'rename', 'rename_target'];
const QS_MODES = ['ctrlalt', 'alt', 'letter', 'off'];
const LISTER_MODES = ['auto', 'text', 'binary', 'hex'];

// BRIDGE: options that other features read from settings.json under their own keys. get(app) gives the option's value
// as the app has it now (undefined when the feature never stored it), set(app, text) stores a new value there. The
// Options dialog shows get() when it has a value, so a choice made elsewhere (the copy dialog's options, the viewer's
// menu) is what the dialog shows, and writes set() for every option the user changes.
const fo = (app, k) => (app.setting('fileops', {}) || {})[k];
const qs = (app, k) => (app.setting('quickSearch', {}) || {})[k];
const li = (app, k) => (app.setting('lister', {}) || {})[k];
const b = (v) => (v === undefined ? undefined : bit(v));
const fileopsBool = (k) => ({ get: (app) => b(fo(app, k)), set: (app, v) => merge(app, 'fileops', { [k]: v === '1' }) });
const qsBool = (k) => ({ get: (app) => b(qs(app, k)), set: (app, v) => merge(app, 'quickSearch', { [k]: v === '1' }) });
const listerBool = (k) => ({ get: (app) => b(li(app, k)), set: (app, v) => merge(app, 'lister', { [k]: v === '1' }) });

export const BRIDGE = {
  'Operation.MarkDirectories': { get: (app) => b(app.setting('markDirectories', undefined)), set: (app, v) => app.setSetting('markDirectories', v === '1') },
  'Display.SortNatural': { get: (app) => b(app.setting('naturalSort', undefined)), set: (app, v) => app.setSetting('naturalSort', v === '1') },
  'Display.SortDirsByName': {
    get: (app) => { const x = app.setting('dirsLikeFiles', undefined); return x === undefined ? undefined : bit(!x); },
    set: (app, v) => app.setSetting('dirsLikeFiles', v !== '1'),
  },
  // The settings file is where "show hidden files" is kept: layout.js puts its value on the panels at start and after a
  // reload, and writes Ctrl+H's toggle back to it, so get() leaves the file's value.
  'Display.ShowHidden': {
    get: () => undefined,
    set: (app, v) => { if ((v === '1') !== !!app.showHidden) return app.run('cm_SwitchHidSys', {}, { quiet: true }); return null; },
  },
  'Layout.Horizontal': { get: (app) => b(app.setting('horizontal', undefined)), set: () => {} }, // applied by layout.js
  'Font.PanelSize': { get: (app) => { const x = app.setting('fontSize', undefined); return x === undefined ? undefined : String(x); }, set: () => {} },
  'Colors.DarkMode': {
    get: (app) => { const t = app.setting('theme', undefined); return t === undefined ? undefined : t === 'light' ? '0' : t === 'dark' ? '1' : '2'; },
    set: (app, v) => app.setSetting('theme', v === '0' ? 'light' : v === '1' ? 'dark' : ''),
  },
  'ViewerEditor.Editor': { get: (app) => app.setting('editor', undefined), set: (app, v) => app.setSetting('editor', v) },
  'CopyDelete.UseTrash': fileopsBool('useTrash'),
  'CopyDelete.ConfirmDelete': fileopsBool('confirmDelete'),
  'CopyDelete.ConfirmDeleteDirs': fileopsBool('confirmDeleteDirs'),
  'CopyDelete.ConfirmDeleteRO': fileopsBool('confirmDeleteRO'),
  'CopyDelete.ConfirmOverwriteRO': fileopsBool('confirmOverwriteRO'),
  'CopyDelete.ConfirmCopy': fileopsBool('confirmCopy'),
  'CopyDelete.BackgroundDefault': fileopsBool('background'),
  'CopyDelete.Verify': fileopsBool('verify'),
  'CopyDelete.CopyTimes': fileopsBool('copyTimes'),
  'CopyDelete.CopyPermissions': fileopsBool('copyPermissions'),
  'CopyDelete.CopyOwner': fileopsBool('copyOwner'),
  'CopyDelete.CopyAttributes': fileopsBool('copyAttributes'),
  'CopyDelete.FollowLinks': fileopsBool('followLinks'),
  'CopyDelete.CheckFreeSpace': fileopsBool('checkFreeSpace'),
  'CopyDelete.BufferKB': {
    get: (app) => { const x = fo(app, 'bufferKB'); return x === undefined ? undefined : String(x); },
    set: (app, v) => merge(app, 'fileops', { bufferKB: parseInt(v, 10) || 1024 }),
  },
  'CopyDelete.BigFileMode': {
    get: (app) => { const x = fo(app, 'bigFileMode'); return x === undefined ? undefined : x ? '1' : '0'; },
    set: (app, v) => merge(app, 'fileops', { bigFileMode: v === '1' }),
  },
  'CopyDelete.OverwriteDefault': {
    get: (app) => { const x = fo(app, 'overwriteDefault'); if (x === undefined) return undefined; const i = OVERWRITE.indexOf(x); return i < 0 ? '0' : String(i); },
    set: (app, v) => merge(app, 'fileops', { overwriteDefault: OVERWRITE[parseInt(v, 10)] || '' }),
  },
  'BackgroundQueue.SpeedLimitKB': {
    get: (app) => { const x = fo(app, 'speedLimitKB'); return x === undefined ? undefined : String(fo(app, 'speedLimitOn') ? x : 0); },
    set: (app, v) => { const kb = parseInt(v, 10) || 0; return merge(app, 'fileops', { speedLimitOn: kb > 0, speedLimitKB: kb || fo(app, 'speedLimitKB') || 0 }); },
  },
  'QuickSearch.Mode': {
    get: (app) => { const m = qs(app, 'mode'); if (m === undefined) return undefined; const i = typeof m === 'number' ? m : QS_MODES.indexOf(m); return i < 0 ? undefined : String(i); },
    set: (app, v) => merge(app, 'quickSearch', { mode: QS_MODES[parseInt(v, 10)] || 'ctrlalt' }),
  },
  'QuickSearch.MatchBeginning': qsBool('beginning'),
  'QuickSearch.MatchEnd': qsBool('matchEnd'),
  'QuickSearch.Filter': qsBool('autoFilter'),
  'QuickSearch.Wildcards': qsBool('wildcards'),
  'QuickSearch.IgnoreAccents': qsBool('ignoreAccents'),
  'Lister.Wrap': listerBool('wrap'),
  'Lister.UseHTML': listerBool('renderHTML'),
  'Lister.UseMedia': listerBool('showMedia'),
  'Lister.Charset': { get: (app) => li(app, 'defaultCharset'), set: (app, v) => merge(app, 'lister', { defaultCharset: v || 'auto' }) },
  'Lister.DefaultMode': {
    get: (app) => { const m = li(app, 'defaultMode'); if (m === undefined) return undefined; const i = LISTER_MODES.indexOf(m); return i < 0 ? undefined : String(i); },
    set: (app, v) => merge(app, 'lister', { defaultMode: LISTER_MODES[parseInt(v, 10)] || 'auto' }),
  },
};

// currentValue is what the dialog shows for an option: the app's live value when another feature keeps it, else the
// settings file's.
export function currentValue(app, name) {
  const br = BRIDGE[name];
  if (br) {
    try { const v = br.get(app); if (v !== undefined && v !== null) return String(v); } catch { /* the file's value */ }
  }
  return opt(name);
}

// ---- small HTML helpers ----
export { esc };
export const h = (tag, attrs = {}, html = '') => {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === false || v === null || v === undefined) continue;
    if (k === 'class') el.className = v; else if (k === 'text') el.textContent = v; else el.setAttribute(k, v === true ? '' : v);
  }
  if (html) el.innerHTML = html;
  return el;
};

// fill sets the <option>s of a <select> from [{value, label}], keeping the chosen value when it is still there.
export function fillSelect(sel, items, keep = true) {
  const old = sel.value;
  sel.innerHTML = items.map((it) => `<option value="${esc(it.value)}">${esc(it.label)}</option>`).join('');
  if (keep && items.some((it) => String(it.value) === old)) sel.value = old;
}

// ---- Total Commander file masks, for colouring files by type in the panels ----
// "*.exe *.com;*.bat | *.bak" : names matching one of the masks before "|" and none after it. A mask ending in "\" or
// "/" matches folders only; "*.*" matches every name, also names without an extension.
function globRe(g) {
  if (g === '*.*' || g === '*') return /^.*$/s;
  const re = g.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.');
  return new RegExp(`^${re}$`, 'is');
}
export function compileMask(text) {
  const [inc, exc = ''] = String(text || '').split('|');
  const parse = (s) => s.split(/[\s;]+/).filter(Boolean).map((t) => {
    t = t.replace(/^"|"$/g, '');
    const dir = /[\\/]$/.test(t);
    return { dir, re: globRe(dir ? t.slice(0, -1) || '*' : t) };
  });
  const a = parse(inc), x = parse(exc);
  const hit = (list, name, isDir) => list.some((m) => (m.dir ? isDir : true) && m.re.test(name));
  return (name, isDir) => !String(text).startsWith('>') && hit(a, name, isDir) && !hit(x, name, isDir);
}

// isDark says whether the page shows the dark theme now.
export function isDark() {
  const t = document.documentElement.dataset.theme;
  if (t === 'dark') return true;
  if (t === 'light') return false;
  return !window.matchMedia?.('(prefers-color-scheme: light)').matches;
}

// panelCtx is the state of both panels for button and Start menu placeholders (%P %N %S %T ...).
export function panelCtx(app) {
  const one = (p) => {
    const sel = p.selectedEntries();
    return { path: p.path || '', cursor: p.cur && !p.cur.up ? p.cur.name : '', cursorDir: !!p.cur?.dir, selected: sel.map((e) => e.name), selectedDirs: sel.map((e) => !!e.dir) };
  };
  return { source: one(app.cur), target: one(app.other), sourceIsRight: app.cur === app.right };
}
