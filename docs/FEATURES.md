# mormiCommander feature checklist (Total Commander 11.x parity)

This is the parity checklist for mormiCommander, a free clone of Total Commander (Ghisler) for macOS, Windows and Linux.
It is based on the Total Commander 11.x menus, keyboard reference and cm_* command list.

## How to use this file

- One line per item: `- [status] F-NNN <feature> — <hotkey> — <acceptance criterion>`.
- Status: `[ ]` todo, `[~]` partly done, `[x]` done and checked by QA (only QA agents set `[x]`).
- `[N/A]` items can't be done the same way on all platforms: `— reason — our equivalent`.
- IDs are fixed. New items go at the end of a section with the next free number (never renumber).
- Hotkey `—` means no default key; the command is still in the command registry (cm_* name) and can be given a key by the user.
- Acceptance criteria must be checked in the real app through control mode (`--control 0`, `scripts/mc_ctl.py`) or unit tests, always in fresh temp folders.
- Where the criterion says "same as menu", the item passes when the menu entry runs the same cm_* command as its hotkey and button.

## Hotkey rule for macOS (and why TC keys still work everywhere)

1. Every Total Commander default hotkey is bound by physical key on all three platforms: Ctrl means the Control key, Alt means Alt (Option on macOS), Shift is Shift. So F5 copies, Ctrl+T opens a tab, Alt+F7 finds files on Windows, Linux and macOS alike.
2. macOS gets extra Cmd aliases for the usual Mac habits (Cmd+C/V/X/A/T/W/Q/,/R/F/L/I, Cmd+Backspace, Cmd+Up/Down, Cmd+Shift+. and Cmd+Shift+G). They run the same cm_* commands. They don't exist on Windows or Linux.
3. Hotkeys match on `KeyboardEvent.code` with a `key` fallback, so Option+letter (which types special characters on a Mac) and non-US layouts such as Hungarian still work.
4. Where macOS takes a key for itself (Ctrl+Up/Down/Left/Right for Mission Control and Spaces, Ctrl+F2/F3 for menu and Dock focus, F11 for Show Desktop), docs/keyboard.md lists a Cmd+Option alternative and the function key button row gives a mouse option.
5. Missing keys: no numpad means Shift+= / Shift+- / Shift+8 work as Num+ / Num- / Num* (only while the command line is empty); no Ins key means Space.
6. The user can change every key in Configuration > Misc > Redefine hotkeys. "Restore defaults" brings back the TC keys.



## 1. Main window and layout

- [ ] F-001 Main window with two file panels side by side, one active (highlighted header) — — — app starts with left and right panel, active panel has a visibly highlighted path bar; clicking a panel makes it active
- [ ] F-002 Switch active panel — Tab — Tab moves focus to the other panel; cursor position in each panel is kept
- [ ] F-003 Vertical / horizontal panel arrangement (cm_HorizontalPanels) — Ctrl+Shift+H (TC 11 default none; ours) — panels can be stacked top/bottom and back; setting is saved
- [ ] F-004 Swap panels (cm_Exchange) — Ctrl+U — left and right panel contents (path, tabs, sort, view) swap places
- [~] F-005 Swap panels with tabs (cm_ExchangeWithTabs) — Ctrl+Shift+U — whole tab sets of both sides swap
- [ ] F-006 Target = source (cm_MatchSrc) — — (Commands menu) — other panel shows the same directory as the active one
- [~] F-007 Make other panel show folder under cursor (cm_TransferLeft / cm_TransferRight) — Ctrl+Left / Ctrl+Right — when cursor is on a folder in the left panel, Ctrl+Right opens it in the right panel (and vice versa); on a file it opens the current folder there
- [~] F-008 Resizable splitter between panels — — — dragging the splitter changes panel widths; double-click resets to 50/50; context menu offers 50/50, 60/40, 70/30 ... splits
- [ ] F-009 Panel size presets (cm_50Percent, cm_100Percent etc.) — — — commands set the splitter position; cm_100Percent maximizes active panel (other panel hidden until toggled back)
- [ ] F-010 Menu bar — — — menu bar with Files, Mark, Commands, Net, Show, Configuration, Start, Help; on macOS the native app menu bar carries the same menus
- [ ] F-011 Path bar per panel (current directory line) — — — shows full path; clicking it allows editing the path; Enter navigates; Esc cancels
- [ ] F-012 Path bar click on part of path — — — clicking a path segment jumps to that parent folder
- [ ] F-013 Path bar right-click / history arrow — — — small buttons at end of path bar open directory history and hotlist
- [ ] F-014 Panel header with column titles — — — column headers shown in Full view; click sorts, click again reverses, drag edge resizes
- [ ] F-015 Status bar per panel — — — shows "x of y KB in a of b files selected" and updates live on selection change
- [~] F-016 Drive info line (free space) — — — above each panel shows the volume name, free and total space of the current drive/volume
- [ ] F-017 Window title shows app name and active path option — — — title is "mormiCommander" (optionally with path); configurable
- [ ] F-018 Remember window position, size, maximized state, splitter — — — after restart, the window reopens where and how it was closed (per-monitor safe, like mormiview winstate.go)
- [ ] F-019 Full-screen / maximize toggle — — — window can be maximized; on macOS native full screen works
- [ ] F-020 Font and color settings applied to panels — — — user-changed font size/colors apply immediately to both panels
- [ ] F-021 Dark and light theme following system (prefers-color-scheme) with manual override — — — switching the OS theme switches the app; Configuration lets the user force light or dark
- [ ] F-022 Show/hide hidden elements: menu, button bar, drive bar, command line, status bar, tab bar, function key buttons, current dir, sort header, drive combobox, vertical button bar — — — each element can be turned on and off in Configuration > Layout and the layout updates without restart
- [ ] F-023 Function key button row at bottom (F3 View ... Alt+F4 Exit) — — — clicking a button runs the same command as the key; labels change while Shift/Alt/Ctrl is held (TC shows shifted functions)
- [ ] F-024 Vertical button bar between panels (TC 9+) — — — optional middle bar with configurable buttons (e.g. copy/move arrows)
- [ ] F-025 Separate tree panel (cm_SeparateTreeOff/1/2) — — — Show > Separate tree: none, one shared tree, or one tree per panel; tree follows panel path; clicking a tree node navigates
- [ ] F-026 Two-panel visual feedback when other panel is target — — — inactive panel is dimmed slightly; target panel is the one copy/move goes to
- [ ] F-766 Focus left / right / source panel directly (cm_FocusLeft / cm_FocusRight / cm_FocusSrc) — — — each command puts keyboard focus in the named panel
- [ ] F-767 Show/hide each layout element by command (cm_VisButtonbar, cm_VisButtonbar2, cm_VisDriveButtons, cm_VisTwoDriveButtons, cm_VisFlatDriveButtons, cm_VisDriveCombo, cm_VisCurDir, cm_VisBreadCrumbs, cm_VisTabHeader, cm_VisStatusbar, cm_VisCmdLine, cm_VisKeyButtons, cm_VisDirTabs, cm_VisHistHotButtons, cm_VisFlatInterface) — — — each command toggles its element, its menu entry shows a check mark, and the state is saved
- [ ] F-768 Focus the button bar for keyboard use (cm_FocusButtonBar) — Alt+Shift+F11 — first button gets focus; Left/Right move, Enter runs, Esc goes back to the file list
- [ ] F-769 Switch between file list and separate tree — Shift+Tab — with the separate tree on, Shift+Tab focuses the tree of the active side; pressing it again goes back to the file list
- [ ] F-770 Minimize / maximize / restore window (cm_Minimize, cm_Maximize, cm_Restore) — Shift+Esc (minimize) — each command changes the window state
- [ ] F-771 Alt+F4 minimizes instead of quitting (option, TC MinimizeOnClose) — Alt+F4 — with the option on, Alt+F4 and the close button minimize; File > Quit still quits
- [ ] F-772 Breadcrumb path bar with keyboard (TC current directory bar) — Alt+F11 / Alt+F12 — opens the left/right path bar with focus; Left/Right move between parts, Down lists the subfolders of that part, Enter goes there, Esc returns

## 2. Folder tabs

- [ ] F-027 Open new tab with current folder (cm_OpenNewTab) — Ctrl+T — a new tab appears in the active panel showing the same folder and becomes active
- [ ] F-028 Open folder under cursor in new tab (cm_OpenDirInNewTab) — Ctrl+Up — new tab is created for the folder under cursor (in background or foreground per setting)
- [ ] F-029 Open folder under cursor in new tab of other panel (cm_OpenDirInNewTabOther) — Ctrl+Shift+Up — tab created in the other panel
- [ ] F-030 Close current tab (cm_CloseCurrentTab) — Ctrl+W — active tab closes; last tab cannot be closed (or shows confirmation per setting)
- [ ] F-031 Close all tabs (cm_CloseAllTabs) — Ctrl+Shift+W — all tabs except the current one are closed in active panel
- [ ] F-032 Next tab / previous tab (cm_SwitchToNextTab / cm_SwitchToPreviousTab) — Ctrl+Tab / Ctrl+Shift+Tab — cycles tabs in the active panel; wraps around
- [~] F-033 Switch to tab by number (cm_SrcActivateTab1..99) — — (assignable) — command activates the n-th tab of the active panel
- [ ] F-034 Tab context menu — — — right-click on a tab: close, close all, lock, lock with folder changes allowed, rename, duplicate, move to other side, save/load tabs
- [ ] F-035 Lock tab (cm_ToggleLockCurrentTab) — — — locked tab shows a marker; navigating away opens a new tab instead of changing the locked tab
- [ ] F-036 Lock tab but allow folder changes (cm_ToggleLockDcaCurrentTab) — — — tab keeps marker, folder changes allowed, returns to home folder on reactivation per option
- [~] F-037 Rename tab — — — user can give a tab a custom caption; caption persists
- [ ] F-038 Duplicate tab / move tab to other panel — — — tab duplicates or moves; tab state (sort, view) is kept
- [~] F-039 Reorder tabs by dragging — — — dragging a tab header changes order; dropping on other panel's tab bar moves it
- [ ] F-040 Drop files onto a tab — — — dropping files on a tab header copies/moves into that tab's folder
- [~] F-041 Double-click on tab bar empty area opens new tab; double-click on tab closes it (option) — — — behaviour matches Configuration > Folder tabs settings
- [~] F-042 Middle-click tab closes it — — — middle-click closes the tab
- [ ] F-043 Save tabs to file / load tabs from file (cm_SaveTabs / cm_LoadTabs) — — — tabs of a panel are written to a .tab-style file and can be restored
- [ ] F-044 Restore tabs on startup — — — after restart all tabs (per side) with their paths, locks and active tab are restored
- [~] F-045 Tabs in directory hotlist (folder entries that open a tab set) — — — hotlist entry can store a tab set; choosing it restores those tabs
- [~] F-046 Tab bar options: show on one line / multiple lines, show only when >1 tab, width limit, "*" marker for locked — — — each option visibly changes the tab bar
- [~] F-047 Per-tab history — — — Alt+Left/Right history is per tab
- [ ] F-048 Ctrl+click on folder opens it in new tab (mouse) — — — Ctrl/Cmd+double-click opens folder in a new tab
- [ ] F-773 Open new tab in background (cm_OpenNewTabBg) — Ctrl+Shift+T — a new tab with the current folder is added; the current tab stays active
- [~] F-774 Activate tab n in left / right / target panel (cm_LeftActivateTab1..99, cm_RightActivateTab1..99, cm_TrgActivateTab1..99) — — (assignable) — command switches the named panel to its n-th tab
- [ ] F-775 Favorite tabs (cm_LoadFavoriteTabs, cm_SaveFavoriteTabs, cm_ConfigFavoriteTabs) — — — both panels' tab sets saved under a name; Favorite tabs menu (tab context menu and tab bar button) restores them; optional hotkey per entry
- [ ] F-776 Reopen last closed tab (cm_ReopenClosedTab, ours) — — — the closed tab comes back with its folder, lock and sort
- [~] F-777 Open in new tab from history and hotlist — Shift+Enter — in the Alt+Down history list and the Ctrl+D hotlist, Shift+Enter opens the chosen folder in a new tab

## 3. Navigation in panels

- [ ] F-049 Move cursor — Up / Down / PgUp / PgDn / Home / End — cursor moves, list scrolls; Home/End go to first/last item
- [ ] F-050 Left / Right in Brief view moves between columns — Left / Right — in Brief view Left/Right moves by a column
- [~] F-051 Open folder / run file — Enter — Enter on a folder opens it; on a file opens it with the system default app; on an archive opens it as folder (per setting)
- [ ] F-052 Go to parent folder — Backspace / Enter on ".." — parent folder is shown and the cursor lands on the folder we came from
- [ ] F-053 Go to root of drive/volume (cm_GoToRoot) — Ctrl+\ (US) / Ctrl+< (European keyboards) — panel shows the root of the current drive (/ or the volume root on macOS/Linux)
- [ ] F-054 Go back / forward in history (cm_GoToPreviousDir / cm_GoToNextDir) — Alt+Left / Alt+Right — goes to previously visited folder and back again
- [~] F-055 Directory history list (cm_DirectoryHistory) — Alt+Down — popup list of recently visited folders; choosing one navigates
- [ ] F-056 Folder history in path bar dropdown — — — arrow on path bar shows same list
- [ ] F-057 Target = source (cm_MatchSrc) from navigation — — — other panel shows the active panel's folder
- [~] F-058 Open drive/volume list (cm_SrcOpenDrives / drive combobox) — Alt+F1 / Alt+F2 — popup for left/right panel lists drives/volumes with labels and free space; choose one to switch
- [~] F-059 Drive bar buttons — — — one button per drive/volume; click switches active panel; right-click shows context menu
- [ ] F-060 Two drive bars (one per panel) option — — — Configuration allows separate drive bar per panel
- [ ] F-061 Edit path bar (cm_EditPath) — — (assignable; Cmd+L on macOS) — path bar enters edit mode with text selected; Enter navigates. Note: TC's Alt+F11 / Alt+F12 open the left/right current directory bar (breadcrumb) for keyboard use, see F-772
- [~] F-062 Change folder from command line with cd — — — typing "cd <path>" in the command line and Enter changes the active panel folder; "cd .." and "cd \" work
- [ ] F-063 Go to home folder — — — "~" in command line or a hotlist entry jumps to the user's home folder
- [ ] F-064 Reread source (cm_RereadSource) — Ctrl+R — active panel refreshes and shows new/removed files; cursor stays on same file if it still exists
- [~] F-065 Automatic refresh when files change on disk — — — a file created by another program appears within ~1 s without manual refresh (fsnotify)
- [~] F-066 Quick search: type letters to jump to file — Ctrl+Alt+letter (TC default) / letter only (option) — a small search box opens under the panel, cursor jumps to first match; Up/Down jump to previous/next match; Esc closes
- [ ] F-067 Quick search options: letters only, Alt+letters, Ctrl+Alt+letters, match beginning or any part, filter mode — — — each option works as set in Configuration > Quick search
- [~] F-068 Quick filter (show only matching files) — Ctrl+S — typing filters the panel to matching files; Esc removes filter
- [ ] F-069 Quick search wildcards (* and ?) — — — "a*b" matches names with a then b
- [~] F-070 Quick search with first-letter jump to folders only (option) — — — option "search files and folders / only folders" respected
- [ ] F-071 Folder path navigation through archives and FTP in path bar — — — path bar can show archive\inner\path and ftp://host/path and navigation works there too
- [ ] F-072 Double-click on empty area goes to parent (option) — — — when enabled, double-click below the list goes up one level
- [ ] F-073 Mouse back/forward buttons navigate history — — — mouse buttons 4/5 trigger Alt+Left/Alt+Right
- [ ] F-074 Cursor follows on folder up (remember last file per folder) — — — going into a folder and back puts cursor on that folder; returning into a recently visited folder restores its last cursor
- [ ] F-075 Go to file under cursor in other panel's folder (cm_GoToDir) — — — command opens the target of a shortcut/symlink and focuses the file
- [~] F-076 Follow symlink / alias target (cm_GoToDir on link) — Ctrl+PgDn on link — for a symlink, the panel jumps to the target folder with cursor on the target
- [ ] F-077 Open parent folder of file in path line — Ctrl+PgUp — Ctrl+PgUp goes to parent folder (same as Backspace)
- [~] F-078 Open folder under cursor — Ctrl+PgDn — Ctrl+PgDn opens folder or archive under cursor
- [ ] F-778 Root jump on European keyboards — Ctrl+< — the key next to left Shift (KeyboardEvent.code IntlBackslash, e.g. the í key on Hungarian layout) jumps to the root like Ctrl+\
- [ ] F-779 Go to first / last entry, first file, next / previous selected (cm_GoToFirstEntry, cm_GoToLastEntry, cm_GoToFirstFile, cm_GoToNextSelected, cm_GoToPrevSelected) — — — cursor jumps as named; with no selection the selected-file commands do nothing
- [~] F-780 Reuse last quick filter — Ctrl+Shift+S — quick filter opens with the last used text already applied
- [~] F-781 Clear quick filter with Ctrl+F10 — Ctrl+F10 — while a quick filter is on, Ctrl+F10 (all files) removes it
- [ ] F-782 Auto-complete in path bar and dialog path fields (copy target, Find files 'Search in') — — — typing part of a path offers matching folders; Tab or Right accepts
- [~] F-783 Type-to-select in drive popup — Alt+F1 / Alt+F2 then letter — typing a drive letter or the first letter of a volume name picks it

## 4. Views and display modes

- [ ] F-079 Brief view (cm_SrcShort / cm_SrcBrief) — Ctrl+F1 — names only in multiple columns
- [ ] F-080 Full view (cm_SrcLong / cm_SrcFull) — Ctrl+F2 — one line per file with name, ext, size, date, attributes
- [ ] F-081 Tree view in panel (cm_SrcTree) — Ctrl+F8 — panel shows folder tree of the drive; Enter switches back to the folder in Full view
- [ ] F-082 Quick view panel (cm_SrcQuickView) — Ctrl+Q — other panel shows preview of the file under cursor (text, image, media, hex) and updates as cursor moves; Ctrl+Q again restores
- [ ] F-083 Thumbnails view (cm_SrcThumbs) — Ctrl+Shift+F1 — panel shows image previews as tiles with names; sizes configurable; cached thumbnails
- [ ] F-084 Custom columns view (cm_SrcCustomViewMenu / custom column sets) — Shift+F1 (column set menu) — user-defined column sets can be chosen per panel
- [ ] F-085 Comments view (cm_SrcComments) — Ctrl+Shift+F2 — shows file comments (descript.ion) in a column
- [ ] F-086 Branch view (cm_DirBranch) — Ctrl+B — flat list of all files in current folder and all subfolders with relative path; Ctrl+B again returns
- [ ] F-087 Branch view of selected files and folders only (cm_DirBranchSel) — Ctrl+Shift+B — flat view only of selected folders
- [ ] F-088 Show all files / programs only / custom filter (cm_SrcAllFiles / cm_SrcExecs / cm_SrcUserSpec) — Ctrl+F10 / Ctrl+F11 / Ctrl+F12 — panel filter changes; status bar or path shows filter is active
- [ ] F-089 Define custom filter (cm_SrcUserDef) — — — dialog lets user enter wildcards like *.jpg;*.png or a saved search; panel shows only matching files
- [x] F-090 Hide/show hidden and system files (cm_SwitchHidSys) — Ctrl+H (ours; TC: config) — dotfiles and hidden files toggle visible/invisible immediately; while shown, each status line ends with "Hidden: shown" (Options > Layout can turn the marker off)
- [~] F-091 Show folder sizes in Full view after Space/calc — — — folder size column shows <DIR> until calculated, then real size
- [ ] F-092 Long names vs 8.3: show file extension in separate column option — — — option to show ext in separate column or with name
- [~] F-093 Display size in bytes / KB / MB / dynamic — — — size format option changes column output
- [~] F-094 Date/time format option — — — date column follows chosen format (system or custom)
- [ ] F-095 Icons: show all associated icons / standard / none; overlay icons — — — each mode shows appropriate icons; system file icons loaded from OS
- [ ] F-096 Selection colours, cursor bar style (filled / frame), alternating row colors — — — each display option applied immediately
- [ ] F-097 Colour by file type (Configuration > Colors > Define colors by file type) — — — files matching a pattern or attribute filter are shown in the chosen colour
- [ ] F-098 Sort folders first, by name always option — — — folders listed before files and sorted by name regardless of file sort (configurable)
- [ ] F-099 Inverse selection marking styles (selected = red text TC default) — — — selected files shown in a distinct color and counted in status bar
- [ ] F-100 Show path in tab / window title options — — — options alter labels
- [~] F-101 Zoom panel font larger/smaller — Ctrl++ / Ctrl+- (ours, Cmd on macOS) — font size of panels changes and is saved
- [ ] F-102 Show ".." entry in every folder (option) — — — ".." line present at top of list unless turned off
- [ ] F-103 Show hidden files greyed / with ghost icon — — — hidden files are visually dimmed when shown
- [ ] F-104 Horizontal scrolling in Full view when columns exceed width — — — horizontal scrollbar appears; no layout break
- [ ] F-105 Long file name ellipsis and tooltip with full name — — — truncated names end with "..."; hover tooltip shows full name (data-tip tooltip style)
- [ ] F-106 Hint tooltip with file info on hover (option) — — — hovering a file shows size, dates, type after a delay
- [ ] F-784 Quick view with the built-in viewer only (cm_SrcQuickInternalOnly) — — — quick view shows text/hex with the built-in viewer, skipping image/media viewers
- [ ] F-785 View modes (cm_SrcViewModeList, Configuration > Display > View modes) — — — a view mode bundles column set, icons, colors and font; can switch on its own by folder or file-type rules; Show menu lists them
- [ ] F-786 Left / right / target variants of view, sort and filter commands (cm_LeftShort, cm_RightLong, cm_TrgThumbs, cm_LeftByName, cm_RightAllFiles, ...) — — — each variant acts only on the named panel; all appear in the command browser
- [ ] F-787 Toggle overlay icons and 'folders always by name' (cm_SwitchOverlayIcons, cm_SwitchDirSort) — — — each command changes the panel at once and the menu shows its state
- [ ] F-788 Toggle ignore list on/off (cm_SwitchIgnoreList) — — — ignored files show up, and hide again on the next toggle
- [ ] F-789 Custom tooltips per file type (Configuration > Display > Tooltips) — — — hovering e.g. an image shows the fields set for images (size, date, image size)

## 5. Sorting

- [ ] F-107 Sort by name (cm_SrcByName) — Ctrl+F3 — sorted by name ascending; again descending
- [ ] F-108 Sort by extension (cm_SrcByExt) — Ctrl+F4 — sorted by extension
- [ ] F-109 Sort by date/time (cm_SrcByDateTime) — Ctrl+F5 — newest first by default
- [ ] F-110 Sort by size (cm_SrcBySize) — Ctrl+F6 — largest first by default
- [ ] F-111 Unsorted (cm_SrcUnsorted) — Ctrl+F7 — file system order
- [ ] F-112 Reverse sort order (cm_SrcNegOrder) — — — order is reversed
- [ ] F-113 Sort by clicking column headers incl. custom columns — — — header shows arrow for direction
- [ ] F-114 Multi-column sort with Ctrl+click header — — — Ctrl+click adds secondary sort key
- [ ] F-115 Natural sorting (numbers as numbers: file2 before file10) option — — — "file2" sorts before "file10" when option set
- [ ] F-116 Case-sensitive / Unicode / locale-aware sorting option — — — options change order per spec
- [~] F-117 Sort settings per tab remembered — — — each tab keeps its sort
- [~] F-790 Sort by column n of the current column set (cm_SrcSortByCol1..99, cm_LeftSortByCol.., cm_RightSortByCol..) — — (assignable) — command sorts by that column; running it again reverses the order

## 6. Selection (marking)

- [~] F-118 Select/unselect file under cursor and move down — Ins / Space — file toggles selection; Space on a folder also calculates folder size
- [ ] F-119 Shift+cursor selects range — Shift+Up/Down/PgUp/PgDn/Home/End — range selection extends with cursor
- [ ] F-120 Select group by wildcard (cm_SelectFiles / cm_SpreadSelection) — Num + — dialog "Select files" with pattern (e.g. *.txt) and saved filters; matching files get selected
- [ ] F-121 Unselect group (cm_ShrinkSelection / cm_UnselectFiles) — Num - — dialog unselects matching files
- [ ] F-122 Select all (cm_SelectAll) — Ctrl+A / Ctrl+Num + — all files (and folders per option) selected
- [ ] F-123 Unselect all (cm_ClearAll) — Ctrl+Num - — selection cleared
- [ ] F-124 Invert selection (cm_ExchangeSelection) — Num * — selection inverted (files only, or files+folders with Shift+Num *)
- [ ] F-125 Invert selection incl. folders (cm_ExchangeSelBoth) — Shift+Num * — folders included
- [ ] F-126 Select all files with same extension (cm_SelectCurrentExtension) — Alt+Num + — all files with extension of the file under cursor are selected
- [ ] F-127 Unselect all with same extension (cm_UnselectCurrentExtension) — Alt+Num - — those files are unselected
- [ ] F-128 Select all files with same name (cm_SelectCurrentName / cm_UnselectCurrentName) — — — by base name
- [ ] F-129 Select files with same name and ext (cm_SelectCurrentNameExt) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-130 Select only files / only folders (cm_SelectAllFiles / cm_SelectAllFolders) — — — menu commands select by type
- [ ] F-131 Restore previous selection (cm_RestoreSelection) — Num / — last selection is restored after an operation cleared it
- [ ] F-132 Save / load selection to file (cm_SaveSelection / cm_LoadSelection / cm_SaveSelectionToFile / cm_LoadSelectionFromFile) — — — names saved to text file, can be reloaded; also from clipboard (cm_LoadSelectionFromClip)
- [~] F-133 Compare directories: select newer / missing (cm_CompareDirs) — Shift+F2 — files that are newer or missing on the other side are selected in both panels
- [~] F-134 Compare directories including subdirectories (cm_CompareDirsWithSubdirs) — — — also marks subfolders that differ
- [ ] F-135 Select by mouse: left-click+drag selects (TC style) or Windows-style Ctrl+click — — — both mouse modes available (Configuration > Operation); Ctrl/Cmd+click toggles, Shift+click range
- [ ] F-136 Right mouse button selection mode (TC classic) — — — option: right button selects, long press opens context menu
- [ ] F-137 Selection count/size shown in status bar — — — status bar shows count and total size of selected files
- [ ] F-138 Copy names to clipboard (cm_CopyNamesToClip) — — — selected file names (one per line) copied to clipboard
- [ ] F-139 Copy full names with path (cm_CopyFullNamesToClip) — — (assignable; Cmd+Alt+C on macOS) — full paths copied, one per line
- [ ] F-140 Copy names with details (cm_CopyDetailsToClip / cm_CopyFullDetailsToClip) — — — names plus size/date/attributes, tab-separated
- [ ] F-141 Copy path of current folder (cm_CopySrcPathToClip) — — — current folder path copied
- [ ] F-142 Copy names as Unix/URL paths, network names (cm_CopyNetNamesToClip) — — — variants supported where meaningful
- [~] F-143 Select same names as in other panel (cm_SelectFromOtherPanel, ours) — — — files whose names are selected in the other panel get selected here
- [ ] F-791 Select group including folders (cm_SelectBoth) — Shift+Num + — like Num +, but selects files and folders when Num + selects only files (and the other way round)
- [ ] F-792 Unselect group from files only (cm_UnselectFiles) — Shift+Num - — removes the selection from matching files only; folders stay selected
- [ ] F-793 Select all including folders (cm_SelectAllBoth) — Ctrl+Shift+Num + — files and folders selected when Ctrl+Num + selects only files
- [ ] F-794 Unselect all files only (cm_ClearFiles) — Ctrl+Shift+Num - — files lose selection, folders keep it
- [ ] F-795 Invert files only / folders only, unselect folders only (cm_ExchangeSelFiles, cm_ExchangeSelFolders, cm_ClearFolders) — — — each command acts on the named kind only
- [ ] F-796 Copy target panel path to clipboard (cm_CopyTrgPathToClip) — — — path of the other panel's folder copied
- [~] F-797 Option: Space selects only, without counting folder size (Operation > Selection with Space) — Space — with the option off, Space on a folder selects it without calculating the size

## 7. Function keys and F-key commands

- [ ] F-144 Help (cm_HelpIndex) — F1 — opens help (offline HTML help page) for the current context
- [ ] F-145 Context help in dialogs — F1 — F1 in a dialog opens that dialog's help section
- [ ] F-146 Rename file under cursor in place (cm_RenameOnly / inline rename) — Shift+F6 — name becomes editable in the list; Enter commits, Esc cancels; extension not selected by default (option)
- [ ] F-147 Reread / refresh (cm_RereadSource) — F2 (TC classic) / Ctrl+R — panel refreshed
- [ ] F-148 View file with Lister (cm_List) — F3 — internal viewer opens the file under cursor
- [ ] F-149 View only file under cursor (cm_List ignoring selection) — Shift+F3 — Lister opens only the file under cursor even when several files are selected
- [ ] F-150 Quick view toggle — Ctrl+Q — see Views
- [ ] F-151 Lister on all selected files — F3 with selection — Lister opens first selected file; N/P keys go to next/previous selected file
- [ ] F-152 Edit file (cm_Edit) — F4 — file opens in the configured editor (default: OS default text editor; configurable path and args)
- [ ] F-153 Edit new file (cm_EditNewFile) — Shift+F4 — asks for a file name, creates it if missing and opens it in the editor
- [x] F-154 Copy (cm_Copy) — F5 — copy dialog with target = other panel folder; Enter copies
- [ ] F-155 Copy in same folder / under new name (cm_CopySamepanel) — Shift+F5 — asks for new name; creates copy in the same folder
- [ ] F-156 Pack files (cm_PackFiles) — Alt+F5 — pack dialog opens with archive name and packer choice
- [ ] F-157 Move/rename (cm_RenMov) — F6 — move dialog with target other panel; moving within the same disk is instant rename
- [ ] F-158 Rename in place (cm_RenameOnly) — Shift+F6 — inline rename editor
- [ ] F-159 Unpack (cm_UnpackFiles) — Alt+F9 — unpack dialog for selected archives
- [ ] F-160 Test archives (cm_TestArchive) — Alt+Shift+F9 — archives are tested; result shown per archive
- [ ] F-161 Make folder (cm_MkDir) — F7 — dialog asks name; supports nested "a/b/c"; new folder gets the cursor
- [ ] F-162 Make folder with template/history list (TC 10+) — F7 — dropdown has last created names and options like "<date>"
- [ ] F-163 Find files (cm_SearchFor) — Alt+F7 — Find files dialog
- [ ] F-164 Find in separate process (cm_SearchStandalone) — — — search window runs independently; main window stays usable
- [ ] F-165 Delete (cm_Delete) — F8 / Delete — confirmation dialog; deletes to trash (Recycle Bin) by default
- [ ] F-166 Delete permanently (cm_Delete with Shift) — Shift+F8 / Shift+Delete — deletes without trash after confirmation
- [ ] F-167 Command line history (cm_ShowCmdLineHistory) — Alt+F8 — dropdown with previous command lines
- [ ] F-168 Activate menu above current panel (cm_ActivateMenu) — F9 — focuses the menu; on macOS opens the app menu bar equivalent (in-window menu)
- [ ] F-169 Unpack — Alt+F9 — see unpack
- [ ] F-170 Activate main menu — F10 — focuses the main menu
- [ ] F-171 Tree of folders (cm_CDtree) — Alt+F10 — popup folder tree of current drive for fast navigation with type-to-search
- [ ] F-172 Show context menu of file (cm_ContextMenu) — Shift+F10 — context menu for file(s) under cursor opens at cursor position
- [ ] F-173 Exit (cm_Exit) — Alt+F4 — app quits (asks to confirm if operations running); on macOS also Cmd+Q; minimize-on-close option see F-771
- [~] F-174 Left drive list — Alt+F1 — drive popup for left panel
- [~] F-175 Right drive list — Alt+F2 — drive popup for right panel
- [ ] F-176 Alternative viewer (cm_ListExternal) — Alt+F3 — file opens with the other viewer (external if internal is default and vice versa)
- [ ] F-177 Lister with the built-in viewer only, no image/media viewers (cm_ListInternalOnly) — Alt+Shift+F3 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-178 Create shortcut / symlink (cm_CreateShortcut) — Ctrl+Shift+F5 — creates a symbolic link (macOS/Linux) or .lnk/symlink (Windows) in the target folder
- [~] F-179 Hard link / symbolic link dialog (cm_CreateHardLink / cm_CreateSymbolicLink) — — — dialog lets user choose hard link or symlink and the name; works in temp dir test
- [ ] F-180 Move to other panel without confirmation (cm_RenMov with confirmation off, ours) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-181 Show other window: compare files by content (cm_CompareFilesByContent) — — — see Compare
- [ ] F-182 Edit comment (cm_EditComment) — Ctrl+Z — edit descript.ion comment of file under cursor
- [~] F-183 Calculate folder sizes (cm_CountDirContent) — Alt+Shift+Enter — sizes of all folders in panel calculated and shown in size column
- [~] F-184 Show properties (cm_Properties) — Alt+Enter — properties window of file (system dialog on Windows; own dialog on macOS/Linux with size, dates, permissions, owner)
- [ ] F-185 Copy command line name (cm_AddFilenameToCmdline) — Ctrl+Enter — file name under cursor inserted into command line
- [ ] F-186 Copy full path to command line (cm_AddPathAndNameToCmdline) — Ctrl+Shift+Enter — full path inserted into command line
- [ ] F-187 Copy path to command line (cm_AddPathToCmdline) — Ctrl+P — current folder path inserted
- [ ] F-188 Previous / next command line (cm_CmdLineHistoryPrev / Next, ours) — Ctrl+E / Ctrl+X (in command line) — previous / next command from history put into command line
- [ ] F-189 Quick search window toggle — Ctrl+Alt+letter — see quick search
- [ ] F-190 Directory hotlist (cm_DirectoryHotlist) — Ctrl+D — hotlist popup opens at cursor
- [ ] F-191 FTP connect (cm_FtpConnect) — Ctrl+F — FTP connections dialog
- [ ] F-192 FTP disconnect (cm_FtpDisconnect) — Ctrl+Shift+F — current FTP connection closed
- [ ] F-193 New FTP connection (cm_FtpNew) — Ctrl+N — dialog to enter quick connection URL
- [~] F-194 Change attributes (cm_SetAttrib) — — (Files menu) — attributes dialog opens for the selection
- [ ] F-195 Print file list — — — see Files menu
- [~] F-196 Open command prompt in current folder (cm_ExecuteDOS) — — — opens system terminal (Terminal.app / default Linux terminal / cmd or Windows Terminal) in current folder
- [ ] F-197 Show file list in other app: Ctrl+M multi-rename (cm_MultiRenameFiles) — Ctrl+M — Multi-Rename tool for selected files
- [~] F-198 Synchronize dirs (cm_SyncDirs) — — — Sync dirs tool
- [~] F-199 Open URL of current file / run (Shift+Enter) — Shift+Enter — runs file in a terminal and keeps window open (Windows: cmd /k); on macOS opens in Terminal; on archives: opens as folder vs runs per TC semantics
- [~] F-200 Open file with custom command / "Open with" — — — context menu Open with lists apps

## 8. Default hotkeys reference (complete list to wire into command registry)

- [ ] F-201 F1 help — F1 — cm_HelpIndex wired
- [ ] F-202 F2 reread — F2 — cm_RereadSource wired (TC 11: also configurable to rename)
- [ ] F-203 F3 view — F3 — cm_List wired
- [ ] F-204 F4 edit — F4 — cm_Edit wired
- [x] F-205 F5 copy — F5 — cm_Copy wired
- [ ] F-206 F6 move — F6 — cm_RenMov wired
- [ ] F-207 F7 mkdir — F7 — cm_MkDir wired
- [ ] F-208 F8 delete — F8 — cm_Delete wired
- [ ] F-209 F9 menu — F9 — cm_ActivateMenu wired
- [ ] F-210 F10 menu — F10 — menu focus wired
- [ ] F-211 Shift+F1 custom column view menu — Shift+F1 — cm_SrcCustomViewMenu wired
- [~] F-212 Shift+F2 compare dirs — Shift+F2 — cm_CompareDirs wired
- [ ] F-213 Shift+F3 view current only — Shift+F3 — wired
- [ ] F-214 Shift+F4 new text file — Shift+F4 — cm_EditNewFile wired
- [ ] F-215 Shift+F5 copy same panel — Shift+F5 — cm_CopySamepanel wired
- [ ] F-216 Shift+F6 rename — Shift+F6 — cm_RenameOnly wired
- [ ] F-217 Shift+F8 delete permanently — Shift+F8 — wired
- [ ] F-218 Shift+F10 context menu — Shift+F10 — cm_ContextMenu wired
- [ ] F-219 Alt+F1 / Alt+F2 drive lists — Alt+F1 / Alt+F2 — cm_LeftOpenDrives / cm_RightOpenDrives wired
- [ ] F-220 Alt+F3 alt viewer — Alt+F3 — wired
- [ ] F-221 Alt+F4 exit — Alt+F4 — cm_Exit wired
- [ ] F-222 Alt+F5 pack — Alt+F5 — cm_PackFiles wired
- [ ] F-223 Alt+Shift+F5 move to archive — Alt+Shift+F5 — cm_PackFiles with "move to archive" checked
- [ ] F-224 Alt+F6 unpack (TC also Alt+F9) — Alt+F6 — cm_UnpackFiles wired
- [ ] F-225 Alt+F7 find — Alt+F7 — cm_SearchFor wired
- [ ] F-226 Alt+F8 cmd history — Alt+F8 — wired
- [ ] F-227 Alt+F9 unpack — Alt+F9 — wired
- [ ] F-228 Alt+Shift+F9 test archive — Alt+Shift+F9 — cm_TestArchive wired
- [ ] F-229 Alt+F10 folder tree — Alt+F10 — cm_CDtree wired
- [ ] F-230 Alt+F11 / Alt+F12 left / right current directory bar (breadcrumb) — Alt+F11 / Alt+F12 — opens the left/right path bar for keyboard use (see F-772)
- [ ] F-231 Ctrl+F1..F12 view/sort/filter — Ctrl+F1..F12 — Brief, Full, sort name/ext/date/size/unsorted, Tree, Print (F9), all files, programs, custom filter wired
- [ ] F-232 Ctrl+F9 print file under cursor — Ctrl+F9 — cm_PrintFile wired (system print of associated app, where supported)
- [ ] F-233 Ctrl+Shift+F1 thumbnails — Ctrl+Shift+F1 — wired
- [ ] F-234 Ctrl+Shift+F2 comments view — Ctrl+Shift+F2 — wired
- [ ] F-235 Ctrl+A select all — Ctrl+A — wired
- [ ] F-236 Ctrl+B branch view — Ctrl+B — wired
- [~] F-237 Ctrl+C / Ctrl+X / Ctrl+V clipboard copy, cut, paste files — Ctrl+C / Ctrl+X / Ctrl+V — cm_CopyToClipboard / cm_CutToClipboard / cm_PasteFromClipboard wired; works with OS file clipboard (Finder/Explorer/Nautilus) where possible
- [ ] F-238 Ctrl+D hotlist — Ctrl+D — wired
- [ ] F-239 Ctrl+E previous command line — Ctrl+E — wired
- [ ] F-240 Ctrl+F FTP connect — Ctrl+F — wired
- [ ] F-241 Ctrl+Shift+F FTP disconnect — Ctrl+Shift+F — wired
- [x] F-242 Toggle hidden files (cm_SwitchHidSys, ours default) — Ctrl+H (Cmd+Shift+. on macOS) — wired; also in Show menu, whose label shows the key ("Hidden files (Ctrl+H)")
- [ ] F-243 Ctrl+I switch to target panel (cm_FocusTrg) — Ctrl+I — focus goes to other panel's file list
- [~] F-244 Ctrl+L calculate occupied space (cm_GetFileSpace) — Ctrl+L — dialog shows total size, file and folder count of selection
- [ ] F-245 Ctrl+M multi-rename — Ctrl+M — wired
- [ ] F-246 Ctrl+N new FTP connection — Ctrl+N — wired
- [ ] F-247 Ctrl+P path to cmdline — Ctrl+P — wired
- [ ] F-248 Ctrl+Q quick view — Ctrl+Q — wired
- [ ] F-249 Ctrl+R reread — Ctrl+R — wired
- [ ] F-250 Ctrl+S quick filter — Ctrl+S — wired
- [ ] F-251 Ctrl+T new tab — Ctrl+T — wired
- [ ] F-252 Ctrl+U swap panels — Ctrl+U — wired
- [ ] F-253 Ctrl+Shift+U swap with tabs — Ctrl+Shift+U — wired
- [ ] F-254 Ctrl+V paste — Ctrl+V — wired
- [ ] F-255 Ctrl+W close tab — Ctrl+W — wired
- [ ] F-256 Ctrl+X cut / next cmdline — Ctrl+X — in file list: cut files; in command line: next history entry
- [ ] F-257 Ctrl+Z edit comment — Ctrl+Z — wired
- [ ] F-258 Ctrl+Up / Ctrl+Shift+Up new tab — Ctrl+Up — wired
- [ ] F-259 Ctrl+Left / Ctrl+Right transfer folder — Ctrl+Left / Ctrl+Right — wired
- [ ] F-260 Ctrl+PgUp / Ctrl+PgDn — Ctrl+PgUp / Ctrl+PgDn — parent / enter folder wired
- [ ] F-261 Ctrl+\ root — Ctrl+\ — wired
- [ ] F-262 Ctrl+Enter / Ctrl+Shift+Enter add name/path to cmdline — Ctrl+Enter — wired
- [~] F-263 Alt+Enter properties — Alt+Enter — wired
- [~] F-264 Alt+Shift+Enter count folder sizes — Alt+Shift+Enter — wired
- [~] F-265 Shift+Enter run in terminal / open archive as file — Shift+Enter — wired
- [ ] F-266 Alt+Left / Alt+Right history — Alt+Left / Alt+Right — wired
- [ ] F-267 Alt+Down folder history — Alt+Down — wired
- [ ] F-268 Num + / Num - / Num * / Num / selection — Num keys — wired; on keyboards without numpad, Shift+= / - / 8 alternatives in macOS map (documented)
- [ ] F-269 Alt+Num + / Alt+Num - same-extension select — Alt+Num +/- — wired
- [ ] F-270 Ctrl+Num + / Ctrl+Num - select/unselect all — Ctrl+Num +/- — wired
- [ ] F-271 Ins / Space mark — Ins / Space — wired
- [ ] F-272 Tab switch panel — Tab — wired; Shift+Tab moves between the file list and the separate tree (see F-769), not between panels
- [ ] F-273 Esc clear command line / close quick search / cancel — Esc — Esc in panel clears the command line text; in dialogs cancels
- [~] F-274 Ctrl+Alt+letter / Alt+letter quick search — — — wired per quick-search setting
- [~] F-275 Letter keys go to the command line — letter —  when quick search mode is Ctrl+Alt, plain letters go to the command line
- [ ] F-276 Shift+Esc minimize (TC) — Shift+Esc — app window minimizes
- [ ] F-277 Ctrl+Shift+F5 create shortcut/link — Ctrl+Shift+F5 — wired
- [ ] F-278 Alt+Shift+F5 move to archive — — — wired
- [ ] F-279 Ctrl+Shift+F8 cycle separate tree — Ctrl+Shift+F8 — cm_SeparateTreeOff -> 1 -> 2 cycles
- [ ] F-280 Print current file list (cm_PrintDir) — — — wired from Files menu
- [ ] F-281 Ctrl+Tab / Ctrl+Shift+Tab tabs — — — wired
- [ ] F-282 User-redefinable hotkeys (Configuration > Misc > Redefine hotkeys) — — — user can assign any key combo to any cm_* or user command; conflict warning; saved to ini; works in both panels
- [ ] F-283 Restore default hotkeys — — — button resets all hotkeys to TC defaults
- [ ] F-798 Alt+Shift+F3 built-in viewer only — Alt+Shift+F3 — cm_ListInternalOnly wired
- [ ] F-799 Alt+Shift+F11 focus button bar — Alt+Shift+F11 — cm_FocusButtonBar wired
- [ ] F-800 Ctrl+Shift+T new tab in background — Ctrl+Shift+T — cm_OpenNewTabBg wired
- [~] F-801 Ctrl+Shift+S reuse last quick filter — Ctrl+Shift+S — wired
- [ ] F-802 Ctrl+Shift+M FTP transfer mode — Ctrl+Shift+M — wired (see Net)
- [ ] F-803 Ctrl+Shift+B branch view of selection — Ctrl+Shift+B — cm_DirBranchSel wired
- [ ] F-804 Ctrl+Down command line history — Ctrl+Down — opens the command line history list (same as Alt+F8)
- [ ] F-805 Shift+Num + / Shift+Num - / Shift+Num * selection variants — Shift+Num keys — cm_SelectBoth / cm_UnselectFiles / cm_ExchangeSelBoth wired
- [ ] F-806 Ctrl+Shift+Num + / Ctrl+Shift+Num - — Ctrl+Shift+Num +/- — cm_SelectAllBoth / cm_ClearFiles wired
- [ ] F-807 Shift+Tab file list / separate tree focus — Shift+Tab — wired
- [ ] F-808 Ctrl+< root on European keyboards — Ctrl+< — cm_GoToRoot wired (IntlBackslash key code)
- [ ] F-809 Delete / Shift+Delete — Delete / Shift+Delete — cm_Delete wired, same as F8 / Shift+F8
- [~] F-810 F8 and Shift+F8 swap when 'delete to trash' is off — F8 / Shift+F8 — with trash turned off in Configuration, F8 deletes permanently and Shift+F8 moves to trash (TC rule)
- [ ] F-811 Apps / Menu key opens file context menu — Menu key — same as Shift+F10 where the keyboard has the key

## 9. macOS key mapping rule (document and implement)

- [ ] F-284 Platform hotkey rule: TC hotkeys use the physical Ctrl/Alt/Shift on all platforms; on macOS, Alt = Option, Ctrl = Control — — — every TC default hotkey works with Control/Option on macOS, identical to Windows/Linux; docs/keyboard.md documents the rule
- [~] F-285 macOS extra Cmd aliases: Cmd+C/X/V copy/cut/paste files, Cmd+A select all, Cmd+T new tab, Cmd+W close tab, Cmd+Q quit, Cmd+, configuration, Cmd+Backspace delete to trash, Cmd+Shift+. toggle hidden, Cmd+R reread, Cmd+Up parent, Cmd+Down open, Cmd+I properties, Cmd+L edit path, Cmd+F find files, Cmd+Shift+G go to folder, Cmd+Z undo in text fields only — macOS only — each alias runs the same cm_* command; aliases do not exist on Windows/Linux; Ctrl-based TC keys still work on macOS
- [~] F-286 Option-key letters handled by physical key code (KeyboardEvent.code) — — — Option+letter quick search and Alt hotkeys work on macOS despite Option producing special characters
- [ ] F-287 Function keys on Mac laptops (fn) — — — F-keys work with fn or with "use F1, F2 as standard keys"; function key buttons always available as mouse alternative; doc mentions this
- [ ] F-288 Keyboards without numpad and without Ins key — — — alternatives documented and wired: Ins = Space; Num+ = Shift+= ; Num- = Shift+- ; Num* = Shift+8 (only while command line is empty); remappable
- [ ] F-289 System shortcut collisions on macOS (Ctrl+Up Mission Control, Ctrl+Left/Right Spaces, Ctrl+F2 menu focus) — — — doc lists them and offers Cmd-based alternatives (e.g. Cmd+Option+Left/Right for transfer), since the OS swallows those keys
- [ ] F-290 Windows/Linux AltGr — — — AltGr combinations (e.g. typing \ on Hungarian layout) are not treated as Ctrl+Alt quick search
- [ ] F-291 Non-US layouts (Hungarian etc.) — — — hotkeys like Ctrl+\ and Num keys work on Hungarian layouts via key code fallback

## 10. Command line

- [ ] F-292 Command line below panels showing current path prompt — — — prompt shows active panel path; Enter runs command with that folder as working directory
- [~] F-293 Run program / shell command — Enter — command runs; console programs run in a terminal window (or captured output, option)
- [~] F-294 Run and keep terminal open — Shift+Enter — terminal stays open after command
- [ ] F-295 Command line history dropdown — Alt+F8 / Ctrl+Down / Ctrl+E / Ctrl+X — previous commands shown/cycled; history saved across restarts (cmdlinehistory)
- [ ] F-296 Auto-complete paths in command line — — — typing a path suggests matching folders/files
- [~] F-297 cd command handling — — — "cd path" changes active panel folder instead of running a shell
- [ ] F-298 Insert file name / full path — Ctrl+Enter / Ctrl+Shift+Enter — inserted with quotes if it has spaces
- [ ] F-299 Insert path — Ctrl+P — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-300 Typing starts command line input (when quick search not on plain letters) — — — first printable key focuses command line
- [~] F-301 Internal commands typed in command line (cm_* names) — — — typing "cm_Copy" and Enter runs that internal command
- [~] F-302 Environment variables in command line — — — %HOME% / $HOME expanded
- [N/A] F-303 Run as administrator/root from command line — macOS/Linux have no safe GUI elevation for arbitrary commands — Windows: run elevated via UAC prompt; macOS/Linux: clear message, user runs sudo in terminal
- [ ] F-812 Enter runs the command line when it has text — Enter — with text in the command line, Enter runs it instead of opening the file under cursor
- [ ] F-813 Ctrl+Enter adds names one after another — Ctrl+Enter — pressing Ctrl+Enter on two files gives both names separated by a space, quoted if needed

## 11. Button bar (toolbar)

- [~] F-304 Main button bar with default buttons (reread, brief, full, thumbnails, tree, invert selection, back, forward, pack, unpack, find, view, multi-rename, sync, FTP, hotlist, edit, compare) — — — default bar shows icon buttons with data-tip tooltips; clicking runs the command
- [~] F-305 Customize button bar (cm_ButtonConfig) — — — dialog to add/remove/reorder buttons; each button: command (cm_* or program), parameters, start path, icon, tooltip, run minimized
- [~] F-306 Drag & drop a program/file onto bar adds a button — — — dropping a file adds a button that launches it
- [~] F-307 Drag & drop files onto a button — — — dropped files are passed as parameters to that button's program
- [~] F-308 Sub-bars (buttons opening another bar as dropdown) — — — button can open a nested bar as a menu
- [~] F-309 Multiple bar files (.bar) and switching — — — user can save and load different bars
- [~] F-310 Button bar parameters placeholders %P %N %S %T %L %F etc. — — — placeholders expand: %P source path, %N name under cursor, %T target path, %S selected names, %L list file of selected names, %F list of names only, %O name without ext, %E ext, %M name in target, %Z list file UTF-8 etc.; tested via command
- [~] F-311 Button "user command" (usercmd.ini em_ commands) — — — user-defined em_ commands appear in command list and can be used by buttons/hotkeys/menus
- [~] F-312 Button bar large/small icons, flat look, wrap to multiple lines — — — options apply
- [~] F-313 Right-click on button: change, delete, copy, paste — — — context menu works
- [ ] F-314 Vertical button bar config — — — separate bar config for the middle bar
- [~] F-814 Parameter prompt in buttons and start menu items — — — a '?' at the start of the parameters shows an edit box with the expanded parameters before running; Cancel runs nothing

## 12. Files menu

- [~] F-315 Change attributes (cm_SetAttrib) — — — dialog: read-only, hidden, system, archive (Windows) / permissions rwx owner/group/other + hidden flag (macOS chflags) on Unix; date/time change; recurse subfolders; apply to selection
- [~] F-316 Change date/time (in attributes dialog) — — — modification date (and creation date where supported) of selected files set to entered value; "Current" button
- [N/A] F-317 Attributes dialog plugin (WDX) fields — WDX plugins are Windows DLLs — built-in fields (EXIF date, image size, audio tags) usable in the date template
- [ ] F-318 Pack (cm_PackFiles) — Alt+F5 — see Packers
- [ ] F-319 Unpack specific files / all (cm_UnpackFiles) — Alt+F9 — see Packers
- [ ] F-320 Test archive (cm_TestArchive) — Alt+Shift+F9 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-321 Compare by content (cm_CompareFilesByContent) — — — opens compare tool for 2 selected files (or file in each panel with same name)
- [N/A] F-322 Change system file associations (cm_Associate) — changing OS associations is OS-specific and risky on macOS/Linux — internal associations (next line) plus 'Open with' and a button to open the OS association settings
- [ ] F-323 Internal associations (cm_InternalAssociate, TC 9+) — — — user maps extensions to commands inside the app; Enter uses them before system default
- [~] F-324 Properties (cm_Properties) — Alt+Enter — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-325 Calculate occupied space (cm_GetFileSpace) — Ctrl+L — dialog with bytes, allocated size, files, folders; cancellable
- [ ] F-326 Multi-Rename tool (cm_MultiRenameFiles) — Ctrl+M — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-327 Edit comment (cm_EditComment) — Ctrl+Z — comment stored in descript.ion in the same folder; shown in comments view
- [~] F-328 Split file (cm_FileSplit) — — — dialog: target folder, part size presets (1.44MB, 650MB, 700MB, 4.7GB, custom, "auto"), creates name.001, name.002, ... and a .crc file with name, size and CRC32
- [~] F-329 Combine files (cm_FileCombine) — — — on name.001 recombines parts in order; checks size and CRC from .crc file; reports error on mismatch
- [~] F-330 Encode file (MIME/Base64, UUE, XXE) (cm_UUEncode) — — — creates encoded file that decodes back byte-identical
- [~] F-331 Decode file (MIME, UUE, XXE, BinHex) (cm_UUDecode) — — — decodes encoded file to original; round-trip test passes
- [~] F-332 Create checksum files (cm_CRCCreate) — — — dialog: CRC32 (.sfv), MD5, SHA1, SHA256, SHA512, BLAKE3 (ours); one file for all or one per file; output matches md5sum/shasum format
- [~] F-333 Verify checksums (cm_CRCCheck) — — — on .md5/.sha/.sfv file checks all listed files and shows OK/FAILED/missing summary
- [ ] F-334 Print file list (cm_PrintDir / cm_PrintDirSub) — — — prints current file list (optionally with subfolders) via system print dialog or a printable page
- [ ] F-335 Print file (via associated app) — — — file under cursor sent to system print where supported
- [ ] F-336 Quit (cm_Exit) — Alt+F4 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-337 Create symbolic link / hard link (cm_CreateSymbolicLink / cm_CreateHardLink) — — — dialog creates link; tested in temp dir
- [~] F-338 Change owner/group (Unix) — — — N/A on Windows; on macOS/Linux attributes dialog allows chown where permitted
- [ ] F-339 Wipe / secure delete (cm_Wipe) — — — file overwritten before deletion (best effort; note SSD limitation); confirmation required
- [ ] F-340 Set file date from EXIF / name (TC via plugins) — — — provided by attributes dialog template field
- [ ] F-341 Open file with ... / Open in other app — — — context menu offers system apps
- [N/A] F-815 Edit ACL / owner / audit dialogs (cm_EditPermissionInfo, cm_EditOwnerInfo, cm_EditAuditInfo) — Windows security dialogs — Windows: open the native security tab; macOS/Linux: permissions and owner in the attributes dialog (F-315, F-338)

## 13. Mark menu

- [ ] F-342 Select group — Num + — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-343 Unselect group — Num - — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-344 Select all — Ctrl+Num + — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-345 Unselect all — Ctrl+Num - — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-346 Invert selection — Num * — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-347 Select all with same extension — Alt+Num + — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-348 Restore selection — Num / — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-349 Save / load selection, from file / clipboard — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-350 Copy selected names / full names / details to clipboard — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-351 Compare directories — Shift+F2 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-352 Mark newer / Hide equal files after compare (cm_CompareDirs + "hide equal") — — — option to show only differing files after compare
- [ ] F-353 Select by search results ("Feed to listbox" back) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button

## 14. Commands menu

- [ ] F-354 Directory hotlist — Ctrl+D — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-355 Directory tree / CD tree — Alt+F10 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-356 Find files — Alt+F7 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-357 Find in separate process — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [N/A] F-358 Locate with Everything (TC 'Everything' search) — Everything is Windows-only — Spotlight (mdfind) on macOS, plocate/locate on Linux when installed, normal search otherwise
- [ ] F-359 Volume label (cm_VolumeId) — — — rename volume where OS allows (Windows label, macOS diskutil rename, Linux disabled unless supported); otherwise menu entry disabled with a note
- [~] F-360 Synchronize dirs — — — see Sync
- [ ] F-361 Source = target (cm_MatchSrc) — — — other panel shows same folder
- [ ] F-362 Swap panels — Ctrl+U — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-363 Open terminal / command prompt here — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-364 Open desktop folder (cm_OpenDesktop) — — — panel goes to Desktop folder
- [N/A] F-365 Control Panel / Printers / Network neighborhood virtual folders (cm_OpenControls, cm_OpenPrinters, cm_OpenNetwork) — Windows shell namespaces — open OS settings; mounted network volumes in drive list
- [~] F-366 Run Explorer/Finder in current folder (cm_Explorer) — — — opens Finder / Explorer / file manager in current folder, revealing file under cursor
- [ ] F-367 Show file in other panel (Ctrl+Left/Right) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [N/A] F-368 Shell virtual folders: fonts, startup, recycle bin namespace — Windows-only shell folders — real folders (~/Library/Fonts, ~/.local/share/fonts) and Trash view (cm_OpenRecycled maps to Trash)
- [ ] F-369 Refresh directory tree (cm_RescanTree, ours) — — — tree rescans and shows new folders
- [ ] F-370 Branch view — Ctrl+B — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-371 Run DOS / terminal — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-372 Command line focus (cm_FocusCmdLine) — — — focus to command line
- [ ] F-373 Browse internal commands (cm_CommandBrowser) — — — dialog lists all cm_* commands with description and hotkey, searchable; can run or assign hotkey
- [ ] F-374 Internal command list completeness — — — every cm_* in this file appears in the command browser
- [ ] F-375 Change start menu (cm_ConfigStartMenu) — — — user menu "Start" editable with items: title, command, parameters, start path, hotkey, submenus
- [ ] F-376 Open folder in new window / second instance — — — allow multiple instances option (TC: only one instance setting)

## 15. Net menu

- [~] F-377 FTP connect — Ctrl+F — connection manager: list of saved servers, new, duplicate, edit, delete, folders, connect
- [ ] F-378 New FTP connection (quick) — Ctrl+N — enter host or URL (ftp://user@host:port/path); connects
- [ ] F-379 FTP disconnect — Ctrl+Shift+F — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-380 Show hidden FTP files / custom LIST command — — — option sends LIST -a; hidden files shown
- [ ] F-381 FTP download list (cm_FtpDownloadList) — — — queue of files to download later; transfer manager
- [ ] F-382 Background transfer manager (cm_BackgroundTransferManager) — — — transfers run in background with progress; can be queued, paused, removed
- [N/A] F-383 Connect/disconnect network drive dialog (cm_NetConnect / cm_NetDisconnect) — Windows-specific dialog — macOS: open smb://server via OS Connect to Server; Linux: gio mount; Windows: native WNetConnectionDialog
- [N/A] F-384 Share / unshare folder (cm_NetShareDir / cm_NetUnshareDir) — sharing is an OS setting — button opens OS sharing settings
- [N/A] F-385 Port connection / direct cable (cm_DirectCableConnect) — obsolete serial/parallel/USB cable link — SFTP/FTP between machines
- [N/A] F-386 Network neighborhood browsing — Windows-only namespace — mounted network volumes appear in drive list; UNC paths on Windows
- [~] F-387 FTP transfer mode binary/ASCII/auto — — — transfer mode selectable; ASCII converts line endings
- [~] F-388 Resume aborted download — — — partially downloaded file resumes from offset (REST)
- [ ] F-389 FTP hidden filter / server-specific settings (passive, proxy, firewall, SSL/TLS, codepage UTF-8) — — — per-connection options saved and used
- [~] F-390 Send FTP command (cm_FtpSendCommand) — — — raw FTP command can be sent; reply shown in log
- [~] F-391 FTP log window — — — log of FTP commands and responses can be shown
- [ ] F-392 Restore lost connection — — — after disconnect, operation reconnects automatically (retries configurable)
- [~] F-816 Change FTP transfer mode — Ctrl+Shift+M — cycles binary / ASCII / auto; the current mode is shown in the FTP status bar
- [~] F-817 FTP status bar above panel while connected (mode, log button, Abort) — — — while connected a small bar shows; Abort stops the running transfer cleanly
- [ ] F-818 Server listing formats (Unix ls, Windows/IIS, MLSD) — — — listings from each format parse into correct names, sizes, dates (unit tests with fixtures and a local test server)

## 16. Show menu

- [ ] F-393 Brief — Ctrl+F1 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-394 Full — Ctrl+F2 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-395 Comments — Ctrl+Shift+F2 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-396 Thumbnails — Ctrl+Shift+F1 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-397 Custom columns submenu — — — lists column sets for choosing
- [ ] F-398 Tree — Ctrl+F8 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-399 Quick view — Ctrl+Q — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-400 Separate tree (off / 1 / 2) — Ctrl+Shift+F8 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-401 All files — Ctrl+F10 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-402 Programs — Ctrl+F11 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-403 *.* custom filter — Ctrl+F12 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-404 Define custom filter (with saved filters / search templates) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-405 Sort by Name/Extension/Time/Size/Unsorted — Ctrl+F3..F7 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-406 Reverse order — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-407 Reread source — Ctrl+R — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-408 Show only selected files / hide selected (cm_ShowOnlySelected / cm_HideSelected, ours names) — — — panel shows only selected (or only unselected) files until reread
- [ ] F-409 Show hidden/system toggle — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-410 Horizontal arrangement — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-411 Show 2 panels / single panel — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-819 View modes submenu — — — lists the view modes; choosing one switches the panel

## 17. Configuration menu and dialog pages

- [~] F-412 Options dialog (cm_Config) — Cmd+, on macOS — tree of pages; changes apply on OK/Apply; Cancel discards
- [~] F-413 Page: Layout (bars, which elements visible, font for main window, panel widths) — — — all toggles apply live
- [~] F-414 Page: Display (show hidden, sorting, square brackets around folders, show extension separately, file size format, date format, icons) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-415 Page: Icons (show icons, overlays, size 16/24/32/48, folder icons, icons in lists) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-416 Page: Font (panel font, dialog font, Lister font) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-417 Page: Colors (text, background, mark, cursor, alternate background, inactive panel, define by file type) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-418 Page: Language — — — UI language can be chosen; English default; strings in a separate translation file (Hungarian optional)
- [ ] F-419 Page: Operation (instance handling, mouse selection mode, quick search options, Enter on archive, select folders with Num+) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-420 Page: Viewer/Editor (external viewer, editor path and parameters, Lister options) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-421 Page: Copy/Delete (confirmations, use trash, buffer size, verify after copy, copy file times/attributes/owner, overwrite rules, background queue default, big file copy mode) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-422 Page: Refresh (auto refresh, refresh only in background, exclude folders) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-423 Page: Quick search — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-424 Page: FTP (passive default, show hidden, timeouts, retries, default download folder, anonymous email) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [N/A] F-425 Page: Plugins (WCX/WFX/WLX/WDX lists) — TC plugins are Windows DLLs — built-in packers/viewers/fields plus external-tool configuration (command-line helpers declared in config)
- [ ] F-426 Page: Thumbnails (size, cache, show name, file types) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-427 Page: Log file (enable log, file path, log which operations) — — — operations are written to a text log when enabled
- [ ] F-428 Page: Ignore list (hide listed files/folders/patterns) — — — files matching the ignore list disappear from panels when enabled
- [ ] F-429 Page: Packer (zip settings: compression level, internal zipper, tar/gz options, default packer) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-430 Page: Zip packer settings (compression level 0-9, use UTF-8 names, set file times) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-431 Page: Misc (redefine hotkeys, window title, single instance, confirm exit) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-432 Page: Folder tabs — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-433 Page: Custom columns — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-434 Page: Favorite tabs (TC: favorite tab sets) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-435 Page: File list colors per type editing — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-436 Page: Background copy / transfer queue options — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-437 Page: Ftp/proxy — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-438 Page: Directory hotlist settings — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-439 Save position (cm_ConfigSavePos) — — — window position, size and splitter saved on demand
- [~] F-440 Save settings (cm_ConfigSaveSettings) — — — settings written immediately to ini
- [ ] F-441 Change button bar — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-442 Change start menu — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-443 Configuration file in wincmd.ini-style format (INI sections [Configuration], [Layout], [Colors], [left], [right], [lefttabs], [righttabs], [DirMenu], [Shortcuts], [SearchName]...) — — — settings stored as human-readable INI in the platform config dir (configdir.go convention); hand edits are picked up on restart
- [~] F-444 Portable mode (ini next to binary) — — — if a writable ini file exists next to the binary it is used instead of the user config dir
- [~] F-445 Config path override via command line /i= /f= — — — starting with -i=<path> uses given ini; -f= for ftp ini
- [~] F-446 Import Total Commander wincmd.ini (hotlist, buttons, colors, custom columns) — — — importing a real wincmd.ini copy (in a test temp dir) brings hotlist and hotkeys over
- [~] F-447 Export / back up settings — — — settings can be saved to a chosen file and restored
- [ ] F-820 Page: View modes — — — create, edit and order view modes and their auto-switch rules
- [ ] F-821 Page: Tooltips — — — define which fields show in tooltips per file type; option to turn tooltips off
- [ ] F-822 Page: Lister (default mode, fonts, tab width, default charset, image fit, auto-detect, which built-in viewers to use) — — — each option changes the Lister
- [ ] F-823 Page: Drag & drop and mouse (selection mode, confirm drop, right-drag menu, spring-loaded folders, middle click) — — — each option changes behaviour
- [ ] F-824 Page: Tree (separate tree options, tree follows folder, show hidden folders in tree) — — — each option changes the tree
- [ ] F-825 Page: Find files defaults (search depth, archives, Spotlight/locate use, encoding) — — — defaults appear in a new Find files dialog
- [~] F-826 Edit settings files directly (cm_ConfigChangeIniFiles) — — — opens the ini file in the editor; after saving the app reloads it (or says a restart is needed)
- [~] F-827 Search box in Options dialog (ours) — — — typing filters the page tree to pages that contain matching options
- [ ] F-828 Custom main menu (TC .mnu menu files) — — — user can load a custom main menu file; 'default' brings the built-in menu back

## 18. Start menu (user menu)

- [~] F-448 Start menu with user-defined entries — — — items run commands, programs, cm_* commands; submenus with "-" prefix; separators
- [~] F-449 Start menu hotkeys — — — each entry can have its own hotkey
- [~] F-450 Change start menu dialog — — — add, delete, edit, reorder, submenu, icons

## 19. Help menu and About

- [~] F-451 Help index / keyboard help / registration info — F1 — built-in HTML help pages: index, keyboard, menus
- [~] F-452 Keyboard help page — — — lists all active hotkeys (generated from the registry, so it is always current)
- [~] F-453 About box — — — shows "mormi" + "Commander" wordmark, version from wails.json, Mormitech copyright, license, link to website; style like other mormi apps
- [ ] F-454 Check for updates (optional, manual) — — — menu item opens release page; no automatic network calls without user action
- [ ] F-455 Visit website / support — — — opens link in browser
- [N/A] F-456 Registration nag / key — mormiCommander is free — no nag screen; About shows license

## 20. Copy / Move dialogs and engine

- [ ] F-457 Copy dialog shows source count/name, target path editable, file types filter "Only files of this type" — F5 — target defaults to other panel; filter like *.txt copies only matching files (recursively)
- [ ] F-458 Target path with rename (copy "a.txt" to "x/b.txt") — — — target ending in a file name renames the copy
- [ ] F-459 Target with wildcards rename (*.bak) — — — "*.bak" in target renames copies with new extension
- [ ] F-460 Copy folders recursively — — — full folder tree copied, empty folders included
- [ ] F-461 Move within same volume = rename (instant) — F6 — no data copy when same volume
- [ ] F-462 Move across volumes = copy + delete after verified copy — F6 — source deleted only after successful copy of each file
- [ ] F-463 Overwrite confirmation dialog — — — shows both files with size and date (and preview option); buttons: Overwrite, Overwrite all, Skip, Skip all, Overwrite all older, Rename, Auto-rename all, Copy all larger, Copy all smaller, Compare by content, Cancel
- [~] F-464 Auto-rename copied file (name(2).ext) — — — option creates "file(2).txt" instead of overwrite
- [ ] F-465 Rename target in overwrite dialog — — — lets user type a new name for that file
- [ ] F-466 Compare from overwrite dialog — — — opens compare by content for the two files
- [ ] F-467 Overwrite read-only/hidden/system files confirmation — — — separate prompt for read-only targets
- [ ] F-468 Queue (F2 in copy dialog / "Queue" button) — F2 in dialog — operation added to a queue; queued operations run one after another
- [ ] F-469 Background (separate thread) operation — — — "Background" button runs the copy while the user keeps working; progress window non-modal; TC: copy/move in background
- [ ] F-470 Background transfer manager window listing all running/queued operations with pause/resume/cancel/speed limit — — — each operation can be paused, resumed, cancelled; speed limit applied
- [ ] F-471 Progress dialog: current file, total, speed, time left, pause, cancel, "background" — — — numbers update; Cancel stops cleanly leaving no partial file (or removes it)
- [ ] F-472 Verify after copy (option in dialog) — — — after copy, target is re-read and compared (hash); mismatch reported
- [ ] F-473 Copy permissions / ownership — — — option keeps Unix mode bits (and owner when permitted); Windows ACL copy best effort or documented as not supported
- [ ] F-474 Keep file dates (mod time, creation where possible, access time) — — — copied files have same modification time as source
- [ ] F-475 Copy extended attributes / ADS / resource forks — — — macOS xattrs and resource forks preserved (option); Windows ADS where possible
- [ ] F-476 Copy symlinks as links or follow them (option) — — — option: copy link itself vs target contents
- [ ] F-477 Handle file-in-use / permission errors with Retry/Skip/Skip all/Cancel — — — error dialog offers these choices
- [ ] F-478 Large file copy mode (big buffer, no cache) — — — option uses large buffer; works for files >4GB
- [~] F-479 Copy to same folder auto-name (Shift+F5) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-480 Copy with drag & drop (left: copy, Shift: move, Ctrl/Alt: menu per TC) — — — dragging between panels copies with confirmation dialog (option to skip)
- [ ] F-481 Drag & drop right button menu (copy/move/link/cancel) — — — right-drag drop shows menu
- [ ] F-482 Copy to archive (target is archive path) — — — copying into a panel showing an archive adds files to the archive
- [ ] F-483 Copy from archive to folder — — — copying out of an archive panel extracts files
- [~] F-484 Copy to/from FTP/SFTP panel — — — transfers via network plugin
- [~] F-485 Copy only newer / missing (via Sync) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-486 Copy file names with long paths (>260 chars on Windows) — — — long paths work on Windows (\\?\ prefix)
- [ ] F-487 Case-only rename (a.txt -> A.txt) on case-insensitive FS — — — rename succeeds on macOS/Windows
- [ ] F-488 Move folders across volumes — — — completed and source removed
- [N/A] F-489 Undo of file operations — TC has no undo either; deletes go to trash — trash restore and MRT undo
- [ ] F-490 Overwrite older files only option in dialog (TC 11 dropdown "Overwrite mode") — — — dropdown: ask, overwrite all, skip all, overwrite older, auto-rename source/target, copy larger/smaller
- [ ] F-491 Copy free space check before start — — — warn if target lacks free space
- [ ] F-492 Copy statistics at end (count, bytes, errors) — — — summary shown in log/status
- [ ] F-829 Copy/move dialog folder tree — F10 in dialog — 'Tree' button (F10) opens a folder tree to pick the target; choice goes into the target field
- [ ] F-830 Copy/move dialog target history and other-panel tabs — — — dropdown lists recent targets and the folders of the other panel's tabs
- [ ] F-831 Move: remove source folders that end up empty — — — after moving a folder tree, no empty source folders stay behind
- [~] F-832 F5/F6 on '..' or empty selection — F5 / F6 — with no selection the file under cursor is used; on '..' nothing happens and a short message shows

## 21. Delete

- [ ] F-493 Delete to trash / Recycle Bin — F8 / Delete — confirmation shows count; files land in OS trash (macOS Trash, XDG trash on Linux, Recycle Bin on Windows)
- [ ] F-494 Delete permanently — Shift+F8 / Shift+Delete — confirmation; files removed permanently
- [ ] F-495 Delete non-empty folder confirmation ("Delete folder with all subfolders?" All/Skip) — — — confirmation per folder with All option
- [ ] F-496 Delete read-only files confirmation — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-497 Delete in background with progress — — — progress and cancel
- [ ] F-498 Delete inside archive — — — file removed from archive
- [ ] F-499 Delete on FTP/SFTP — — — file removed on server
- [ ] F-500 Wipe (secure delete) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-501 Trash restore (via Trash folder view) — — — files in trash can be restored to original place (where OS supports)
- [ ] F-833 Volumes without trash — F8 — on volumes that have no trash (some USB sticks, network shares) the confirmation says files will be deleted permanently

## 22. Rename and Multi-Rename tool

- [~] F-502 Inline rename — Shift+F6 — edit box in list; pressing Shift+F6 again cycles selection: name only, extension only, full name
- [ ] F-503 Rename via move dialog — F6 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-504 Multi-Rename tool window — Ctrl+M — list of selected files with old name and new name preview
- [ ] F-505 MRT rename mask [N] name, [E] extension, [C] counter, [N1-5] substring, [N2,3] etc. — — — mask "[N]_[C]" gives file_1, file_2 ...; preview updates live
- [ ] F-506 MRT placeholders: [N] name, [E] ext, [C] counter, [P] parent folder, [G] grandparent, [Y][M][D][h][m][s] date/time parts, [d] date, [t] time, [U] uppercase from here, [L] lowercase from here, [F] first letter upper, [n] original name with ext — — — each placeholder replaced as specified; [=plugin.field] replaced by built-in fields (EXIF date, image size, audio tags)
- [ ] F-507 MRT counter: start, step, digits, reset per folder — — — settings applied
- [ ] F-508 MRT search & replace (plain, case-sensitive, regex, replace only first, "E" in extension) — — — replacements applied; regex with groups $1
- [ ] F-509 MRT multiple search/replace pairs separated by `|` — — — "a|b" replaces several strings at once
- [ ] F-510 MRT upper/lower case options — — — dropdown applies case
- [ ] F-511 MRT include subdirectories / file list — — — renames files in branch view as well
- [ ] F-512 MRT edit names in editor — F3 in MRT (ours) — names written to a temp file, opened in editor, loaded back after save
- [ ] F-513 MRT save/load settings presets — — — named presets saved
- [ ] F-514 MRT undo last rename (cm_ "Undo") — — — undo restores old names (from undo log)
- [ ] F-515 MRT result: conflicts and errors highlighted — — — duplicate target names prevented with warning
- [ ] F-516 MRT rename folders too — — — folders can be renamed
- [ ] F-517 MRT start/ OK / Close — — — renames executed, list refreshes
- [~] F-518 MRT reset fields — — — all mask fields return to defaults
- [ ] F-519 MRT sort list / drag to reorder — — — counter follows the list order
- [ ] F-834 MRT extra mask forms: [C10+5:3] inline counter, [E2-3] part of extension, [N-8,5] counted from the end, [[] and []] literal brackets — — — each form gives the expected name in unit tests
- [ ] F-835 MRT load new names from a text file or clipboard — — — new names taken line by line; line count mismatch shows a warning
- [ ] F-836 MRT remove accents option (ours) — — — 'árvíztűrő' becomes 'arvizturo' in preview and after rename

## 23. Make folder

- [ ] F-520 Make folder dialog — F7 — dialog with name; creates folder; cursor on new folder
- [ ] F-521 Make nested folders (a/b/c) — F7 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-522 Make folder with placeholders (date) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-523 Make folder in archive / FTP — — — folder created there

## 24. Find files

- [ ] F-524 Find files dialog (cm_SearchFor) — Alt+F7 — tabs: General, Advanced, Plugins, Load/Save, Results
- [ ] F-525 General: search for (wildcards, multiple patterns separated by ; and `|` for exclusions) — — — `*.txt|old*` finds txt not starting with old
- [ ] F-526 General: search in (multiple folders separated by ;, drive list, "selected folders") — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-527 General: search subdirectories depth (all, current only, 1..n levels) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-528 General: search archives (look inside zip etc.) — — — files inside archives listed with archive path
- [ ] F-529 General: find text (containing text), case-sensitive, whole words, regex, hex, not containing, Unicode/UTF-8/UTF-16, ANSI/OEM codepages — — — each option works against test files
- [ ] F-530 General: Regular expressions in file name — — — name pattern as regex
- [ ] F-531 Advanced: date between / not older than n minutes/hours/days/weeks/months/years — — — filters work
- [ ] F-532 Advanced: size equal/greater/less with units — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-533 Advanced: attributes / permissions filter — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-534 Advanced: find duplicate files (same name, same size, same content) — — — duplicates grouped in results
- [ ] F-535 Advanced: search for folders only / files only, follow symlinks — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [N/A] F-536 Find files Plugins tab (WDX fields) — WDX plugins are Windows DLLs — built-in fields (EXIF, image dimensions, audio tags, text length) in the same tab
- [~] F-537 Load/Save search templates — — — templates saved by name; usable in filters (custom filter, select group)
- [ ] F-538 Results list live while searching, stop button — — — results appear progressively; Stop works
- [~] F-539 Results: go to file (jump to file in panel) — — — "Go to file" opens folder and focuses file
- [~] F-540 Results: view (F3), edit (F4) from list — F3/F4 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-541 Results: feed to listbox (cm_ "Feed to listbox") — — — results shown in a panel as virtual folder; normal file operations (copy, delete, pack) work on it
- [~] F-542 New search / last search remembered — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-543 Search in separate process window — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-544 Search in found files (refine) — — — search again only within results
- [~] F-545 Search on FTP/SFTP — — — name search works on remote folders (content search not required)
- [ ] F-837 Find: exclude folders by pattern ('|vendor/' or '|.git\') — — — `*.go|vendor/` skips everything under vendor folders
- [~] F-838 Find: copy results to clipboard / save results to a file — — — result names (with paths) copied or written as text

## 25. Synchronize directories

- [ ] F-546 Synchronize dirs tool (cm_SyncDirs) — — — window with left and right path, filter, options
- [ ] F-547 Compare modes: asymmetric, by content, ignore date, ignore seconds/2s difference (FAT), ignore timezone 1h — — — options affect results
- [ ] F-548 Subdirectories / ignore empty folders — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-549 Show: left newer, equal, different, right newer, singles, duplicates (filter buttons) — — — filter buttons hide/show categories
- [ ] F-550 Compare result with arrows per file; user can change direction or exclude — — — right-click/arrows change action
- [ ] F-551 Synchronize button: copy selected actions with dialog (left->right, right->left, delete) — — — after sync, re-compare shows all equal
- [ ] F-552 Compare by content in sync (Ctrl+... ) — — — content compare for "=" vs "≠"
- [~] F-553 Open compare tool for a pair from sync list — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-554 Save/load sync settings — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-555 Sync with archives / FTP as one side — — — works where virtual FS supports it

## 26. Compare by content (file compare tool)

- [ ] F-556 Compare two files (text) — — — side-by-side view with differences highlighted, line by line
- [ ] F-557 Compare binary files — — — hex mode shows differing bytes
- [ ] F-558 Next/previous difference navigation — — — buttons/keys jump between differences
- [ ] F-559 Edit mode and save in compare tool — — — lines can be edited and copied left/right and saved
- [ ] F-560 Ignore case / whitespace / repeated lines options — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-561 Compare files from two panels with same name automatically — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-562 Compare folders recursively by content (Sync) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-563 External diff tool option — — — user can set external compare program
- [ ] F-839 Compare tool: line numbers, synced scrolling, encoding choice — — — both sides scroll together; encoding menu redisplays text

## 27. Lister (internal viewer)

- [ ] F-564 Lister window opens file — F3 — separate window; Esc closes; F3 or N/P for next/previous file
- [ ] F-565 Text mode with automatic encoding detection (ANSI, UTF-8, UTF-16, ASCII/OEM) — — — correct display of UTF-8 Hungarian text (áéíóöőúüű)
- [ ] F-566 Binary mode (fixed line width) — 2 — key 2 switches to binary mode
- [ ] F-567 Hex mode — 3 — key 3 switches to hex dump with offsets and ASCII column
- [ ] F-568 Text mode — 1 — key 1 text
- [ ] F-569 Multimedia/image mode — 4 — key 4 shows image/audio/video via webview
- [ ] F-570 Unicode / UTF-8 / encoding choice — — (Encoding menu) — menu chooses encoding; text redisplays correctly
- [~] F-571 HTML/Markdown rendered mode — — (View menu) — renders HTML/markdown inside viewer (scripts disabled)
- [ ] F-572 Wrap text — W — toggles line wrap
- [ ] F-573 Fit image to window / original size / zoom — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-574 Image rotate / flip (Lister images) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-575 Search in Lister — F7 / Ctrl+F — search box: case, whole words, hex, regex, backwards
- [ ] F-576 Find next / previous in Lister — F3 / Shift+F3 — next/previous match highlighted and scrolled into view
- [ ] F-577 Select text and copy — Ctrl+C — selected text to clipboard; Ctrl+A select all
- [ ] F-578 Go to line/position — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-579 Large files (multi-GB) opened instantly without loading fully — — — 10 GB file opens instantly and scrolls (backend streams ranges)
- [ ] F-580 Lister font selection (fixed vs variable) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-581 Print from Lister — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-582 Save selection / Save as — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-583 Next / previous file in folder — N / P — Lister shows next/previous file
- [ ] F-584 Reload / follow file (tail -f, "Auto-reload") — — — appended text appears
- [N/A] F-585 Lister plugins (WLX) — Windows DLL plugins — built-in viewers for images (incl. HEIC on macOS), PDF, audio, video, markdown, HTML, archive listing, font preview
- [ ] F-586 Show EXIF / image info in Lister — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-587 Copy as hex, code page switching (cm in Lister menu) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-588 Lister options: tab width, ANSI/ASCII charset default — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-589 Quick view uses Lister engine — Ctrl+Q — same modes in quick view panel
- [ ] F-590 External viewer option — — — F3 can be set to launch an external viewer; Alt+F3 the other one
- [ ] F-840 Lister charset keys: ANSI / ASCII (DOS) / variable-width font — A / S / V — each key changes the display at once
- [ ] F-841 Lister Unicode / UTF-8 keys — U / 8 — UTF-16 and UTF-8 files show correctly after the key
- [ ] F-842 Lister reload — F2 — file read again from disk
- [ ] F-843 Lister print — Ctrl+P — print dialog for the shown file
- [ ] F-844 Lister close — Esc (also Q) — window closes; focus returns to the panel
- [ ] F-845 Lister image zoom — + / - — image zooms in/out; fit-to-window toggles back
- [ ] F-846 Several Lister windows at once and remembered window size — — — F3 on another file opens a second window; size and position kept across uses

## 28. Editor

- [ ] F-591 Edit with external editor — F4 — default: system text editor (TextEdit with plain text on macOS, xdg default on Linux, Notepad on Windows) or configured app with %1 placeholder
- [ ] F-592 Edit file inside archive (extract, edit, repack on save) — F4 — after editor closes/file changes, app asks to update the archive
- [ ] F-593 Edit file on FTP/SFTP (download, edit, upload on change) — F4 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-594 Simple built-in text editor (optional) — — — small editor for quick edits (our addition; TC uses notepad)
- [ ] F-595 New file editing — Shift+F4 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button

## 29. Packers and archives

- [ ] F-596 Enter archive as folder (Enter on archive) — Enter — zip/tar/tgz/7z etc. contents shown like a folder; ".." exits
- [ ] F-597 Open archive under cursor as file instead (Ctrl+PgDn vs Enter option) — Ctrl+PgDn — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-598 Pack dialog: archive name, packer (zip, tar, tgz, tbz, tar.xz, gz, bz2, xz, 7z, zstd), recursive subfolders, store paths, multi-volume, move to archive, create separate archives (one per file/folder), encrypt (zip AES), compression level, self-extracting — Alt+F5 — each option produces correct archive readable by standard tools (unzip -t, tar -t)
- [ ] F-599 Internal ZIP packer (no external tools) — — — zip created/read with Go stdlib; UTF-8 names
- [ ] F-600 ZIP: create, read, add, delete, update, test — — — all operations on temp zip work
- [ ] F-601 ZIP: password-protected (ZipCrypto read, AES-256 create/read) — — — encrypted zip opens after password
- [ ] F-602 ZIP: large (>4GB) zip64 — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-603 ZIP: spanned/multi-volume (z01) — — — create and read multi-part
- [ ] F-604 TAR: create/read, with gzip, bzip2 (read; write via library), xz, zstd — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-605 GZ / BZ2 / XZ single-file packers — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-606 7z: read (list, extract) — — — 7z archive contents viewed and extracted (Go library)
- [~] F-607 7z: create — — — via external 7z if present; otherwise disabled with note
- [ ] F-608 RAR: read/extract — — — rar archives list and extract via Go library (read-only); creating rar uses external rar only if installed
- [N/A] F-609 ARJ / LZH / ACE / UC2 archives — obsolete formats, ACE dropped for security — open via external 7z if installed; CAB read via library where possible
- [ ] F-610 ISO read (browse) — — — ISO images open as folder
- [ ] F-611 DMG / APK / JAR / docx etc. as zip — — — jar/apk/docx/xlsx/epub open as zip (by content sniffing, Ctrl+PgDn)
- [ ] F-612 Unpack dialog: target folder, file mask, with paths, overwrite existing, unpack each archive to separate subfolder (archive name) — Alt+F9 — options respected
- [ ] F-613 Unpack all selected archives — Alt+F9 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-614 Test archive integrity — Alt+Shift+F9 — report ok/broken per archive
- [ ] F-615 Copy into archive (F5 to archive panel) — F5 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-616 Move to archive (Alt+Shift+F5 / option) — — — sources removed after verified pack
- [ ] F-617 Delete from archive — F8 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-618 Rename in archive — Shift+F6 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-619 Make folder in archive — F7 — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-620 Nested archives (zip inside zip) — — — entering inner archive works
- [ ] F-621 Archive with long paths / Unicode names — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-622 Archive comments — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [N/A] F-623 Packer plugins (WCX) — Windows DLLs — built-in formats + external CLI packer configuration (path + arguments)
- [ ] F-624 External packers configuration (pkzip/arj/rar/ace paths in TC) — — — user sets path for external 7z/rar executables
- [ ] F-625 Pack dialog: "Pack path names", "Recursively pack subdirectories", "Create separate archives, one per selected file/dir", "Multiple disk archive", "Encrypt", "Configure" — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-626 Show archive as folder in Branch view / Find files inside archives — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [N/A] F-627 Self-extracting archives (SFX) — Windows SFX stub, not portable — plain zip; documented
- [ ] F-628 Convert archive to another format (unpack+pack) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-847 Info on packed file — Alt+Enter / Enter on archive entry (per setting) — shows packed size, ratio, date, CRC and method
- [ ] F-848 Archive name encoding for old non-UTF-8 zips — — — user can choose the codepage; Hungarian names from an old Windows zip show correctly

## 30. FTP and network file systems

- [ ] F-629 FTP client: connect, list, navigate, download, upload, delete, rename, mkdir, chmod (site chmod) — — — all verified against a local FTP server on 127.0.0.1
- [ ] F-630 Passive / active mode — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-631 FTPS (explicit / implicit TLS) — — — connects to local TLS FTP test server
- [~] F-632 SFTP (SSH) client: password and key auth, agent, known_hosts check — — — verified against local SSH server on 127.0.0.1; host key prompt on first connect
- [ ] F-633 SFTP: chmod, symlinks, resume — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-634 Connection manager with folders, saved passwords (encrypted with master password / OS keychain) — — — saved passwords never stored in plain text; uses OS keychain or master password
- [~] F-635 Master password (TC) — — — master password protects stored passwords
- [ ] F-636 Quick connect via URL in path bar/command line (ftp://, sftp://) — — — typing a URL in path bar connects
- [ ] F-637 Proxy / firewall settings (SOCKS, HTTP CONNECT) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-638 FTP server-side copy/move within server — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-639 FTP time zone offset / time correction — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-640 FTP codepage (UTF-8 / local) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-641 Upload with same folder structure — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-642 Keepalive (NOOP) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-643 Transfer queue with parallel connections — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-644 HTTP/HTTPS download (TC: URL in FTP connect) — — — URL file downloads to current folder with progress
- [N/A] F-645 WebDAV / S3 / cloud via FS plugins — WFX plugins are Windows DLLs — later built-in feature; OS-mounted cloud folders work as normal folders
- [ ] F-646 SMB browsing (\\server\share) — — — Windows UNC paths work; macOS/Linux via mounted volumes
- [N/A] F-647 FS plugins (WFX: registry, process list, etc.) — Windows DLLs, registry is Windows-only — built-in virtual folders (Trash, search results, archives, FTP/SFTP)
- [ ] F-648 Edit/view remote file (F3/F4) — — — downloads to temp, opens, uploads on change
- [ ] F-649 Remote file properties — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-650 FTP disconnect on exit / idle timeout — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button

## 31. Directory hotlist and history

- [ ] F-651 Directory hotlist popup — Ctrl+D — list of favorite folders; choose to navigate
- [ ] F-652 Add current folder to hotlist — Ctrl+D then A — "Add current folder" entry
- [ ] F-653 Add both panels' folders (target too) — — — entry stores source and target paths
- [~] F-654 Configure hotlist (cm_ConfigDirHotlist) — — — dialog: add, delete, rename, reorder, submenus, separators, hotkey letters (&)
- [ ] F-655 Hotlist entries with commands (cd, cm_*, program) — — — entry may run a command
- [~] F-656 Hotlist hotkeys (&letter accelerators) — — — pressing letter in popup selects entry
- [~] F-657 Hotlist on drive bar / path bar button — — — star button opens hotlist
- [~] F-658 Hotlist search / type to filter — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-659 Hotlist in other panel (Ctrl+D with target) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-660 Directory history — Alt+Down — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-661 Frequently used folders — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-662 History size setting and clear — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-663 Hotlist import from TC [DirMenu] — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-664 Hotlist folder for tabs (tab sets) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-849 Hotlist paths with variables ($HOME, %COMMANDER_PATH%-style) — — — entries using variables open the expanded folder on every platform

## 32. Custom columns and column sets

- [ ] F-665 Custom column set editor (Configuration > Custom columns) — — — create named set; add columns: title, field (name, ext, size, date, attributes, permissions, owner, group, created, accessed, link target, comment, file type, MIME, image width/height, EXIF date, audio tags), width, alignment
- [ ] F-666 Field placeholders [=tc.size] style syntax, multiple fields in one column — — — e.g. "[=tc.size] bytes" works
- [ ] F-667 Units per field (size in kB/MB; date formats) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-668 Column set assigned per panel / per folder type (auto switch by folder) — — — column set switches automatically for matching folder (e.g. Pictures)
- [N/A] F-669 Custom columns from content plugins (WDX) — Windows DLLs — built-in field list above
- [ ] F-670 Sort by custom column — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-671 Show column sets in Show menu and header context menu — Shift+F1 — menu lists sets; choosing one switches the panel
- [ ] F-672 Header right-click: show/hide columns, resize — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-673 Column auto-width / equal-width — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-674 Built-in tc fields: name, ext, size, writedate, creationdate, accessdate, attributestr, path, fullname, comment, filetype — — — each available

## 33. Thumbnails

- [ ] F-675 Thumbnail view — Ctrl+Shift+F1 — images shown as previews
- [ ] F-676 Thumbnails for images (jpg, png, gif, webp, heic on macOS), PDFs, videos (macOS QuickLook), icons for others — — — previews generated in background, cache in config dir
- [ ] F-677 Thumbnail size adjust — — — size slider/option 32..512px
- [ ] F-678 Thumbnail cache (cm "Delete thumbnails cache") — — — cache can be cleared
- [ ] F-679 Thumbnails with custom field lines under picture — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-680 Thumbnail cache across sessions — — — reopening a folder shows cached thumbs instantly
- [ ] F-850 Keyboard moves in the thumbnail grid — arrows / PgUp / PgDn — arrows move by row and column; selection keys work as in other views

## 34. File properties and attributes

- [~] F-681 Properties dialog (own, cross-platform) — Alt+Enter — name, type, path, size, size on disk, dates, permissions, owner, link target, Get info (macOS) button
- [~] F-682 Open system properties (Windows shell dialog, macOS Get Info, Linux file manager) — — — button opens native dialog
- [~] F-683 Change attributes dialog — — — see Files menu
- [~] F-684 Change dates recursively — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-685 Attribute filters in attributes dialog via search template — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-686 Show attributes column (rhsa / rwxr-xr-x) — — — column shows Windows attributes on Windows, Unix permissions on macOS/Linux

## 35. Split / combine / checksums / encoding

- [~] F-687 Split by size — — — see Files menu
- [~] F-688 Combine — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-689 CRC32 SFV, MD5, SHA1, SHA256, SHA512 create — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-690 Checksum verify — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-691 Checksum of single file shown in properties / dialog — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-692 Base64/MIME, UUE, XXE encode/decode — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-693 Calculate checksum of selected files to clipboard — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button

## 36. Drag and drop

- [~] F-694 Drag between panels — — — copy with confirmation; Shift = move; Ctrl+Shift/Alt = link (TC rules), right-drag = menu
- [~] F-695 Drag to/from Finder/Explorer/other apps — — — files dropped from OS file manager into a panel are copied there; dragging from panel to OS app delivers file paths
- [~] F-696 Drag onto folder in list — — — drops into that folder
- [~] F-697 Drag onto drive button / tab / path bar segment — — — drops into that location
- [ ] F-698 Drag onto button bar — — — adds button / passes files to program
- [~] F-699 Drag to command line — — — inserts path
- [ ] F-700 Rubber-band selection with mouse (option, ours) — — — dragging on empty area selects files in rectangle (Full/Brief/Thumbnails)
- [~] F-701 Auto-scroll while dragging near edge — — — list scrolls
- [~] F-702 Spring-loaded folders (hover opens) — — — hovering over folder while dragging opens it after delay (option)
- [~] F-851 macOS drag modifiers follow Finder too — — — on macOS Option-drag copies, Cmd-drag moves, Cmd+Option-drag links, besides the TC rules

## 37. Context menus

- [~] F-703 File context menu (shell menu on Windows; own menu on macOS/Linux) — Shift+F10 / right-click — items: Open, Open with, View, Edit, Copy, Cut, Paste, Rename, Delete, Pack, Properties, Copy path, Reveal in Finder/Explorer/file manager
- [~] F-704 Background (empty area) context menu — — — New folder, New file, Paste, Refresh, Properties of folder
- [~] F-705 Drive button context menu — — — Eject/unmount (removable), Properties
- [ ] F-706 Tab context menu — — — see tabs
- [ ] F-707 Header context menu — — — columns
- [~] F-708 Shell extension items (Windows) — — — Windows shows the real Explorer context menu; macOS/Linux show our own menu with 'Open with'

## 38. Drives, volumes and disk operations

- [~] F-709 Drive list (Windows drive letters; macOS /Volumes and /; Linux mounts /media /mnt /run/media + home) — — — all mounted volumes listed with label and icons; updates when a USB drive is plugged in
- [~] F-710 Eject / unmount removable volume — — — eject from drive context menu
- [N/A] F-711 Format disk (cm_FormatDisk) — dangerous and OS-specific — button opens OS disk utility
- [N/A] F-712 Copy diskette (cm_CopyDisk) — obsolete — none
- [ ] F-713 Volume label edit — — — see Commands
- [ ] F-714 Free space indicator in drive bar — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-715 Network drives in list — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-716 Virtual root "My computer" / list of drives as folder — — — "/" root on macOS/Linux; drive list as virtual folder on Windows

## 39. Logging, errors and safety

- [~] F-717 Operation log file — — — see config
- [~] F-718 Error dialogs with clear plain English messages — — — e.g. "Can't copy: the disk is full."
- [~] F-719 Confirmations configurable (copy, delete, drag & drop, overwrite, exit) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-720 Elevation when access denied (Windows UAC "Continue as administrator") — — — Windows: retry elevated helper; macOS/Linux: show clear message (N/A sudo)
- [~] F-721 Protect against copying folder into itself — — — copying a folder into its own subfolder is refused with message
- [ ] F-722 Never follow symlink loops infinitely — — — recursive copy/size calc detects loops

## 40. Scripted control mode and testing infrastructure (our addition)

- [ ] F-723 Control mode --control PORT (0 = auto) writes control file with address and token — — — app starts with --control 0, file contains host:port and token; unauthenticated requests rejected
- [ ] F-724 Control commands drive same code paths as keys/mouse (run cm_* command, send key, click element, get state, list panel, screenshot) — — — scripts/mc_ctl.py can run cm_Copy in temp dirs and read panel state
- [ ] F-725 Screenshot command — — — returns PNG of the window
- [ ] F-726 docs/control.md documents control protocol — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-727 Demo/mock mode (like mormidiskfree mock.go) — — — --demo shows a fake file tree for screenshots without touching real files

## 41. Command-line parameters (TC wincmd parameters)

- [ ] F-728 Start with left/right paths (/L= /R=, or two positional paths) — — — "mormicommander -L=/tmp/a -R=/tmp/b" opens those folders
- [~] F-729 /O (open in existing instance) — — — path sent to running instance which navigates there
- [~] F-730 /T (open in new tab) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-731 /S (swap: interpret paths as source/target) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-732 /P=L or /P=R (activate panel) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-733 /I= /F= ini paths — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-734 /N (new instance) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-735 Single instance handling (re-exec like mormiview reexec*.go) — — — second launch focuses first window when single-instance option is on
- [~] F-852 Standalone tools from the command line (TC /S=L Lister) — — — 'mormicommander /S=L file' opens only the Lister; ours: same idea for compare and sync

## 42. Miscellaneous TC features

- [~] F-736 Directory size calculation on Space — Space — folder size shown in size column
- [~] F-737 Show occupied space (Ctrl+L) — Ctrl+L — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-738 "Smart" Enter on .lnk / .url / .webloc / .desktop files — — — Enter follows shortcut/symlink to target folder or opens URL
- [~] F-739 Clipboard file copy/paste interop with OS — Ctrl+C/V — files copied in Finder paste into a panel and vice versa (where OS clipboard supports file lists)
- [~] F-740 Paste text or image from clipboard as new file (TC 10+) — Ctrl+V — with text/image in clipboard, asks for a name and saves it as a file in the current folder
- [~] F-741 Long-running directory listing cancelable (Esc while reading slow folder) — Esc — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-742 Very large folders (100k files) list smoothly (virtual scrolling) — — — scrolling stays fluid; listing streams
- [ ] F-743 Show file count of folder while reading — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-744 Hide folders/files via ignore list — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-745 Descript.ion comments preserved on copy/move/rename — — — comments travel with files
- [ ] F-746 Open .url / .webloc file in browser — Enter — URL files open in the default browser
- [ ] F-747 Sound on operation finish (TC option) — — — optional system sound / notification when long background operation completes
- [ ] F-748 Always on top option (ours) — — — window stays above others when enabled
- [ ] F-749 Taskbar/dock progress indicator — — — dock badge/progress shows background copy progress where supported
- [N/A] F-750 Minimize to tray — no tray concept on macOS — normal minimize/Dock; optional tray on Windows/Linux
- [ ] F-751 Compare folders side by side highlighting (after Shift+F2) — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [~] F-752 "Calculate space" in status for selected folders — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-753 Wildcard select dialog history and saved selections — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-754 Unicode filename support everywhere (NFC/NFD on macOS) — — — Hungarian accented names created in Finder (NFD) match NFC searches and sort correctly
- [ ] F-755 Case-sensitive file systems — — — two files differing only in case are both shown and handled on Linux
- [ ] F-756 Permissions errors folders shown with message, not crash — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-757 Startup speed — — — app shows panels within 1 s on a normal machine
- [ ] F-758 Help text and labels in plain short English — — — review checklist passes (no jargon)
- [ ] F-759 Version single-sourced in wails.json and shown in About and --version — — — --version prints same version as About
- [ ] F-760 CHANGELOG.md maintained — — — each release has an entry
- [ ] F-761 Packaging: macOS .app/.dmg, Windows installer/zip, Linux AppImage/deb/tar (packaging/, scripts/make-*.mjs) — — — CI workflow builds all three (Linux with -tags webkit2_41)
- [N/A] F-762 Automatic update download — no unsolicited network traffic — manual 'Check for updates' opens the release page
- [ ] F-763 TC "Encode/Decode" and "Split" also in context menu — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-764 Panel focus indicator for keyboard users and screen readers (accessibility labels) — — — list items have aria labels; focus visible
- [ ] F-765 Localized decimal/thousand separators in sizes — — — same as menu: the entry is in its menu, runs its cm_* command, and gives the same result as the hotkey/button
- [ ] F-853 Environment for launched programs (COMMANDER_PATH, COMMANDER_INI) — — — programs started from the app see these variables

---

Total: 853 items (26 marked N/A).
