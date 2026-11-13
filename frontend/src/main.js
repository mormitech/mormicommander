// Start-up of the page. The core (src/core) draws the panels and owns the keyboard; everything a user can do is a
// command registered by a feature module in src/features/. Every file there is imported automatically (no list to
// edit): importing it registers its commands, and an exported init(app) runs once the panels exist.
// See docs/ARCHITECTURE.md.

import './style.css';
import { go, on } from './core/go.js';
import { app } from './core/app.js';
import { Panel } from './core/panel.js';
import { setupKeys, setApp } from './core/registry.js';
import { loadSettings, getSetting } from './core/settings.js';
import { initKeyboard, releaseModifiers } from './core/keyboard.js';
import { initFkeys } from './core/fkeys.js';
import { initMenus } from './core/menus.js';
import { initControl } from './core/control.js';
import { initTips } from './core/tooltip.js';
import { initSplitter } from './core/splitter.js';
import { showAbout } from './core/about.js';

const features = import.meta.glob('./features/*.js', { eager: true });

// open shows a folder in a panel, falling back to the home folder and then the root when it can't be read.
async function open(panel, ...paths) {
  for (const p of paths) if (p && await panel.go(p, { history: false })) return;
}

async function start() {
  const mode = await go.App.Mode().catch(() => '');
  if (mode === 'about') { await showAbout(); return; }
  const [platform, info, sep, home] = await Promise.all([
    go.App.Platform().catch(() => 'darwin'), go.App.StartInfo().catch(() => ({})), go.FS.Separator().catch(() => '/'), go.FS.Home().catch(() => '/'),
  ]);
  Object.assign(app, { platform, sep, home, demo: !!info.demo });
  document.documentElement.dataset.platform = platform;
  await loadSettings();
  const theme = info.theme || getSetting('theme', '');
  if (theme === 'light' || theme === 'dark') document.documentElement.dataset.theme = theme;
  setupKeys(platform, getSetting('keymap', {}));
  setApp(app);

  const session = app.demo ? {} : getSetting('session', {}) || {};
  app.showHidden = !!session.showHidden;
  app.panels = ['left', 'right'].map((side) => new Panel(app, side, document.querySelector(`.panel[data-side="${side}"]`)));
  for (const p of app.panels) if (session[p.side]?.sort) p.sort = session[p.side].sort;

  initKeyboard();
  // Go reports the window losing the keyboard (a dialog window opened, another app came forward, minimized), which the
  // page does not always see as a blur: the function key bar must not stay on the shifted labels (#35).
  on('window-inactive', releaseModifiers);
  // A right-click doesn't activate an inactive window on macOS, and an inactive window gets no hover events, so a
  // context menu opened there wouldn't follow the mouse: bring the window to the front first.
  window.addEventListener('contextmenu', () => { if (!document.hasFocus()) go.App.Activate().catch(() => {}); }, true);
  initMenus();
  initFkeys();
  initControl();
  initTips();
  initSplitter(session.split);
  for (const [file, m] of Object.entries(features)) {
    try { await m.init?.(app); } catch (e) { console.error(`feature ${file}:`, e); }
  }

  document.getElementById('app').hidden = false;
  await app.loadVolumes();
  const demoRight = app.demo ? `${home}/Pictures` : '';
  await Promise.all([
    open(app.left, info.left, session.left?.path, home, '/'),
    open(app.right, info.right, session.right?.path, demoRight, home, '/'),
  ]);
  const active = info.active === 'R' ? 1 : info.active === 'L' ? 0 : session.active === 1 ? 1 : 0;
  app.active = -1; // so setActive sees a change and updates the title
  app.setActive(active);
  app.emit('ready');
  // dialogs show in windows of their own (core/mirror.js): two hidden ones are made in advance
  if (getSetting('dialogsInWindows', true) !== false) go.DialogWindows.Prepare().catch(() => {});
}

start();
