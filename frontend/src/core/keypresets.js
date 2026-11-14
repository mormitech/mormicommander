// Keyboard presets (board issue #6, F-NEW-1): a few ready-made sets of keys the first-run setup page offers. A preset
// is laid over the default keys (keymap.js) and under the user's own changes (Configuration > Keyboard shortcuts), so
// whatever the user redefined still wins. "tc" is Total Commander's keys exactly as they are today and is the default:
// with it nothing changes at all. The choice is kept in settings.json as "keyPreset".
//
// This module is pure (no DOM, no Go), so the tests check it alone; features/setup.js shows the page and
// core/registry.js lays the chosen keys into the keymap.

export const DEFAULT_PRESET = 'tc';

export const KEY_PRESETS = {
  tc: {
    label: 'Total Commander',
    note: 'The keys of Total Commander, unchanged: F3–F8 for view, edit, copy, move, new folder and delete, Insert and Space to select.',
    keys: {},
  },
  mac: {
    label: 'Mac-native',
    note: 'Total Commander keys plus Finder habits: Space shows the file, ⌘O opens, ⇧⌘N makes a folder, ⌘D duplicates, ⌥⌘⌫ deletes at once. Shift+arrows select, so no Insert key is needed.',
    keys: {
      Space: 'cm_List', 'Cmd+Y': 'cm_List', 'Cmd+O': 'cm_Enter', 'Shift+Cmd+N': 'cm_MkDir', 'Cmd+D': 'cm_CopySamepanel',
      'Alt+Cmd+Backspace': 'cm_DeletePermanent', 'Cmd+E': 'cm_EjectVolume',
      // selection without Insert (F-119): Shift with the arrows, Home, End and the page keys
      'Shift+Up': 'cm_SelUp', 'Shift+Down': 'cm_SelDown', 'Shift+Left': 'cm_SelLeft', 'Shift+Right': 'cm_SelRight',
      'Shift+Home': 'cm_SelHome', 'Shift+End': 'cm_SelEnd', 'Shift+PgUp': 'cm_SelPageUp', 'Shift+PgDn': 'cm_SelPageDown',
    },
  },
  vim: {
    label: 'Vim',
    note: 'Total Commander keys plus vim movement: J/K down and up, H parent folder, L open, G and Shift+G first and last, / quick search, : command line, V select, Y copy, P paste. Letters no longer go straight into the command line: press : first.',
    keys: {
      J: 'cm_CursorDown', K: 'cm_CursorUp', H: 'cm_GoToParent', L: 'cm_Enter', G: 'cm_GoToFirstEntry', 'Shift+G': 'cm_GoToLastEntry',
      '/': 'cm_ShowQuickSearch', 'Shift+;': 'cm_FocusCmdLine', V: 'cm_SelectToggle', Y: 'cm_CopyToClipboard', P: 'cm_PasteFromClipboard',
    },
  },
};

// presetName is a known preset's name, or the default for anything else (an unknown or missing setting).
export const presetName = (name) => (Object.hasOwn(KEY_PRESETS, name) ? name : DEFAULT_PRESET);

// QUICK_LOOK_KEYS are the Mac-native preset's keys that open Quick Look instead of the built-in viewer when the setting
// "quickLookSpace" is on (board issue #19, F-863). It is off by default, so the preset keeps its keys as they were.
export const QUICK_LOOK_KEYS = { Space: 'cm_QuickLook', 'Cmd+Y': 'cm_QuickLook' };

// presetKeys is the keys a preset adds or changes, {combo: cm_* name}. opts.quickLookSpace: the Mac-native preset's
// Space and ⌘Y open Quick Look (other presets are not changed by it).
export const presetKeys = (name, opts = {}) => {
  const n = presetName(name);
  const keys = { ...KEY_PRESETS[n].keys };
  if (n === 'mac' && opts?.quickLookSpace) Object.assign(keys, QUICK_LOOK_KEYS);
  return keys;
};

// shouldShowSetup says whether the setup page opens by itself at start: only on the first run of the app on this
// computer, never in control mode or the demo, and never again once it was shown.
export function shouldShowSetup(info, done) {
  return !!info?.firstRun && !info.control && !info.demo && !done;
}

// fnAdvice is what the F5 test says. pressed: F5 reached the app; platform: "darwin"…; fnKeys: how macOS treats the
// top row ("standard", "media" or "" when unknown, see Setup.FnKeys in feature_setup.go).
export function fnAdvice({ pressed, platform, fnKeys }) {
  const mac = platform === 'darwin';
  if (!pressed) {
    if (!mac) return 'Press F5 to check the function keys.';
    if (fnKeys === 'standard') return 'Press F5. Your Mac sends F1, F2… without fn, so F5 alone should arrive here.';
    return 'Press F5. If the screen brightness or keyboard light changes and nothing shows here, hold fn and press F5 again.';
  }
  if (!mac) return 'F5 works: the function keys reach mormiCommander.';
  if (fnKeys === 'standard') return 'F5 works without fn: the function keys reach mormiCommander directly.';
  return 'F5 arrived. Your Mac treats the top row as brightness and volume keys, so hold fn for F1–F12 (or a keyboard with real F-keys). To use them without fn, turn on "Use F1, F2, etc. keys as standard function keys" in System Settings > Keyboard > Keyboard Shortcuts > Function Keys.';
}
