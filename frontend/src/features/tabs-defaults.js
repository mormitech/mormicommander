// The default folder tab options, kept in a module of their own (no imports) so both the tabs feature (tabs.js) and
// the Options dialog's bridge to them (config/optbridge.js, board issue #94) use the same values, and the tests can load
// them in Node. The options themselves are stored in the settings key "folderTabs"; a key missing there means its
// default from this list.

export const TAB_DEFAULTS = {
  show: true,              // show folder tabs at all (cm_VisDirTabs)
  showOne: false,          // show the tab bar also when there is only one tab
  confirmCloseAll: true,   // ask before "Close all tabs"
  dblClickClose: true,     // double-click a tab closes it
  ctrlUpForeground: false, // Ctrl+Up opens the new tab in the foreground (off: in the background, like TC)
  nextToCurrent: true,     // new tabs go right after the current tab (off: at the end)
  multiLine: false,        // tabs on several lines instead of one scrolling line
  limitWidth: false,       // cut long captions…
  widthLimit: 32,          // …to this many characters
  starLocked: true,        // "*" before the caption of a locked tab
  activateTarget: false,   // Ctrl+Shift+Up also makes the other panel active
  closeButton: true,       // a × on each tab (shown on hover and on the active tab)
  resetLocked: false,      // a "folder changes allowed" tab goes back to its locked folder when activated again
  lockedChange: 0,         // folder change in a locked tab: 0 new tab, 1 reuse a tab with that folder, 2 first unlocked, 3 last unlocked
  bottom: false,           // the tab bar sits below the file list instead of above it
  restoreTabs: true,       // the tabs of the last session come back at start (off: only each panel's active tab)
  favBothPanels: true,     // favourite tabs open in both panels (off: only the active panel's tabs)
  favCloseOld: true,       // favourite tabs replace the open tabs (off: they are added after them)
  favConfirm: false,       // ask before favourite tabs replace the open tabs
};
