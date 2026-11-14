// Start-up of a Lister window (viewer.html), the F3 viewer in a real window of its own. Go makes the window hidden
// (feature_viewer_window.go); this page loads the settings and the theme, tells Go it is ready, and waits for a file
// ("viewer-open", or the answer of ListerWindows.Ready when the file was asked for while the page was still loading).
// It shows the file with the same view as the in-page Lister and quick view (features/viewer/view.js), then asks Go to
// show the window, so the window never appears empty or with an old file.
//
// Keys: the view's own (1-7, N, P, F7, Ctrl+C...), Esc (stops a search first) and Q / Alt+F4 / Cmd+W close the window.
// N and P walk the list of names the window was given, or its list of full paths (files in an archive, Find files
// results, a fed list; see features/viewer/walk.js); Go then sets the title and moves the file manager's cursor.
// Control mode reaches this page through Go ("viewer-ctl" events, answered with ListerWindows.CtlReply), since the
// file manager's page cannot see into another window.

import './style.css';
import './features/viewer.css';
import { go, on, errText } from './core/go.js';
import { app } from './core/app.js';
import { setupKeys, setApp } from './core/registry.js';
import { loadSettings, getSetting, onSettingChange } from './core/settings.js';
import { comboFromEvent, eventInit } from './core/keys.js';
import { popupOpen, currentPopup, choosePopup } from './core/popup.js';
import { currentDialog, answerDialog, dialogOpen } from './core/dialogs.js';
import { idle } from './core/busy.js';
import { ListerView, applyListerOptions, listerSettings } from './features/viewer/view.js';
import { makeWalker, walkList } from './features/viewer/walk.js';
// The view's menus run cm_ListExternal ("Open with another app") and cm_ListerConfig (Lister options); importing the
// feature registers them here too. Its init (panel keys, quick view) is for the file manager and is not run.
import './features/viewer.js';

const MENUS = ['File', 'Edit', 'Options', 'Image', 'Encoding'];
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

let me = ''; // this window's name ("lister-3")
let cur = null; // {view, req, path, walker}
let opening = Promise.resolve();

function applyTheme(info) {
  const theme = info.theme || getSetting('theme', '');
  if (theme === 'light' || theme === 'dark') document.documentElement.dataset.theme = theme;
  else delete document.documentElement.dataset.theme;
}

const fileOf = (path) => path.slice(Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')) + 1);

// show puts a file in the window (replacing the one there) and has Go show the window.
async function show(req, info) {
  await loadSettings(); // (settings may have changed since the window was made in advance)
  applyTheme(info);
  cur?.view.destroy();
  const view = new ListerView(app, { internalOnly: !!req.internalOnly });
  view.onNeighbor = (d) => neighbor(d);
  view.onClose = () => closeWindow();
  const walker = makeWalker(req, { resolve: (n, d) => app.resolvePath(n, d), localCopy: (p) => go.FS.LocalCopy(p) });
  cur = { view, req, path: req.path, walker };
  document.getElementById('viewer-body').replaceChildren(view.el);
  try {
    await view.open(req.path, req.mode || 'auto');
  } catch (e) {
    view.showNote(errText(e));
  }
  document.title = `Lister – ${fileOf(req.path)}`;
  await go.ListerWindows.ShowMe(view.id || 0).catch(() => {});
  view.focus();
}

// neighbor opens the next (d = 1) or previous (d = -1) file of the list.
async function neighbor(d) {
  if (!cur) return;
  const c = cur;
  const { view } = c;
  let next;
  try {
    next = await c.walker.step(c.path, d);
    if (!next) { view.note(d > 0 ? 'Last file' : 'First file'); return; }
    if (cur !== c) return; // (another file came in while this one was unpacked)
    await view.open(next.path);
  } catch (e) {
    view.note(errText(e), true);
    return;
  }
  next.done();
  const path = next.path;
  cur.path = path;
  document.title = `Lister – ${fileOf(path)}`;
  go.ListerWindows.Moved(path, view.id || 0).catch(() => {});
}

function closeWindow() {
  if (cur) { cur.view.destroy(); cur = null; }
  go.ListerWindows.CloseMe().catch(() => {});
}

function onKey(ev) {
  if (!cur || popupOpen() || dialogOpen()) return; // a dialog or a menu over the Lister has the keys
  if (ev.target?.matches?.('input, select, textarea') && ev.key !== 'Escape') return;
  const combo = comboFromEvent(ev);
  if (!combo) return;
  const closeKeys = ['Esc', 'Q', 'Alt+F4', ...(app.platform === 'darwin' ? ['Cmd+W'] : [])];
  if (closeKeys.includes(combo)) {
    ev.preventDefault();
    ev.stopImmediatePropagation();
    if (combo === 'Esc' && cur.view.cancelWork()) return; // Esc stops a search first
    closeWindow();
    return;
  }
  if (cur.view.key(combo)) { ev.preventDefault(); ev.stopImmediatePropagation(); }
}

// ---- control mode (through Go: control.go → feature_viewer_window.go → here) ----

function press(combo) {
  const target = document.activeElement && document.activeElement !== document.body ? document.activeElement : cur?.view.el || document.body;
  const init = eventInit(combo);
  target.dispatchEvent(new KeyboardEvent('keydown', init));
  target.dispatchEvent(new KeyboardEvent('keyup', { ...init, ctrlKey: false, altKey: false, shiftKey: false, metaKey: false }));
}

function state() {
  return {
    ok: true, open: !!cur, list: walkList(cur?.req), dir: cur?.req.dir ?? null, side: cur?.req.side ?? null,
    lister: cur ? cur.view.state() : null, dialog: currentDialog(), popup: currentPopup(), documentTitle: document.title,
    theme: document.documentElement.dataset.theme || '', focus: document.activeElement?.className || document.activeElement?.tagName?.toLowerCase() || '',
  };
}

async function settle(ms = 30) { await sleep(ms); await idle(); }

async function ctl(m) {
  switch (m.cmd) {
    case 'state': return state();
    case 'key': { // {keys: "W"} or {keys: ["Down", "N"]}
      const list = Array.isArray(m.keys) ? m.keys : [m.keys ?? m.key];
      for (const k of list) { press(k); await settle(m.delay || 30); }
      return state();
    }
    case 'menu': { // {menu: "Options", item: "Wrap text"}
      if (!cur) throw new Error('nothing is open');
      const it = cur.view.menuItems(m.menu).find((x) => !x.sep && x.label.replace(/^✓ /, '').toLowerCase() === String(m.item).toLowerCase());
      if (!it) throw new Error(`the ${m.menu} menu has no item "${m.item}"`);
      if (it.disabled) throw new Error(`"${m.item}" is not available now`);
      it.run();
      await settle();
      return state();
    }
    case 'select': // {start, end}: the selection by bytes
      if (!cur) throw new Error('nothing is open');
      cur.view.select(m.start ?? 0, m.end ?? 0);
      return state();
    case 'dialog': // {button, fields}: answers a dialog of this window (Find, Go to...); without button: submits
      answerDialog(m.button, m.fields || {});
      await settle(m.wait || 50);
      return state();
    case 'popup': // {index} or {label}; null closes
      choosePopup(m.index ?? m.label ?? null, !!m.shift);
      await settle(m.wait || 50);
      return state();
  }
  throw new Error(`unknown Lister window command "${m.cmd}"`);
}

async function start() {
  const [platform, info, sep, home] = await Promise.all([
    go.App.Platform().catch(() => 'darwin'), go.App.StartInfo().catch(() => ({})), go.FS.Separator().catch(() => '/'), go.FS.Home().catch(() => '/'),
  ]);
  Object.assign(app, { platform, sep, home, demo: !!info.demo });
  document.documentElement.dataset.platform = platform;
  await loadSettings();
  applyTheme(info);
  setupKeys(platform, getSetting('keymap', {}));
  setApp(app);

  const menus = document.getElementById('viewer-menus');
  menus.innerHTML = MENUS.map((m) => `<button type="button" class="mc-viewer-menu" data-menu="${m}">${m}</button>`).join('');
  menus.querySelectorAll('.mc-viewer-menu').forEach((b) => { b.onclick = () => cur?.view.menu(b.dataset.menu, b); });
  window.addEventListener('keydown', onKey, true);

  // Options changed in another window (or in this one's Lister options) apply to the open file at once.
  onSettingChange((key, value) => {
    if (key === 'lister' && cur) applyListerOptions(cur.view, listerSettings(app));
    if (key === 'theme') applyTheme(info);
  });
  app.on('lister-options', (lo) => { if (cur) applyListerOptions(cur.view, lo); });
  on('viewer-open', (req) => { opening = opening.then(() => show(req, info)).catch((e) => console.error(e)); });
  on('viewer-ctl', async (id, m) => {
    let res;
    try { res = await ctl(m || {}); } catch (e) { res = { error: errText(e) }; }
    try { await go.ListerWindows.CtlReply(id, JSON.stringify(res)); } catch { /* the window is closing */ }
  });

  const r = await go.ListerWindows.Ready();
  me = r?.name || '';
  document.body.dataset.window = me;
  if (r?.request) opening = opening.then(() => show(r.request, info));
}

start();
