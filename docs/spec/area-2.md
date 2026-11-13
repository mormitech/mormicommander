# Spec area 2: panels, views, sorting, tabs, quick search and quick filter

This document describes, as precisely as we can, how real Total Commander 11 (TC) behaves in the areas a user touches most: the two file panels, the ways a panel can show its contents (brief, full, tree, thumbnails, comments, custom columns, quick view, branch view), how the list is sorted, how folder tabs work, and how quick search and quick filter behave. It exists so that the UI agents can build mormiCommander's panels without having to own a Windows machine with TC on it, and so that reviewers have one place to check "does it behave like TC?". Every rule is written as an implementable statement, and every section names the FEATURES.md IDs it backs, so the checklist and this spec stay linked.

How to read the markers used below. A rule with no marker is well-established TC behaviour (documented in the TC help, the TC wiki, or long-standing and widely discussed on the ghisler.ch forum). A rule marked **(verify)** is our best recollection of TC 11 and must be checked on a real TC 11 before it is treated as a hard requirement; if it cannot be checked, implement it as written, because it is still the most likely behaviour. A rule marked **(ours)** is a deliberate mormiCommander choice that differs from or extends TC, usually because of macOS or Linux. Dialog labels are given in TC's English wording; `&` marks the accelerator letter where we know it. Exact pixel sizes are never required: match the layout and order, not the pixels.

Sources used: the TC 11 help file (Configuration pages, keyboard list, menu reference), the TotalcmdWiki pages "Wincmd.ini" and "Ini-settings" (ghisler.ch/wiki), ghisler.ch forum threads on quick search, quick filter, `QuickSearchAutoFilter` and `QuickSearchExactMatch`, and hands-on knowledge of TC 9.x to 11.x.

---

## 0. Terms used in this spec

- **Panel**: one of the two file lists (left, right). Exactly one is the **source** (active) panel; the other is the **target** panel.
- **Cursor**: the highlighted row (bar or frame) in a panel. It is not the same as the selection.
- **Selection / marked files**: files flagged for an operation, drawn in the selection colour (TC default: red text). Selection rules belong to area 3 except where views and filters interact with it (section 9 here).
- **Entry**: one row: a file, a folder, or the `..` parent entry.
- **Current directory bar** (TC: "current dir", `[Layout] CurDir`): the path line above the column header of each panel.
- **Tab header** (TC: `[Layout] TabHeader`): the sort header with column titles (Name, Ext, Size, Date, Attr) in Full view. Not to be confused with **folder tabs** (`[Layout] DirectoryTabs`).
- **Drive info line**: the line above the current dir bar showing `[volume_label]  12,345,678 k of 99,999,999 k free`.
- **Status bar** (`[Layout] StatusBar`): the line below each panel showing selection totals.
- **View**: how entries are laid out (brief, full, thumbnails, comments, custom columns, tree, quick view).
- **Filter**: which entries are listed (all files, programs, user-defined wildcards, quick filter, ignore list, hidden/system).

---

## 1. The panel: structure, focus and layout (F-001, F-002, F-003, F-004, F-005, F-006, F-007, F-008, F-009, F-011, F-014, F-015, F-016, F-022, F-025, F-026, F-766, F-767, F-769)

### 1.1 Vertical stacking of one panel, top to bottom

Each panel is a column made of these strips, each individually switchable in Configuration > Layout and by its `cm_Vis*` command:

1. Drive button bar (one shared bar above both panels, or one per panel when "Show two drive button bars" is on; `[Layout] DriveBar1`, `DriveBar2`, `DriveBarFlat`).
2. Drive combobox + drive info line (`[Layout] DriveCombo`; the info line itself is always shown when the combobox row is shown). The combobox shows the current drive letter with icon; next to it the info text `[label]  free k of total k free`. Clicking the info text opens the drive list too **(verify)**. On macOS/Linux **(ours)** the "drive" is the mounted volume containing the current path; label = volume name; numbers come from `internal/disk`.
3. Folder tabs row (`[Layout] DirectoryTabs` / `cm_VisDirTabs`; hidden when only one tab exists unless the folder-tabs option "Show tab header also when there is only one tab" is on, see 6.8).
4. Current directory bar (`[Layout] CurDir`, `cm_VisCurDir`). Shows the full path followed by the active filter mask, e.g. `c:\Windows\*.*`. With breadcrumbs on (`cm_VisBreadCrumbs`, TC 10+), path segments are clickable. At its right end: the history button (`*`, opens directory history) and hotlist button (opens Ctrl+D hotlist) when `[Layout] HistoryHotlistButtons=1`.
5. Sort header / column titles (`[Layout] TabHeader`, `cm_VisTabHeader`). Shown in Full, Comments and Custom columns views; in Brief and Thumbnails views it shows the sort buttons `Name`, `Ext`, `Size`, `Date` side by side without widths **(verify: TC shows the header also in Brief view as a row of sort buttons)**.
6. File list.
7. Status bar (`[Layout] StatusBar`, `cm_VisStatusbar`).

Below both panels: command line (`[Layout] CmdLine`) and the function key button row (`[Layout] KeyButtons`). Above both: menu, button bar (`[Layout] ButtonBar`), optional second button bar (`ButtonBar2`, TC 10+). Between the panels: optional vertical button bar (TC 9+, `cm_VisButtonbar2` controls the second/vertical bar **(verify)**).

### 1.2 Active panel

- Exactly one panel is active. Its current directory bar is drawn in the "active" highlight colour (Windows title-bar colour by default); the inactive panel's bar is grey. Its cursor bar is filled; the inactive panel's cursor is drawn as a frame (dotted/thin rectangle) or not at all, depending on Configuration > Colors > "Cursor: frame in inactive panel" **(verify)**.
- Clicking anywhere in a panel (list, header, path bar, status bar, tab) makes it active.
- `Tab` switches the active panel. Each panel keeps its own cursor row, scroll position and selection. `Tab` does nothing else (it does not move between tabs or controls).
- `Shift+Tab`: with a separate tree visible, moves focus between the file list and the tree of the same side (F-769). Without a separate tree it behaves like `Tab` **(verify)**.
- `Ctrl+I` is not a TC default for focus; TC 11 uses `cm_FocusTrg` unbound. FEATURES assigns Ctrl+I (F-243) **(ours)**.
- When the command line has focus, typing goes there, but cursor keys still move the panel cursor (see 10.4). `Esc` clears the command line.
- `cm_FocusLeft`, `cm_FocusRight`, `cm_FocusSrc` (= current), `cm_FocusTrg` (= other) set focus explicitly (F-766).

### 1.3 Arrangement and splitter

- Default arrangement: side by side. `cm_HorizontalPanels` (menu Show > Horizontal arrangement) stacks them top/bottom; the command is a toggle and the menu shows a check mark. Saved as `[Configuration] PanelsVertical=0/1` (the ini name is historically inverted: 1 = one above the other).
- The splitter (divider) between panels is a draggable bar. Its position is stored in permille of the client width as `Divider=500` in the per-resolution section, e.g. `[1920x1080 (8x16)]`. Separate positions: `DividerComments` (comments view), `DividerQuickView` (quick view).
- While dragging, TC shows a small tooltip with the percentage, e.g. `50%` **(verify)**.
- Right-click on the splitter: menu with percentages `10%/90%`, `20%/80%`, `30%/70%`, `40%/60%`, `50%/50%`, `60%/40%`, `70%/30%`, `80%/20%`, `90%/10%` **(verify exact list)**.
- Double-click the splitter: resets to 50/50 **(verify)**.
- Commands: `cm_50Percent`, `cm_100Percent` (TC 11: active panel takes the full width; running it again or any other percentage command restores two panels) **(verify command name in TC 11)**.
- Minimum width per panel: enough for the Name column plus a scrollbar; dragging beyond clamps.

### 1.4 Swap and match commands

- `Ctrl+U` (`cm_Exchange`): left and right panels exchange their current directory, view mode, sort, filter and selection. Folder tabs other than the active one stay where they are. The active side stays the same physical side (focus remains on the left if it was on the left, now showing the former right content).
- `Ctrl+Shift+U` (`cm_ExchangeWithTabs`): the entire tab sets swap sides, including lock state and captions.
- `cm_MatchSrc` (Commands > Target = Source; no default hotkey in TC 11, leave unbound) puts the target panel into the source panel's directory. View and sort of the target are not changed.
- `Ctrl+Left` / `Ctrl+Right` (`cm_TransferLeft` / `cm_TransferRight`): when the active panel is the right one and the user presses `Ctrl+Left`, the left panel opens the folder under the cursor (or, if the cursor is on a file or `..`, the current directory of the active panel). Pressing the arrow that points to the active panel itself opens the folder under the cursor in the active panel (`Ctrl+Left` in the left panel = enter folder) **(verify)**. On an archive the archive is opened in the other panel. macOS: Ctrl+Left/Right are taken by Spaces; see F-289.

### 1.5 Status bar text

Format (English TC 11), with thousands separators from the OS locale:

```
0 k / 1,234 k in 0 / 27 file(s), 0 / 5 dir(s)
```

- First pair: size of selected files / size of all files in the current listing (filtered entries excluded).
- Second pair: number of selected files / total files. Third: selected folders / total folders. `..` is not counted.
- Units follow `SizeFooter` (`[Configuration]`): 0 = bytes, 1 = kbytes (`k`, default), 2 = dynamic (`k`, `M`, `G`), 3 = MB **(verify value mapping)**. Configuration > Display > "File size format in footer" combobox.
- Selected folder sizes are only included after they are calculated (Space on a folder, `Ctrl+L`, `cm_CountDirContent`). Uncalculated folders contribute 0.
- `DirsInFooter=1` (default) shows the dir(s) part; 0 hides it.
- When the cursor is on a single file and nothing is selected, TC 11 still shows the totals line, not file details. (Hovering shows tooltips; see 3.7.)
- In Branch view the counts cover all listed files of all subfolders.
- While a quick filter or user filter is active, totals are for the filtered listing.
- Clicking the status bar does nothing except activate the panel. Right-click: none **(verify)**.

### 1.6 Drive info line text

`[label]  1,234,567 k of 9,876,543 k free`. The label is in square brackets, `[_none_]` when the volume has no label. Units follow `SizeStyle`/`SizeFooter` rules (k by default). For network/FTP/plugin file systems the text shows the plugin or connection name and free space only when the file system reports it. Inside archives TC keeps the drive info of the drive holding the archive **(verify)**.

### 1.7 Separate tree (F-025, F-400, F-279, F-824)

- `Ctrl+Shift+F8` cycles: no separate tree -> one separate tree (shared, left of both panels, follows the active panel) -> two separate trees (one per panel, above or to the left of each panel) -> off. Commands: `cm_SeparateTreeOff`, `cm_SeparateTree1`, `cm_SeparateTree2`, `cm_SwitchSeparateTree` (cycle).
- The tree follows the panel: changing directory in the panel selects the node in the tree (expanding as needed).
- Clicking a tree node (single click) changes the panel directory. Keyboard in the tree: Up/Down move and change the panel after a short delay **(verify: TC changes immediately in the separate tree)**; Left collapses / goes to parent node; Right expands / goes to first child; `+`/`-` expand/collapse; Enter or `Shift+Tab` returns to the file list.
- Configuration > Tree page (TC 10+): "Separate tree: [x] tree follows directory change", "Show hidden/system folders in tree", "Expand on single click", "Tree width" etc. **(verify labels)**. The tree's width is resizable with its own splitter, saved per resolution section (`SeparateTreeWidth` **(verify key)**).

---

## 2. File list contents common to all views (F-091, F-092, F-093, F-094, F-095, F-097, F-098, F-099, F-102, F-103, F-105, F-106, F-414, F-415, F-417, F-789, F-821, F-090, F-428, F-788)

### 2.1 Which entries are listed

1. `..` (parent) is the first entry in every folder except the root of a drive. TC 9.5+ option `ShowParentDirInRoot=1` (`[Configuration]`) also shows `..` in roots; Enter on it opens the drive list / "My Computer" view **(verify)**. In archives the `..` of the archive root leaves the archive. FEATURES F-102 makes this an option; default **on** except in roots.
2. Folders.
3. Files.

Folders are always listed before files, regardless of sort column, sort direction or view (TC has no "mix folders and files" option in 11; there is `SortDirsByName` affecting folder order only, see 5.4). `..` is never sorted and never selected by Select All.

Hidden entries: Configuration > Display > "Show hidden files" and "Show system files" (TC 11 has separate checkboxes, ini `ShowHiddenSystem=0/1` plus TC 9+ split keys **(verify)**). Hidden files, when shown, are drawn with a "ghosted" (lighter) icon and, optionally, grey text (Configuration > Colors > define by attribute). On macOS/Linux **(ours)**: a name starting with `.` counts as hidden; on macOS the `UF_HIDDEN` flag too. `cm_SwitchHidSys` toggles showing hidden+system at once; TC 11 binds it to no key by default (FEATURES: Ctrl+H; Cmd+Shift+. on macOS).

Ignore list (Configuration > Display > "Ignore list" checkbox + "Edit..." button, file `ignore.txt`, ini `IgnoreListFile=` and `IgnoreList=1`): patterns (one per line, full paths or name masks like `*.bak` or `desktop.ini`) are never listed. `cm_SwitchIgnoreList` toggles (F-788, F-428).

Filters (Show menu) restrict files but never folders (folders are always listed under All/Programs/Custom filter), see 4.10.

### 2.2 Name display

- Folders: shown in square brackets `[Windows]` when Configuration > Display > "Show square brackets around directory names" is on (`DirBrackets=1`, default **on**). TC shows the brackets regardless of whether icons are shown. We default it **off** when icons are shown **(ours)**, but keep the option.
- Extension: in Full view and Brief view, Configuration > Display > "Show extension separately" / TC Full view always has a separate Ext column unless a custom column set says otherwise. In Brief view the extension is aligned in a sub-column when `Aligned extension=1` ("Brief view: extension aligned" option). Names without a dot have an empty Ext. Names starting with a dot (e.g. `.profile`): TC treats the whole name as extension-less? TC treats `.profile` as name "" ext "profile" **(verify)**; we treat a leading dot as part of the name **(ours, Unix convention)** and show no extension.
- Folders have no extension column value in TC: `[My.Folder]` keeps the dot inside the brackets and the Ext column is empty **(verify)**; TC sorts folders by full name.
- Long names are cut with an ellipsis `...` at the end (Full view, Name column); TC cuts at the end and keeps the extension column separate. Tooltip shows the full name when the name is truncated (F-105).
- Case: TC shows names exactly as stored. Option "Show lowercase names" (`ShowLowerCase` **(verify)**) exists for 8.3 uppercase names only; not needed on Unix.

### 2.3 Size column

- Files: size per Configuration > Display > "File size format" (TC 11 combobox): `Bytes`, `kbytes`, `Mbytes`, `Gbytes`, `Dynamic (x.xx k/M/G)`, `Dynamic (x.x k/M/G)`, `Dynamic (xxx k/M/G)` **(verify exact list)**. Ini `SizeStyle` (list) and `SizeFooter` (status/header). Default: bytes with thousands separators in the list.
- Folders: `<DIR>` until the size is calculated; afterwards the size in the same format. Calculated folder sizes are kept until the folder is reread **(verify: TC keeps them until refresh)**.
- `..`: `<DIR>`.
- Links: symlink to a folder shows `<LNK>`? TC shows `<DIR>` with a link overlay; mormiCommander **(ours)** shows `<LNK>` for symlinks to folders and the target size for symlinks to files, plus a link overlay icon.
- Right aligned.

### 2.4 Date column

- Shows the last-write (modification) time in the OS short date format plus `hh:mm`, e.g. `13.11.2026 14:05`. Configuration > Display > date options: "Show century" (`ShowCentury=1`), "Show AM/PM" / 24h (`ShowAMPM=0`), "Show seconds" (TC 10+, `ShowSeconds` **(verify)**). Custom format via `DateTimeFormat` **(verify)**.
- Folders show their own modification time. `..` shows the current folder's time **(verify)**.

### 2.5 Attr column

- Windows: `rahs` letters in order, `-` for unset, e.g. `-a--`. TC 11 adds `c` (compressed), `e` (encrypted) **(verify order: r a h s c e)**.
- macOS/Linux **(ours)**: `rwxr-xr-x` permission string (10 chars including type char optional). Custom columns can show owner/group.

### 2.6 Icons (Configuration > Icons page, F-095, F-415)

- "Show icons": ( ) All associated + EXE/LNK (slow), ( ) All associated, ( ) Standard icons only, ( ) No icons. Ini `IconsMode` **(verify; older `ShowIcons`)**.
- "Show overlay icons" (`IconOverlays=1`), `cm_SwitchOverlayIcons` toggles (F-787).
- "Icon size in file lists": 16, 24 (TC 9+), 32, 48? TC 11 offers 16/20/24/32/40/48? **(verify)**. Ini `IconSize`.
- "Show icons in menus", "Show icons for folders: standard / associated (desktop.ini)" **(verify)**.
- Icons are loaded asynchronously after the list is drawn; placeholders first.

### 2.7 Colours (Configuration > Color page, F-096, F-097, F-099, F-417, F-435)

Controls (TC 11): Font colour, Background, Background 2 (alternating lines; checkbox "Alternating lines"), Mark colour, Cursor colour, Cursor text, "Inverse cursor" / "Inverse selection" checkboxes, "Frame cursor" (cursor only as frame), "Use different colours for inactive panel" with its own background/cursor colours, "Define colours by file type..." button opening the "Define colours by file type" list (each entry: search template or wildcard mask + attribute filter + colour; first match wins). Ini `[Colors]`: `ForeColor`, `BackColor`, `BackColor2`, `MarkColor`, `CursorColor`, `CursorText`, `InverseCursor`, `InverseSelection`, `ColorFilter1=*.exe`, `ColorFilter1Color=...` etc.

Defaults: black text, white background, red mark colour, dark blue cursor with white text **(verify)**. mormiCommander follows the system light/dark theme by default (F-021) and keeps these as overrides.

### 2.8 Tooltips (F-106, F-789, F-821)

- Hovering the mouse over a truncated name shows its full name instantly-ish (Windows tooltip delay).
- Configuration > Display > "Show tooltips" / TC 11 Configuration > Tooltips page: "Show file tooltips: ( ) no ( ) only for truncated names ( ) always with file details"; per file type rule list (wildcard + fields such as `[=tc.size]`, `[=tc.writedate]`, image dimensions). Default tooltip text for a file: name, `Size: 1,234 bytes`, `Date/time: ...`, Explorer-type info **(verify)**.

### 2.9 Auto-refresh (F-065, F-422)

Not area 2, but listing behaviour: when a change notification arrives, the panel rereads after a short delay, keeping cursor on the same name (or the next entry if deleted), keeping selection of entries that still exist, keeping the quick filter, the scroll offset as close as possible.

---

## 3. Views (F-079 .. F-090, F-393 .. F-399, F-784, F-785, F-786, F-819, F-820)

The view of a panel is one of: **Brief**, **Full**, **Comments**, **Custom columns n**, **Thumbnails**, **Tree**, plus the overlay states **Quick view** (other panel) and **Branch view** (flat listing, combinable with Brief/Full/Custom). View, sort and filter are per panel and per folder tab. All view commands exist in `cm_Src*` (source), `cm_Left*`, `cm_Right*` and `cm_Trg*` variants (F-786).

| View | Command (src/left/right/trg) | Hotkey | Show menu item | ini `[left]`/`[right]` | IDs |
|---|---|---|---|---|---|
| Brief | `cm_SrcShort` / `cm_LeftShort` / `cm_RightShort` / `cm_TrgShort` | Ctrl+F1 | Show > Brief | `ShowAllDetails=0`, `SpecialView=0` | F-079, F-393 |
| Full | `cm_SrcLong` / `cm_LeftLong` / ... | Ctrl+F2 | Show > Full | `ShowAllDetails=1`, `SpecialView=0` | F-080, F-394 |
| Comments | `cm_SrcComments` / ... | Ctrl+Shift+F2 | Show > Comments | (verify) | F-085, F-395 |
| Thumbnails | `cm_SrcThumbs` / ... | Ctrl+Shift+F1 | Show > Thumbnails | (verify) | F-083, F-396, F-675 |
| Custom columns menu | `cm_SrcCustomViewMenu` | Shift+F1 | Show > Custom columns > ... | `SpecialView=n` | F-084, F-397, F-671 |
| Custom view n | `cm_SrcCustomView1..99` | (assignable) | | `SpecialView=n` | F-084 |
| Next custom view | `cm_SrcNextCustomView` / `cm_SrcPrevCustomView` **(verify)** | (assignable) | | | F-084 |
| Tree | `cm_SrcTree` / ... | Ctrl+F8 | Show > Tree | `ShowAllDetails=2` | F-081, F-398 |
| Quick view | `cm_SrcQuickview` | Ctrl+Q | Show > Quick View | not saved | F-082, F-399 |
| Quick view internal only | `cm_SrcQuickInternalOnly` | (assignable) | | | F-784 |
| Branch view | `cm_DirBranch` | Ctrl+B | Commands > Branch View | not saved | F-086 |
| Branch view (selected) | `cm_DirBranchSel` | Ctrl+Shift+B | | not saved | F-087 |
| View modes | `cm_SrcViewModeList`, `cm_SrcViewMode0..n` **(verify)** | (assignable) | Show > View modes | | F-785, F-819 |

Choosing the view the panel is already in does nothing (no toggle), except Ctrl+Q (toggle), Ctrl+B (toggle) and Ctrl+F8 (toggles back to the previous view **(verify: TC returns to the previous Brief/Full)**).

### 3.1 Brief view (F-079, F-050)

- Entries are laid out top-to-bottom in as many columns as fit, then left-to-right (newspaper columns). Each column has the same width = width of the longest name in the folder (icon + name + aligned extension), capped by `BriefViewWidthLimit` (pixels, `[Configuration]`; 0 = no cap) **(verify)**. The rightmost partial column is visible but clipped.
- Horizontal scrollbar when columns exceed the panel width; no vertical scrollbar.
- Keys: Up/Down move one row (and wrap from bottom of one column to top of the next? No: Down at the bottom of a column moves to the top of the next column **(verify)**); Left/Right move one column (same row; at the last column, Right goes to the last entry); PgUp/PgDn move one column page? TC: PgDn moves to the bottom of the current column, then one column page further **(verify)**; Home/End first/last entry.
- Mouse wheel scrolls horizontally by one column **(verify)**.
- No column header widths; the sort header row shows sort buttons when visible.

### 3.2 Full view (F-080, F-014, F-104)

Default columns, in order: **Name**, **Ext**, **Size**, **Date**, **Attr**. Header captions in TC 11: `Name`, `Ext`, `Size`, `Date`, `Attr`. One row per entry, icon then name.

- Name column takes the remaining width (`[Tabstops] AdjustWidth=1`: when the window is resized the Name column absorbs the change). Other columns have fixed widths stored as `Tabstops=` per side in the resolution section (e.g. `[left]`? TC: `[1920x1080 (8x16)] Tabstops=...` / `RightTabstops=`) **(verify exact key location)**.
- Dragging a header separator resizes the column to its left; the Name column then shrinks/grows accordingly. If total width exceeds the panel, a horizontal scrollbar appears (F-104).
- Double-click on a header separator: auto-fit column width to content **(verify; TC 9+)**.
- Header click: sort by that column (section 5). Clicking the Ext column sorts by extension; clicking Attr sorts by attributes? TC: Attr column click does nothing in default view **(verify)**; in custom columns any column can sort.
- Right-click on header: context menu listing `Default` (built-in Full view), each defined custom column set (checked = current), separator, `Configure custom columns...`, and in TC 10+ `Auto-size columns` **(verify last item)**. Choosing a set switches the view (F-671, F-672).
- Column alignment: Name, Ext left; Size right; Date left; Attr left.

### 3.3 Comments view (F-085, F-395)

- Ctrl+Shift+F2. The panel shows the Full view columns Name and Ext (and, in TC 11, the default comment column) plus a **Comment** column showing the description of each file from `descript.ion` (4DOS/4NT format: one line per file, `name comment`, names with spaces quoted). In TC the comments view is a variant of Full view where the size/date/attr columns are replaced by the comment **(verify)**. Its splitter position is `DividerComments` (comments view usually needs a wider panel).
- `Ctrl+Z` (`cm_EditComment`) opens the "Change comment" dialog: title `Change comment`, label `Comment for <filename>:`, multi-line edit field, buttons `OK`, `Cancel`. Saving writes `descript.ion` in that folder (created if missing; file is made hidden on Windows). An empty comment removes the line; if `descript.ion` becomes empty, it is deleted **(verify)**.
- Copy/move/rename/delete carry comments along when Configuration > Misc? "Copy comments with files" (`CopyComments=` bitmask: 1 copy, 2 move, 4 rename/delete... **(verify bits)**). That part belongs to the file-op engine (area 1); the view only reads.
- macOS **(ours)**: also offer Finder comments (`kMDItemFinderComment`) as a read-only fallback field. Default source is `descript.ion` for cross-platform parity.

### 3.4 Custom columns view (F-084, F-665 .. F-674, F-433, F-397)

**Show > Custom columns** submenu and the Shift+F1 popup (`cm_SrcCustomViewMenu`) list: `Default` (Full view), the custom sets by name in configured order with a check mark on the active one, separator, `Configure custom columns...`. Number keys 1-9 pick the first nine sets in the Shift+F1 popup **(verify)**.

**Configuration > Custom columns** page (TC 11):

- List box "Custom column views:" with the defined set names, in display order.
- Buttons beside it: `New...`, `Copy...` **(verify)**, `Edit...`, `Delete`, `Up`/`Down` (or drag to reorder) **(verify)**.
- Option "Show custom columns view in menu" per set? Not in TC 11 **(verify)**.

**Modify custom columns view** dialog (opened by New/Edit), TC 11 layout from top:

1. `Name of column set:` edit field (e.g. `Pictures`).
2. Checkbox `Use this view for folders/files matching:`? No: auto-switching is done by View modes (3.9), not here.
3. A live header preview row showing the columns as they will appear; dragging separators in it sets widths; clicking a column selects it for editing.
4. Grid of columns, one row per column, with fields:
   - `Title` (header caption),
   - `Width` (pixels),
   - `Alignment`: left `<-` / right `->` (toggle button; negative width in the ini means right aligned),
   - `Field contents` (template, e.g. `[=tc.size.bytes]`, may combine several fields and literal text: `[=tc.width]x[=tc.height]`).
   The first column is always Name (with icon) and cannot be removed; checkbox `Name` + `Ext` together or `Name` with extension inside **(verify: TC fixes the first column as Name, the second optional "Ext")**.
5. Buttons: `+` (add column after selection), `-` (delete column), `Up`/`Down` arrows? `>>` button next to Field contents opens the field chooser menu: `tc` (built-in fields), then plugin names (N/A in ours); each field has a submenu of units/variants (e.g. `size` -> `bytes`, `kbytes`, `Mbytes`, `Gbytes`, `dynamic`, `float`).
6. Checkbox `Use custom font` with `Font...` button **(verify)**; checkbox `Adjust width of the first column to fit` **(verify)**; colour options for the set ("Use own colours") **(verify, TC 10+)**.
7. `OK`, `Cancel`, `Help`.

Built-in `tc` fields that must exist (F-674): `name`, `ext`, `size` (units: bytes, kbytes, Mbytes, Gbytes, Dynamic), `writedate` / `writetime` (TC: `tc.writedate`, `tc.writetime`, `tc.writedatetime` **(verify)**), `creationdate`, `creationtime`, `accessdate`, `accesstime`, `attributes` / `attributestr`, `path`, `fullname`, `comment`, `filetype` (Explorer description). Ours adds: `permissions`, `owner`, `group`, `linktarget`, `mime`, `imagewidth`, `imageheight`, `exifdate`, audio tags (F-665).

Sorting: clicking a custom column header sorts by its first field; numeric fields sort numerically, dates chronologically, text case-insensitively (section 5). Fields that are slow (image size, EXIF) are computed in the background; rows show empty cells first and the list re-sorts when finished only if sorted by that column **(verify: TC re-sorts after all values are loaded)**.

wincmd.ini storage (needed for import, F-446): section `[CustomFields]` with keys per set n: `Widths<n>=` (comma list, first value Name width, negative = right aligned), `Headers<n>=` (titles separated by `\n`), `Contents<n>=` (templates separated by `\n`), `Options<n>=` (e.g. `-1|0` flags), and `CustomView<n>=`?/ name key **(verify exact key names against a real TC 11 wincmd.ini before writing the importer)**; the panel's active set is `[left] SpecialView=n` / `[right] SpecialView=n`; `customsortorder=100+k` sorts by custom column k.

### 3.5 Thumbnails view (F-083, F-675 .. F-680, F-850, F-426)

- Ctrl+Shift+F1. Entries shown as tiles in a grid: square preview area (width x height from options), name below (one or two lines, ellipsis), optionally custom field lines below the name (F-679). Folders show the folder icon (or, with `WmfAllowed` bit set, a folder preview of the first images **(verify)**). `..` is shown as a tile with the up-folder icon.
- Tiles are laid out left-to-right then top-to-bottom; vertical scrollbar.
- Keys (F-850): Left/Right previous/next tile, Up/Down one row up/down, PgUp/PgDn one screen, Home/End first/last. Selection keys (Space, Insert, Num+, etc.) work as in other views. Enter opens.
- Mouse: click selects per mouse mode; double-click opens. `Ctrl+mouse wheel` changes thumbnail size **(verify; TC 9.5+?)**.
- Thumbnail generation runs in background threads; placeholders (file-type icon) first, then images replace them as they arrive, visible area first. `PreloadThumbnails`: 1 = load when opening a folder (default), 2 = also in branch view, 3 = only visible + next two lines.
- Cache: database in `ThumbsLocation` (default local app data); key = full path + size + mtime. Cleared via Configuration > Thumbnails "Delete thumbnails cache" **(verify button label)** / `cm_ClearThumbnailCache`? **(verify command name)**.

**Configuration > Thumbnails** page (TC 11), from top:

- Group `Thumbnail size`: `Width:` [96] `Height:` [96] (pixels). Ini `ThumbWidthNoCache`, `ThumbHeightNoCache` (names historic). FEATURES asks 32..512 (F-677).
- `[x] Save thumbnails in a database (cache)` with location edit + browse button; `Delete thumbnails...`/`Delete cache` button.
- Group `Thumbnail generation`: checkboxes per method (TC: Explorer/OS, Lister plugins, IrfanView/XnView, text files show first lines) each with a file mask (`ThumbOptions` bitmask 1/2/4/8; `ThumbExplTypes`, `ThumbPlgTypes`, `ThumbIrfXnTypes`, `ThumbTxtTypes`). Ours: built-in image decoders, PDF/video via OS (QuickLook on macOS), text first lines.
- `[ ] Show custom fields below thumbnail` (`ThumbsCustomFieldsEnabled`) + `Define fields...` (same template syntax as custom columns, one line per field).
- `[x] Copy/delete thumbnails with files` (`ThumbsCopyDel=1`).
- Frame colour in Colors page: `[Colors] ThumbnailFrame=-1` (-1 default, -2 none).

### 3.6 Tree view in panel (F-081, F-398)

- Ctrl+F8 shows the directory tree of the current drive in the panel (not the separate tree). The node of the current directory is selected.
- First use on a drive builds the tree (TC: reads `treeinfo.wc`/`treeinfo.wc` cache or scans; `OldStyleTree=0` means load lazily). Shows progress `Reading directory tree...` **(verify text)**. Ours: lazy expansion, folders read on demand.
- Keys: Up/Down move; Left collapse or go to parent node; Right expand / first child; Enter switches the panel back to the previous list view (Brief/Full) in the selected folder; `Tab` goes to other panel; typing letters does quick search among node names; F5/F6/F8 operate on the selected folder.
- `Autotreechange=1`: moving the cursor in tree view changes the other panel's directory (single click/keys) **(verify semantics)**.
- `Tree=0/1` sorts the tree by name / unsorted.
- Ctrl+F8 again returns to the previous view without changing directory **(verify)**.
- Alt+F10 (`cm_CDtree`) is a different dialog (Find folder in tree dialog with search field) — area 3 (F-229).

### 3.7 Quick view (F-082, F-399, F-784)

- Ctrl+Q toggles. The **other** panel is replaced by a Lister instance showing the file under the cursor of the active panel. Moving the cursor updates the preview after a short delay (TC: immediately for small files). Folders: shows folder info (TC: nothing / folder size info **(verify)**). Multiple selection does not matter; only the cursor counts.
- The quick view panel keeps the other panel's tabs hidden; the drive info line shows `Quick View`? **(verify)**. Its splitter position is `DividerQuickView`.
- `Tab` moves focus into the quick view (scrolling with keys), `Tab` again returns **(verify)**.
- `cm_SrcQuickInternalOnly`: like Ctrl+Q but uses only the built-in viewer (text/hex), no plugins.
- Ctrl+Q while the target is in quick view and the user switches to it: TC ends quick view when the other panel becomes active via mouse click **(verify)**.
- Not saved across restarts.

### 3.8 Branch view (F-086, F-087)

- Ctrl+B (`cm_DirBranch`): the panel lists all files of the current folder and all subfolders recursively, flat. Folders themselves are not listed (TC: only files; TC 9+ option to include folders: holding Shift? `cm_DirBranch` with parameter **(verify)**). The Name column shows the relative path: `sub\deeper\file.txt` (the path part in the name column, shown before the name) **(verify: TC shows `sub\deeper\` in the name column, sorted by full relative path when sorted by name)**.
- The current dir bar shows the folder followed by `*.*` and a `[Branch view]`? TC shows the path normally; the window shows a "branch" state in the tab caption? **(verify)**.
- Operations (copy, delete, rename F2/Shift+F6) work on the listed files at their real locations. Copying from branch view to another panel flattens? TC asks/keeps relative paths: copy dialog has option "Keep folder structure" **(verify)**; area 1.
- Ctrl+B again, Backspace, or any directory change leaves branch view. Ctrl+R rereads branch view.
- Ctrl+Shift+B (`cm_DirBranchSel`): branch view of only the selected folders (and selected files) in the current folder.
- Large trees: listing builds progressively; Esc cancels the scan and keeps what has been read **(verify)**.

### 3.9 View modes (F-785, F-819, F-820)

TC 10+ "View modes" (Configuration > Display > View modes; Show > View modes):

- A view mode is a named bundle of: column set (Brief, Full, or a custom set), icon options, colours, font, thumbnail settings and optionally sort order.
- Auto-switch rules: for a folder path (wildcards), or for folders containing mostly certain file types (e.g. `*.jpg;*.png` > 50 %?) **(verify rule kinds)**, or plugin file systems. When entering a matching folder, the panel switches to that view mode; leaving restores the previous mode.
- `Show > View modes` lists them; `<none>` turns auto-switching off for the panel **(verify)**.
- Page layout: list of modes, `Add`, `Remove`, `Up`, `Down`, `Edit` buttons; edit dialog with name, column set combobox, auto-switch condition fields. This is the lowest-priority feature in this area; implement after custom columns.

### 3.10 Display filters: All / Programs / Custom (F-088, F-089, F-401 .. F-404)

- Ctrl+F10 `cm_SrcAllFiles`: all files. Ctrl+F11 `cm_SrcExecs`: programs only (Windows: extensions from `[Configuration] Executables? / PATHEXT` default `*.exe;*.com;*.bat;*.cmd;*.pif`). Ours on Unix: files with any execute bit, plus `*.app` bundles on macOS **(ours)**. Ctrl+F12 `cm_SrcUserSpec`: the last user-defined filter (`userspec=` key, default `*.*`).
- `cm_SrcUserDef` (Show > Custom...): dialog titled `Define selection`? TC title is `Select files` style "Show > Custom ..." **(verify)**: a combobox with history of masks (`*.txt *.doc` separated by spaces or `;`, `|` separates exclude masks: `*.* | *.bak`), button `Define...` (opens the Find files dialog in "search template" mode to define a saved template), `OK`, `Cancel`, `Help`. Choosing a saved search template (from Find files) is allowed: typed by name or chosen from the list. Folders are always shown regardless of the mask **(verify; TC applies masks to files only unless the mask contains `\`)**.
- When a filter other than All is active, the current dir bar shows the mask instead of `*.*` (`c:\dir\*.txt`), and Show menu checks the matching item. Ini `[left] show=1/2/3`, `userspec=`.
- Filter is per tab and saved.

### 3.11 Show only selected / hide selected (F-408)

TC 11 commands **(verify names)**: `cm_ShowOnlySelected` hides all unselected entries; `cm_HideSelected` hides the selected ones. State lasts until reread (Ctrl+R) or directory change. Current dir bar shows `*` marker? **(verify)**.

---

## 4. Navigation inside a view that matters for views (F-049, F-050, F-064, F-072, F-073, F-074)

Covered mainly in area 3; the view-dependent rules are:

- Full/Comments/Custom: Up/Down one row, PgUp/PgDn one page minus one row, Home/End first/last, Left/Right scroll horizontally only if a horizontal scrollbar exists, else Left = Home? TC: in Full view Left/Right do nothing unless "Left/Right in full view: go to parent/enter folder" option? There is no such TC option; Left/Right scroll horizontally **(verify)**.
- Brief: see 3.1. Thumbnails: see 3.5.
- After leaving a folder (Backspace / `..`) the cursor lands on the folder just left (F-074). Re-entering a folder visited recently restores its last cursor entry **(verify: TC remembers per history entry)**.
- After view change (e.g. Brief -> Full) the cursor stays on the same entry and is scrolled into view; selection is kept.
- After sort change: cursor stays on the same entry; the list scrolls to keep it visible.
- Double-click on empty space below the list: goes to parent if Configuration > Operation? `Double click in empty space: go to parent` **(verify option; TC 9+ `DblClickToParent=1`)** (F-072).
- Mouse buttons 4/5: history back/forward (F-073).

---

## 5. Sorting (F-107 .. F-117, F-405, F-406, F-670, F-790, F-098, F-787)

### 5.1 Commands and keys

| Sort | Command (src; also `cm_Left*`, `cm_Right*`, `cm_Trg*`) | Hotkey | Default direction | ini `sortorder` |
|---|---|---|---|---|
| Name | `cm_SrcByName` | Ctrl+F3 | ascending A-Z | 0 |
| Extension | `cm_SrcByExt` | Ctrl+F4 | ascending, then name | 1 |
| Date/time | `cm_SrcByDateTime` | Ctrl+F5 | descending (newest first) | 3 |
| Size | `cm_SrcBySize` | Ctrl+F6 | descending (largest first) | 2 |
| Unsorted | `cm_SrcUnsorted` | Ctrl+F7 | file-system order | 4 |
| Reverse | `cm_SrcNegOrder` | none | toggles direction | `negative Sortorder=0/1` (note the space in the key) |
| Custom column k | `cm_SrcSortByCol1..99` | assignable | column-type default | `customsortorder=100+k-1` |

- Pressing the hotkey of the sort column that is already active reverses the direction; pressing a different one switches column with that column's default direction.
- Show menu lists `Name`, `Extension`, `Time`, `Size`, `Unsorted` as radio items and `Reverse order` as a checkbox item (F-405, F-406).

### 5.2 Header click

- Click on a header caption: sort by it (default direction); click again: reverse. The active sort column shows a small triangle/arrow next to the caption: up-pointing for ascending, down-pointing for descending **(verify glyph: TC uses `^`/`v` arrows drawn next to the title)**.
- Clicking `Name` header in Full view: sort by name; `Ext`: by extension; `Size`: size; `Date`: date; `Attr`: no action **(verify)**.
- Ctrl+click on another header (TC 8+): adds it as a secondary sort key; up to 4 keys? The header shows the key order as small numbers or smaller arrows **(verify display)**. Plain click resets to a single key (F-114).
- Header drag beyond 4 px is a resize, not a click.

### 5.3 Tie-breaking and comparison rules

- Folders always above files; `..` always first.
- Name sort: compares the whole name including extension? TC compares name part first, then extension **(verify: TC sorts "a.txt" before "a b.txt"? because it splits at the last dot)**. We compare by full name with the extension as a secondary key only when the name parts are equal.
- Extension sort: by extension, then by name. Files without extension come first.
- Date sort: by modification time; equal times fall back to name ascending regardless of direction **(verify)**.
- Size sort: equal sizes fall back to name ascending. Folders: by name unless sizes are calculated and `SortDirsByName=0` (5.4).
- Unsorted: order returned by the file system (`readdir` order). On macOS APFS that is not alphabetical; that is correct.
- Case: case-insensitive by default.

### 5.4 Configuration > Display: sorting options (F-098, F-115, F-116, F-787)

- `Sort directories` group **(verify labels)**: ( ) `Always by name` (`SortDirsByName=1`, default), ( ) `Like files (also by date)`, ( ) `Like files, by size only if size was calculated` (TC 9+). `cm_SwitchDirSort` toggles always-by-name on/off.
- `Sorting method` combobox (`SortUpper` key): 0 `Alphabetical, considering accents` (Windows locale compare; default), 1 same but `_` sorts last? (historic: "files starting with _ at end"), 2 `Natural sorting: alphabetical and numbers` (TC's own), 3 `Natural sorting (Windows StrCmpLogicalW)`; TC 10+ adds `Strict Unicode (ordinal)` **(verify combobox wording and value 4)**.
  - Natural sort: digit runs compare by numeric value (`file2` < `file10`), leading zeros: `file02` == `file2` numerically, tie broken by shorter string first **(verify)**.
- Default for mormiCommander **(ours)**: natural sorting on (matches Finder), configurable.
- "Sort by extension: folders also by extension" option? not in TC.

### 5.5 Persistence

- Sort column + direction per panel per tab (F-117); saved in `[left]`/`[right]` (`sortorder`, `negative Sortorder`, `customsortorder`) and per tab in `[lefttabs]`/`[righttabs]` `<n>_options` (6.9).
- Changing directory keeps the sort.

---

## 6. Folder tabs (F-027 .. F-048, F-773 .. F-777, F-432, F-434, F-100)

### 6.1 Tab bar appearance

- One tab bar per panel, above the current dir bar. Each tab shows a caption: the folder name of its current directory (`Windows`), the drive root as `c:` / ours `/` or the volume name, a custom name when renamed. Option "Show drive letter in tab" prefixes `c:` **(verify; ours: off)**.
- Locked tabs show a `*` before the caption when "Show locked tabs with a star" is on (default on) (F-035, F-046). Tabs locked with directory changes allowed show `+`? **(verify marker; TC shows `*` for both kinds?)**.
- Caption length is limited to `DirTabLimit=32` characters when "Limit tab width to N characters" is on; longer captions are cut with `...`. Tooltip on a tab shows the full path.
- Active tab is drawn raised/highlighted; inactive panel's active tab also highlighted but weaker.
- One line with scroll arrows when tabs overflow, or multiple lines when "Tabs on multiple lines" is on (F-046).
- Tab bar hidden when only one tab exists unless "Show tab header also when there is only one tab" is on (F-046).
- TC 10+: optional favourite-tabs button and close buttons on tabs **(verify; ours: close button on hover)**.

### 6.2 Keyboard

| Action | Command | Hotkey (TC 11 default) | ID |
|---|---|---|---|
| New tab with current folder (foreground) | `cm_OpenNewTab` | Ctrl+T | F-027 |
| New tab, background | `cm_OpenNewTabBg` | Ctrl+Shift+T (ours; TC 11: none **(verify)**) | F-773 |
| Folder under cursor in new tab | `cm_OpenDirInNewTab` | Ctrl+Up | F-028 |
| Folder under cursor in new tab of other panel | `cm_OpenDirInNewTabOther` | Ctrl+Shift+Up | F-029 |
| Close current tab | `cm_CloseCurrentTab` | Ctrl+W | F-030 |
| Close all tabs except current | `cm_CloseAllTabs` | Ctrl+Shift+W | F-031 |
| Next tab / previous tab | `cm_SwitchToNextTab` / `cm_SwitchToPreviousTab` | Ctrl+Tab / Ctrl+Shift+Tab | F-032 |
| Activate tab n | `cm_SrcActivateTab1..99`, `cm_LeftActivateTab..`, `cm_RightActivateTab..`, `cm_TrgActivateTab..` | none | F-033, F-774 |
| Lock / unlock | `cm_ToggleLockCurrentTab` | none | F-035 |
| Lock with dir changes | `cm_ToggleLockDcaCurrentTab` | none | F-036 |
| Duplicate / copy to other side | `cm_CopyTabToOtherSide`? **(verify)** | none | F-038 |
| Save / load tabs | `cm_SaveTabs` / `cm_LoadTabs` | none | F-043 |
| Favourite tabs | `cm_LoadFavoriteTabs`, `cm_SaveFavoriteTabs`, `cm_ConfigFavoriteTabs`, `cm_NextFavoriteTab`, `cm_PrevFavoriteTab` | none | F-775 |
| Reopen closed tab | `cm_ReopenClosedTab` **(ours)** | none (suggest Ctrl+Shift+Z? leave unbound) | F-776 |

Rules:
- Ctrl+Up on a file: TC opens the current folder in a new tab? With option "Ctrl+Up opens archive in new tab" archives open as folder in the new tab; on a plain file: new tab of current folder **(verify)**. On `..`: new tab with the parent folder.
- Ctrl+Tab wraps from last to first. With only one tab: does nothing.
- Ctrl+W on the last remaining tab does nothing (no message) **(verify)**.
- Ctrl+W on a locked tab: TC asks `This tab is locked! Close it anyway?` **(verify wording)**, buttons Yes/No. With a not-locked tab no question.
- Ctrl+Shift+W asks `Close all tabs?` / `Really close all tabs?` only when "Confirm closing of all tabs" is on **(verify text)**; locked tabs are closed too? TC keeps locked tabs **(verify)**.
- New tabs are inserted directly after the current tab when "Open new tabs next to current tab" is on (TC 9+ **(verify)**), else at the end.
- Closing the active tab activates the tab to its right, or the left one if it was last **(verify; some TC versions activate the previously active tab)**.

### 6.3 Mouse

- Click a tab: activates it (and the panel).
- Double-click a tab: closes it when "Close tab with double click" option is on; otherwise nothing / rename? **(verify default: on)**.
- Double-click empty area of the tab bar: opens a new tab with the current folder (F-041).
- Middle click a tab: closes it (F-042). Middle click on a folder in the list: opens it in a new tab **(verify; TC 9+ option)**.
- Ctrl+click on a folder? No: Ctrl+double-click? TC: Ctrl+Shift+double-click? FEATURES F-048 says Ctrl+double-click opens folder in new tab **(ours/verify)**.
- Drag a tab within its bar: reorders (F-039). Drag a tab onto the other panel's tab bar: moves it there; with Ctrl: copies **(verify)**.
- Drag files onto a tab: drops into that tab's folder (copy by default, Shift move, as normal drop rules) (F-040). Hovering a tab during a drag for ~1 s activates it **(verify)**.
- Right-click a tab: context menu (6.4).
- Mouse wheel over the tab bar: switches tabs **(verify)**.

### 6.4 Tab context menu (right-click on a tab) (F-034)

TC 11 items, in order **(verify exact order and wording)**:

1. `Close tab`
2. `Close all tabs` (except this one? TC: "Close all other tabs" in TC 10+)
3. separator
4. `Lock tab` (check item)
5. `Locked, but directory changes allowed` (check item)
6. `Locked, but directory changes allowed, reset to locked directory when reactivated`? — TC has a third lock state via the rename/lock dialog **(verify)**
7. `Rename/lock tab...`
8. separator
9. `Copy tab to other side`
10. `Move tab to other side` **(verify; TC 10+)**
11. `Open new tab`? / `Duplicate tab` **(verify)**
12. separator
13. `Save tabs to file...`, `Load tabs from file...`
14. `Favorite tabs` submenu: saved favourite sets, separator, `Add current tabs to favorites...`, `Configure...`
15. `Reopen closed tab` **(ours)**

### 6.5 Rename/lock tab dialog (F-037)

Title `Rename/lock tab`. Controls, tab order:

1. `New name:` edit field, prefilled with the current caption, text selected. Empty = use folder name again.
2. Radio group or checkboxes **(verify)**: `( ) Not locked`, `( ) Locked`, `( ) Locked, but directory changes allowed`, `[ ] Reset to locked directory when tab is reactivated`? **(verify)**.
3. `OK`, `Cancel`.

Esc cancels. Captions survive restarts (`<n>_caption=` in tabs section).

### 6.6 Locked tabs behaviour (F-035, F-036)

- A locked tab keeps its folder. Any directory change in it (Enter on a folder, Backspace, history, hotlist, drive change) opens a new tab instead, governed by `LockedTabChange` (`[Configuration]`): 0 = open new tab (default) **(verify default; wiki example shows 1)**, 1 = reuse an existing tab already showing that folder, 2 = reuse the first unlocked tab, 3 = reuse the last unlocked tab.
- `TabEnterDirChange` (Enter on a directory in a locked tab): 0 nothing, 1 open new tab (default), 2 ask.
- `LockedGoToDriveRoot`: the root command (Ctrl+\) in a locked tab: 0 = go to the tab's locked base folder, 1 = drive root (in a new tab).
- "Locked, but directory changes allowed": navigation works in place; the tab caption keeps the locked name; when the user switches back to this tab after leaving it, the folder resets to the locked base folder if that option is chosen **(verify)**.
- Locked tabs are marked with `*` (option).

### 6.7 Saving and restoring tabs (F-043, F-044, F-045, F-775)

- On exit (and on `cm_ConfigSaveSettings`) TC writes both tab sets to `[lefttabs]` / `[righttabs]` in wincmd.ini (when Configuration > Folder tabs "Save tabs on exit"? TC always saves **(verify)**).
- `cm_SaveTabs` opens a Save dialog for `*.tab` files (default from `TabDir`); writes the active panel's tabs. `cm_LoadTabs` loads a `.tab` file into the active panel, replacing its tabs **(verify: replace vs append; TC replaces)**. A hotlist entry may store a `.tab` file to load (F-045): command `cd` with a `.tab` path? TC: hotlist entries can be "Tabs file" via `cm_LoadTabs <file>` **(verify)**.
- Favourite tabs (TC 8.5+): save both panels' tab sets under a name; menu `Favorite tabs` in the tab context menu and optional tab-bar button; each entry can restore left only, right only, or both, and can have a hotkey; Configuration > Favorite tabs page edits them (F-434).

### 6.8 Configuration > Folder tabs page (F-432, F-046)

Controls in TC 11 order **(verify exact labels; group membership is reliable)**:

Group `Folder tabs`:
- `[x] Show folder tabs` (= `[Layout] DirectoryTabs`)
- `[ ] Show tab header also when there is only one tab`
- `[x] Confirm closing of all tabs`
- `[ ] Tabs on multiple lines`
- `[x] Show locked tabs with a star *`
- `[ ] Limit tab width to [32] characters` (`DirTabLimit`)
- `[x] Close tab with double click`
- `[ ] Ctrl+Up opens new tab in background` (TC: "Open new tabs in background") **(verify)**
- `[ ] Activate target panel when opening tab in other panel` (Ctrl+Shift+Up) **(verify)**
- `[ ] Open new tabs next to the current tab` **(verify)**
- `[ ] Show drive letter in tab` **(verify)**
- `[x] Show close button on tabs` (TC 10+) **(verify)**
- `[ ] Show favourite tabs button`? **(verify)**

Group `Locked tabs`:
- `When changing directory in a locked tab:` radio — `( ) Open new tab`, `( ) Reuse tab with same directory`, `( ) Reuse first unlocked tab`, `( ) Reuse last unlocked tab` (= `LockedTabChange` 0..3).
- `Enter in locked tab:` / `[x] Allow opening of directories in locked tabs with Enter (opens new tab)` (= `TabEnterDirChange`).
- `[ ] Root button / Ctrl+\ goes to drive root instead of locked directory` (= `LockedGoToDriveRoot`).

The checkbox group is stored as the bitmask `DirTabOptions` in `[Configuration]`. Exact bit values are not documented on the wiki; the importer should map only `DirTabLimit`, `LockedTabChange`, `TabEnterDirChange`, `LockedGoToDriveRoot` and leave bits at our defaults unless verified.

### 6.9 wincmd.ini tab sections (for import, F-443, F-446)

```
[lefttabs]
0_path=C:\Windows\
0_options=1|0|0|0|0|0|0
0_caption=Win
0_locked=1                ; (verify: lock state may be encoded in options)
1_path=D:\Projects\
1_options=...
activetab=1
activecaption=           ; caption of the active tab (verify)
activelocked=0           ; lock state of the active tab (verify)
[righttabs]
...
```

The active tab's path is not repeated in the tabs section; it is `[left] path=` / `[right] path=` **(verify)**. `<n>_options` is a `|`-separated list: sortorder, negative sort, ShowAllDetails (view), show (filter), SpecialView, lock flag(s) **(verify order)**. Our own config uses explicit keys instead; only the importer needs this.

### 6.10 Per-tab state (F-047, F-117, F-038)

Each tab carries: path, caption, lock mode, locked base path, view (+ custom set), sort (+ direction + secondary keys), filter (show + userspec), quick filter text? (no: quick filter is dropped on tab switch? TC keeps it until dir change **(verify)**), history (back/forward list; Alt+Left/Right per tab), cursor entry, scroll offset. Selection is per tab in memory only.

Tab switch restores everything except selection? TC keeps selection when switching tabs **(verify; ours: keep)**.

### 6.11 Edge cases

- Opening a tab for a path that no longer exists (on restore): TC goes up to the nearest existing parent; if the drive is gone, falls back to the current folder of another tab / `c:\` **(verify)**. Ours: nearest existing parent, else home folder.
- Duplicate tabs with the same folder are allowed.
- Swap panels (Ctrl+U) swaps only the active tabs' contents; Ctrl+Shift+U swaps whole sets (1.4).
- Maximum number of tabs: no hard limit (TC limited to 99 addressable via ActivateTab).
- Closing a tab whose folder has background operations: allowed; the job continues.

---

## 7. Quick search (F-066, F-067, F-069, F-070, F-274, F-275, F-286, F-290, F-423, F-419)

### 7.1 Trigger modes (`AltSearch` in `[Configuration]`)

Configuration > Quick search page (TC 8+; before that in Operation), group `Quick search (current dir)`:

- `( ) Ctrl+Alt+Letters` — value 0 (TC default). Plain letters go to the command line (F-275).
- `( ) Alt+Letters` — value 1. Alt+F-keys and menu accelerators: menu access then needs F10 / Alt alone.
- `( ) Letter only` — value 2. Typing letters starts quick search; the command line gets text only after clicking into it or via Ctrl+Enter / Ctrl+P etc.
- `( ) Letter, with search dialog`? — value 3 **(verify label)**: letter only, with the search box always visible.
- `( ) Off` (no quick search) — value 4 **(verify)**.

mormiCommander default **(ours)**: Letter only on macOS? Keep TC default Ctrl+Alt on all platforms per FEATURES, but on macOS Ctrl+Option is awkward; F-067 says follow config. Decision for UI: default `Ctrl+Alt+Letters` (TC parity), with the onboarding hint to switch.

Rules:
- Matching uses the physical key's character from the current layout, not the composed character from Alt/Option (F-286). AltGr combinations (Ctrl+Alt on Windows layouts that produce a character, e.g. `\` on Hungarian) must not start quick search when they produce a printable character the user is typing into the command line (F-290). TC handles this by checking whether the key combination produces a character in the layout **(verify)**.
- Digits and punctuation start quick search too (`_`, `-`, `.`); Space does not (Space selects). TC 11: Space can be part of the search string once the box is open (fixed in 10.x after "spaces no longer work" reports) **(verify)**.
- Starting characters that cannot match any entry: the box does not open and nothing happens (TC beeps? no sound) **(verify)**.

### 7.2 The search box

- A small single-line edit box appears at the bottom of the active panel, overlapping the status bar area, left-aligned under the file list, with a magnifier/no icon **(verify)**. In TC 8+ it may also show a small filter button (funnel) at its right edge to toggle filter mode.
- The typed character appears; the cursor jumps to the first entry (from the top of the list, not from the current position **(verify: TC searches from the top)**) whose name matches.
- Each further character narrows the match. If the extended text matches nothing, the character is not accepted (the box keeps the previous text and the cursor does not move) **(verify; TC 11 behaviour: text is kept and cursor stays — character rejected)**.
- Backspace removes the last character and re-searches from the top.

### 7.3 Keys while the box is open

| Key | Effect |
|---|---|
| letters/digits/punctuation (with the configured modifier or none) | append and search |
| Backspace | remove last char |
| Down / Ctrl+S? | next matching entry (wraps? **(verify: no wrap)**) |
| Up | previous matching entry |
| Ctrl+Down / Ctrl+Up | ? **(verify; TC 9: first/last match)** |
| Enter | close box and execute Enter on the entry under the cursor (open folder / run file) |
| Esc | close box; cursor stays where it is |
| Tab | close box; switch panel? **(verify: closes box only)** |
| Left / Right / Home / End | edit within the box text? TC: close box and act on list **(verify)** |
| Insert / Space | select entry under cursor and keep the box? TC 11: Insert marks and moves to next match **(verify)** |
| Ctrl+S | toggle filter mode (only when `QuickSearchAutoFilter` allows), see 8 |
| PgUp / PgDn | close box and page the list **(verify)** |
| Any F-key or other hotkey | close box, then run the hotkey |
| Mouse click anywhere | close box |

The box also closes after a directory change, when the panel loses focus, or on reread? (keeps text across reread **(verify)**).

### 7.4 Matching rules (`QuickSearchExactMatch` and friends)

Group `Exact name match` on the Quick search page **(verify labels)**:

- `[x] Beginning (name begins with search string)` — when checked (default), the typed text must match from the start of the name. When unchecked, the text can match anywhere in the name (implicit `*` in front).
- `[ ] Ending (extension begins with search string after dot)` — affects typing a `.`: when **unchecked** (default), the dot splits the search into a name part and an extension part, each matched as a prefix: `te.tx` finds `Test.txt`. When **checked**, the name before the dot must match exactly: `te.tx` only finds `te.txt`.
- Checking both = exact match from beginning to end; typing `*` overrides for one search.
- Ini: `QuickSearchExactMatch` historically 0/1 (1 = exact), TC 8+ split into bits: 1 = beginning, 2 = ending **(verify exact encoding; also `QuickSearchMatchBeginning` may exist)**.

Further matching rules:
- Case-insensitive always.
- `*` matches any run of characters, `?` exactly one (F-069). A `*` at the start disables "beginning".
- Search is on the displayed name; in Brief/Full with separate extension the name+ext are matched as `name.ext`.
- Folders: names matched without brackets. Option `Search for: ( ) files and directories ( ) only files ( ) only directories`? TC 11 has `[x] Search also in directories`? **(verify)** — FEATURES F-070 requires "files and folders / only folders"; implement a combobox `Files and folders` (default) / `Only files` / `Only folders`.
- `..` never matches.
- Accents: option `[ ] Ignore accents (e.g. a matches á)`? TC 10+ **(verify)**. Chinese Pinyin first letters: `[ ] Pinyin: allow searching for first letters` (TC 9+, `QuickSearchPinyin`?) — low priority (ours: implement accent folding, skip Pinyin).
- Branch view: matches the file name part, not the relative path **(verify)**.
- Thumbnails/tree: same rules on the displayed names.

### 7.5 Edge cases

- Empty folder: box opens but no match: per 7.2 the first char is rejected so the box shows empty or doesn't open.
- Very large folders (100k entries): search must stay < 50 ms per keystroke; precompute folded names.
- Search wraps when started below? Not applicable (always from top).
- Quick search in dialogs (copy dialog file list, Find files result list) uses its own simpler rules — not this spec.

---

## 8. Quick filter (F-068, F-780, F-781, F-801)

### 8.1 Behaviour

- `Ctrl+S` opens the quick search box in **filter mode** (the box shows a funnel symbol or the label "Filter:" **(verify)**). Typing filters the list live: only entries matching the text (rules of 7.4, but always "match anywhere" **(verify: TC filter uses the same Beginning setting?)**) remain. Folders are filtered too unless the option "Filter: do not hide directories" **(verify)** is set; `..` always stays.
- `Ctrl+Shift+S` opens filter mode with the last filter text pre-filled and applied (F-780).
- While typing, the cursor stays on the first matching entry; Up/Down move within the filtered list.
- `Enter`: closes the box but **keeps** the filter active; the current dir bar shows the filter as part of the mask, e.g. `C:\Windows\*abc*` **(verify)**; the status bar counts only visible entries. Then Enter in the list opens the entry normally.
- `Esc` while typing: closes the box and removes the filter **(verify: TC 11 Esc keeps? Most reports: Esc removes filter)**.
- With the filter active and the box closed: `Ctrl+F10` (All files) removes it (F-781); changing directory removes it; Ctrl+R keeps it **(verify)**; switching tabs keeps it on the tab.
- Selecting files under a filter: Ctrl+A / Num+ select only visible entries. Hidden-by-filter entries keep their previous selection state but are not counted? TC: selection of hidden entries is cleared when filtering **(verify)**. Ours: operations act only on visible selected entries; we clear selection of entries that become hidden, and say so in docs.
- Filter text that matches nothing: allowed (unlike quick search) — the list shows only `..` and a status of 0 files.
- Quick search vs. quick filter toggle: in the open box, Ctrl+S switches between search and filter modes (when `QuickSearchAutoFilter` allows).

### 8.2 Options and ini

- `QuickSearchAutoFilter` (`[Configuration]`): 0 = quick search never filters (Ctrl+S needed), 1 = the quick search box starts in filter mode automatically and the state (on/off) is saved on exit, 2 = like 1 but state not saved (filter always on at start), -1 = like current value but don't save the toggled state on exit. Default 0 **(verify value semantics; from forum posts by the author)**.
- Quick search page group `Quick filter` **(verify labels)**: `[ ] Quick search starts as filter (Ctrl+S toggles)` (= AutoFilter), `[ ] Filter also directories`, `[ ] Remember filter until directory change` .
- Last filter text kept in memory per panel for Ctrl+Shift+S; not saved across restarts **(verify)**.

---

## 9. Interactions between views, filters, sorting and selection

- View change, sort change, filter change: keep the cursor on the same entry if it is still listed; otherwise move to the nearest following entry in the new order, else the last entry.
- Selection survives view and sort changes. Entries hidden by a display filter (Ctrl+F11/F12) lose their selection **(verify)**; entries hidden by a quick filter: see 8.1.
- Calculated folder sizes survive view/sort changes but not rereads.
- Status bar recomputes after every change.
- Reread (Ctrl+R / F2 when configured): rebuilds the listing with the same view, sort, filter; keeps selection by name; keeps cursor by name.
- Auto-refresh while a quick search box is open: list updates, box stays open, current match re-evaluated **(verify)**.

---

## 10. Complete hotkey list for this area

All keys are by physical key on every platform (FEATURES "Hotkey rule"). macOS aliases from F-285 are additional.

| Key | Command | Effect | IDs |
|---|---|---|---|
| Tab | — | switch active panel | F-002, F-272 |
| Shift+Tab | — | file list <-> separate tree | F-769, F-807 |
| Ctrl+U | cm_Exchange | swap panels | F-004, F-252 |
| Ctrl+Shift+U | cm_ExchangeWithTabs | swap with tabs | F-005, F-253 |
| Ctrl+Left / Ctrl+Right | cm_TransferLeft / cm_TransferRight | folder under cursor to left/right panel | F-007 |
| Ctrl+F1 | cm_SrcShort | Brief | F-079, F-231, F-393 |
| Ctrl+F2 | cm_SrcLong | Full | F-080, F-231, F-394 |
| Ctrl+Shift+F1 | cm_SrcThumbs | Thumbnails | F-083, F-233, F-396 |
| Ctrl+Shift+F2 | cm_SrcComments | Comments | F-085, F-234, F-395 |
| Shift+F1 | cm_SrcCustomViewMenu | custom column set menu | F-084, F-211, F-671 |
| Ctrl+F3 | cm_SrcByName | sort by name / reverse | F-107 |
| Ctrl+F4 | cm_SrcByExt | sort by extension | F-108 |
| Ctrl+F5 | cm_SrcByDateTime | sort by date | F-109 |
| Ctrl+F6 | cm_SrcBySize | sort by size | F-110 |
| Ctrl+F7 | cm_SrcUnsorted | unsorted | F-111 |
| Ctrl+F8 | cm_SrcTree | tree in panel | F-081, F-398 |
| Ctrl+Shift+F8 | cm_SwitchSeparateTree | cycle separate tree | F-279, F-400 |
| Ctrl+F10 | cm_SrcAllFiles | all files (also clears quick filter) | F-088, F-781 |
| Ctrl+F11 | cm_SrcExecs | programs only | F-088 |
| Ctrl+F12 | cm_SrcUserSpec | user filter | F-088 |
| Ctrl+Q | cm_SrcQuickview | quick view toggle | F-082, F-248 |
| Ctrl+B | cm_DirBranch | branch view toggle | F-086, F-236 |
| Ctrl+Shift+B | cm_DirBranchSel | branch of selection | F-087, F-803 |
| Ctrl+Z | cm_EditComment | edit file comment | F-257 |
| Ctrl+R | cm_RereadSource | reread | F-064 |
| Ctrl+T | cm_OpenNewTab | new tab | F-027, F-251 |
| Ctrl+Shift+T | cm_OpenNewTabBg | new tab in background | F-773, F-800 |
| Ctrl+Up | cm_OpenDirInNewTab | folder in new tab | F-028, F-258 |
| Ctrl+Shift+Up | cm_OpenDirInNewTabOther | folder in new tab, other side | F-029 |
| Ctrl+W | cm_CloseCurrentTab | close tab | F-030, F-255 |
| Ctrl+Shift+W | cm_CloseAllTabs | close other tabs | F-031 |
| Ctrl+Tab / Ctrl+Shift+Tab | cm_SwitchToNextTab / cm_SwitchToPreviousTab | cycle tabs | F-032, F-281 |
| Ctrl+Alt+letter (default) / Alt+letter / letter | — | quick search | F-066, F-274 |
| Ctrl+S | — | quick filter | F-068, F-250 |
| Ctrl+Shift+S | — | quick filter with last text | F-780, F-801 |
| Ctrl+H (ours) / Cmd+Shift+. (macOS) | cm_SwitchHidSys | toggle hidden/system | F-090, F-242 |
| Ctrl++ / Ctrl+- (ours) | — | zoom panel font | F-101 |

macOS collisions (F-289): Ctrl+Up (Mission Control), Ctrl+Left/Right (Spaces), Ctrl+F2 (menu bar focus) are often taken by the OS; offer Cmd-based alternates documented in the hotkey docs.

---

## 11. Mouse actions summary for this area

| Where | Action | Effect |
|---|---|---|
| panel anywhere | left click | activate panel; list click moves cursor (+ selection per mouse mode, area 3) |
| list | double-click entry | Enter on it |
| list empty area | double-click | parent folder if option on (F-072) |
| list | wheel | scroll vertically (Brief: horizontally **(verify)**) |
| list | mouse button 4 / 5 | history back / forward (F-073) |
| list, folder | middle click | open in new tab **(verify)** |
| header caption | click | sort / reverse (5.2) |
| header caption | Ctrl+click | add secondary sort key (F-114) |
| header separator | drag | resize column |
| header separator | double-click | auto-fit **(verify)** |
| header | right-click | column set menu (3.2) |
| splitter | drag / right-click / double-click | resize / percentage menu / 50% (1.3) |
| tab | click / double / middle / right / drag | activate / close (opt.) / close / menu / reorder or move (6.3) |
| tab bar empty | double-click | new tab |
| tab | drop files | copy/move into tab folder |
| current dir bar | click | edit path (or breadcrumb segment) — area 3 |
| status bar | click | activate panel only |
| thumbnails | Ctrl+wheel | thumbnail size **(verify)** |

---

## 12. Error and confirmation messages in this area

| Situation | TC 11 behaviour | Message |
|---|---|---|
| Close locked tab | confirmation | `This tab is locked! Close it anyway?` **(verify)** Yes / No |
| Close all tabs with option on | confirmation | `Close all tabs?` **(verify)** Yes / No |
| Ctrl+W on last tab | nothing | — |
| Load tabs file not found / invalid | error | `Cannot open file <path>` / `Invalid tab file` **(verify)** |
| Restore tab whose folder is gone | silent fallback to nearest parent | — |
| Custom filter with invalid mask (empty) | treated as `*.*` | — |
| Comment edit on read-only folder | error | `Cannot write descript.ion` / OS error text **(verify)** |
| Custom column field unavailable (no value) | empty cell | — |
| Thumbnail generation fails | file-type icon remains | — |
| Quick search: no match | char rejected, no message | — |
| Quick filter: no match | empty list (only `..`) | — |
| Branch view on huge tree | progress in status/titlebar, Esc to stop **(verify)** | — |
| Tree view: drive not ready | `Drive not ready` / OS error **(verify)** | Retry / Cancel |

---

## 13. Persistence summary (our config must hold at least this)

Per panel and per tab: path, caption, lock mode + base path, view (+ custom set id / view mode), sort column(s) + directions, filter kind + userspec, history. Global: arrangement (`PanelsVertical`), splitter positions (normal / comments / quick view), visibility of every layout element (`[Layout]`), column widths per view, Brief width limit, icon options, colours, sort method (`SortUpper`), dir sort (`SortDirsByName`), hidden/system, ignore list, brackets, size/date formats, quick search mode (`AltSearch`), exact-match options, auto-filter (`QuickSearchAutoFilter`), folder-tab options (`DirTabOptions`, `DirTabLimit`, `LockedTabChange`, `TabEnterDirChange`, `LockedGoToDriveRoot`), thumbnail options (`ThumbWidthNoCache`, `ThumbHeightNoCache`, `ThumbsLocation`, `ThumbsCustomFieldsEnabled`, `PreloadThumbnails`, `ThumbsCopyDel`), custom column sets (`[CustomFields]`), view modes, separate tree mode and width.

wincmd.ini keys referenced in this document (importer map, F-446):

| Section | Key | Meaning |
|---|---|---|
| Configuration | PanelsVertical | 0 side by side, 1 stacked |
| Configuration | Tree, OldStyleTree, Autotreechange | tree view options |
| Configuration | DirBrackets | brackets around folders |
| Configuration | ShowHiddenSystem | show hidden/system |
| Configuration | ShowParentDirInRoot | `..` in roots |
| Configuration | SizeStyle, SizeFooter, DirsInFooter | size formats, footer |
| Configuration | ShowCentury, ShowAMPM | date format |
| Configuration | Aligned extension | Brief extension alignment |
| Configuration | BriefViewWidthLimit | Brief column cap |
| Configuration | SortDirsByName, SortUpper | sorting |
| Configuration | AltSearch, QuickSearchExactMatch, QuickSearchAutoFilter | quick search/filter |
| Configuration | DirTabOptions, DirTabLimit, LockedTabChange, TabEnterDirChange, LockedGoToDriveRoot, TabDir | folder tabs |
| Configuration | IgnoreList?, IgnoreListFile? **(verify)** | ignore list |
| Configuration | PreloadThumbnails, ThumbnailMemoryLimit, ThumbOptions, Thumb*Types, ThumbWidthNoCache, ThumbHeightNoCache, ThumbsCopyDel, ThumbsCustomFieldsEnabled, ThumbsLocation, WmfAllowed | thumbnails |
| Configuration | CopyComments | comments carried by file ops |
| Layout | ButtonBar, CmdLine, CurDir, DirectoryTabs, DriveBar1, DriveBar2, DriveBarFlat, DriveCombo, HistoryHotlistButtons, InterfaceFlat, KeyButtons, StatusBar, TabHeader | layout elements |
| left / right | path, ShowAllDetails, show, SpecialView, userspec, sortorder, negative Sortorder, customsortorder | panel state |
| lefttabs / righttabs | n_path, n_options, n_caption, activetab, ... | tabs |
| `<resolution>` | x, y, dx, dy, maximized, Divider, DividerComments, DividerQuickView, Tabstops | window/splitter/columns |
| Tabstops | 0..5, AdjustWidth | default column widths |
| CustomFields | Widths n, Headers n, Contents n, Options n | custom column sets |
| Colors | ForeColor, BackColor, BackColor2, MarkColor, CursorColor, CursorText, InverseCursor, InverseSelection, ColorFilter n, ThumbnailFrame | colours |

---

## 14. Acceptance test outline (for the control-mode test suite, FEATURES section 40)

Each item is a scripted scenario the control mode can drive with a temp directory fixture:

1. Fixture `a.txt b.TXT file2 file10 _x .hidden dir1/ Dir2/`. Ctrl+F3 -> order `.. dir1 Dir2 _x a.txt b.TXT file2 file10` (natural sort, case-insensitive, hidden off). Ctrl+F3 again -> folders still first, files reversed.
2. Ctrl+F6 on files of sizes 1,3,2 -> 3,2,1; Ctrl+F6 again -> 1,2,3; equal sizes ordered by name.
3. Ctrl+F5 -> newest first.
4. Brief: Right moves by one column; Full: header click on Size sorts; Ctrl+click Date adds secondary key.
5. Quick search (letter-only mode) `fi` -> cursor on `file2`; Down -> `file10`; type `x` (no match) -> text stays `fi`; Esc -> box closed, cursor on `file10`.
6. Exact match off ("Beginning" unchecked): `10` -> `file10`. Dot rule: `a.t` -> `a.txt`.
7. Ctrl+S `file` -> only `.. file2 file10`; status `0 / 2 file(s)`; Enter keeps filter; Ctrl+F10 clears; Ctrl+Shift+S restores `file`.
8. Tabs: Ctrl+T, Ctrl+Tab wraps, lock tab then Enter on dir1 opens a new tab; Ctrl+W on locked asks; restart restores tabs, captions, locks, sort per tab.
9. Ctrl+U swaps paths/sort/view; Ctrl+Shift+U swaps tab sets.
10. Ctrl+B lists `dir1/x.txt` style relative names; Ctrl+B again returns.
11. Thumbnails: 3 PNGs show previews after load; arrows move by tile.
12. Custom column set with `[=tc.size.kbytes]` shows kB values and sorts numerically.
