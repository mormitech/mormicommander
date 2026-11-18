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
import { cellSize, date, attr } from '../../core/format.js';
import { setTipMore } from '../../core/tooltip.js';
import { on, num, opt, applyOptions, compileMask } from './util.js';

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

// ---- tooltip fields per file type (Options > Tooltips > Tooltip fields per file type, F-821) ----
//
// "masks = fields; masks = fields": the first entry whose masks match a file decides the lines of its tooltip, in the
// order given. Fields: name, kind, size, date, attributes, path, and the ones Go reads from the file when the tooltip
// shows (dimensions, exif, artist, title, album). A file no entry matches, and every file while the option is empty
// (the default), gets the tooltip as before.

export const TIP_FIELDS = ['name', 'kind', 'size', 'date', 'attributes', 'path', 'dimensions', 'exif', 'artist', 'title', 'album'];
const LATE = { dimensions: 'image', exif: 'image', artist: 'audio', title: 'audio', album: 'audio' };
const ALIAS = { modified: 'date', time: 'date', type: 'kind', attr: 'attributes', attrs: 'attributes', picture: 'dimensions', exifdate: 'exif', taken: 'exif' };

let fieldRules = null, fieldsFor = null;

// parseTipFields reads the option into [{masks, test, fields}] (unknown field names are left out).
export function parseTipFields(text) {
  const out = [];
  for (const part of String(text || '').split(/[;\n]/)) {
    const at = part.indexOf('=');
    if (at < 0) continue;
    const masks = part.slice(0, at).trim();
    const fields = part.slice(at + 1).toLowerCase().split(/[\s,]+/).map((f) => ALIAS[f] || f).filter((f) => TIP_FIELDS.includes(f));
    if (masks && fields.length) out.push({ masks, test: compileMask(masks), fields: [...new Set(fields)] });
  }
  return out;
}

function fieldsOf(e) {
  const text = opt('Tooltips.Fields');
  if (text !== fieldsFor) { fieldsFor = text; fieldRules = parseTipFields(text); }
  if (!fieldRules.length) return null;
  return fieldRules.find((r) => r.test(e.name, !!e.dir))?.fields || null;
}

// tipLines are the lines of the fields known without reading the file; late are the fields Go reads when it shows.
export function tipLines(p, e, fields, mode) {
  const lines = [], late = [];
  for (const f of fields) {
    if (f === 'name') { if (mode !== 3) lines.push(e.name); } else if (f === 'kind') lines.push(kindOf(e));
    else if (f === 'size') { const sz = e.dir ? e.dirSize : e.size; if (sz != null) lines.push(`Size: ${cellSize(sz)}`); }
    else if (f === 'date') { if (e.mtime) lines.push(`Modified: ${date(e.mtime)}`); }
    else if (f === 'attributes') { const a = attr(e.mode, e.flags, app?.platform); if (a) lines.push(`Attributes: ${a}`); }
    else if (f === 'path') lines.push(p.fullPath(e.name));
    else if (LATE[f] && !e.dir) late.push(f);
  }
  return { lines, late };
}

// lateLines turns Go's facts (Views.TipFacts) into the lines of the late fields, in their order.
export function lateLines(facts, fields) {
  const out = [];
  for (const f of fields) {
    if (f === 'dimensions' && facts?.w) out.push(`Picture: ${facts.w} × ${facts.h}`);
    else if (f === 'exif' && facts?.exif) out.push(`Taken: ${date(facts.exif)}`);
    else if (f === 'artist' && facts?.artist) out.push(`Artist: ${facts.artist}`);
    else if (f === 'title' && facts?.title) out.push(`Title: ${facts.title}`);
    else if (f === 'album' && facts?.album) out.push(`Album: ${facts.album}`);
  }
  return out;
}

// tipAttr builds the title (and the data attributes core/tooltip.js reads) of a row's name.
function tipAttr(p, e, cut) {
  if (e.up) return '';
  const mode = Math.max(0, Math.min(3, num('Tooltips.Show', 1)));
  if (mode === 0) return '';
  const delay = ` data-tip-delay="${Math.max(0, num('Tooltips.DelayMs', 700))}"`;
  const facts = mode >= 2 && (on('Tooltips.ShowOnNet') || !onNetwork(p));
  if (!facts) return mode === 3 ? '' : ` title="${esc(e.name)}" data-tip-cut="1"${delay}`;
  const own = fieldsOf(e);
  if (own) {
    const { lines: ln, late } = tipLines(p, e, own, mode);
    if (!ln.length && !late.length) return '';
    const more = late.length ? ` data-tip-img="${esc(p.fullPath(e.name))}" data-tip-fields="${late.join(' ')}"` : '';
    return ` title="${esc(ln.join('\n') || ' ')}"${delay}${more}`;
  }
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
  setTipMore(async (path, fields) => {
    if (fields) { // the late fields of Tooltip fields per file type
      const want = fields.split(' ').filter(Boolean);
      const groups = [...new Set(want.map((f) => LATE[f]).filter(Boolean))];
      return lateLines(await go.Views.TipFacts(path, groups), want).join('\n');
    }
    const s = await go.Views.ImageSize(path);
    return s?.w ? `Picture: ${s.w} × ${s.h}` : '';
  });
}
