// Configuration, the button bar, the Start menu, help and the start-up parameters (FEATURES.md sections 11, 17, 18, 19
// and 41). The settings live in mormicommander.ini, a wincmd.ini-style file kept by internal/config (Go:
// feature_config*.go); the parts are in features/config/:
//
//   options.js    the Options dialog (cm_Config) and the Configuration menu commands
//   layout.js     what the options change on screen; the cm_Vis* toggles
//   buttonbar.js  the button bars and Change button bar (cm_ButtonConfig)
//   startmenu.js  the Start menu and Change start menu (cm_ConfigStartMenu)
//   usercmds.js   em_ user commands as commands
//   run.js        running a button, a Start menu entry or a user command
//   help.js       F1 help and the keyboard page
//   start.js      -T, -S, -P, -O and the standalone tools, applied after start-up
//
// Control mode: the dialogs are ordinary page dialogs (kinds "config", "button-config", "startmenu-config", "help");
// "config" returns the options, "buttonbar" and "startmenu" what those show.

import './config.css';
import { go, on as onGo } from '../core/go.js';
import { setupKeys } from '../core/registry.js';
import { refreshMenu } from '../core/menus.js';
import { renderFkeys } from '../core/fkeys.js';
import { addControlCommand } from '../core/control.js';
import { state, loadValues, loadSchema } from './config/util.js';
import './config/keys.js';
import { initOptions } from './config/options.js';
import { initLayout, applyAll, loadColors } from './config/layout.js';
import { initListOpts } from './config/listopts.js';
import { initFileIcons } from './config/fileicons.js';
import { initSounds } from './config/sounds.js';
import { initButtonBar, loadBars } from './config/buttonbar.js';
import { initStartMenu, loadStartMenu } from './config/startmenu.js';
import { registerUserCommands } from './config/usercmds.js';
import { initStart } from './config/start.js';
import './config/help.js';

// mergeKeymap lays the hotkeys of the settings file ([Shortcuts]: edited by hand or imported from Total Commander) over
// the ones in settings.json.
async function mergeKeymap(app) {
  const ini = await go.Configuration.Keymap().catch(() => null);
  if (!ini || !Object.keys(ini).length) return;
  setupKeys(app.platform, { ...(app.setting('keymap', {}) || {}), ...ini });
  refreshMenu();
  renderFkeys();
}

async function reloadAll(app) {
  await loadValues(app);
  await loadColors();
  await mergeKeymap(app);
  applyAll(app.startTheme);
  await Promise.all([loadBars(), loadStartMenu(), registerUserCommands(app)]);
  app.emit('config', null);
  app.emit('settings');
}

export async function init(app) {
  const info = await go.App.StartInfo().catch(() => ({}));
  app.startTheme = info.theme || '';
  app.config = state; // other features may read app.config.values["Page.Field"]
  await loadValues(app);
  await loadColors();
  initOptions(app);
  initListOpts(app);
  await initFileIcons(app);
  initSounds(app);
  initLayout(app);
  await mergeKeymap(app);
  applyAll(app.startTheme);
  initStart(app);
  await Promise.all([initButtonBar(app), initStartMenu(), registerUserCommands(app)]);
  onGo('config:changed', () => reloadAll(app).catch((e) => console.error(e)));
  onGo('config:reloaded', (path) => app.flash(`The settings file was read again${path ? ` (${path.split(/[\\/]/).pop()})` : ''}.`));
}

addControlCommand('config', async (m) => {
  if (m.reload) await loadValues();
  const schema = m.schema ? await loadSchema() : undefined;
  const info = await go.Configuration.Info().catch(() => ({}));
  return { ok: true, values: state.values, info, schema };
});
