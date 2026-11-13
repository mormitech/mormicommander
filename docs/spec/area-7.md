# Area 7: Configuration dialog (Options), every page, defaults and wincmd.ini keys

This document describes how the Configuration dialog in Total Commander 11 behaves (TC calls it "Options", opened from Configuration > Options...). It covers every page, every option with its default, the wincmd.ini key that stores it, and the matching mormiCommander FEATURES.md IDs. The UI agents should be able to build the dialog from this file without opening TC. The config package should be able to read and write the same keys, which also lets F-446 import a real wincmd.ini. Where we differ from TC on purpose (macOS/Linux, web UI), the difference is written as a **mormi:** note right under the TC behaviour. That way the reference behaviour and our decision sit side by side.

Sources: the TC help (each page's F1 topic), the ghisler.ch wiki pages "Options" and "Wincmd.ini", TC history.txt (changelog up to 11.x), forum threads on individual keys, and working knowledge of TC 9 to 11. The wiki wincmd.ini page is old (TC 6/7 era). Where TC 10/11 changed a default or moved a control, the newer behaviour is described. Items marked **[verify]** come from memory and were not confirmed against a TC 11 install or its help file. Check them before relying on exact label wording or numeric ids.

Legend for the control tables:

- **Type**: `chk` checkbox, `rad` radio button (radio buttons sharing a group are listed together), `cmb` drop-down combobox (non-editable unless noted), `edt` edit field, `spn` numeric edit, `btn` push button, `lst` list box, `grp` group box caption.
- **Default**: the factory value of a new TC 11 install with no wincmd.ini. `on`/`off` for checkboxes, `•` marks the default radio button.
- **INI**: `Section/Key=value`. `[Configuration]` is abbreviated `C/`, `[Layout]` `L/`, `[Colors]` `Co/`, `[Confirmation]` `Cf/`, `[Packer]` `P/`, `[Lister]` `Li/`. A dash means the setting has no ini key or lives in another file (named in the row).

---

## 1. Entry points

| How | TC 11 | mormi |
|---|---|---|
| Menu | Configuration > Options... | same; on macOS also app menu "Settings..." (F-412) |
| Command | `cm_Config` (490) opens the dialog on the **last used page** (first open: Layout) | same |
| Hotkey | none by default (the user can bind one in Misc) | Cmd+, on macOS (F-285, F-412) |
| Page commands | each page has its own command that opens the dialog **directly on that page** (table below) | same names, so buttons and hotkeys imported from TC keep working (F-446) |
| Button bar | the default bar has a "Configure" button that runs `cm_Config` | same |
| Other dialogs | several dialogs have a "Configure..." / "Options..." button that opens the matching page (for example Pack dialog > Configure... opens Packer, Lister > Options > Configure... opens Edit/View > Lister, Find files > Configure... opens Operation) | same |

Page commands (numeric ids from totalcmd.inc, **[verify]** the numbers; the names are stable since TC 7):

| Command | Id | Opens page |
|---|---|---|
| `cm_Config` | 490 | last used page |
| `cm_ConfigToLayout`/`cm_LayoutConfig` | 476 | Layout **[verify name]** |
| `cm_DisplayConfig` | 486 | Display |
| `cm_IconConfig` | 477 | Display > Icons |
| `cm_FontConfig` | 492 | Display > Font |
| `cm_ColorConfig` | 494 | Display > Color |
| `cm_LanguageConfig` | 499 | Display > Language |
| `cm_ConfTabChange` | 497 | Tabstops |
| `cm_DirTabsConfig` | 488 | Folder tabs |
| `cm_CustomColumnConfig` | 483 | Custom columns |
| `cm_CustomColumnDlg` | 2920 | the column editor for the current custom column view (no tree) |
| `cm_ConfigViewModes` | **[verify]** | Custom columns > View modes |
| `cm_Config2` | 516 | Operation |
| `cm_EditConfig` | 496 | Edit/View |
| `cm_CopyConfig` | 487 | Copy/Delete |
| `cm_RefreshConfig` | 478 | Refresh |
| `cm_QuickSearchConfig` | 479 | Quick search |
| `cm_FtpConfig` | 489 | FTP |
| `cm_PluginsConfig` | 484 | Plugins |
| `cm_ThumbnailsConfig` | 482 | Thumbnails |
| `cm_LogConfig` | 481 | Log file |
| `cm_IgnoreConfig` | 480 | Ignore list |
| `cm_PackerConfig` | 491 | Packer |
| `cm_ZipPackerConfig` | 485 | Packer > Zip packer |
| `cm_Confirmation` | 495 | Misc |

Related Configuration-menu commands with their own dialogs or no dialog (section 6): `cm_ButtonConfig` (498), `cm_ConfigToolbars` (**[verify]**), `cm_ConfigSavePos` (493), `cm_ConfigSaveSettings` (580), `cm_ConfigChangeIniFiles` (581), `cm_ConfigSaveDirHistory` (582), `cm_ChangeStartMenu` (700 **[verify]**), `cm_ConfigDirHotlist`, `cm_ConfigFavoriteTabs`, `cm_ConfigMenu`/main menu file (Language page).

Configuration menu in TC 11, top to bottom (F-010):

```
Options...
Layout...                       (opens Layout page)
Display... / Color... etc.      (in TC 11 these are not separate items; only Options...)  [verify]
-
Change button bar...
Change main/start menu...       ("Change Start menu...")
Edit settings files...           cm_ConfigChangeIniFiles  (TC 9.10+; hidden in some builds) [verify position]
-
Save position                    cm_ConfigSavePos
Save settings                    cm_ConfigSaveSettings
```

---

## 2. The dialog frame

### 2.1 Layout

```
+-- Configuration ---------------------------------------------------------+
| +------------------+  +-----------------------------------------------+  |
| | Layout           |  |  <page title as grp caption, e.g.             |  |
| | Display          |  |   "Screen layout">                            |  |
| |   Icons          |  |                                               |  |
| |   Font           |  |    ... page controls ...                      |  |
| |   Color          |  |                                               |  |
| |   Language       |  |                                               |  |
| | Operation        |  |                                               |  |
| | Tabstops (*)     |  |                                               |  |
| | Folder tabs      |  |                                               |  |
| | Custom columns   |  |                                               |  |
| |   View modes     |  |                                               |  |
| | Edit/View        |  |                                               |  |
| |   Lister         |  |                                               |  |
| | Copy/Delete      |  |                                               |  |
| | Refresh          |  |                                               |  |
| | Quick search     |  |                                               |  |
| | FTP              |  |                                               |  |
| | Plugins          |  |                                               |  |
| | Thumbnails       |  |                                               |  |
| | Log file         |  |                                               |  |
| | Ignore list      |  |                                               |  |
| | Packer           |  |                                               |  |
| |   Zip packer     |  |                                               |  |
| | Misc.            |  |                                               |  |
| +------------------+  +-----------------------------------------------+  |
|                               [  OK  ] [ Cancel ] [ Apply ] [ Help ]     |
+--------------------------------------------------------------------------+
```

(*) TC 9+ folded the old "Tabstops" page into Display / Custom columns in some layouts; TC 11 still lists it as a page. **[verify]** whether TC 11 shows "Tabstops" as a top-level item or under Display. The tree order above is TC 10/11's order as best known. The things that matter for us are the parent/child structure (Display > Icons/Font/Color/Language, Custom columns > View modes, Edit/View > Lister, Packer > Zip packer) and that Misc. is last.

TC 11 does not have a separate "Tooltips" page. The per-file-type tooltip fields are set from Display ("Help texts" group, "+" / "Custom fields" button), which opens a sub-dialog (section 5.4). FEATURES.md F-821 asks for a page; **mormi:** we put it as a child of Display named "Tooltips". Likewise FEATURES lists F-823 Drag & drop/mouse and F-824 Tree as pages. In TC those options are spread over Operation (mouse) and Layout/Display (tree). **mormi:** we keep TC's controls on TC's pages, and also add the two pages as ours, both showing the same bindings, so a TC user finds them where they expect.

### 2.2 Frame behaviour

1. Modal dialog, centred on the main window. Title "Configuration". In TC 11 the dialog can be resized (TC 10+) and remembers its size in `C/ConfigDialogSize` **[verify key]**. On a small screen the page area scrolls.
2. The left tree is a TreeView with every node always expanded (no collapse buttons). Clicking a node shows its page on the right. The page title is drawn as the first group-box caption on the page.
3. **OK**: saves all pages (only changed values are rewritten) to wincmd.ini at once, applies them to the running program, closes the dialog. Default button (Enter in a single-line edit or checkbox triggers OK).
4. **Cancel** / Esc / window close: throws away every change made since the dialog opened or since the last Apply. Values already written by Apply stay.
5. **Apply**: saves and applies like OK but keeps the dialog open. Disabled is not used. The button stays enabled at all times.
6. **Help** / F1: opens the help topic for the current page.
7. The last visible page is remembered in memory for the session. In TC 11 it is also stored **[verify]**. `cm_Config` reopens it. Page commands always open their own page.
8. Keyboard: Tab / Shift+Tab move through the tree and then the page controls in visual order (top to bottom, left to right, inside each group). In the tree, Up/Down change page immediately. Alt+underlined letter activates the control on the current page. Ctrl+Tab / Ctrl+Shift+Tab move to the next/previous page **[verify]**. **mormi:** implement Ctrl+Tab / Ctrl+Shift+Tab and Ctrl+PgDn/PgUp anyway, plus Cmd+[ / Cmd+] on macOS.
9. Values are checked when the user presses OK/Apply or leaves a page. For a bad number TC shows the standard Windows edit balloon for numeric edits, or the message "Invalid value!" **[verify text]**, and puts focus back on the field. Paths are not checked for existence. Exceptions are noted per page (for example the Log file path is created on first write, and if it cannot be created you see an error at the first logged operation, not in the dialog).
10. Options that need a restart: TC shows "Some changes will only take effect after restarting Total Commander!" **[verify text]** after OK when any of these changed: Language, main menu file, "Allow only 1 copy", icon size in some cases, separate tree mode on old versions. Everything else applies live (F-413 asks that all Layout toggles apply live).
11. If wincmd.ini is read-only or cannot be written (UAC virtualisation, network share), TC shows: "Error writing to wincmd.ini! Settings could not be saved." **[verify text]**. The in-memory settings stay applied for the session.
12. "Save settings on exit" is not on any page of the Options dialog. Configuration > Save settings (`cm_ConfigSaveSettings`) and the Operation page's "Save on exit" group control what is written when the program closes (section 4.7).

**mormi:**
- F-827: add a search box above the tree. Typing filters the tree to pages whose label text matches (case-insensitive, accents folded so "Atrendezes" finds "Átrendezés"). Matching controls are highlighted on the page. Esc in the box clears it. This is ours, not TC's.
- F-412: on macOS the dialog also opens from Cmd+, and the window can be a normal resizable sheet. Same OK/Cancel/Apply semantics. We do **not** switch to macOS instant-apply, because TC users expect Cancel to undo.
- F-021: the theme override (light/dark/system) goes on Display > Color, mirroring TC 10+'s dark mode setting.

---

## 3. Page reference

Each page section has: the command, the layout, a control table, behaviour and edge cases, FEATURES IDs.

### 3.1 Layout (`cm_LayoutConfig`) - F-413, F-022, F-024, F-060, F-003, F-824

```
+- Screen layout ----------------------------------------------------+
| [x] Show button bar                    [x] Show vertical button bar|
| [x] Show drive buttons                                             |
|     [x] Show two drive button bars (fixed width, above file windows)|
|     [ ] Flat icons                                                  |
| [x] Show drive combobox(es)                                        |
| [x] Show folder tabs                                                |
| [x] Show current directory                                          |
|     [x] Show history and hotlist buttons                            |
|     [x] Show breadcrumb bar (TC 10+)                    [verify]     |
| [x] Show sort header                                                |
| [x] Show status bar                                                 |
| [x] Show command line                                               |
| [x] Show function key buttons                                       |
| [ ] Flat user interface                                             |
+- Window ------------------------------------------------------------+
| [x] Windows XP/visual style for menus and all bars (TC: "Use theme")|
| [ ] Show free space as bar in drive combobox     (TC 9+) [verify]   |
+- Separate tree -----------------------------------------------------+
| (•) None  ( ) One for both file panels  ( ) One per file panel      |
|  Options: [x] Tree follows file panel  [ ] Show hidden folders ...  |
+- Panel ---------------------------------------------------------------+
|  Show panel: [x] side by side ( ) stacked (horizontal panels)       |
+--------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| Show button bar | chk | on | `L/ButtonBar=1` | F-022 |
| Show vertical button bar (TC 9+) | chk | off | `L/ButtonBarVertical=0` | middle bar between panels; F-024. Its buttons come from `[Buttonbar]VerticalBar` / its own .bar file, edited via Change button bar > vertical (F-314) |
| Show drive buttons | chk | on | `L/DriveBar1=1` | drive bar above panels |
| Show two drive button bars | chk | on | `L/DriveBar2=1` | one per panel (F-060); enabled only if drive buttons is on |
| Flat icons (drive bar) | chk | off | `L/DriveBarFlat=0` | |
| Show drive combobox | chk | on | `L/DriveCombo=1` | TC 11 default on **[verify]**; wiki says 0 in old TC |
| Show folder tabs | chk | on | `L/DirectoryTabs=1` | tab bar; whether it shows with only one tab is on Folder tabs page |
| Show current directory | chk | on | `L/CurDir=1` | path line above each panel |
| Show history and hotlist buttons | chk | on | `L/HistoryHotlistButtons=1` | the `*` and down-arrow buttons at the right of the path line; enabled only with current directory |
| Show breadcrumb bar | chk | on | `L/BreadCrumbBar=1` **[verify]** | TC 10+: path line segments clickable |
| Show sort header (tabstop header) | chk | on | `L/TabHeader=1` | |
| Show status bar | chk | on | `L/StatusBar=1` | file count/size line below each panel |
| Show command line | chk | on | `L/CmdLine=1` | |
| Show function key buttons | chk | on | `L/KeyButtons=1` | F3 View ... Alt+F4 Exit row |
| Flat user interface | chk | off | `L/InterfaceFlat=0` | flat bars and path headers |
| Use XP theme background | chk | on | `L/XPthemeBg=1` **[verify]** | Windows only |
| Separate tree | rad | • None | `C/SeparateTree=0` (0 none, 1 one for both, 2 one per panel) | TC 9.0+; F-824 |
| Tree follows file panel / folder | chk | on | `C/TreeFollowsPanel=1` **[verify]** | |
| Panels arrangement | — | side by side | `C/PanelsVertical=0` (1 = stacked) | not on the page in TC; set by cm_HorizontalPanels and saved. **mormi:** show it here as radio (F-003) |

Behaviour:
- Every checkbox applies on Apply/OK without restart. The main window re-lays out at once.
- Child checkboxes are disabled (greyed, value kept) while their parent is off.
- The same items can be toggled by commands (button bar or hotkey): `cm_VisButtonBar`, `cm_VisDriveButtons`, `cm_VisTwoDriveButtons`, `cm_VisFlatDriveButtons`, `cm_VisFlatInterface`, `cm_VisDriveCombo`, `cm_VisCurDir`, `cm_VisBreadCrumbs`, `cm_VisHistHotButtons`, `cm_VisDirTabs`, `cm_VisXPThemeBackground`, `cm_VisTabHeader`, `cm_VisStatusBar`, `cm_VisCmdLine`, `cm_VisKeyButtons`, `cm_VisButtonBar2` (vertical bar). Each one flips the same ini key, so the checkbox and the command never disagree. Toggling with a command writes the ini immediately **[verify]**; mormi writes on the next save point.
- The menu bar itself cannot be hidden from this page in TC. TC hides it only via `[Configuration] ... ` custom empty .mnu. **mormi:** F-022 lists "menu" as hideable; add "Show menu bar" (default on, `L/MenuBar=1`, ours). On macOS the global menu bar is always there, so the checkbox is hidden.
- **mormi:** use the same keys for all these toggles. The "Show path in window title" option lives on Misc (3.24), not here.

### 3.2 Display (`cm_DisplayConfig`) - F-414, F-090, F-092, F-093, F-094, F-098, F-102, F-106, F-115, F-116

```
+- Show files ----------------------------------------------------------+
| [ ] Show hidden files              [ ] Show system files              |
| [x] Use long names                                                    |
| [ ] Show old 8.3 filenames in lowercase (Win only)                   |
| [x] Show parent directory [..] also in drive root                     |
| [x] Show square brackets around directories                           |
| [ ] Show icons in brief view? (→ Icons page)                          |
| [ ] Limit column width in Brief view to: [ 300 ] pixels              |
+- Sorting ------------------------------------------------------------+
| Sort method: [ Natural sorting: alphabetical and numbers ▼ ]          |
| Sort directories: (•) always by name  ( ) like files (also by time)   |
| [ ] Sort by name: case sensitive  (TC 11: in sort method list)       |
+- Help texts (tooltips) ----------------------------------------------+
| [x] Tooltips for buttons in button bar and drive bar                  |
| [x] Show volume labels / network paths in drive tooltips             |
| [x] Show filename tooltip if name is truncated                        |
| [x] Win32-style tips with file comments                               |
|     [x] comments from descript.ion  [x] comments from Explorer        |
|     [ + ] Custom fields depending on file type...                     |
| Delay: [ 500 ] ms  (TC 11 [verify])                                  |
+- Tree --------------------------------------------------------------+
| [ ] Always load complete tree  (Alt+F10 / tree view)                  |
| [ ] Enter (or double click) opens directory in other panel            |
+--------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| Show hidden files | chk | off | `C/ShowHiddenSystem=0` bit 1 | TC 9+ splits hidden and system: `ShowHiddenSystem` is 0/1 and the system part follows `C/ShowSystem`? **[verify]**. Older: one checkbox "Show hidden/system files (for experts only!)". F-090 (`cm_SwitchHidSys` toggles). **mormi:** "Show hidden files (dotfiles and hidden flag)"; system files are N/A on macOS/Linux |
| Use long names | chk | on | `C/UseLongNames=1` | |
| Show 8.3 names in lowercase | chk | on | `C/Small83names=1` | Windows only; **mormi:** omit |
| Show parent dir [..] also in root | chk | off | `C/ShowParentDirInRoot=0` | F-102 shows ".." in every folder; root of a drive has none unless this is on |
| Show square brackets around directories | chk | on | `C/DirBrackets=1` | |
| Limit Brief column width | chk+spn | off, 300 | `C/BriefViewWidthLimit=0` (0 = off, else px) **[verify]** | |
| Sort method | cmb | Natural sorting: alphabetical and numbers | `C/SortUpper=` 0 Default/locale (`CompareString`), 1 Classic ASCII/underscore last, 2 natural (numbers), 3 StrCmpLogicalW (Explorer-like), 4 natural case-sensitive? **[verify list order]**. TC 11 default is natural (`SortUpper=2`? **[verify]**) | F-115, F-116 |
| Sort directories always by name | rad | • always by name | `C/SortDirsByName=1` (0 = like files) | F-098 |
| Directories first | — | always | — | TC always lists folders before files; `C/SortDirsExtFirst`? no. Folders-first cannot be turned off in TC. **mormi:** same, plus hidden ini key `C/DirsFirst=1` (ours) |
| Tooltips for buttons | chk | on | `C/Tips=3` bit 1 | |
| Drive tooltips | chk | on | `C/Tips=3` bit 2 | |
| Filename tooltip if truncated | chk | on | `C/FileTipWindows=1` | |
| Win32 tips with comments | chk | on | `C/Win32TipWindows=1` | F-106: hover shows size/date/type |
| Comments from descript.ion / Explorer | chk | on/on | `C/OleComments=1`, `C/CopyComments` bit 4 | **mormi:** descript.ion only (F-?? comments area); Explorer comments N/A, map to macOS Finder comment xattr (ours) |
| Custom fields depending on file type | btn | — | `[CustomFields]` section | opens the tooltip sub-dialog (5.4). F-789, F-821 |
| Always load complete tree | chk | off | `C/TreeLoadAll`? **[verify]** | |
| Enter opens directory in other panel (tree view) | chk | off | `C/TreeEnterOther`? **[verify]** | |

Behaviour and edge cases:
- Turning hidden files off while the cursor sits on a hidden file moves the cursor to the next visible item (or the previous one if it was last). Selection on now-hidden files is dropped, so a following F8 never deletes invisible files. That is the TC rule: what you can't see isn't marked.
- Sort method changes re-sort both panels at once, keeping the cursor on the same file.
- Size/date format and extension alignment live on the Tabstops page (3.7), not here.

**mormi:** dotfiles are "hidden" on all platforms. On macOS the `UF_HIDDEN` flag also counts. On Windows the hidden attribute counts and system attribute files are shown only with "Show system files". Default for mormi: hidden off (TC default).

### 3.3 Display > Icons (`cm_IconConfig`) - F-415

```
+- Show symbols / icons -----------------------------------------------+
| (•) All associated + EXE/LNK (slow)                                  |
| ( ) All associated icons                                              |
| ( ) Only standard icons                                               |
| ( ) No icons                                                          |
|   [x] Don't load EXE/LNK icons from removable/floppy  (TC: "not on floppy")
|   [x] Don't load EXE/LNK icons on network drives (not on net)         |
| [x] Show overlay icons, e.g. for links                                |
| Special folder icons: [x] normal file system  [x] virtual folders     |
| [x] Show icons in file lists for:  Brief [x]  Full [x]               |
+- Icon size in file lists --------------------------------------------+
| ( ) 16x16 (•) 20x20? ... Size: [ 16 ▼ ]  (16, 20, 24, 32, 48, 64...)  |
+- Icons in main menu (TC 9+) -----------------------------------------+
| [x] Show   Size: [ 16 ▼ ]   [ ] 3D frame  [ ] checked items as pressed|
+- Icons in tree / folder tabs -----------------------------------------+
| [x] Show icons in tree   [ ] Show icons in folder tabs (TC 11?) [verify]|
+--------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| Icon mode | rad | • All associated + EXE/LNK | `C/Showicons=2` (2 all+exe, 1 all associated? 1 standard?, 0 none) | wiki: 0 none, 1 standard, 2 all. TC 9+: `ShowEXEandLNKicons` separate. **[verify mapping]** |
| EXE/LNK icons: not on removable | chk | on | `C/IconsOnRemovable`? **[verify]** | |
| EXE/LNK icons: not on network | chk | on | `C/IconsOnNet=0` (inverse: 1 = load on net) | |
| Load icons in background thread | — | on | `C/IconsInThread=1` | no UI |
| Overlay icons | chk | on | `C/IconOverlays=1` | link arrow, cloud sync state |
| Special folder icons | chk×2 | on, on | `C/IconsSpecialFolders=3` (bit 1 local, bit 2 virtual/network) | |
| Icon size in file lists | cmb | 16 | `[<resolution>]/Iconsize32=16` (per screen resolution section, for example `[1920x1080 (8x16)]`), TC 10+ also `[AllResolutions]` | values 16, 20, 24, 32, 40, 48, 64 **[verify list]** |
| Main menu icons: show | chk | on | `C/IconsInMenus=16` (0 = off, else size) | TC 7.5+ |
| Menu icon size | cmb | 16 | same key | 16, 24, 32, 48 |
| 3D frame / checked as pressed | chk | off/off | `C/IconsInMenus3D`? **[verify]** | |
| Custom icon library | — | empty | `C/IconLib=` | no UI; TC uses for drive/folder icons |

Behaviour: changing icon mode or size reloads both panels. Icon size per screen resolution means a laptop and an external monitor can have different sizes. TC picks the section by the current resolution and font size and falls back to `[AllResolutions]` if `C/ResolutionSpecific=0` **[verify]**.

**mormi:** icon sizes 16/24/32/48 (F-415). EXE/LNK options do not apply. Map "not on network" to "don't extract real icons on network volumes (show generic)". Overlay icons: symlink arrow everywhere, plus macOS iCloud placeholder state (ours). One `[Display] IconSize` key, not per resolution. On import (F-446) we take the size from the resolution section that matches the largest monitor, else the first found.

### 3.4 Display > Font (`cm_FontConfig`) - F-416, F-020

```
+- Fonts ----------------------------------------------------------------+
| File list:       [ Change font... ]   Tahoma, 9 pt          (sample)    |
| Main window:     [ Change font... ]   MS Sans Serif, 8 pt  (sample)     |
| Dialog boxes:    [ Change font... ]   (system default)    (sample)      |
| [ Default ]                                                             |
+------------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| File list font | btn | Tahoma 9 (TC 11: "Segoe UI" 9 on Win10+ **[verify]**) | `[<res>]/FontName=`, `FontSize=`, `FontWeight=` (400/700), `FontCharset=` | the font for both file panels, tree, folder tab text? (tabs use main window font) |
| Main window font | btn | system UI font | `[<res>]/FontNameWindow=`, `FontSizeWindow=`, `FontWeightWindow=`, `FontCharsetWindow=` | path line, status bar, command line, tabs, function keys |
| Dialog font | btn | system | `[<res>]/FontNameDialog=`, `FontSizeDialog=` | TC 8+; applies after the next dialog opens |
| Lister font | — | — | `[Lister]/Font1..Font3` | set from Lister options, not here (3.9) |

Each "Change font..." opens the system font chooser (Windows ChooseFont with face, style, size, script; no colour/effects). Cancel in the chooser leaves the font alone. The sample label shows the name and size in that font.

**mormi:** our own font picker: family (installed fonts list plus "System UI" and "System monospace" entries), size (6..36, step 1, also typing), bold. Ini keys `[Font]` `FileList=`, `FileListSize=`, `Window=`, `WindowSize=`, `Dialog=`, `DialogSize=` (ours, flat, no resolution sections). F-020: applies immediately on Apply. Also Ctrl+Plus / Ctrl+Minus zoom of file list font is ours (not TC).

### 3.5 Display > Color (`cm_ColorConfig`) - F-417, F-096, F-097, F-435, F-021

```
+- Main window -----------------------------------------------------------+
| Font:              [■ ▼]       Background:        [■ ▼]                 |
| Background 2:      [■ ▼]  (alternate lines; same as Background = off)   |
| Mark (selected):   [■ ▼]       Cursor:            [■ ▼]                 |
| Cursor text:       [■ ▼]                                               |
| [ ] Inverted cursor (solid bar)  [ ] Inverted selection                 |
| [ ] Cursor only frame (TC 10+: "Cursor as frame") [verify]              |
| Inactive panel: [ ] use different cursor color  [■ ▼]  [verify TC 11]  |
|                 [ ] use different background  [■ ▼]                    |
+- Others -----------------------------------------------------------------+
| Lister text / background / selected text / selected background / links  |
| Compare by content: differences (text) / selected; binary / selected    |
| Thumbnail frame: [■ ▼]                                                  |
+- Dark mode (TC 10+) ----------------------------------------------------+
| Dark mode: (•) never ( ) when Windows uses dark mode ( ) always  [verify]|
| [ Dark mode colors... ]                                                  |
+-------------------------------------------------------------------------+
| [ Define colors by file type... ]                       [ Default ]      |
| Preview:  [ sample file list with all colours ]                          |
+-------------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| Font (text) | color | -1 (window text, black) | `Co/ForeColor=-1` | colours stored as decimal BGR int (R + 256·G + 65536·B), -1 = system default |
| Background | color | -1 (window, white) | `Co/BackColor=-1` (wiki shows 16777215) | |
| Background 2 | color | same as background (off) | `Co/BackColor2=-1` | F-096 alternating rows |
| Mark | color | 255 (red) | `Co/MarkColor=255` | colour of selected file names |
| Cursor | color | 8421504? TC 11: highlight colour **[verify]** | `Co/CursorColor=` | |
| Cursor text | color | 16777215 (white) | `Co/CursorText=` | with inverted cursor only |
| Inverted cursor | chk | on in TC 11 **[verify]** (off in old TC) | `Co/InverseCursor=1` | solid bar vs frame. F-096 |
| Inverted selection | chk | off | `Co/InverseSelection=0` | selected files as solid boxes instead of red text |
| Inactive cursor colour | chk+color | off | `Co/InactiveCursorColor=-1`, `Co/InactiveMarkColor=-1` **[verify]** | |
| Lister colours | color×5 | system | `Li/FgColor`, `BgColor`, `FgSelColor`, `BgSelColor`, `LinkColor` **[verify section]** | |
| Compare colours | color×4 | red/... | `Co/CompareText`, `CompareTextSel`, `CompareBinary`, `CompareBinarySel` | used by Compare by content (area 9) |
| Thumbnail frame | color | -1 | `Co/ThumbnailFrame=-1` (-2 invisible) | |
| Dark mode | rad | never? TC 11 default "when Windows uses dark mode" **[verify]** | `C/DarkMode=` 0 never, 1 always, 2 auto **[verify values]** | dark palette saved with `Dark` suffix keys in `[Colors]` (`BackColorDark` ...) **[verify]** |
| Define colors by file type | btn | — | `C/ColorFilters=1` plus `[Colors]ColorFilter1=*.exe`, `ColorFilter1Color=` ... | sub-dialog 5.3; F-097, F-435 |

Each colour drop-down shows a swatch list of 16 basic colours, "Default" (the -1 value), and "Other..." which opens the system colour picker. Changes are shown in the preview at once and in the panels on Apply.

**mormi:** colours as `#rrggbb` strings in our ini, `default` for theme colour. Import converts TC's BGR ints (careful: TC's int is R in the low byte, so `255` is pure red). F-021: theme radio "Follow system / Light / Dark", key `[Colors] Theme=system|light|dark`. Separate light and dark palettes, keys `Light.Background=` / `Dark.Background=` (ours). The defaults come from the app CSS tokens, so "default" means "follow the theme" rather than a fixed colour.

### 3.6 Display > Language (`cm_LanguageConfig`) - F-418, F-828

```
+- Language ----------------------------------------------------------+
| ( ) English                                                         |
| ( ) Deutsch      WCMD_DEU.LNG                                       |
| ( ) Magyar       WCMD_HUN.LNG                                       |
|   ... one radio per *.lng file found in %COMMANDER_PATH%\Language   |
+- Main menu --------------------------------------------------------+
| Menu file: [ WCMD_ENG.MNU                    ▼ ] [ Edit... ]       |
+--------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| Language list | lst/rad | English (built in) | `C/LanguageIni=` (empty = English; else file name like `WCMD_HUN.LNG`) | list built by scanning the Language folder; each line shows the language name read from the file's header |
| Main menu file | cmb | default (WCMD_ENG.MNU) | `C/Mainmenu=` | the .mnu that builds the menu bar (F-828). Choosing a language also switches to that language's .mnu if one exists |
| Edit... | btn | — | — | opens the .mnu in the editor (F4 editor) |
| First run language prompt | — | — | `C/FirstTime=`, `C/FirstLang=0` | no UI |

Language changes apply immediately in TC 7+ for most strings. The menu is rebuilt. A few strings (function key bar captions) update after restart. A missing or broken .lng falls back to English without an error.

**mormi:** languages from embedded JSON (en, hu). External files in `<configdir>/lang/*.json` also listed. Key `[Configuration] Language=en`, empty = follow the OS locale (ours; TC has no "follow OS"). F-828 main menu file: ours loads a `.mnu`-compatible text file. "default" restores the built-in menu.

### 3.7 Tabstops (`cm_ConfTabChange`) - F-092, F-093, F-094

```
+- Show extensions ----------------------------------------------------+
| ( ) directly after filename   (•) aligned (with tab)                  |
+- Tabstops (column positions in Full view) ---------------------------+
| Extension [ 104 ] Size [ 109 ] Date/Time [ 158 ] Attributes [ 58 ]    |
| [x] Adjust tabs to window width (name column fills the rest)          |
+- Date/time ----------------------------------------------------------+
| [x] Show century (2025 instead of 25)   [ ] 12-hour AM/PM time         |
| Date format: [system short date ▼]  (TC 11: custom format [verify])   |
| [x] Show seconds? (no, TC shows hh:mm)                                 |
+- Size display --------------------------------------------------------+
|              In file list            In footer (status bar)            |
|  ( ) bytes                (•)         ( )                              |
|  ( ) kbytes               ( )         ( )                              |
|  ( ) dynamic (x k/M/G)    ( )         ( )                              |
|  ( ) dynamic (x.x k/M/G)  ( )         (•)                              |
|  ( ) dynamic (x k/M)      ( )         ( )                              |
|  ( ) dynamic (x.x k/M)    ( )         ( )                              |
| [x] Show directory count in footer                                     |
+--------------------------------------------------------------------- + 
| [ Default ]                                                            |
+------------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| Show extensions aligned | rad | • aligned | `C/Aligned extension=1` (key has a space) | F-092 |
| Tabstop positions | spn×4 | 104,109,158,58 (px at 96 dpi) | `[<res>]/Tabstops=104,109,158,58,374,93` | changed also by dragging header separators |
| Adjust tabs to window width | chk | on | `C/AdjustWidth=1` | |
| Show century | chk | on | `C/ShowCentury=1` | F-094 |
| 12-hour AM/PM | chk | off | `C/ShowAMPM=0` | |
| Size in file list | rad | • bytes | `C/SizeStyle=0` (0 bytes, 1 kB, 2 dyn int, 3 dyn 1 decimal, 4 dyn k/M int, 5 dyn k/M 1 dec) | F-093 |
| Size in footer | rad | • dynamic x.x | `C/SizeFooter=3` | |
| Directory count in footer | chk | on | `C/DirsInFooter=1` | |
| Default | btn | — | — | resets this page's controls only |

Edge cases: kB is 1024 bytes in TC (it shows "k", not "KiB"). "Dynamic" picks the largest unit with a value of at least 1. Bytes are shown with thousands separators from the locale. **mormi:** same 1024 base by default. Add a `C/SizeBase1000=0` option (ours) because macOS Finder uses 1000. Date format: TC uses the Windows short date and time format. **mormi:** "System" (locale via Intl) or a custom pattern, `C/DateFormat=` (ours), F-094.

### 3.8 Folder tabs (`cm_DirTabsConfig`) - F-432, F-030, F-036, F-041, F-046, F-028

```
+- Folder tabs ---------------------------------------------------------+
| [x] Show tab header also when there is only one tab                    |
| [ ] Confirm closing of all tabs                                         |
| [x] Close tab on double click                                           |
| [x] Ctrl+Up opens directory in new tab in foreground? (off = background)|
| [x] Open new tabs near current tab                                      |
| [x] Tabs on multiple lines                                              |
|     [ ] Each tab line has own width? (TC: "All tabs have same width")   |
| [ ] Limit tab title length to: [ 32 ] characters                        |
| [x] Show locked tabs with an asterisk *                                 |
| [x] Activate target panel when clicking on one of its tabs              |
| [ ] Always show drive letter in tab title                               |
| [x] Show close button on tabs? (TC 11: "Show X on tabs") [verify]       |
| Double click on empty tab bar: [ open new tab ▼ ]  (TC 10+) [verify]   |
+- Locked tabs ---------------------------------------------------------+
| Tabs locked with "allow directory changes":                             |
|   [ ] Go to root of drive when going to the locked tab                  |
| Changing directory in locked tab:                                       |
|   ( ) New tab  (•) Same tab when dir already open ...  [verify]         |
+- Favorite tabs -------------------------------------------------------+
| [ Configure favorite tabs... ]   (cm_ConfigFavoriteTabs)                |
+-----------------------------------------------------------------------+
```

The ini key `C/DirTabOptions` holds most checkboxes as a bit sum (TC packs them). Known bits **[verify all]**:

| Bit | Value | Meaning | Default |
|---|---|---|---|
| 1 | 1 | show tab header also with one tab | on |
| 2 | 2 | confirm close all tabs | off |
| 3 | 4 | close tab on double click | on |
| 4 | 8 | Ctrl+Up opens new tab in foreground | off |
| 5 | 16 | open new tabs near current tab | on |
| 6 | 32 | tabs on multiple lines | on |
| 7 | 64 | all tabs have same width | off |
| 8 | 128 | show locked tabs with asterisk | on |
| 9 | 256 | activate target panel when clicking its tab | on |
| 10 | 512 | always show drive letter | off |
| 11 | 1024 | limit title length | off |

Default `DirTabOptions=500` is consistent with the bits above (4+16+32+128+256+... **[verify]**). The other keys:

| Label | Type | Default | INI |
|---|---|---|---|
| Limit tab title length to N | spn | 32 | `C/DirTabLimit=32` |
| Remove spaces? | — | — | `C/DirTabRemoveSpaces` **[verify]** |
| Locked tab with dir change goes to root | chk | off | `C/LockedGoToDriveRoot=0` |
| Directory change in locked tab | rad | 1 | `C/LockedTabChange=` (0 new tab, 1 reuse existing tab with that dir, 2 first unlocked tab, 3 last unlocked tab) |
| Enter on locked tab | — | 1 | `C/TabEnterDirChange=` (0 nothing, 1 new tab, 2 ask) |
| Default tab file | — | `%COMMANDER_PATH%\main.tab`? | `C/TabDir=` |

Tabs themselves are stored in `[lefttabs]` / `[righttabs]` (`0_path=`, `0_options=`, `activetab=`, `0_caption=` for renamed tabs, `0_lockpath=`). That belongs to area 2 (tabs) but the keys must not collide with config.

**mormi:** store the checkboxes as separate named keys (`[Tabs] ShowSingle=1`, `ConfirmCloseAll=0`, ...). On import decode `DirTabOptions` bits. Close button on tabs: ours, default on (web users expect it). Last tab cannot be closed (F-030). Ctrl+W on the last tab does nothing, no message. With "confirm close all tabs" on, `cm_CloseAllTabs` asks "Close all tabs?" **[verify text]** with Yes/No.

### 3.9 Custom columns (`cm_CustomColumnConfig`) - F-433, F-665

```
+- Custom columns -------------------------------------------------------+
| Configure columns for file system:  [ General ▼ ]  (plugins add more)   |
| +-----------------------------------------------+  [ New... ]            |
| | Standard (built in, can't edit)               |  [ Copy... ]           |
| | 1  Detailed                                    |  [ Edit... ]           |
| | 2  Image info                                  |  [ Rename ]            |
| |                                                |  [ Delete ]            |
| +-----------------------------------------------+  [ Up ] [ Down ]       |
| (drag and drop to reorder)                                               |
| [x] Show custom column views in Show menu / column header context menu  |
+-------------------------------------------------------------------------+
```

Column sets are stored in `[CustomFields]`? No: in `[CustomColumns]`? TC uses sections `[Custom columns]`? Real names **[verify]**: `[ColumnView1]` ... with keys `Name=`, `Column1=[=tc.size]`, `Column1Width=`, `Column1Align=`, `Column2=...`, plus `[Columns]`/`[CustomFields]` for tooltips. Selected view per panel: `[left]`/`[right]` `SpecialView=2`.

Column editor (sub-dialog, also `cm_CustomColumnDlg`): table with columns Title | Width | Align (←/→) | Field contents, buttons "+" (add column), "−" (delete), "..." field chooser (popup of `[=tc.name]`, `[=tc.ext]`, `[=tc.size]` with units `.bytes/.k/.M/.G/.dynamic`, `[=tc.writedate]` with format, `[=tc.attributes]`, `[=tc.path]`, `[=tc.comment]`, plugin fields `[=plugin.field.unit]`), "Hide first column"? no: first column (name) is fixed. Options: "Use as tooltip", "Configure field formatting". OK/Cancel. Editing is live: the panel shows the result while the dialog is open.

**mormi:** fields list from F-665 (name, ext, size, date, attributes, permissions, owner, group, created, accessed, link target, comment, file type, MIME, image width/height, EXIF date, audio tags). Field syntax kept TC-compatible (`[=tc.size.dynamic]`) so import works; unknown plugin fields import as empty columns with a warning list after import.

### 3.10 Custom columns > View modes (TC 9.0+) - F-785, F-820, F-819

```
+- View modes -----------------------------------------------------------+
| +----------------------------------+  [ Add ] [ Copy ] [ Delete ]        |
| | Name          | Auto switch      |  [ Up ] [ Down ]                    |
| | Images        | *.jpg;*.png ...  |                                     |
| +----------------------------------+                                     |
| Name: [ Images                  ]  Hotkey: [            ]               |
| Columns: [ Thumbnails ▼ ]  (Brief / Full / custom view / thumbnails)    |
| Icons: [ default ▼ ]  Colors: [ default ▼ ]  Font: [ default ▼ ]       |
| Auto switch when:  [ Folder contains mostly files of type ▼ ] [*.jpg ]  |
|                    or path matches [                       ]           |
| [x] Enable auto switch mode                                              |
+------------------------------------------------------------------------+
```

INI: `[ViewModes]` with `1_name=`, `1_columns=`, `1_icons=`, `1_colors=`, `1_font=`, `1_auto=` **[verify exact names]**, and `C/ViewModesAuto=1`. Auto-switch rules are tried in list order. The first match wins, so Up/Down matter. Switching manually (Show > View modes, `cm_SrcViewModeList`) turns auto mode off for that panel until the folder changes **[verify]**.

### 3.11 Operation (`cm_Config2`) - F-419, F-135, F-136, F-797, F-122, F-735, F-376, F-750, F-146, F-051

```
+- Main program --------------------------------------------------------+
| [ ] Allow only 1 copy of Total Commander at a time                     |
| [ ] Minimize to system tray? (TC 11: "Show in tray: [ ] always [x] when minimized") |
| [ ] Always go to the root directory when changing drives               |
| [ ] Open files and directories with a single click (Web style)         |
| [x] Select only the file name when renaming (without extension)        |
| [x] Auto-complete paths in command line, copy dialog, etc.             |
|     [x] Auto-append suggested name                                      |
| [x] Rename on double click? no -> "In-place rename on click on cursor" |
+- Mouse selection mode -------------------------------------------------+
| (•) Right mouse button (Norton Commander style, context menu on long click)|
| ( ) Left mouse button (Windows standard)                                |
+- Selection with Gray+ / Ctrl+A -----------------------------------------+
| ( ) Select files only  (•) Also select directories                      |
+- Selection with Space ---------------------------------------------------+
| [x] Space moves cursor down? (TC: SpaceMovesDown) [verify on page]       |
| [x] Calculate space occupied by subdirectories when selecting with Space |
+- NTFS / time ---------------------------------------------------------+
| [ ] Ignore 1 hour time difference (daylight saving) when comparing      |
+- Save on exit ---------------------------------------------------------+
| [x] Directories  [x] Panels (Brief/Full, sort)  [x] Old command lines   |
| [x] Directory history                                                   |
+- File comments --------------------------------------------------------+
| Preferred: ( ) files.bbs (•) descript.ion  [ ] DOS charset              |
| [x] Copy comments with files  [x] Read both                             |
+- Calculate subdirectory sizes -----------------------------------------+
| [x] when selecting with space   [x] before copying/deleting (2nd bar)   |
+------------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| Allow only 1 copy | chk | off | `C/OnlyOnce=0` | F-735: second start activates first window, passes the path params to it |
| Minimize to tray | chk | off | `C/TrayIcon=0` (wiki: 1 in old TC) | F-750 N/A on macOS |
| Always go to root on drive change | chk | off | `C/AlwaysToRoot=0` | off = remember last dir per drive |
| Single click opens | chk | off | `C/SingleClickStart=0` | |
| Select only name when renaming | chk | on | `C/RenameSelOnlyName=1` | F-146: inline rename and F6 dialog |
| Auto-complete paths | chk | on | `C/AutoComplete=1` **[verify]** | |
| Auto-append suggested name | chk | on? | `C/AutoCompleteAppend` **[verify]** | |
| In-place rename on click | — | on | `C/InplaceRename=1` | click on the name under the cursor after a pause starts rename |
| Mouse selection mode | rad | • right button | `C/UseRightButton=1` (0 left/Windows) | F-135, F-136; right button mode: right click selects, right press-and-hold opens context menu (hold time ~ 0.5 s) |
| Gray+ selects directories | rad | • also directories? TC default: files only **[verify]** | `C/SelectDirs`? / `C/SelectFolders` **[verify]** | F-122 |
| Space moves down | — | on | `C/SpaceMovesDown=1`? (wiki default 0) **[verify]** | |
| Count space on Space | chk | on | `C/CountSpace=1` | F-797 |
| Count before copy/delete | chk | on | `C/CountMarked=1` | total progress bar |
| Ignore 1 hour difference | chk | off | `C/1hourdif=0` | |
| Save on exit: directories | chk | on | `C/Savepath=1` | |
| Save on exit: panels | chk | on | `C/Savepanels=1` | |
| Save on exit: command lines | chk | on | `C/SaveCommands=1` | |
| Save on exit: dir history | chk | on | `C/SaveHistory=1` | |
| Comments preferred file | rad | • descript.ion | `C/CopyComments` bit sum: 1 files.bbs, 2 descript.ion, 4 read both, 8 copy with files, 16 DOS charset; default 6? **[verify]** | |
| Enter on archive opens as folder | — | on | `P/ZIPlikeDirectory=1` | on the Packer page (3.22), not here; F-051 |
| Right panel active at start | — | off | `C/ActiveRight=0` | no UI |

Edge cases:
- Switching mouse mode applies at once. In right-button mode a quick right click selects/deselects. Holding the right button opens the context menu. Right-drag selects a range.
- "Allow only 1 copy" works through a named mutex on Windows. Command-line `/O` forces it for one start. `/N` forces a new instance.

**mormi:** F-735 single instance via our reexec/IPC (mormiview convention). F-376 option text "Allow only 1 copy at a time" kept. Tray N/A on macOS (F-750). Right-button selection works with a mouse. On a trackpad the "long press" is a two-finger click held for 0.5 s, which is awkward, so the default on macOS is **left button** (deviation, documented).

### 3.12 Edit/View (`cm_EditConfig`) - F-420, F-152, F-590, F-591

```
+- Viewer for F3 --------------------------------------------------------+
| (•) Internal viewer: file under cursor (F3)                              |
|     selected files (Shift+F3)                                           |
| ( ) External viewer (Alt+F3 = internal)                                 |
|     [ notepad.exe                         ] [ >> ]                      |
| [ Associations... ]  (per-type external viewers, [Extensions] section)  |
| [ Configure internal viewer... ]  (opens Edit/View > Lister)            |
+- Editor for F4 ---------------------------------------------------------+
| [ notepad.exe                             ] [ >> ]                      |
| [ ] Use the internal editor? (no: TC has none)                           |
+-------------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| F3 viewer mode | rad | • internal | `C/Viewertype=` 1 internal cursor file, 0 internal selected files?, 2 external **[verify mapping]** (wiki: 0 internal current, 1 marked, 2 external) | |
| External viewer path | edt+btn | empty / notepad.exe | `C/Viewer=` | `>>` opens file chooser (filter "Programs (*.exe;*.com;*.bat)") |
| Associations | btn | — | `[Extensions]` / `[Associations]`? | sub-dialog: list of `*.ext` -> program for Alt+F3 |
| Editor path | edt+btn | `notepad.exe` | `C/Editor=notepad.exe` | `%1` is not required; TC appends the quoted file name. Supports parameters after the exe name; `"path with spaces"` quoting |

Behaviour: if the editor path does not exist, F4 shows the Windows error "Windows cannot find 'xxx'" (CreateProcess error text) and nothing else. Viewer/editor paths accept environment variables (`%COMMANDER_PATH%`, `%ProgramFiles%`).

**mormi:** defaults per F-591: macOS `open -t` (TextEdit plain text), Linux `xdg-open`, Windows `notepad.exe`. Placeholder `%1` (file path, quoted for us). If missing, the path is appended. F-594 built-in editor is an extra radio "Built-in editor" (ours). Errors: "Could not start editor: <reason>" toast plus details.

### 3.13 Edit/View > Lister (TC 9.10+ page **[verify]**) - F-822, F-588

```
+- Lister (F3) ----------------------------------------------------------+
| Startup mode: [ Auto ▼ ]  (Auto, Text, Binary, Hex, Multimedia, HTML)   |
| Text: Character set [ ANSI (Windows) ▼ ]  [x] Auto-detect UTF-8/16      |
| Tab width: [ 8 ]   Text width (wrap): [ 80 ]  [x] Wrap lines            |
| Binary width: [ 75 ]   [x] Auto-detect RTF  [x] HTML as text with links |
| Images: [x] Always load images  [ ] Fit to window (shrink large)        |
|         [ ] Center   Background: [■]                                      |
| [x] Multimedia: always play sound/video                                  |
| Lines shown above found text: [ 3 ]                                     |
| Fonts: [ Change ANSI font ] [ Change ASCII font ] [ Variable font ]      |
| Plugins: [ ] Use plugins first   [ Configure Lister plugins... ]        |
+-------------------------------------------------------------------------+
```

| Label | Default | INI |
|---|---|---|
| Startup mode | Auto | `Li/Startup=0` (0 auto, 1 text, 2 binary, 3 hex; +16 ANSI, +32 ASCII, +48 variable) |
| Text width | 80 | `Li/TextWidth=80` |
| Binary width | 75 | `Li/binwidth=75` |
| Wrap | on | `Li/Wrap=1` |
| Tab width | 8 | `Li/TabWidth=8` |
| HTML as text | on | `Li/HTMLAsText=1`, `Li/LinkBraces=1` |
| Always load images | on | `Li/BmpStartup=1` |
| Multimedia | on | `Li/Multimedia=1` |
| Auto-detect RTF | on | `Li/RTF=1` |
| Fit image | off | `Li/ShrinkBitmap=0` |
| Lines above found text | 3 | `Li/SearchGoBack=3` |
| Fonts | Arial/Courier | `Li/Font1..Font3=Arial,-15,b`... (Font4/5 printer) |
| IrfanView path | empty | `Li/IView=1`, `Li/IViewPath=` (Windows-only, **mormi:** omit) |

Lister details belong to area 10 (viewer). Only the configuration surface is here.

### 3.14 Copy/Delete (`cm_CopyConfig`) - F-421, F-436, F-478, F-472, F-473, F-475, F-810, F-719

```
+- Copy ----------------------------------------------------------------+
| [ ] Use standard copy method only (Explorer/CopyFileEx)               |
| Small files buffer:  same disk [ 1024 ] kB  other disk [ 1024 ] kB? [verify]|
| [ ] Big file copy mode (no cache) above [ 10 ] MB? Buffer: same [10240] other [32] |
| Drives on the same physical disk: [ c,d,e                       ]     |
| Compatibility mode (slow, for faulty drivers): drives [   ]  [x] USB sticks |
| [ ] Copy+paste via Explorer (only for problems)                        |
| [x] Copy date/time of directories                                      |
| [x] Copy NTFS permissions? (TC 11: "Copy permissions (NTFS)" option in F5 dialog) |
| [ ] Verify after copy? (TC 10+: checkbox in F5 dialog, default here)   |
| Default for F5/F6: [ ] Always in background (queue)                     |
|                    Background transfer: (•) separate thread ( ) BTM      |
| [x] Check free space before copying                                     |
+- Delete ---------------------------------------------------------------+
| [x] F8/Del deletes to recycle bin (Shift+F8/Del: delete directly)        |
| [ ] Use Explorer delete method (can't handle comments)                  |
+------------------------------------------------------------------------+
```

| Label | Type | Default | INI | Notes |
|---|---|---|---|---|
| Use standard copy method only | chk | off | `C/ExplorerForCopy=0`? / `C/CopyMethod` **[verify]** | |
| Block size same disk | spn | 1024 kB (TC 9+) | `C/CopyBlockSize=1024` (min 32) | wiki default 32 is pre-TC 8 |
| Block size other disk | spn | 1024 kB? | `C/CopyBlockSizeOther=` **[verify default]** | |
| Big file copy mode | chk | off | `C/CopyHugeBlockSize=` empty = off; min 10240 kB | F-478 |
| Big file buffer other disk | spn | 32? MB | `C/CopyHugeBlockSizeOther=` | |
| Same physical disk drives | edt | empty | `C/SamePartitions=` (comma groups like `cd,ef`) | Windows only |
| Compatibility mode drives | edt | empty | `C/CopyCompatDrives=` (`*` = all) | |
| Compat for USB sticks | chk | on | `C/CopyCompatUsbSticks=1` | |
| Copy dir date/time | chk | on (TC 9+) | `C/CopyDirTimeStamp=1` | |
| Always copy in background | chk/rad | off | `C/AlwaysCopyInBackground=0` (1 background, 2 BTM queue) | F-436 |
| Check free space | — | on | `C/SizeCheck=1` | no UI in some versions |
| Copy NTFS streams | — | off | `C/CopyStreams=0` | F-475 |
| Delete to recycle bin | chk | on | `C/UseTrash=1` | F-810: when off, F8 deletes directly and Shift+F8 uses trash (swap) |
| Explorer delete method | chk | off? | `C/Win95Delete=-1` (0 TC, 1 Explorer, -1 auto) | |

Edge cases: the free space check shows "Not enough space on target drive! Continue anyway?" **[verify text]** before copying. Trash on a drive without a recycle bin (network, USB with FAT on some Windows) shows "Cannot move to recycle bin! Delete directly?" **[verify]**.

**mormi:** "Drives on same physical disk" and compat modes are N/A. Replace them with "Use system copy (clonefile on APFS / copy_file_range on Linux) when possible" (ours, default on). Verify after copy (F-472): default for the dialog checkbox, `C/VerifyCopy=0`. Copy permissions (F-473): `C/CopyPermissions=1` on Unix. Copy xattrs (F-475): `C/CopyXattrs=1` on macOS. Symlinks (F-476): `C/CopySymlinksAsLinks=1`. Trash: macOS Trash, freedesktop trash on Linux, recycle bin on Windows.

### 3.15 Refresh (`cm_RefreshConfig`) - F-422

```
+- On activation (switching to TC / clicking a panel) ------------------+
| No refresh for these drives (removable/net): [ AB\\        ]            |
| Don't remember these drives on exit:        [ AB\\        ]            |
+- On file system change --------------------------------------------- -+
| [x] Refresh on create, delete, rename                                    |
|     [x] Also on size, date, attribute change                             |
|     [ ] Add new files at the end of the list                             |
|     [x] Don't react while TC is in background? (TC 11: "Only when active") |
|     [x] Update count in footer   [x] Update free space in header         |
|     [ ] Beep on change? (bit 256, no UI) [verify]                        |
| No auto refresh for drives: [ AB\\  ]                                    |
+------------------------------------------------------------------------+
```

| Label | Default | INI |
|---|---|---|
| Refresh flags | 1+2+16+32 = 51? TC 11 default `WatchDirs=` **[verify]** | `C/WatchDirs` bits: 1 on, 2 size/date/attr, 4 new at end, 8 no updates when inactive, 16 footer, 32 header, 256 beep |
| No auto refresh for drives | `AB\\` (floppies, `\\` = network UNC) | `C/WatchDirsExclude=` |
| No refresh on activation | `AB\\`? | `C/Noreread=` |
| Don't remember on exit | `AB\\` | `C/Noreread2=` |
| Device change messages | on | `C/DeviceChange=1` (no UI) |

**mormi:** drive letters do not exist on macOS/Linux. Replace with an editable list of path prefixes (one per line) plus checkboxes "Network volumes" and "Removable volumes", keys `[Refresh] Exclude=` (`;` separated), `ExcludeNetwork=1`, `ExcludeRemovable=0`. Watching uses fsnotify (FSEvents/inotify/ReadDirectoryChangesW). The F2 manual reread always works.

### 3.16 Quick search (`cm_QuickSearchConfig`) - F-423, F-066, F-067, F-070, F-274

```
+- Quick search (current dir) ------------------------------------------+
| (•) Ctrl+Alt+Letters   ( ) Alt+Letters   ( ) Letter only                |
| ( ) Letters, with search dialog  ( ) No quick search                     |
+- Exact name match ----------------------------------------------------+
| [x] Beginning (name starts with typed text)                             |
| [ ] Ending (extension must match after typed .)                         |
+- Options (TC 7.5+) ----------------------------------------------------+
| [ ] Filter (show only matching files) - toggled with ... in search box  |
| [ ] Search for files and directories / only directories? [verify]       |
| [x] Show search box above? (no, always below the panel)                  |
+- Quick filter (TC 8+) ------------------------------------------------+
| [x] Quick filter remembers text when re-opened? [verify]                |
+------------------------------------------------------------------------+
```

| Label | Default | INI | Notes |
|---|---|---|---|
| Mode | Ctrl+Alt+letters | `C/AltSearch=0` (0 Ctrl+Alt, 1 Alt, 2 letter only, 3 letter with dialog, 4 off) | F-066, F-274 |
| Match beginning | on | `C/QuickSearchMatchBeginning=1` **[verify]** | off = match anywhere |
| Match ending | off | `C/QuickSearchExactMatch=0` | |
| Filter mode | off | `C/QuickSearchAutoFilter=0` | F-067 |

Behaviour (summary, full detail in area 3 quick search): in "letter only" mode the letters go to quick search instead of the command line, and the command line needs Ctrl+Enter or a click.

**mormi:** on macOS Ctrl+Option+letters is the TC default too, matched by KeyboardEvent.code (F-286). Our default stays TC's (Ctrl+Alt). The first-run wizard may suggest "Letter only" (ours).

### 3.17 FTP (`cm_FtpConfig`) - F-424, F-437, F-637, F-389, F-392

```
+- FTP ------------------------------------------------------------------+
| Retries if server busy: [ 10 ]  Delay between retries: [ 5 ] s           |
| Default transfer mode: (•) Automatic ( ) Binary ( ) Text                 |
|   Text mode for: [ *.txt *.htm *.html *.php ... ]                        |
| New connections in: (•) active panel ( ) left ( ) right                  |
| [ ] Log file: [ c:\ftp.log                ]                             |
| [ ] All transfers in background                                          |
| [x] Passive mode by default (PASV)                                       |
| [ ] Limit PORT mode to ports: [10000] : [11000]                          |
| [x] Compress during transfer (MODE Z) [x] not in LAN                     |
| Auto-resume if no data for [ 30 ] s   [x] Keep connection alive? [verify]|
| Anonymous password (e-mail): [ anonymous@...  ]                          |
| Default proxy: [ none ▼ ]  [ Define proxies... ]                         |
+--------------------------------------------------------------------------+
```

FTP settings live in **wcx_ftp.ini** (path `C/FtpIniName=` or `/F=` command-line), section `[General]`: `Retries=10`, `RetryDelay=5`, `TransferMode=` (`A`/`I`/`T`), `TextModeTypes=`, `NewConnInActive=`, `FtpLogFile=`, `BackgroundTransfer=0`, `PassiveMode=1`, `PortRange=`, `ModeZ=1`, `AutoResumeTime=30`, `Anonymous=`, `DefaultFirewall=` **[verify names]**. Proxies are `[Firewall1]`... sections. Connections are `[connections]` + one section per connection name.

**mormi:** our `ftp.ini` (F-445 `-f=` override). Same fields plus SFTP defaults (known_hosts path, key file). Network config details belong to area 13.

### 3.18 Plugins (`cm_PluginsConfig`) - F-425 (N/A), F-623

```
+- Plugins --------------------------------------------------------------+
| [ Download... ] (opens www.ghisler.com/plugins.htm)                      |
| Packer plugins (.WCX)      [ Configure ]                                 |
| File system plugins (.WFX) [ Configure ]                                 |
| Lister plugins (.WLX)      [ Configure ]                                 |
| Content plugins (.WDX)     [ Configure ]                                 |
| [x] Install plugins automatically when opening an archive with pluginst.inf |
| Plugin base directory: [ %COMMANDER_PATH%\Plugins ]                      |
+--------------------------------------------------------------------------+
```

INI: `[PackerPlugins]` (`ext=caps,path`), `[FileSystemPlugins]`, `[ListerPlugins]` (`0=path`, `0_detect=`), `[ContentPlugins]`, `[DisabledPlugins]`, `C/AutoInstallPlugins=1`, `C/PluginBaseDir=`.

**mormi:** N/A (Windows DLLs). The page becomes "External tools": a table of command-line helpers (name, executable, arguments with `%1`, file masks, kind: packer/viewer). See F-624. Import from TC: plugin sections are skipped and listed in the import report.

### 3.19 Thumbnails (`cm_ThumbnailsConfig`) - F-426, F-676, F-677, F-083

```
+- Thumbnails -----------------------------------------------------------+
| Database location: [ %$LOCAL_APPDATA%\GHISLER        ] [ Default ]      |
|   (empty = no caching)                                                  |
| [x] Copy/delete thumbnails with files                                    |
| Thumbnail size: [ 96 ] x [ 96 ] pixels                                   |
| Custom fields below thumbnails: [ + ] [ Fields by type... ]              |
| Load thumbnails via:                                                     |
|   [x] Lister plugins for [ *.*                 ]                         |
|   [x] Explorer (OLE2) for [ *.* | *.htm *.html ]                         |
|   [x] Irfanview/XnView for [ *.*  ]                                      |
|   [x] First lines of text for [ *.txt *.ini ]                            |
| Compact database: [x] missing files [ ] ejected CDs [ ] inaccessible [Start] |
+--------------------------------------------------------------------------+
```

| Label | Default | INI |
|---|---|---|
| Database location | `%$LOCAL_APPDATA%\GHISLER` | `C/ThumbsLocation=` |
| Copy/delete with files | on | `C/ThumbsCopyDel=1` |
| Size | 96×96 | `C/ThumbWidthNoCache=96`, `ThumbHeightNoCache=96` (TC 11 also per-resolution `[<res>]ThumbWidth` **[verify]**) |
| Custom fields | off | `C/ThumbsCustomFieldsEnabled=` |
| Sources bitmask | 15 | `C/ThumbOptions=15` (1 Explorer, 2 Lister plugins, 4 IrfanView, 8 text) |
| Masks | as shown | `ThumbExplTypes`, `ThumbPlgTypes`, `ThumbIrfXnTypes`, `ThumbTxtTypes` |
| Preload | 1 | `C/PreloadThumbnails=1` (no UI) |
| Memory limit | 500 | `C/ThumbnailMemoryLimit=` (no UI) |

"Start" (compact) runs in the dialog with a progress label and then shows "Removed N entries" **[verify text]**.

**mormi:** cache in `<configdir>/thumbs` (default), size slider 32..512 (F-677), sources: built-in image decoders, macOS QuickLook, first lines of text. Compact = "Clean cache now".

### 3.20 Log file (`cm_LogConfig`) - F-427, F-717

```
+- Log file -------------------------------------------------------------+
| [ ] Enable log file:  [ %COMMANDER_PATH%\totalcmd.log        ] [ >> ]   |
+- Log operations ------------------------------------------------------+
| [x] Copy/Move/Create shortcut   [x] Delete                              |
| [x] Create/delete folder         [x] Pack/Unpack/Test                    |
| [x] File system plugins                                                 |
+- Log status -----------------------------------------------------------+
| [x] Successful operations  [x] Errors  [x] Skipped files                |
+- Rotation (TC 9+) -----------------------------------------------------+
| (•) Never  ( ) Daily after midnight  ( ) when larger than [ 1024 ] kB   |
|     Keep max. [ 10 ] copies                                              |
| [ ] Include date in file name? (%DATE% placeholder) [verify]             |
+--------------------------------------------------------------------------+
```

| Label | Default | INI |
|---|---|---|
| Enable log | off | `C/LogFile=` path; `C/LogOptions=` bit sum (enable + ops + statuses) **[verify bits]** |
| Rotation | never | `C/LogRotateLimit=0` (0 never, -1 daily, >0 kB), `C/LogKeepCount=10` **[verify names]** |

Log line format (TC): `dd.mm.yyyy hh:mm:ss: Copy (OK): C:\a.txt -> D:\a.txt`, errors `... Copy (Error): ...`, skipped `(Skipped)`. Environment variables allowed in the path. If the folder does not exist it is created. If the file cannot be opened, the operation still runs and a one-time error appears **[verify]**.

**mormi:** same line format with ISO dates (`2026-11-13 14:02:11`), default path `<configdir>/mormicommander.log`.

### 3.21 Ignore list (`cm_IgnoreConfig`) - F-428

```
+- Ignore list ---------------------------------------------------------+
| [ ] Ignore (don't show) the following files/folders:                    |
| +-------------------------------------------------------------------+   |
| | desktop.ini                                                       |   |
| | thumbs.db                                                         |   |
| | C:\pagefile.sys                                                   |   |
| +-------------------------------------------------------------------+   |
| Save list in: [ %COMMANDER_PATH%\tcignore.txt        ] [ >> ]           |
| [ Add selected with path ]  [ Add selected names ]                      |
+--------------------------------------------------------------------------+
```

| Label | Default | INI |
|---|---|---|
| Enable | off | `C/IgnoreListFileEnabled=0` |
| File | `%COMMANDER_PATH%\tcignore.txt` | `C/IgnoreListFile=` |

List syntax: one entry per line. A bare name or wildcard (`*.bak`) hides matching names in every folder. An entry with a path hides that exact item. A trailing `\` means folders only. Lines starting with `;` are comments. The list hides items from panels, but operations on a folder still include ignored files inside it **[verify]**.

**mormi:** `<configdir>/ignore.txt`, `/` paths, same rules. Also wire `cm_SwitchIgnoreList` to toggle (F-428).

### 3.22 Packer (`cm_PackerConfig`) - F-429, F-624, F-051, F-597

```
+- Packer --------------------------------------------------------------+
| [x] Treat archives like directories (Enter opens, Ctrl+PgDn always)     |
| ARJ: [ arj.exe   ] [x] Use internal un-ARJ  [ ] pass long filenames     |
| LHA: [ lha.exe   ] [x] Use internal un-LZH                              |
| RAR: [ rar.exe   ] [x] Use internal un-RAR                              |
| UC2: [ uc.exe    ]                                                       |
| ACE: [ ace32.exe ] [x] Use internal un-ACE                              |
| TAR: [x] Create TAR for Linux (unchecked: SunOS)                        |
| [ ] Pack in background by default  (TC: AlwaysPackInBackground)          |
| [ Configure packer extension WCXs ]                                     |
+--------------------------------------------------------------------------+
```

INI `[Packer]`: `ZIPlikeDirectory=1`, `ARJ=`, `LHA=`, `RAR=`, `UC2=`, `ACE=`, `InternalUnarj=1`, `InternalUnlzh=1`, `InternalUnrar=1`, `InternalUnace=1`, `ARJlongnames=0`, `LinuxCompatible=1`, `ExpertMode=0`, `C/AlwaysPackInBackground=0` (1 zip in bg, 2 also external, 3 minimized).

**mormi:** replace legacy packers with "External 7z/rar path" fields (F-624), keep "Treat archives like directories" and the TAR flag (always Linux-compatible, so the checkbox is gone). Formats: zip, tar, tgz, tbz, txz, gz, bz2, xz, 7z, zstd (F-598).

### 3.23 Packer > Zip packer (`cm_ZipPackerConfig`) - F-430

```
+- Zip packer ----------------------------------------------------------+
| ZIP packer: [ pkzip.exe ]  [x] Use internal ZIP packer                   |
| ZIP unpacker: [ pkunzip.exe ] [x] Always use internal unzip              |
| Compression: (•) Normal (6) ( ) Maximum (9) ( ) Fastest (1) ( ) Other [ ]|
| Temp dir: (•) default (TEMP) ( ) other [            ]                    |
| [ ] Use 8.3 names  [ ] Open partial ZIP files  [ ] Set ZIP date to newest|
| [ ] Keep damaged files (bad CRC)   [x] Use UTF-8 names (TC 7.5+)        |
| [x] Test archive after packing? (VerifyZIP)                              |
+--------------------------------------------------------------------------+
```

INI `[Packer]`: `ZIP=`, `UnZIP=`, `InternalZip=1`, `InternalUnzip=1`, `InternalZipRate=6` (0..9), `InternalzipTemp=`, `Zip83Name=0`, `OpenPartial=0`, `ZipSetDateToNewest=0`, `NoDelete=0`, `VerifyZIP=1`, `ZipUtf8=`? **[verify]**, `ZIPDirectoryOfFile=1` (pack paths), `ZIPRecurseSubdirs=1`, `UnZIPOverwrite=0`, `ExtractPathInZIP=1`.

**mormi:** internal only. Compression level 0-9 (default 6), UTF-8 names always on (flag set), set file times, temp dir. Pack/unpack dialog checkboxes take their defaults from here.

### 3.24 Misc. (`cm_Confirmation`) - F-431, F-282, F-719, F-017, F-747, F-771, F-748

```
+- Confirmation ---------------------------------------------------------+
| Confirm before:                                                          |
| [x] Deleting non-empty directories                                       |
| [x] Overwriting files                                                    |
| [x] Overwriting/deleting read-only files                                 |
| [x] Overwriting/deleting hidden/system files                             |
| [x] Drag & drop copying/moving                                           |
| [ ] Closing TC while background operations run? (always asked)           |
+- Redefine hotkeys (Keyboard remapping) --------------------------------+
| [ ] Ctrl  [ ] Alt  [ ] Shift  [ ] Win    Key: [ F1          ▼ ]          |
|   Current: cm_HelpIndex (from TC)                                        |
| Command: [ cm_xxx                         ] [ >> ] (command browser)     |
|   [ ✓ ] assign    [ ✗ ] remove    [ Help ] on command                    |
| [ ] Use [ Win ] key? / [ ] Restore default                               |
+- Custom sounds --------------------------------------------------------+
| [ Configure... ] (Windows sound control panel)  Only for operations      |
| longer than [ 10 ] s                                                      |
+- Window (TC 10+) ------------------------------------------------------+
| [ ] Alt+F4 / close button minimizes instead of exit? (MinimizeOnClose)  |
| Title: show path? [verify]                                               |
+--------------------------------------------------------------------------+
```

| Label | Default | INI | Notes |
|---|---|---|---|
| Confirm delete non-empty dir | on | `Cf/deleteDirs=1` | F-495 |
| Confirm overwrite | on | `Cf/OverwriteFiles=1` | F-463 |
| Confirm read-only | on | `Cf/OverwriteReadonly=1` | |
| Confirm hidden/system | on | `Cf/OverwriteHidSys=1` | |
| Confirm drag & drop | on | `Cf/MouseActions=1` (wiki shows 0) **[verify]** | F-480 |
| Hotkey modifier checkboxes + key | chk×4 + cmb | none | writes `[Shortcuts]` (`C+F1=cm_...`) and `[ShortcutsWin]` for Win key combos | F-282 |
| Command | edt+btn | — | value of the key entry; cm_ names, em_ user commands, or numeric ids | `>>` opens the command browser (cm_CommandBrowser, categories on the left, filter field, Ctrl+C copies name) |
| Sound threshold | 10 s | `C/SoundDelay=10` | F-747; `-N` means also play when minimized? **[verify]** |
| Minimize on close | off | `C/MinimizeOnClose=0` **[verify]** | F-771 |

Hotkey remapping behaviour (F-282):
1. Pick modifiers and key. The label under it shows what that combination does now: "(not assigned)", the TC default action ("default: Copy files"), or the user command.
2. Type or browse a command, press ✓. If the combination is already used by another user-defined key, TC asks "Hotkey already assigned to: <command>. Overwrite?" **[verify text]** Yes/No.
3. ✗ removes the user definition. The TC default (if any) comes back.
4. Keys in the list: F1-F12, A-Z, 0-9, Num 0-9, Num +-*/, Enter, Space, Backspace, Tab, Insert, Delete, Home, End, PgUp, PgDn, cursor keys, `` ` ``, `-`, `=`, `[`, `]`, `;`, `'`, `,`, `.`, `/`, `\`.
5. `[Shortcuts]` key syntax: modifier letters `C` (Ctrl), `A` (Alt), `S` (Shift) in that order + `+` + key name: `CS+F5=cm_PackFiles`, `A+X=em_myCommand`, `F1=cm_HelpIndex`. Win combos go to `[ShortcutsWin]` with the same syntax. A command may carry parameters after a space in TC 9.10+ **[verify]**.
6. Hotkeys assigned to buttons in the button bar and start menu entries are stored with those items, not here.

**mormi:** F-282 adds "Restore defaults" (wipes our `[Shortcuts]`), conflict warning showing both commands, a searchable full list of current bindings (TC 11 has no such list on this page; ours). On macOS a Cmd checkbox replaces Win and stores under `[ShortcutsCmd]` (ours) so Windows imports stay clean. Key matching by `KeyboardEvent.code` (F-286).
Also on Misc for mormi: "Window title shows current path" (F-017, `C/TitlePath=0`), "Always on top" (F-748, ours, `C/AlwaysOnTop=0`), "Check for updates" link (F-454, manual only).

---

## 4. wincmd.ini format reference (F-443, F-444, F-445, F-446, F-733, F-853)

### 4.1 Location rules (TC)

1. Command-line `/I=path\file.ini` (and `/F=` for wcx_ftp.ini) wins. A relative path is relative to TC's folder. `/I=` with a name only ("myini.ini") looks in the TC folder.
2. Else registry `HKCU\Software\Ghisler\Total Commander\IniFileName` / `FtpIniName` (set by the installer).
3. Else `%APPDATA%\GHISLER\wincmd.ini` (TC 7.5+ installer default) or `wincmd.ini` next to totalcmd.exe if `UseIniInProgramDir` is set there.
4. `[Configuration] UseIniInProgramDir=` bit sum in the ini next to the exe: 1 use wincmd.ini from program dir, 2 use wcx_ftp.ini from program dir, 4 use the ini in program dir even if registry points elsewhere. This is TC's portable mode (F-444).
5. `RedirectSection=` inside a section: `[Configuration] RedirectSection=%COMMANDER_PATH%\shared.ini` makes TC read and write that whole section from another file. Used to share settings between users. `AlternateUserIni=` in `[Configuration]` points every section to a user file when the main one is shared/read-only.
6. 32-bit and 64-bit TC share one wincmd.ini. Sections can be split with `RedirectSection=` using `%PROCESSOR_ARCHITECTURE%`.

Environment variables TC sets for child programs and for its own paths: `COMMANDER_PATH` (folder of totalcmd.exe), `COMMANDER_INI` (full ini path), `COMMANDER_EXE` (TC 9+). Paths in the ini may use `%VAR%` and `%$CSIDL%` style names (`%$LOCAL_APPDATA%`, `%$DESKTOP%`).

**mormi** (F-443 to F-445, F-733, F-853):
- default `os.UserConfigDir()/mormiCommander/mormicmd.ini` (configdir.go convention); FTP connections in `ftp.ini` next to it.
- portable: a writable `mormicmd.ini` next to the binary (on macOS: next to the .app bundle) wins.
- `-i=<path>` / `/i=` and `-f=` / `/f=` override; both prefixes accepted on all platforms.
- children get `COMMANDER_PATH`, `COMMANDER_INI` (F-853) plus `MORMI_*` aliases.
- `RedirectSection` supported for reading (read-only); writes go to the main file. Simple, and enough for shared defaults.

### 4.2 Encoding and syntax

- TC ≤ 8: ANSI (system code page). TC 9+: if the file starts with a UTF-16 LE BOM, it is Unicode. TC writes a new file as UTF-16 **only** if a non-ANSI character must be saved **[verify]**. Otherwise it keeps the existing encoding.
- Lines `[Section]`, `Key=Value`. Keys are case-insensitive. Duplicates: the first wins (Windows `GetPrivateProfileString`). Comments: lines starting with `;`. There are no inline comments. Value whitespace is trimmed at both ends. Quotes are kept as part of the value.
- Booleans are `0`/`1`. Colours are decimal BGR ints. Lists are numbered keys (`1=`, `2=` or `menu1=`, `cmd1=`).
- TC writes the file through Windows profile APIs, so unknown sections and keys written by hand survive TC's saves.

**mormi:** the reader accepts UTF-8 (with or without BOM), UTF-16 LE/BE with BOM, and ANSI (decoded as Windows-1252, or the system code page given on import). Writes are always UTF-8 without BOM. We keep unknown keys and their order (round-trip), write atomically (temp file + rename), keep the last good copy as `mormicmd.ini.bak`. A parse error never loses the file: we load what we can and report "Line N: ignored" in the log.

### 4.3 Sections (TC 11, the ones the Options dialog and its siblings touch)

| Section | Content | Area |
|---|---|---|
| `[Configuration]` | almost all page options (sections 3.x) | 7 |
| `[Layout]` | Layout page checkboxes | 7 |
| `[Colors]` | Color page + colour filters | 7 |
| `[Confirmation]` | Misc page confirmations | 7 |
| `[Packer]` | Packer + Zip packer | 7 |
| `[Lister]` | Lister options | 10 |
| `[Shortcuts]`, `[ShortcutsWin]` | user hotkeys | 7 |
| `[<W>x<H> (<fw>x<fh>)]`, `[AllResolutions]` | fonts, icon size, tabstops, window position (`x=`, `y=`, `dx=`, `dy=`, `Divider=`, `maximized=`) | 1, 7 |
| `[left]`, `[right]` | per-panel view: `path=`, `ShowAllDetails=`, `SpecialView=`, `sortorder=`, `negative Sortorder=`, `show=`, `Tree=` | 1 |
| `[lefttabs]`, `[righttabs]` | tabs | 2 |
| `[LeftHistory]`, `[RightHistory]`, `[Command line history]`, `[SearchName]`, `[SearchText]`, `[MkDirHistory]`, `[Selection]`, `[RenameTemplates]`... | histories | various |
| `[DirMenu]` | hotlist (`menu1=`, `cmd1=cd ...`, `path1=`) | 12 |
| `[user]` | Start menu (`menu1=`, `cmd1=`, `param1=`, `path1=`, `key1=`) | 18 |
| `[Buttonbar]` | `Buttonbar=path.bar`, `Buttonheight=`, `FlatIcons=`, `SmallIcons=`, `SmallIconSize=`, `XPstyle=` | 15 |
| `[Extensions]`, `[Associations]` | Alt+F3 viewers, internal associations | 15 |
| `[CustomFields]`, `[CustomColumns]`?, `[ViewModes]` | tooltips, column sets, view modes | 7 |
| `[PackerPlugins]`, `[FileSystemPlugins]`, `[ListerPlugins]`, `[ContentPlugins]`, `[DisabledPlugins]` | plugins | N/A |
| `[Favorites]` / `[FavoriteTabs]`? | favourite tab sets (the set files are `.tab` in the config folder) **[verify]** | 2 |
| `[Sync]`, `[Searches]`, `[RenameTemplates]` | saved tool settings | 8, 9 |

### 4.4 Save points

- On OK/Apply of the Options dialog: the changed keys.
- On exit: if Save on exit is on, paths, panel modes, tabs, histories (`Savepath`, `Savepanels`, `SaveCommands`, `SaveHistory`). Window position is **not** saved on exit unless `C/SavePos`? no: TC saves position only via cm_ConfigSavePos (and on exit if `[Configuration] SavePosOnExit=1`? **[verify]**). This is why TC users press "Save position" once.
- `cm_ConfigSavePos` (F-439): writes window rect, maximized flag, panel divider position to the resolution section. No message.
- `cm_ConfigSaveSettings` (F-440): writes everything that would be written on exit (paths, tabs, histories, panel modes) now. No message.
- `cm_ConfigSaveDirHistory`: writes the folder histories only.
- `cm_ConfigChangeIniFiles` (F-826): opens wincmd.ini and wcx_ftp.ini in the F4 editor. TC 9.10+ watches nothing: changes take effect after restart, and TC overwrites hand edits of keys it holds in memory on exit. **[verify]** whether TC 11 shows a hint. **mormi:** we save first, open the file, watch it, and on change reload (or say "Restart needed for: …" for the few keys that need it).

**mormi:** F-447 export/back up: Configuration > "Export settings..." writes a copy of the ini (and ftp.ini, ignore list, button bars) into a .zip; "Import settings..." restores it after a confirmation "Replace current settings?". F-446 import of TC's wincmd.ini: user picks the file; we map known keys (this document's tables are the map), convert colours, resolution-section fonts and sizes, decode `DirTabOptions`, translate `C:\` paths only where they exist (others kept but flagged), and show an import report: imported / converted / skipped (with reason, e.g. "plugin", "Windows-only").

---

## 5. Sub-dialogs opened from Options pages

### 5.1 Command browser (`cm_CommandBrowser`) - F-282, F-305
Two panes: categories (All, File, Mark, Command, Net, Show, Config, Window, Help, Tabs, Misc, user em_ commands) and the command list (name, id, hotkey, description). Filter field on top matches names and descriptions. OK returns the selected name into the caller's field. Double click = OK. Ctrl+C copies the name. Also opened from the button bar editor and start menu editor.

### 5.2 Font chooser
System chooser in TC (3.4 Display > Font). **mormi:** ours, see 3.4.

### 5.3 Define colors by file type (F-097, F-435)
List of entries "Search template -> colour", each showing a sample text in that colour. Buttons: Add..., Delete, Edit..., Up/Down (first match wins, so order matters), Default (clear all). Add/Edit: "Define search template" combobox (saved searches from Find files, or "Define..." which opens the Find files dialog in template mode with name masks, attributes, size, date, content and plugin conditions), "Foreground color" and "Color for selected files" pickers. INI: `[Colors] ColorFilter1=>templatename` or `*.exe`, `ColorFilter1Color=`, `ColorFilter1ColorSel=`? **[verify]**; enabled by `C/ColorFilters=1`. Templates live in `[Searches]` (`name_SearchFor=`, `name_SearchIn=`, `name_SearchText=`, `name_SearchFlags=`).

### 5.4 Custom tooltip fields by type (F-789, F-821)
List: file type mask (`*.jpg;*.png`) -> field string (`[=tc.size.bytes]\n[=tc.writedate]\n[=imageinfo.size]`). Buttons Add/Delete/Up/Down, field chooser ("+" popup of fields). Option "Show default tooltip below custom fields" **[verify]**. INI `[CustomFields]` `Fields1=`, `Masks1=`, `Options1=` **[verify]**.

### 5.5 Associations (Edit/View > Associations)
Table: extension mask -> program for Alt+F3 / internal open. INI `[Extensions]`: `txt=c:\tools\viewer.exe "%1"`. **mormi:** internal associations (F-322 replacement).

### 5.6 Change button bar (`cm_ButtonConfig`) and Change Start menu (`cm_ChangeStartMenu`)
Not part of the Options dialog. Specified in area 15/18. Listed here only because they are on the Configuration menu (F-441, F-442).

---

## 6. Hotkeys and mouse inside the dialog (summary)

| Input | Effect |
|---|---|
| Enter | OK (except in multi-line lists, where it edits/adds) |
| Esc | Cancel |
| F1 | help for current page |
| Tab / Shift+Tab | next / previous control (tree is first) |
| Up/Down in tree | switch page immediately |
| Home/End in tree | first/last page |
| Alt+letter | control with that underlined letter on the current page |
| Ctrl+Tab / Ctrl+Shift+Tab | next / previous page **[verify]**, mormi: yes |
| Space on checkbox | toggle |
| Double click on colour swatch | colour picker |
| Drag in list (custom columns, view modes, colour filters) | reorder |
| Double click on list entry | Edit... |
| Del in list | Delete (with "Delete entry?" confirmation for column sets and view modes **[verify]**) |
| mormi: Ctrl+F / Cmd+F | focus the options search box (F-827) |
| mormi: Cmd+[ / Cmd+] (macOS) | previous / next page |

---

## 7. Edge cases (cross-page)

1. Opening Options while a background copy runs is allowed. Changing Copy/Delete buffer sizes affects only operations started later.
2. Options dialog is modal, so hotkeys of the main window do nothing while it is open. Background refresh still runs.
3. Two TC instances share one wincmd.ini. The last one to save wins per key. TC does not merge. **mormi:** same, but before writing we re-read the file and only replace keys we changed in this session (avoids clobbering the other instance's unrelated changes).
4. Values out of range in the ini (hand edited) are clamped on load to the nearest valid value. A non-numeric value falls back to the default. No message.
5. Unknown values in a radio-mapped key (for example `AltSearch=9`) select no radio button. Saving keeps the original value unless the user picks one. **mormi:** same (round-trip).
6. A path option pointing to a missing folder (thumbnails, log, ignore list) is accepted. The folder is created on first use. An error appears only if creation fails.
7. Dialog font change: affects dialogs opened afterwards, not the open Options dialog.
8. High DPI: tabstops and sizes stored per resolution section. Moving between monitors with different DPI can pick another section. **mormi:** CSS pixels, one value.

---

## 8. FEATURES.md mapping

| ID | Where in this spec |
|---|---|
| F-412 | 1, 2 |
| F-413, F-022, F-024, F-060 | 3.1 |
| F-003 | 3.1 (PanelsVertical) |
| F-414, F-090, F-098, F-102, F-106, F-115, F-116 | 3.2 |
| F-092, F-093, F-094 | 3.7 |
| F-415 | 3.3 |
| F-416, F-020 | 3.4 |
| F-417, F-096, F-097, F-435, F-021 | 3.5, 5.3 |
| F-418, F-828 | 3.6 |
| F-419, F-122, F-135, F-136, F-146, F-376, F-735, F-750, F-797 | 3.11 |
| F-420, F-152, F-590, F-591, F-594 | 3.12 |
| F-822, F-588 | 3.13 |
| F-421, F-436, F-472, F-473, F-475, F-476, F-478, F-810 | 3.14 |
| F-422 | 3.15 |
| F-423, F-066, F-067, F-070, F-274, F-286 | 3.16 |
| F-424, F-437, F-389, F-637 | 3.17 |
| F-425 (N/A), F-623, F-624 | 3.18, 3.22 |
| F-426, F-676, F-677 | 3.19 |
| F-427, F-717 | 3.20 |
| F-428 | 3.21 |
| F-429, F-051, F-597 | 3.22 |
| F-430 | 3.23 |
| F-431, F-282, F-719, F-463, F-495, F-480, F-747, F-771, F-748, F-017 | 3.24 |
| F-432, F-030, F-036, F-041, F-046, F-028 | 3.8 |
| F-433, F-665 | 3.9 |
| F-434, F-775 | 3.8 (Favorite tabs button) |
| F-438 | hotlist settings: `C/DirMenuShowPaths`? lives in the hotlist dialog (area 12) **[verify]** |
| F-439, F-440, F-826 | 4.4 |
| F-441, F-442 | 5.6 |
| F-443, F-444, F-445, F-733, F-853 | 4.1, 4.2 |
| F-446, F-447 | 4.4 |
| F-785, F-819, F-820 | 3.10 |
| F-789, F-821 | 3.2, 5.4 |
| F-823 | 3.11 (mouse), 3.24 (drag confirm), mormi page in 2.1 |
| F-824 | 3.1 (separate tree), 3.2 (tree group) |
| F-825 | Find files defaults: TC has no Options page; defaults come from the last search and saved templates. mormi page (ours) |
| F-827 | 2.2 (mormi search box) |

---

## 9. Open items to check against a TC 11 install

Collected **[verify]** points that change implementation, in priority order:

1. Exact TC 11 tree (is Tabstops still a page; is Lister a child of Edit/View; is there a Tooltips page).
2. `DirTabOptions` bit values and the TC 11 default sum.
3. `SortUpper` values and which one is the TC 11 default.
4. Dark mode keys and values (`DarkMode`, `*Dark` colour keys).
5. Default of `InverseCursor`, `DriveCombo`, `UseRightButton` on a fresh TC 11 install.
6. `Showicons` value mapping.
7. `LogOptions` bits and rotation keys.
8. Exact message texts quoted with **[verify]**.
9. Page command numeric ids (names are reliable).
10. Window position save-on-exit behaviour.
