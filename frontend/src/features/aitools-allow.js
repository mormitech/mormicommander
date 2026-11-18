// What the AI may run and change (issue #146, step 3): the reviewed allow-lists behind run_command and set_setting.
// The owner's rule is that the toolset has no destructive actions at all: nothing that deletes, moves, overwrites,
// renames, wipes or resets, and nothing that runs programs, the command line or a shell, or changes which program the
// app runs. So these lists are allow-lists, not deny-lists: a command or an option that is not written here is refused,
// and a command added to the app later stays out of the model's reach until someone reviews it and adds it here with
// the reason it is safe. The deny rules below only give the model a better sentence for the refusal (and keep a
// careless addition of a dangerous name from passing the tests); they never let anything in.
//
// Commands that open a dialog or a popup and wait for the user (Select a group…, the hotlist, the drive list) are left
// out too, even when they are harmless: the model would sit waiting for an answer it cannot give, and the tools of
// aitools-basic.js do those jobs directly (select_files, set_panel_path). Opening a folder under the cursor goes
// through the open_folder tool, not cm_Enter or cm_EnterDir, because those open a file with its program when the
// cursor is on a file.
//
// Nothing here touches the page or Go, so test/aitools-allow.test.js checks the lists as they are, and that every
// command named here exists in the registry once every feature is loaded.

// ---- commands ----

// The four panel prefixes of Total Commander's per-panel commands: Src is the active panel, Trg the other one.
const SIDES = { Src: 'the active panel', Left: 'the left panel', Right: 'the right panel', Trg: 'the other panel' };

// Per-panel commands that only change how a panel shows its folder (view mode, column set, sort order).
const PER_SIDE = {
  Short: 'brief view: a view mode, no files change',
  Long: 'full view: a view mode, no files change',
  Thumbs: 'thumbnail view: a view mode, no files change',
  Tree: 'tree view: a view mode, no files change',
  Comments: 'the view with file comments: a view mode, nothing is edited',
  NextCustomView: 'next column set: a view mode',
  PrevCustomView: 'previous column set: a view mode',
  ByName: 'sorts by name: only the order on screen changes',
  ByExt: 'sorts by extension: only the order on screen changes',
  BySize: 'sorts by size: only the order on screen changes',
  ByDateTime: 'sorts by date: only the order on screen changes',
  Unsorted: 'shows the folder unsorted: only the order on screen changes',
  NegOrder: 'reverses the sort order: only the order on screen changes',
};

const perSide = () => {
  const out = {};
  for (const [s, where] of Object.entries(SIDES)) for (const [c, why] of Object.entries(PER_SIDE)) out[`cm_${s}${c}`] = `${why} (${where})`;
  return out;
};

// ALLOWED_COMMANDS: every command the model may run, with the reason it is safe.
export const ALLOWED_COMMANDS = {
  // navigation: a panel shows another folder; nothing is opened, run or changed
  cm_GoToParent: 'goes to the parent folder',
  cm_GoToRoot: 'goes to the root folder of the drive',
  cm_GoToHome: 'goes to the home folder',
  cm_GoToPreviousDir: 'goes back in the folder history',
  cm_GoToNextDir: 'goes forward in the folder history',
  cm_OpenDesktop: 'goes to the desktop folder',
  cm_MatchSrc: 'the other panel shows the same folder as the active one',
  cm_TransferLeft: 'the left panel shows the folder under the cursor (on a file: the file\'s own folder); nothing is opened',
  cm_TransferRight: 'the right panel shows the folder under the cursor (on a file: the file\'s own folder); nothing is opened',
  cm_OpenDirInNewTab: 'opens the folder under the cursor in a new tab; on a file it opens the file\'s folder, never the file',
  cm_OpenDirInNewTabOther: 'the same in a new tab of the other panel',
  // the cursor
  cm_CursorUp: 'moves the cursor', cm_CursorDown: 'moves the cursor', cm_CursorLeft: 'moves the cursor', cm_CursorRight: 'moves the cursor',
  cm_PageUp: 'moves the cursor a page', cm_PageDown: 'moves the cursor a page',
  cm_GoToFirstEntry: 'puts the cursor on the first entry', cm_GoToLastEntry: 'puts the cursor on the last entry',
  cm_GoToFirstFile: 'puts the cursor on the first file',
  cm_GoToNextSelected: 'puts the cursor on the next selected entry', cm_GoToPrevSelected: 'puts the cursor on the previous selected entry',
  // the panels
  cm_FocusTrg: 'switches to the other panel (Tab)',
  cm_FocusLeft: 'makes the left panel the active one', cm_FocusRight: 'makes the right panel the active one',
  cm_Exchange: 'swaps the folders of the two panels (Ctrl+U)',
  cm_ExchangeWithTabs: 'swaps the panels with their tabs (Ctrl+Shift+U)',
  cm_HorizontalPanels: 'panels side by side or on top of each other: a layout change',
  // folder tabs: a tab is a view of a folder, so closing one loses no data (and cm_ReopenClosedTab brings it back)
  cm_OpenNewTab: 'opens a new tab', cm_OpenNewTabBg: 'opens a new tab in the background', cm_DuplicateTab: 'opens a copy of the tab',
  cm_SwitchToNextTab: 'shows the next tab', cm_SwitchToPreviousTab: 'shows the previous tab',
  cm_CloseCurrentTab: 'closes the current tab (a view; no files change, and it can be reopened)',
  cm_ReopenClosedTab: 'reopens the last closed tab',
  cm_CopyTabToOtherSide: 'opens the tab in the other panel too (a tab, not files)',
  cm_MoveTabToOtherSide: 'moves the tab to the other panel (a tab, not files)',
  cm_ToggleLockCurrentTab: 'locks or unlocks the tab', cm_ToggleLockDcaCurrentTab: 'locks or unlocks the tab, folder changes allowed',
  cm_VisDirTabs: 'shows or hides the tab bar',
  // view modes and sorting of every panel (see PER_SIDE)
  ...perSide(),
  cm_SwitchDirSort: 'folders always sorted by name, or with the files: an order on screen',
  cm_SwitchNaturalSort: 'natural sorting on or off: an order on screen',
  // what a panel shows
  cm_SwitchHidSys: 'shows or hides hidden files',
  cm_SrcAllFiles: 'removes the panel\'s file filter',
  cm_QuickFilter: 'the quick filter box; args {text} filters the panel by that text (only what is shown changes)',
  cm_QuickFilterLast: 'the quick filter with the last text',
  cm_RereadSource: 'reads the folder again',
  cm_DirBranch: 'branch view: all files of the subfolders in one list (read only)',
  cm_DirBranchSel: 'branch view of the selected folders (read only)',
  cm_SrcQuickInternalOnly: 'quick view with the built-in viewer in the other panel (read only; never an external program)',
  cm_CountDirContent: 'calculates the sizes of the folders (reads only)',
  // compare folders: it only selects the entries that differ
  cm_CompareDirs: 'compares the panels and selects the entries that differ (read only)',
  cm_CompareDirsWithSubdirs: 'the same, with subfolders (read only)',
  cm_CompareDirsHideEqual: 'the same, hiding equal files (read only)',
  cm_CompareDirsOneSidedFolders: 'the same, selecting the folders found on one side only (read only)',
  // the selection: what is selected changes, never the files
  cm_SelectAll: 'selects all', cm_SelectAllBoth: 'selects all, folders too', cm_SelectAllFiles: 'selects all files',
  cm_SelectAllFolders: 'selects all folders', cm_ClearAll: 'unselects all', cm_ClearFiles: 'unselects the files',
  cm_ClearFolders: 'unselects the folders', cm_ExchangeSelection: 'inverts the selection',
  cm_ExchangeSelBoth: 'inverts the selection, folders too', cm_ExchangeSelFiles: 'inverts the selection of files',
  cm_ExchangeSelFolders: 'inverts the selection of folders',
  cm_SaveSelection: 'keeps the selection in memory for cm_RestoreSelection (nothing is written)',
  cm_RestoreSelection: 'selects again what was selected before',
  cm_SelectCurrentExtension: 'selects the entries with the extension under the cursor',
  cm_SelectCurrentName: 'selects the entries with the name under the cursor',
  cm_SelectCurrentNameExt: 'selects the entries with the name and extension under the cursor',
  cm_UnselectCurrentExtension: 'unselects the entries with the extension under the cursor',
  cm_UnselectCurrentName: 'unselects the entries with the name under the cursor',
  cm_UnselectCurrentNameExt: 'unselects the entries with the name and extension under the cursor',
  cm_SelectFromOtherPanel: 'selects the names selected in the other panel',
  cm_SelectSearchResults: 'selects the entries the last Find files search found (no dialog)',
  cm_SelectToggle: 'selects or unselects the entry under the cursor', cm_SelectToggleDown: 'the same, and moves down',
  cm_SelUp: 'selects while moving the cursor', cm_SelDown: 'selects while moving the cursor',
  cm_SelLeft: 'selects while moving the cursor', cm_SelRight: 'selects while moving the cursor',
  cm_SelPageUp: 'selects while moving the cursor', cm_SelPageDown: 'selects while moving the cursor',
  cm_SelHome: 'selects to the top', cm_SelEnd: 'selects to the end',
  // bars and text size: layout options, changed back the same way
  cm_VisButtonbar: 'shows or hides the button bar', cm_VisButtonbar2: 'shows or hides the vertical button bar',
  cm_VisDriveButtons: 'shows or hides the drive buttons', cm_VisCurDir: 'shows or hides the current folder line',
  cm_VisTabHeader: 'shows or hides the sort header', cm_VisStatusbar: 'shows or hides the status bar',
  cm_VisCmdLine: 'shows or hides the command line (nothing is run)', cm_VisKeyButtons: 'shows or hides the function key buttons',
  cm_VisHistHotButtons: 'shows or hides the history and hotlist buttons', cm_VisDriveCombo: 'shows or hides the free space line',
  cm_VisFlatInterface: 'flat interface on or off', cm_VisFlatDriveButtons: 'flat drive buttons on or off',
  cm_ZoomIn: 'larger text', cm_ZoomOut: 'smaller text',
};

// ALLOWED_FAMILIES: numbered commands, reviewed as a family (the numbers only pick which tab or column).
export const ALLOWED_FAMILIES = [
  { re: /^cm_(Src|Left|Right|Trg)ActivateTab([1-9]\d?)$/, name: 'cm_<Src|Left|Right|Trg>ActivateTab<N>', label: 'Shows tab N (1-99) of the active, left, right or other panel', reason: 'shows another tab' },
  { re: /^cm_(Src|Left|Right|Trg)SortByCol([1-9]\d?)$/, name: 'cm_<Src|Left|Right|Trg>SortByCol<N>', label: 'Sorts by column N (1-99) of the active, left, right or other panel', reason: 'only the order on screen changes' },
  { re: /^cm_(Src|Left|Right|Trg)CustomView([1-9])$/, name: 'cm_<Src|Left|Right|Trg>CustomView<N>', label: 'Shows column set N (1-9) in the active, left, right or other panel', reason: 'a view mode' },
];

// DENIED_COMMANDS: commands the model asks for often, with the reason it gets. They are refused anyway (they are not on
// the allow-list); the reasons make the refusal clear, and the test checks none of them slipped onto the list.
export const DENIED_COMMANDS = {
  cm_Delete: 'it deletes files', cm_DeletePermanent: 'it deletes files', cm_Wipe: 'it wipes files',
  cm_RenMov: 'it moves or renames files', cm_RenameOnly: 'it renames files', cm_MultiRenameFiles: 'it renames files',
  cm_Copy: 'copying can overwrite files at the target', cm_CopySamepanel: 'copying can overwrite files',
  cm_PackFiles: 'use the pack_files tool, which never overwrites an archive', cm_PackFilesMove: 'it deletes the files after packing',
  cm_UnpackFiles: 'unpacking can overwrite files', cm_MkDir: 'it opens a dialog; folders are not created by the AI',
  cm_Edit: 'it runs the editor', cm_EditNewFile: 'it creates a file and runs the editor', cm_EditInternal: 'it edits and saves files', cm_EditComment: 'it edits file comments',
  cm_Enter: 'on a file it opens the file with its program; use open_folder to open a folder',
  cm_EnterDir: 'on a file it opens the file with its program; use open_folder to open a folder',
  cm_OpenWith: 'it runs a program', cm_InternalAssociate: 'it changes which program opens a file', cm_ExecuteDOS: 'it opens a terminal', cm_ExecuteCmdLine: 'it runs the command line',
  cm_RunKeepOpen: 'it runs a program in a terminal', cm_Exit: 'it quits mormiCommander',
  cm_Config: 'use list_settings, get_setting and set_setting', cm_ConfigChangeIniFiles: 'it opens the settings file in an editor',
  cm_PasteFromClipboard: 'pasting copies or moves files', cm_CutToClipboard: 'cutting moves files when pasted',
  cm_RestoreFromTrash: 'it moves files out of the trash',
  cm_SetAttrib: 'it changes file attributes', cm_UndoFileOp: 'undoing can move or delete files', cm_SyncDirs: 'synchronizing copies and deletes files',
  cm_ClearThumbnailCache: 'it deletes the thumbnail cache', cm_ClearDirHistory: 'it clears the folder history',
  cm_CloseAllTabs: 'it closes every tab at once', cm_FtpSendCommand: 'it sends commands to a server',
  cm_SrcQuickView: 'quick view may use an external viewer program; cm_SrcQuickInternalOnly uses the built-in one',
  cm_QuickLook: 'it hands the file to the system\'s Quick Look', cm_GitStatus: 'it runs the git program',
  cm_RemoveQuarantine: 'it removes macOS\'s quarantine flag, a security check, from files',
  cm_TerminalPane: 'it starts a shell', cm_TerminalFocus: 'it starts a shell', cm_TerminalFollow: 'it makes the terminal change folder',
  cm_TerminalInsertNames: 'it types into the terminal', cm_TerminalInsertPaths: 'it types into the terminal',
};

// Words in a command's name that mean it changes files, runs something or changes settings. A name with one of these
// is refused with this reason unless it is on the allow-list (reviewed: cm_OpenNewTab has "Open", cm_SaveSelection
// "Save", and neither does any of that).
const RISKY_WORDS = /Delete|Wipe|Move|Rename|RenMov|Exec|Run|Open|Edit|Pack|Copy|Paste|Cut|Exit|Config|Undo|Sync|Attrib|Split|Combine|Encode|Decode|Create|MkDir|Ftp|Save|Load|Import|Reset|Clear|Setup|Register|Update|Remove|Quarantine|Xattr/;

// commandVerdict answers whether the model may run a command: {ok: true, reason} or {ok: false, reason}, the reason
// being the sentence the model reads. exists tells whether the registry has the command.
export function commandVerdict(name, exists = true) {
  const n = String(name ?? '').trim();
  if (Object.hasOwn(ALLOWED_COMMANDS, n)) return exists ? { ok: true, reason: ALLOWED_COMMANDS[n] } : { ok: false, reason: `There is no command called ${n} in this version of mormiCommander.` };
  const fam = ALLOWED_FAMILIES.find((f) => f.re.test(n));
  if (fam) return exists ? { ok: true, reason: fam.reason } : { ok: false, reason: `There is no command called ${n} (that tab or column does not exist).` };
  if (!exists && !/^cm_\w+$/.test(n)) return { ok: false, reason: `There is no command called "${n}". list_commands lists the commands you can run.` };
  if (Object.hasOwn(DENIED_COMMANDS, n)) return { ok: false, reason: `${n} cannot be run by the AI: ${DENIED_COMMANDS[n]}.` };
  if (RISKY_WORDS.test(n.replace(/^cm_/, ''))) return { ok: false, reason: `${n} cannot be run by the AI: commands that may change, move or delete files, run programs or change settings are left out.` };
  if (!exists) return { ok: false, reason: `There is no command called ${n}. list_commands lists the commands you can run.` };
  return { ok: false, reason: `${n} is not on the list of commands the AI may run (navigation, view, sorting and selection only). list_commands lists them.` };
}

// ---- options (the Options dialog's fields, by "Page.Field") ----

// ALLOWED_SETTINGS: the options the model may read and change, by page: how the panels and the viewer look, sorting,
// quick search and folder tabs. The field names are written without their page on purpose: the Options audit
// (options_audit_test.go) counts a quoted "Page.Field" in the page's code as the option being read, and this list
// reads nothing. Options with no effect yet (internal/config/noeffect.go) are refused when asked for, from the schema.
export const ALLOWED_SETTINGS = {
  Layout: { why: 'which bars are shown and where the panels are: layout only',
    fields: ['ButtonBar', 'ButtonBarVertical', 'DriveBar1', 'DriveBar2', 'DriveBarFlat', 'InterfaceFlat', 'DriveCombo', 'CurDir', 'BreadCrumbBar',
      'TabHeader', 'StatusBar', 'CmdLine', 'KeyButtons', 'HistoryHotlist', 'FreeSpace', 'HiddenMarker', 'MenuBar', 'Horizontal'] },
  Display: { why: 'how the file lists look and sort',
    fields: ['ShowHidden', 'ShowSystem', 'DirBrackets', 'SeparateExt', 'SortDirsByName', 'SortUpper', 'SortNatural', 'SizeStyle', 'SizeFooter',
      'DateFormat', 'Time24', 'ShowSeconds', 'ShowParentInRoot', 'FullRowSelect', 'HoverHighlight', 'ThousandsSeparator', 'LongNames'] },
  Icons: { why: 'which icons are shown', fields: ['ShowIcons', 'IconOverlays', 'IconSize', 'IconsSpecialFolders', 'IconsInMenus'] },
  Font: { why: 'fonts and their sizes',
    fields: ['PanelFont', 'PanelSize', 'PanelBold', 'WindowFont', 'WindowSize', 'DialogFont', 'DialogSize', 'ListerFont', 'ListerSize', 'ZoomPercent'] },
  Colors: { why: 'dark mode and how the colours are used (the colours themselves: set_colors)',
    fields: ['DarkMode', 'AlternateLines', 'ColorByType', 'InactiveDimmed', 'CursorFrameOnly', 'GradientCursor', 'MarkBold', 'SelectionBgColor'] },
  QuickSearch: { why: 'how quick search matches names', fields: ['Mode', 'MatchBeginning', 'MatchEnd', 'Filter', 'Wildcards', 'IgnoreAccents', 'ShowDropDown'] },
  FolderTabs: { why: 'how the tab bar looks', fields: ['Show', 'MultiLine', 'Bottom', 'CloseButton', 'LockedIcon', 'MaxTitleChars', 'NewNextToCur', 'ActivateNew'] },
  Lister: { why: 'how the built-in viewer shows files',
    fields: ['DefaultMode', 'TabWidth', 'Charset', 'AutoDetect', 'Wrap', 'ImageFit', 'UseImages', 'UseMedia', 'UseHTML', 'UsePDF', 'BinaryWidth'] },
  Tooltips: { why: 'the tooltips of the file lists', fields: ['Show', 'DelayMs', 'ByType'] },
  Thumbnails: { why: 'how thumbnails look', fields: ['Width', 'Height', 'ShowName'] },
  Tree: { why: 'what the folder tree shows', fields: ['FollowFolder', 'ShowHidden', 'ShowFiles', 'AutoExpand'] },
  CustomColumns: { why: 'how column sets are shown', fields: ['AutoSwitch', 'ShowInShowMenu', 'EqualWidthOnOpen'] },
  ViewModes: { why: 'view modes switching by folder', fields: ['AutoSwitch'] },
};

// Text options need a check of their own: anything else could hold a path or a command line. Each allowed text field
// has a pattern its value must match ("" is always allowed: it means the default).
export const TEXT_FIELDS = {
  'Font:PanelFont': { re: /^[\p{L}\p{N} ._'-]{0,64}$/u, hint: 'a font name of letters, digits, spaces, dots and dashes (at most 64), like "Menlo"' },
  'Font:WindowFont': { re: /^[\p{L}\p{N} ._'-]{0,64}$/u, hint: 'a font name of letters, digits, spaces, dots and dashes (at most 64)' },
  'Font:DialogFont': { re: /^[\p{L}\p{N} ._'-]{0,64}$/u, hint: 'a font name of letters, digits, spaces, dots and dashes (at most 64)' },
  'Font:ListerFont': { re: /^[\p{L}\p{N} ._'-]{0,64}$/u, hint: 'a font name of letters, digits, spaces, dots and dashes (at most 64)' },
  'Display:DateFormat': { re: /^[yMd .\/,_-]{0,24}$/, hint: 'a date pattern of yyyy, yy, MM, M, dd, d and separators, like "dd.MM.yyyy"' },
  'Lister:Charset': { re: /^[A-Za-z0-9_.-]{1,32}$/, hint: 'a character set name like "auto", "utf-8" or "windows-1250"' },
  'Colors:SelectionBgColor': { re: /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/, hint: 'a colour written as #rrggbb' },
};

// Words in an option's field name (RISKY_SETTING) or label (RISKY_LABEL) that mean a program, a path, a confirmation,
// the trash, the network or the AI itself. An option with one of these is refused even when someone puts it on the list
// above. (The label check leaves out "path": "Breadcrumb path bar" is about how the folder is shown.)
const RISKY_SETTING = /path|program|editor|viewer|command|url|proxy|logfile|confirm|trash|network|onnet|plugin|external|^ai|password|server/i;
const RISKY_LABEL = /program|editor|viewer|proxy|log file|confirm|trash|network|plugin|external|password|server/i;
const DENIED_PAGES = new Set(['ViewerEditor', 'CopyDelete', 'Plugins', 'Log', 'AI', 'FTP', 'Proxy', 'Packer', 'Zip', 'Operation',
  'IgnoreList', 'Refresh', 'BackgroundQueue', 'DragDrop', 'FindFiles', 'Language', 'Hotlist', 'FavoriteTabs', 'Misc']);

const splitName = (name) => { const i = String(name ?? '').indexOf('.'); return i > 0 ? [name.slice(0, i), name.slice(i + 1)] : ['', String(name ?? '')]; };

// settingVerdict answers whether the model may read and change an option. field is the schema's field ({name, label,
// type, noEffect}) or null when the schema has no such option.
export function settingVerdict(name, field) {
  const n = String(name ?? '').trim();
  const [page, key] = splitName(n);
  if (!field) return { ok: false, reason: `There is no option called "${n}". Options are named Page.Field, like Display.ShowHidden; list_settings lists the ones you can use.` };
  const listed = ALLOWED_SETTINGS[page]?.fields.includes(key);
  if (DENIED_PAGES.has(page)) return { ok: false, reason: `The options of the ${page} page cannot be read or changed by the AI: they name programs, paths, confirmations, the network or the AI, or change what file operations do.` };
  if (!listed) return { ok: false, reason: `${n} is not one of the options the AI may change (how things look, sorting, quick search, folder tabs, the viewer's display). list_settings lists them.` };
  if (RISKY_SETTING.test(key) || RISKY_LABEL.test(field.label || '')) return { ok: false, reason: `${n} cannot be changed by the AI: it is about programs, paths, confirmations, the trash or the network.` };
  if (field.type === 'string' && !TEXT_FIELDS[`${page}:${key}`]) return { ok: false, reason: `${n} is a text option the AI may not change.` };
  if (field.noEffect) return { ok: false, reason: `${n} has no effect yet in this version of mormiCommander, so changing it would do nothing.` };
  return { ok: true, reason: ALLOWED_SETTINGS[page].why };
}

// allowedSettingNames lists "Page.Field" for every option on the list (the schema then says which exist).
export const allowedSettingNames = () => Object.entries(ALLOWED_SETTINGS).flatMap(([p, x]) => x.fields.map((f) => `${p}.${f}`));
