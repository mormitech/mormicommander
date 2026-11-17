// What Options > Tooltips and Options > Ignore list change in the file lists (board issue #94, F-106, F-788, F-789).
//
// Tooltips: 0 off, 1 the whole name when it is cut off (what the lists always did), 2 the name and the file's facts
// (size, date, and with "per-type fields" its kind, and an image's picture size), 3 the facts only. The delay is the
// option's; "Tooltips for network files" off keeps the facts away from network folders, where reading them is slow.
//
// Ignore list: the masks of the ignore file (one per line, TC's ignore.txt) hide matching files and folders in every
// panel while IgnoreList.Enabled is on; cm_SwitchIgnoreList turns that on and off.

import { Panel } from '../../core/panel.js';
import { go } from '../../core/go.js';
import { command } from '../../core/registry.js';
import { esc } from '../../core/dialogs.js';
import { cellSize, date } from '../../core/format.js';
import { setTipMore } from '../../core/tooltip.js';
import { on, num, applyOptions, compileMask } from './util.js';

let app = null;

const REMOTE = /^[a-z][a-z0-9+.-]*:\/\//i;
const IMAGE = /\.(jpe?g|png|gif|bmp|webp|tiff?|heic|heif)$/i;

// onNetwork: the panel shows a server folder or a folder on a network volume.
function onNetwork(p) {
  if (REMOTE.test(p.path || '')) return true;
  const v = app?.volumeFor?.(p.path || '');
  return !!v && (v.kind === 'network' || v.net);
}

function kindOf(e) {
  if (e.dir) return e.link ? 'Link to a folder' : 'Folder';
  if (e.link) return e.broken ? 'Link (broken)' : 'Link';
  if (e.exec) return 'Program';
  return e.ext ? `${e.ext.toUpperCase()} file` : 'File';
}

// tipAttr builds the title (and the data attributes core/tooltip.js reads) of a row's name.
function tipAttr(p, e, cut) {
  if (e.up) return '';
  const mode = Math.max(0, Math.min(3, num('Tooltips.Show', 1)));
  if (mode === 0) return '';
  const delay = ` data-tip-delay="${Math.max(0, num('Tooltips.DelayMs', 700))}"`;
  const facts = mode >= 2 && (on('Tooltips.ShowOnNet') || !onNetwork(p));
  if (!facts) return mode === 3 ? '' : ` title="${esc(e.name)}" data-tip-cut="1"${delay}`;
  const lines = mode === 2 ? [e.name] : [];
  if (on('Tooltips.ByType')) lines.push(kindOf(e));
  const sz = e.dir ? e.dirSize : e.size;
  if (sz != null) lines.push(`Size: ${cellSize(sz)}`);
  if (e.mtime) lines.push(`Modified: ${date(e.mtime)}`);
  const img = on('Tooltips.ByType') && !e.dir && IMAGE.test(e.name) ? ` data-tip-img="${esc(p.fullPath(e.name))}"` : '';
  return ` title="${esc(lines.join('\n'))}"${delay}${img}`;
}

// ---- the ignore list ----

let masks = [];       // compiled: {full} for a whole path, {test} for a mask
let loadedFor = null; // the file the masks were read from, null before the first read

const win = () => app?.platform === 'windows';
const norm = (s) => (win() ? s.toLowerCase().replace(/\//g, '\\') : s);

function compile(lines) {
  return lines.map((l) => (/[\\/]/.test(l) && !l.includes('|') ? { full: norm(l.replace(/[\\/]+$/, '')) } : { test: compileMask(l) }));
}

function ignored(p, e) {
  if (e.up || !masks.length) return false;
  for (const m of masks) {
    if (m.full) { if (norm(p.fullPath(e.name)) === m.full) return true; } else if (m.test(e.name, !!e.dir)) return true;
  }
  return false;
}

function rebuildPanels() {
  for (const p of app?.panels || []) { try { if (p.path) { p.rebuild(); app.panelChanged(p, { moved: false }); } } catch { /* not ready yet */ } }
}

let wasOn = false;
// applyIgnoreList reads the ignore file again (when the list is on) and redraws the panels when what they hide changes.
export async function applyIgnoreList() {
  const enabled = on('IgnoreList.Enabled');
  if (enabled) {
    try {
      const r = await go.Configuration.IgnoreList();
      masks = compile(r?.masks || []);
      loadedFor = r?.file || '';
    } catch { masks = []; }
  }
  if (enabled || wasOn) rebuildPanels();
  wasOn = enabled;
}

command({
  name: 'cm_SwitchIgnoreList', label: 'Hide files from the ignore list', checked: () => on('IgnoreList.Enabled'), menu: 'Show', menuAfter: 'cm_SwitchHidSys',
  run: async () => {
    const now = !on('IgnoreList.Enabled');
    await applyOptions({ 'IgnoreList.Enabled': now ? '1' : '0' });
    await applyIgnoreList();
    if (now && !masks.length) app.flash(`The ignore list is on, but ${loadedFor || 'its file'} has no masks yet.`);
    else app.flash(now ? 'Files from the ignore list are hidden.' : 'Files from the ignore list are shown.');
    return true;
  },
});

export function initListOpts(a) {
  app = a;
  Panel.hideHooks.push((p, e) => on('IgnoreList.Enabled') && ignored(p, e));
  Panel.tipAttr = tipAttr;
  setTipMore(async (path) => {
    const s = await go.Views.ImageSize(path);
    return s?.w ? `Picture: ${s.w} × ${s.h}` : '';
  });
}
