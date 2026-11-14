// The default keys: Total Commander's (docs/FEATURES.md sections 3, 7, 8 and 9), key → cm_* command. They are all
// here, also for commands no feature implements yet: pressing such a key says "… is not ready yet", and the key is
// already taken, so two features can never fight over one. A feature that implements cm_Copy just registers the
// command; F5 already runs it. Commands of our own (no TC equivalent) are marked "ours" in FEATURES.md.
//
// User changes (Configuration > Redefine hotkeys, or settings.json "keymap": {"Ctrl+K": "cm_Copy", "F2": ""}) are laid
// over these; "" takes a key away.

export const DEFAULT_KEYS = {
  // cursor and folders
  Up: 'cm_CursorUp', Down: 'cm_CursorDown', PgUp: 'cm_PageUp', PgDn: 'cm_PageDown', Home: 'cm_GoToFirstEntry', End: 'cm_GoToLastEntry',
  Left: 'cm_CursorLeft', Right: 'cm_CursorRight',
  Enter: 'cm_Enter', Backspace: 'cm_GoToParent', 'Ctrl+PgUp': 'cm_GoToParent', 'Ctrl+PgDn': 'cm_EnterDir',
  'Ctrl+\\': 'cm_GoToRoot', 'Ctrl+<': 'cm_GoToRoot',
  'Alt+Left': 'cm_GoToPreviousDir', 'Alt+Right': 'cm_GoToNextDir', 'Alt+Down': 'cm_DirectoryHistory',
  Tab: 'cm_FocusTrg', 'Ctrl+I': 'cm_FocusTrg', 'Shift+Tab': 'cm_FocusTreeToggle',
  'Ctrl+Left': 'cm_TransferLeft', 'Ctrl+Right': 'cm_TransferRight',
  'Ctrl+U': 'cm_Exchange', 'Ctrl+Shift+U': 'cm_ExchangeWithTabs', 'Ctrl+Shift+H': 'cm_HorizontalPanels',
  'Alt+F1': 'cm_LeftOpenDrives', 'Alt+F2': 'cm_RightOpenDrives', 'Alt+F11': 'cm_LeftCurDirBar', 'Alt+F12': 'cm_RightCurDirBar',
  'Ctrl+R': 'cm_RereadSource', F2: 'cm_RereadSource', 'Ctrl+H': 'cm_SwitchHidSys', 'Ctrl+D': 'cm_DirectoryHotlist',
  Esc: 'cm_ClearCmdLine', 'Shift+Esc': 'cm_Minimize',
  // selection
  Ins: 'cm_SelectToggleDown', Space: 'cm_SelectToggle',
  'Shift+Up': 'cm_SelUp', 'Shift+Down': 'cm_SelDown', 'Shift+PgUp': 'cm_SelPageUp', 'Shift+PgDn': 'cm_SelPageDown', 'Shift+Home': 'cm_SelHome', 'Shift+End': 'cm_SelEnd',
  'Shift+Left': 'cm_SelLeft', 'Shift+Right': 'cm_SelRight',
  'Num+': 'cm_SpreadSelection', 'Num-': 'cm_ShrinkSelection', 'Num*': 'cm_ExchangeSelection', 'Num/': 'cm_RestoreSelection',
  'Shift+Num+': 'cm_SelectBoth', 'Shift+Num-': 'cm_UnselectFiles', 'Shift+Num*': 'cm_ExchangeSelBoth',
  'Ctrl+Num+': 'cm_SelectAll', 'Ctrl+Num-': 'cm_ClearAll', 'Ctrl+Shift+Num+': 'cm_SelectAllBoth', 'Ctrl+Shift+Num-': 'cm_ClearFiles',
  'Alt+Num+': 'cm_SelectCurrentExtension', 'Alt+Num-': 'cm_UnselectCurrentExtension', 'Ctrl+A': 'cm_SelectAll',
  'Shift+F2': 'cm_CompareDirs',
  // function keys
  F1: 'cm_HelpIndex', F3: 'cm_List', F4: 'cm_Edit', F5: 'cm_Copy', F6: 'cm_RenMov', F7: 'cm_MkDir', F8: 'cm_Delete', F9: 'cm_ActivateMenu', F10: 'cm_ActivateMenu',
  'Shift+F1': 'cm_SrcCustomViewMenu', 'Shift+F3': 'cm_ListCurrent', 'Shift+F4': 'cm_EditNewFile', 'Shift+F5': 'cm_CopySamepanel',
  'Shift+F6': 'cm_RenameOnly', 'Shift+F8': 'cm_DeletePermanent', 'Shift+F10': 'cm_ContextMenu', Apps: 'cm_ContextMenu',
  Delete: 'cm_Delete', 'Shift+Delete': 'cm_DeletePermanent',
  'Alt+F3': 'cm_ListExternal', 'Alt+Shift+F3': 'cm_ListInternalOnly', 'Alt+F4': 'cm_Exit', 'Alt+F5': 'cm_PackFiles', 'Alt+Shift+F5': 'cm_PackFilesMove',
  'Alt+F6': 'cm_UnpackFiles', 'Alt+F7': 'cm_SearchFor', 'Alt+F8': 'cm_ShowCmdLineHistory', 'Ctrl+Down': 'cm_ShowCmdLineHistory',
  'Alt+F9': 'cm_UnpackFiles', 'Alt+Shift+F9': 'cm_TestArchive', 'Alt+F10': 'cm_CDtree', 'Alt+Shift+F11': 'cm_FocusButtonBar',
  'Alt+Enter': 'cm_Properties', 'Alt+Shift+Enter': 'cm_CountDirContent', 'Shift+Enter': 'cm_RunKeepOpen',
  'Ctrl+F1': 'cm_SrcShort', 'Ctrl+F2': 'cm_SrcLong', 'Ctrl+F3': 'cm_SrcByName', 'Ctrl+F4': 'cm_SrcByExt', 'Ctrl+F5': 'cm_SrcByDateTime',
  'Ctrl+F6': 'cm_SrcBySize', 'Ctrl+F7': 'cm_SrcUnsorted', 'Ctrl+F8': 'cm_SrcTree', 'Ctrl+F9': 'cm_PrintFile', 'Ctrl+F10': 'cm_SrcAllFiles',
  'Ctrl+F11': 'cm_SrcExecs', 'Ctrl+F12': 'cm_SrcUserSpec', 'Ctrl+Shift+F1': 'cm_SrcThumbs', 'Ctrl+Shift+F2': 'cm_SrcComments',
  'Ctrl+Shift+F5': 'cm_CreateShortcut', 'Ctrl+Shift+F8': 'cm_SeparateTreeCycle',
  // letters
  'Ctrl+B': 'cm_DirBranch', 'Ctrl+Shift+B': 'cm_DirBranchSel', 'Ctrl+C': 'cm_CopyToClipboard', 'Ctrl+X': 'cm_CutToClipboard', 'Ctrl+V': 'cm_PasteFromClipboard',
  'Ctrl+E': 'cm_CmdLineHistoryPrev', 'Ctrl+F': 'cm_FtpConnect', 'Ctrl+Shift+F': 'cm_FtpDisconnect', 'Ctrl+N': 'cm_FtpNew', 'Ctrl+Shift+M': 'cm_FtpTransferMode',
  'Ctrl+L': 'cm_GetFileSpace', 'Ctrl+M': 'cm_MultiRenameFiles', 'Ctrl+P': 'cm_AddPathToCmdline', 'Ctrl+Q': 'cm_SrcQuickView',
  'Ctrl+S': 'cm_QuickFilter', 'Ctrl+Shift+S': 'cm_QuickFilterLast', 'Ctrl+T': 'cm_OpenNewTab', 'Ctrl+Shift+T': 'cm_OpenNewTabBg',
  'Ctrl+W': 'cm_CloseCurrentTab', 'Ctrl+Shift+W': 'cm_CloseAllTabs', 'Ctrl+Z': 'cm_EditComment',
  'Ctrl+Tab': 'cm_SwitchToNextTab', 'Ctrl+Shift+Tab': 'cm_SwitchToPreviousTab', 'Ctrl+Up': 'cm_OpenDirInNewTab', 'Ctrl+Shift+Up': 'cm_OpenDirInNewTabOther',
  'Ctrl+Enter': 'cm_AddFilenameToCmdline', 'Ctrl+Shift+Enter': 'cm_AddPathAndNameToCmdline',
  'Ctrl+=': 'cm_ZoomIn', 'Ctrl+-': 'cm_ZoomOut', 'Ctrl+Shift+=': 'cm_ZoomIn', // (Ctrl++ : the + is Shift+= on most keyboards)
};

// Extra keys on macOS only (F-285, F-289). The TC keys above work on macOS too, with Control and Option.
export const MAC_KEYS = {
  'Cmd+C': 'cm_CopyToClipboard', 'Cmd+X': 'cm_CutToClipboard', 'Cmd+V': 'cm_PasteFromClipboard', 'Cmd+A': 'cm_SelectAll',
  'Cmd+T': 'cm_OpenNewTab', 'Cmd+W': 'cm_CloseCurrentTab', 'Cmd+Q': 'cm_Exit', 'Cmd+,': 'cm_Config', 'Cmd+Backspace': 'cm_Delete',
  'Shift+Cmd+.': 'cm_SwitchHidSys', 'Cmd+R': 'cm_RereadSource', 'Cmd+Up': 'cm_GoToParent', 'Cmd+Down': 'cm_Enter', 'Cmd+I': 'cm_Properties',
  'Cmd+L': 'cm_EditPath', 'Cmd+F': 'cm_SearchFor', 'Shift+Cmd+G': 'cm_EditPath', 'Alt+Cmd+C': 'cm_CopyFullNamesToClip',
  'Alt+Shift+Cmd+C': 'cm_CopySrcPathToClip', 'Cmd+Z': 'cm_UndoFileOp', // undo of file operations (ours, board #13)
  'Alt+Cmd+Left': 'cm_TransferLeft', 'Alt+Cmd+Right': 'cm_TransferRight', 'Alt+Cmd+Up': 'cm_OpenDirInNewTab',
  'Cmd+=': 'cm_ZoomIn', 'Cmd+-': 'cm_ZoomOut', 'Shift+Cmd+=': 'cm_ZoomIn', 'Cmd+[': 'cm_GoToPreviousDir', 'Cmd+]': 'cm_GoToNextDir',
};

// Keyboards without a numpad (most laptops): Shift+= / Shift+- / Shift+8 act as Num+ / Num- / Num*, but only while the
// command line is empty, so the characters can still be typed (F-288).
export const NUMPAD_FALLBACK = { 'Shift+=': 'Num+', 'Shift+-': 'Num-', 'Shift+8': 'Num*' };
