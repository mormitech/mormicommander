// Options > Icons in the file lists (board issue #94, F-095, F-787): Show icons (none, the standard ones, or the
// system's own icon of every file: Finder's on macOS, Explorer's on Windows; Linux keeps the standard ones), the icon
// size, the link arrow ("overlay icons", cm_SwitchOverlayIcons), folder-specific and program-specific icons, and
// whether icons are loaded in network folders. The icons come from Go (feature_icons.go) by file type, or by path for
// programs and folders with icons of their own; they are asked for the rows on screen only, and kept.

import { Panel } from '../../core/panel.js';
import { go } from '../../core/go.js';
import { command } from '../../core/registry.js';
import { esc } from '../../core/dialogs.js';
import { on, num, applyOptions } from './util.js';

let app = null;
let supported = false;
const byType = new Map(); // "ext@size" → data URL ('' when the system has none)
const byPath = new Map(); // "path@size" → data URL
const wantTypes = new Set(), wantPaths = new Set();
let timer = 0;

const REMOTE = /^[a-z][a-z0-9+.-]*:\/\//i;
const PROGRAM = { darwin: /\.app$/i, windows: /\.(exe|lnk|ico|cpl|scr|msc|url)$/i };

const mode = () => Math.max(0, Math.min(2, num('Icons.ShowIcons', 1)));
const size = () => [16, 24, 32, 48].includes(num('Icons.IconSize', 16)) ? num('Icons.IconSize', 16) : 16;

function onNetwork(p) {
  const v = app?.volumeFor?.(p.path || '');
  return !!v && (v.kind === 'network' || v.net);
}

// iconHTML is the icon element of a row.
function iconHTML(p, e) {
  if (e.up || mode() < 2 || !supported || REMOTE.test(p.path || '')) return '<i class="ic"></i>';
  const net = onNetwork(p);
  if (net && !on('Icons.IconsOnNet')) return '<i class="ic"></i>';
  const sz = size();
  const special = num('Icons.IconsSpecialFolders', 1); // 0 never, 1 local, 2 local and network, 3 always
  const prog = on('Icons.ExeIcons') && PROGRAM[app.platform]?.test(e.name);
  const own = prog || (e.dir && (special === 3 || special === 2 || (special === 1 && !net)));
  let url;
  if (own) {
    const path = p.fullPath(e.name), k = `${path}@${sz}`;
    url = byPath.get(k);
    if (url === undefined) { wantPaths.add(path); ask(); }
  } else {
    const t = e.dir ? '/' : (e.ext || '').toLowerCase(), k = `${t}@${sz}`;
    url = byType.get(k);
    if (url === undefined) { wantTypes.add(t); ask(); }
  }
  return url ? `<img class="ic" src="${esc(url)}" alt="" draggable="false">` : '<i class="ic"></i>';
}

// ask gathers the icons the rows drawn just now need and fetches them in one go; the rows are drawn again when they come.
function ask() {
  if (timer) return;
  timer = setTimeout(async () => {
    timer = 0;
    const sz = size();
    const types = [...wantTypes], paths = [...wantPaths];
    wantTypes.clear(); wantPaths.clear();
    try {
      const [t, p] = await Promise.all([
        types.length ? go.Icons.ForTypes(types, sz) : {}, paths.length ? go.Icons.ForPaths(paths, sz) : {},
      ]);
      for (const x of types) byType.set(`${x}@${sz}`, t?.[x] || '');
      for (const x of paths) byPath.set(`${x}@${sz}`, p?.[x] || '');
    } catch {
      for (const x of types) byType.set(`${x}@${sz}`, '');
      for (const x of paths) byPath.set(`${x}@${sz}`, '');
    }
    for (const pn of app.panels) pn.renderRows();
  }, 30);
}

// applyIcons puts the Icons options on screen (at start and after OK / Apply in Options).
export function applyIcons() {
  const b = document.body.classList;
  b.toggle('mc-config-no-icons', mode() === 0);
  b.toggle('mc-config-no-overlays', !on('Icons.IconOverlays'));
  const sz = size();
  document.documentElement.style.setProperty('--ic-size', sz === 16 ? '15px' : `${sz}px`);
}

// rowHeight is the row height the icons need: the font's, or the icon size and a little room when that is taller.
export function rowHeight(fontRow) {
  const sz = size();
  return sz > 16 ? Math.max(fontRow, sz + 6) : fontRow;
}

command({
  name: 'cm_SwitchOverlayIcons', label: 'Overlay icons (link arrows)', checked: () => on('Icons.IconOverlays'), menu: 'Show', menuAfter: 'cm_SwitchHidSys',
  run: async () => {
    await applyOptions({ 'Icons.IconOverlays': on('Icons.IconOverlays') ? '0' : '1' });
    applyIcons();
    return true;
  },
});

export async function initFileIcons(a) {
  app = a;
  Panel.iconHTML = iconHTML;
  try { supported = !!(await go.Icons.Supported()); } catch { supported = false; }
}
