# Area 3 spec: selection model and the main menus (Files, Mark, Commands, Net, Show, Configuration, Start)

This document describes how real Total Commander 11.x behaves for marking (selecting) files and for every item of the main
menus Files, Mark, Commands, Net, Show, Configuration and Start. It is written for the agents that build the mormiCommander
UI, the command registry and the panel model, so that they can implement TC behaviour without having TC at hand. Each rule
says what TC does, what mormiCommander must do (which is the same thing unless a platform reason says otherwise), the
matching FEATURES.md IDs, and how sure we are about the detail. The Help menu (F-451..F-456) and the dialogs owned by other
areas (Copy/Move, Delete, Multi-Rename, Find, Sync, Lister, FTP engine) are only referenced here, not specified.

## 0. How to read this spec

### 0.1 Confidence tags

Total Commander's help file (TOTALCMD.CHM) is not freely mirrored online, the forum blocks automated reading, and several
English strings changed in 11.01 ("select" became "mark" in many places). So every rule carries one tag:

| Tag | Meaning | What the implementer does |
|-----|---------|---------------------------|
| **[S]** | Confirmed by a written source (ghisler.ch wiki pages "Selecting files", "Wincmd.ini", "Ini-settings", ghisler.com beginners guide, or forum threads quoted in search results). | Implement as written. |
| **[K]** | Long-standing TC behaviour known from the program and its keyboard.txt; very likely right for 11.x. | Implement as written. |
| **[?]** | Best recollection; label text, order or an edge case may differ in 11.x. | Implement as written (it is a sane default), but QA checks it against a real TC 11 install and fixes this file when it differs. |
| **[MC]** | A mormiCommander decision (platform mapping or a gap TC leaves open). | Implement as written; not a TC parity question. |

### 0.2 Terms

- **Panel**: one file list (left or right). The **source** (active) panel has keyboard focus; the other is the **target**.
- **Entry**: one row of a panel. The pseudo entry `..` (parent folder) is an entry but is **never** selectable. [K]
- **Cursor**: the focus rectangle on one entry (TC calls it the caret/cursor bar). Exactly one entry has the cursor while the
  list is not empty. Cursor and selection are independent. [K]
- **Selected / marked**: entries in the panel's selection set. Drawn in the mark colour (default red text, `[Colors]
  MarkColor=255`, which is BGR for pure red) or as inverse blocks when `[Colors] InverseSelection=1`. [S]
- **Operation set**: the set a file command works on: all selected entries if at least one is selected, otherwise the entry
  under the cursor if it is not `..`, otherwise nothing ("No files selected!" message, see 1.12). [K]
- **Folder rule**: the setting that decides whether group commands touch folders. `[Configuration] MarkDirectories=0`
  (default: files only) or `1` (files and folders). Options page: Configuration > Options > Operation, group "Selection". [S]
- **Mouse mode**: `[Configuration] UseRightButton=1` (right button selects, TC/Norton style) or `0` (left button selects,
  Windows style). [S] The TC 11 installer offers the choice; a fresh ini with no key behaves as left-button mode on current
  versions. [?] mormiCommander default: left-button mode (Windows/macOS habit). [MC]

### 0.3 The macOS key rule (from FEATURES.md, repeated where it matters here)

TC keys are bound by physical key on all three platforms. On keyboards without a numeric keypad (every MacBook), the keys
`Shift+=`, `Shift+-` and `Shift+8` act as Num+, Num- and Num*, and `/` (unshifted, US position) acts as Num/, but **only
while the command line is empty and quick search is not active**; with text in the command line they type the character.
[MC] Space replaces Ins where there is no Ins key. Cmd+A is an alias of Ctrl+A on macOS. [MC]

## 1. Selection model

### 1.1 Data model (implementation-ready) [MC]

Each panel (per folder tab) keeps:

```
type PanelSelection struct {
    Selected   map[EntryKey]bool // EntryKey = name as listed (case-preserving); ".." never present
    Anchor     int               // row index where the current Shift-range started; -1 = none
    Saved      []string          // names stored by cm_SaveSelection or automatically before an operation
    DirSizes   map[EntryKey]int64 // sizes computed by Space / Alt+Shift+Enter, shown instead of <DIR>
}
```

Rules that follow from TC behaviour:

1. Selection is per panel **and per tab**: switching tabs keeps each tab's own set. [K]
2. Selection survives a reread (F2 / Ctrl+R / auto refresh) by name: names that still exist stay selected, vanished names
   drop out. [K]
3. Changing folder (Enter, Backspace, Ctrl+PgUp, hotlist, cd ...) clears the selection of that panel. The cleared set is
   **not** kept for Num/ (Num/ restores the set saved before the last file operation, see 1.8). [K]
4. Filters (Ctrl+F11, Ctrl+F12, quick filter) hide entries; hidden entries are deselected when they disappear and all group
   commands only touch visible entries. [K] [?] for whether TC 11 keeps hidden entries marked through a quick filter; we
   drop them. [MC]
5. After a file operation (copy, move, delete, pack, attribute change ...) every entry that was processed successfully is
   deselected; entries that were skipped or failed stay selected, so the user can retry. [K]
6. Before any file operation starts, the current selection is stored in `Saved` (overwriting the previous one). [S]
7. The status bar is recalculated on every change (1.11).

### 1.2 Keyboard: single entries

| Key | Command | Behaviour | FEATURES |
|-----|---------|-----------|----------|
| Ins | (built-in, no cm_ name needed; registry name `cm_SelectToggleDown` [MC]) | Toggle the entry under the cursor, then move the cursor down one row. On the last row it toggles and stays. On `..` it does not toggle but still moves down. [K] | F-118 |
| Space | (built-in; `cm_SelectToggle` [MC]) | Toggle the entry under the cursor. The cursor does **not** move unless `[Configuration] SpaceMovesDown=1`. [S] When the toggled entry is a folder becoming selected and the "calculate space" option is on, the folder size is computed (1.3). With text in the command line Space types a space instead. [K] On `..` Space does nothing. [?] | F-118, F-736, F-797 |
| Shift+Up / Shift+Down | built-in | Range step, see 1.4. [S] | F-119 |
| Shift+PgUp / Shift+PgDn | built-in | Range by a page. [K] | F-119 |
| Shift+Home / Shift+End | built-in | Range to first / last entry. [S] | F-119 |
| Shift+Left / Shift+Right | built-in | In Brief view (columns) moves one column with range marking; in Full view same as Shift+PgUp/PgDn. [?] | F-119 |

### 1.3 Space on a folder: size calculation [S]

- Option: Configuration > Options > Operation, checkbox in the "Selection" group reading approximately "Calculate space
  occupied by subdirectories when selecting with Space" [?]. ini: `[Configuration] CountSpace=1` (default on). [S]
- When on: selecting a folder with Space starts a size scan of that folder (recursive, follows nothing special: symlinks /
  junctions are not followed [K]). While scanning, the size column shows the progress number; Esc aborts the scan (the
  folder stays selected, size column goes back to `<DIR>`). [K] The result replaces `<DIR>` in the size column and is
  included in the status-bar selected total. Deselecting with Space keeps the computed size visible until reread. [K]
- Toggling with Ins never calculates. [K] Selecting with mouse never calculates. [K]
- Large folders: the scan runs without blocking the UI; further key presses are queued (TC processes them after the scan,
  mormiCommander may run scans in the background and accept keys at once). [MC]
- Alt+Shift+Enter (`cm_CountDirContent`) computes sizes of **all** folders in the panel without changing the selection. [K]
  (Owned by Area for navigation; listed for completeness, F-736.)

### 1.4 Shift range marking (keyboard) — exact algorithm [K]/[?]

TC does not keep a Windows-style anchor for keyboard ranges; it paints a state over the rows the cursor leaves:

```
on Shift+<move>:
    if this is the first Shift+move since the last non-Shift key:
        paintState = NOT isSelected(cursorRow)          // [?] decided from the row under the cursor
    rows = rows from cursorRow up to (but excluding) newRow, in move direction
    for r in rows: setSelected(r, paintState)            // ".." is skipped
    cursorRow = newRow
```

- Shift+Down on an unselected file selects it and moves down; repeating extends; Shift+Up back over the same rows deselects
  them (because paintState is re-derived when direction changes and the row under the cursor is now selected). [?]
- Shift+Home / Shift+End paint from the cursor row **inclusive** to the first / last row **inclusive** (the end row gets
  painted too because the cursor cannot move further). Already-marked files in the range get the paint state, which the
  wiki words as "files already selected in that range are deselected" when the start row was selected. [S]
- Shift+PgDn paints the cursor row and every row passed; the row landed on is not painted. [?]

### 1.5 Numeric keypad and group commands

Keys follow TC 11 keyboard.txt. "Folder rule" = MarkDirectories (0.2). All group commands skip `..`.

| Key | cm_ command | What it does | FEATURES |
|-----|-------------|--------------|----------|
| Num + | `cm_SpreadSelection` | Opens the **Select files** dialog (1.6) and **adds** matching entries (files, plus folders when MarkDirectories=1). [S] | F-120, F-342 |
| Shift+Num + | `cm_SelectBoth` | Same dialog, but the folder rule is inverted for this call: folders included when MarkDirectories=0, files only when 1. [K] | F-791 |
| Num - | `cm_ShrinkSelection` | Opens **Unselect files** dialog (same layout) and **removes** matching entries (folder rule as Num +). [S] | F-121, F-343 |
| Shift+Num - | `cm_UnselectFiles` | Unselect dialog that only ever removes files; folders keep their state. [K] | F-792 |
| Ctrl+Num + / Ctrl+A | `cm_SelectAll` | Selects all entries (files; folders per folder rule). No dialog. [S] | F-122, F-344 |
| Ctrl+Shift+Num + | `cm_SelectAllBoth` | Selects all files **and** folders regardless of the rule. [K] | F-793 |
| Ctrl+Num - | `cm_ClearAll` | Deselects everything (files and folders). [S] | F-123, F-345 |
| Ctrl+Shift+Num - | `cm_ClearFiles` | Deselects files only; folders keep their state. [K] | F-794 |
| Num * | `cm_ExchangeSelection` | Inverts: files flip; folders flip only when MarkDirectories=1, otherwise folders keep their state. [S] | F-124, F-346 |
| Shift+Num * | `cm_ExchangeSelBoth` | Inverts files and folders. [K] | F-125 |
| Alt+Num + | `cm_SelectCurrentExtension` | Adds all entries with the same extension as the entry under the cursor (case-insensitive). A file without extension selects all files without extension. On a folder: uses the folder's "extension" (text after the last dot) and selects folders only if the folder rule allows. [K]/[?] | F-126, F-347 |
| Alt+Num - | `cm_UnselectCurrentExtension` | Removes those. [S] | F-127 |
| Num / | `cm_RestoreSelection` | Restores `Saved` (1.8). [S] | F-131, F-348 |
| — | `cm_SelectCurrentName` / `cm_UnselectCurrentName` | Add / remove all entries whose base name (without extension) equals the cursor entry's base name. [K] | F-128 |
| — | `cm_SelectCurrentNameExt` / `cm_UnselectCurrentNameExt` | Same name + extension (useful in branch view / search results where duplicates appear). [K] | F-129 |
| — | `cm_SelectCurrentPath` / `cm_UnselectCurrentPath` | In branch view and search results: entries in the same folder as the cursor entry. [K] | F-129 (note) |
| — | `cm_SelectFiles` / `cm_SelectFolders` | Select-group dialog limited to files / to folders. [K] | F-130 |
| — | `cm_SelectAllFiles` / `cm_SelectAllFolders` | Select all files only / all folders only, no dialog. [K] | F-130 |
| — | `cm_ClearFolders` | Deselect folders only. [K] | F-795 |
| — | `cm_ExchangeSelFiles` / `cm_ExchangeSelFolders` | Invert files only / folders only. [K] | F-795 |
| — | `cm_SaveSelection` | Copy current selection into `Saved`. [S] | F-132, F-349 |
| — | `cm_SaveSelectionToFile` / `cm_LoadSelectionFromFile` | 1.9. [S] | F-132 |
| — | `cm_LoadSelectionFromClip` | 1.9. [S] | F-132 |
| — | `cm_SelectFromOtherPanel` (ours) | Selects the names that are selected in the target panel. [MC] | F-143 |

Group commands accept a **parameter** when called from a button, start-menu item or the command line (TC 9+): e.g.
`cm_SelectFiles *.txt`. In TC 11 the `cm_SelectFiles` / `cm_SelectFolders` / `cm_SelectBoth` family takes a mask
parameter and then skips the dialog and applies the mask directly. [?] mormiCommander: every
dialog-based select/unselect command accepts an optional mask parameter; with a parameter the dialog is skipped. [MC]

### 1.6 The "Select files" / "Unselect files" dialog (Num + / Num -)

Window: small modal dialog, not resizable, centred on the main window. [K]

| # (tab order) | Control | Label / content | Default | Notes |
|---|---------|-----------------|---------|-------|
| 1 | Editable combo box | (no separate label; dialog caption says "Select files" or "Unselect files" — TC 11.01+ English may say "Mark files"/"Unmark files" [?]) | The most recently used mask (first history entry), fully selected so typing replaces it; `*.*` when history is empty. [K] | Drop-down lists the history (newest first, max 20 [?]) followed by saved search templates (see below). |
| 2 | Button | "OK" (default button, Enter) | — | Applies the mask and closes. |
| 3 | Button | "Define..." [?] | — | Opens Find Files in template mode; a template saved there becomes available in the combo. [S] ("Predefined filters can be saved with the define button. They support the same options as Search.") |
| 4 | Button | "Cancel" (Esc) | — | Closes without changes; mask not added to history. |
| 5 | Button | "Help" (F1) | — | Opens help page for selecting. |

Storage: history in `[Selection]` section of wincmd.ini as `0=<newest>`, `1=...` [S] (the wiki: "stores the last wildcards
used for selection (Gray+)"). Both the select and unselect dialogs share this history. [K] Section may be redirected with
`RedirectSection=` to a history ini. [S]

#### Mask syntax (shared with Ctrl+F12 filter and Find Files "Search for") [S]

1. Several masks separated by spaces (or `;` [K]). A mask containing spaces must be in double quotes: `"my file*.txt"`. [K]
2. `|` splits include masks (left) from exclude masks (right): `w*.*|*.bak *.old` selects files starting with `w` that do
   not end in .bak or .old; `|*.exe` selects everything except .exe files. [S] An empty left side means `*.*`. [S]
3. Wildcards: `*` any run of characters (including dots), `?` exactly one character. `*.*` matches every name, including
   names without a dot (DOS semantics). `*.` matches names without extension. `*` alone also matches everything. [K]
4. A mask ending in a path separator (`\` in TC, `/` or `\` accepted in mormiCommander [MC]) applies to folders only, e.g.
   `*.*|.git\ node_modules\` excludes those folders when folders are being selected. [K]
5. Regular expressions: the wiki states that an expression starting with `<` is treated as a regex. [S] (Verify the exact
   prefix in TC 11; the Find Files dialog uses a "RegEx" checkbox instead. [?]) mormiCommander supports `<regex` with Go
   RE2 syntax, case-insensitive by default. [MC]
6. Matching is case-insensitive in TC (Windows). mormiCommander: case-insensitive on all platforms for parity, even on
   case-sensitive Linux file systems. [MC]
7. Choosing a **search template** from the combo selects entries matching all template criteria (name masks, date, size,
   attributes, even text content). Template names are shown in the list after the history; in the edit field a template is
   displayed by its name. [S]/[?] To tell templates from masks mormiCommander prefixes them with `>` in the drop-down (`>Big
   videos`). [MC]
8. Empty mask + OK: nothing happens, nothing is stored. [?]
9. Invalid regex: message box "Invalid regular expression: <detail>" and the dialog stays open. [MC]

#### Results

- Select adds to the existing selection; it never clears first. Unselect only removes. [K]
- Folders follow the folder rule of the command that opened the dialog (1.5). [S]
- If nothing matched: no message, no change (TC is silent). [K]
- The mask is pushed to the front of `[Selection]` history (duplicates removed). [K]

### 1.7 Mouse selection [S] unless tagged

Option page: Configuration > Options > Operation, group "Mouse selection mode" with two radio buttons, approximately
"( ) Use right mouse button (as in Norton Commander)" and "( ) Use left mouse button (as in Windows)" [?]. ini
`[Configuration] UseRightButton=0|1`. [S] FEATURES F-135, F-136, F-419, F-823.

#### Left-button mode (Windows style, UseRightButton=0)

| Action | Result |
|--------|--------|
| Click on an entry | Moves the cursor there. If other entries were selected, they are deselected and the clicked one is selected (a click always leaves exactly the clicked entry marked, except on `..`). [?] — TC 11 has the option to not mark the single clicked file; mormiCommander follows Windows: single click = cursor + exclusive selection of that entry, and the operation set is the same either way. [MC] |
| Ctrl+click (Cmd+click on macOS) | Toggles that entry, moves cursor; other entries unchanged. Ctrl+click on a selected entry deselects it. [S] |
| Shift+click | Selects the range from the cursor (anchor) to the clicked entry inclusive; other entries outside the range are deselected. [S]/[?] |
| Ctrl+Shift+click | Adds the range to the existing selection. [?] |
| Press on a selected entry and drag | Starts drag & drop of the whole selection (Area drag & drop, F-698..). [K] |
| Press on an unselected entry and drag | Drag & drop of that one entry (it becomes the only selected one). [K] |
| Click on empty area below the last row | Cursor goes to the last entry, selection unchanged. [?] |
| Double-click | Opens / enters (same as Enter). [K] |
| Click twice slowly on the **name** of the cursor entry | In-place rename (`[Configuration] InplaceRename=1`, default on). [S] |
| Right-click | Moves cursor and opens context menu; if the clicked entry is not selected the selection becomes just that entry first. [K] |

#### Right-button mode (TC/Norton style, UseRightButton=1)

| Action | Result |
|--------|--------|
| Left click | Moves cursor only; selection unchanged. [S] |
| Right click | Toggles the entry; selections persist without Ctrl. Right click on a selected entry deselects it. [S] |
| Right button held and dragged over entries | Paints the new state of the first entry over every entry passed (select run or deselect run). [S] |
| Right button held still (~0.5 s) | Opens the context menu of the selection (or of the entry if nothing selected). [K] Hold time is fixed in TC; mormiCommander uses 500 ms. [MC] |
| Shift+F10 / context-menu key | Context menu without mouse. [K] |
| Shift+click, Ctrl+click (left) | Work as in left mode, so users can use both. [K] |
| Left drag | Drag & drop (selection if the pressed entry is selected, otherwise that entry). [K] |

macOS trackpads have no separate right button, and Ctrl+click is the TC toggle, so in right-button mode on macOS the
context menu opens with a two-finger click held for 500 ms, or with Shift+F10. A short two-finger click toggles. [MC]

### 1.8 Restore selection (Num /) [S]

- `Saved` is written automatically right before every file operation and explicitly by `cm_SaveSelection`. It is **not**
  written by Ctrl+Num -, folder changes or rereads. [S] ("The selection is also saved automatically before each file
  operation.")
- Num / selects every entry of the current panel whose name is in `Saved` (adds to the current selection). It works after
  changing folder too: names are matched in whatever folder is shown, which is handy for applying the same names to a
  second folder. [K]
- Saved set is per panel (left/right), not per tab. [?] mormiCommander keeps it per panel. [MC]
- Empty `Saved`: nothing happens, no message. [K]

### 1.9 Save / load selection to file and clipboard [S]

| Command | Dialog | File format |
|---------|--------|-------------|
| `cm_SaveSelectionToFile` | Standard Save dialog, title "Save selection to file" [?], filter "Text files (*.txt)\|*.txt\|All files\|*.*", default name `selection.txt` [?], initial folder = current panel folder. Existing file: OS overwrite prompt. | One name per line (name only, no path; folders without trailing separator), CRLF on Windows, LF elsewhere [MC]; UTF-8 without BOM on macOS/Linux, UTF-8 **with** BOM on Windows for TC compatibility [MC] (TC 11 writes UTF-8 with BOM when names need it, ANSI otherwise [?]). |
| `cm_LoadSelectionFromFile` | Standard Open dialog. | Reads lines; each line may be a bare name or a full path (only the last path component is used when its folder is the current folder; full paths of other folders are ignored [?]). Blank lines ignored, leading/trailing spaces trimmed, BOM accepted. Adds to current selection. Names not present are silently ignored. |
| `cm_LoadSelectionFromClip` | none | Same parsing on clipboard text. Empty clipboard or non-text: nothing happens. |

### 1.10 Copy names to clipboard (Mark menu block) [K]

Operation set as in 0.2 (selection, or cursor entry). Order = panel display order. Lines joined with CRLF on Windows, LF on
macOS/Linux [MC] (TC: CRLF). No trailing newline after the last line when only one entry; TC adds CRLF after each line
except the last [?].

| Command | Line content | FEATURES |
|---------|--------------|----------|
| `cm_CopyNamesToClip` | `name.ext` | F-138, F-350 |
| `cm_CopyFullNamesToClip` | `C:\path\name.ext` (folders without trailing separator) | F-139 |
| `cm_CopyDetailsToClip` | `name.ext<TAB>size<TAB>date time<TAB>attributes` in the panel's display formats; folders show `<DIR>` (or computed size) | F-140 |
| `cm_CopyFullDetailsToClip` | same with full path | F-140 |
| `cm_CopyNetNamesToClip` | UNC form of the full name on Windows mapped drives; elsewhere the full path [MC] | F-142 |
| `cm_CopySrcPathToClip` / `cm_CopyTrgPathToClip` | source / target folder path, no trailing separator | F-141, F-796 |

In branch view and search results, the "names" commands copy the relative path shown in the list. [K]

### 1.11 Status bar text [K]

Per panel, below the list:

```
<selKB> k / <totalKB> k in <selFiles> / <totalFiles> file(s), <selDirs> / <totalDirs> dir(s)
```

Example: `12 k / 1,234 k in 2 / 12 file(s), 1 / 3 dir(s)`. Sizes use the size format from Configuration > Display
(bytes/k/dynamic). Folder sizes are included only when computed (1.3). The status bar of the cursor entry (when nothing is
selected) still shows the totals line; TC 11 can instead show cursor-entry details via tooltip. F-137, F-015.

### 1.12 Edge cases and messages [K]

| Situation | Behaviour / message |
|-----------|---------------------|
| File command with empty operation set (cursor on `..`, nothing selected) | Message box, caption "Total Commander" (ours: "mormiCommander"), text **"No files selected!"**, OK button, icon warning. |
| Group select in an empty folder | Nothing. |
| Selection in archive, FTP, search-result, branch view | Same rules; names are the displayed (relative) names. |
| Entry renamed / deleted externally, then auto-refresh | Dropped from selection silently. |
| Quick search active (typing) | Num+/Num-/Num* end quick search first and then act. [?] |
| Command line not empty | Space, `+`, `-`, `*`, `/` go to the command line; Num keys on a real keypad still select only when the command line is empty (TC types the character otherwise). [K] |
| Very large selection (100k entries) | Must remain O(n); status bar updates batched. [MC] |

## 2. Menus — general rules

1. Menu bar order in TC 11: **Files, Mark, Commands, Net, Show, Configuration, Start, Help**. [K] On macOS the app menu
   (mormiCommander > About, Settings... Cmd+, , Hide, Quit Cmd+Q) is added in front; Configuration > Options also stays in
   place. [MC]
2. Every item runs exactly one cm_ command (or opens a submenu). The right column shows the current hotkey taken from the
   live key map, so a redefined key appears in the menu. [K] FEATURES "same as menu" criterion.
3. The whole main menu can be replaced by a user `.mnu` file: `[Configuration] Mainmenu=<path>`; format (TC):
   `POPUP "&Files"` ... `MENUITEM "&Pack...\tAlt+F5", cm_PackFiles` ... `MENUITEM SEPARATOR` ... `END_POPUP`. [S]/[K]
   F-828.
4. `[Configuration] RestrictInterface` bit sum hides/disables parts: 1 Start menu, 2 command line, 4 button bar change,
   8 drive context menu, 16 file context menu, 32 hotlist change, 64 FTP (Ctrl+N, Ctrl+F), 128 Enter key, 256 file-system
   plugins. [S] Disabled items stay visible but greyed. [?]
5. Items that need a file under the cursor or a selection are **not** greyed in TC; they show "No files selected!" when
   invoked with an empty operation set. [K] mormiCommander does the same (keeps menus cheap to build). [MC]
6. Radio items (view mode, sort, filter) show a bullet on the current state of the **active** panel. [K]
7. F10 / Alt alone focuses the menu bar (Windows/Linux). [K] On macOS the system menu bar is used; F10 opens our menu via
   accessibility focus only. [MC]
8. Accelerators: the underlined letters listed below (`&`) are TC's English ones. [?]

## 3. Files menu (F-315..F-341)

| # | Label (TC 11 English) | Key | cm_ command | FEATURES |
|---|-----------------------|-----|-------------|----------|
| 1 | Change &Attributes... | — | cm_SetAttrib | F-315, F-316, F-338 |
| 2 | &Pack... | Alt+F5 | cm_PackFiles | F-318 |
| 3 | &Unpack Specific Files... | Alt+F9 | cm_UnpackFiles | F-319 |
| 4 | &Test Archive(s) | Alt+Shift+F9 | cm_TestArchive | F-320 |
| 5 | Compare &By Content... | — | cm_CompareFilesByContent | F-321 |
| 6 | Associate Wit&h... | — | cm_Associate | F-322 (N/A) |
| 7 | Internal Associations (TC only)... | — | cm_InternalAssociate | F-323 |
| 8 | Show File &Properties | Alt+Enter | cm_Properties | F-324 |
| 9 | Calculate &Occupied Space... | Ctrl+L | cm_GetFileSpace | F-325, F-737 |
| 10 | Multi-&Rename Tool... | Ctrl+M | cm_MultiRenameFiles | F-326 |
| 11 | Edit &Comment... | Ctrl+Z | cm_EditComment | F-327 |
| — | separator | | | |
| 12 | Print ▸ File &List... / File List With &Subdirs... / Current &File | — | cm_PrintDir / cm_PrintDirSub / cm_PrintFile | F-334, F-335 |
| — | separator | | | |
| 13 | &Split File... | — | cm_FileSplit | F-328 |
| 14 | Com&bine Files... | — | cm_FileCombine | F-329 |
| 15 | &Encode File... | — | cm_UUEncode | F-330 |
| 16 | &Decode File... | — | cm_UUDecode | F-331 |
| 17 | Create Checksum File(s) (CRC, MD5, SHA)... | — | cm_CRCCreate | F-332 |
| 18 | Verify Checksums (CRC, MD5, SHA)... | — | cm_CRCCheck | F-333 |
| — | separator | | | |
| 19 | &Quit | Alt+F4 | cm_Exit | F-336 |

Order and exact wording: [?] (structure [K]). Commands without a default menu entry but owned by this menu in
mormiCommander: Create Symbolic Link... (`cm_CreateSymbolicLink`, F-337), Create Hard Link... (`cm_CreateHardLink`,
F-337), Wipe... (`cm_Wipe`, F-339), Open With... (F-341). mormiCommander inserts them after item 11. [MC]

### 3.1 Change Attributes (cm_SetAttrib) [K] layout, [?] exact labels

Caption "Change attributes". Applies to the operation set. Modal.

| Tab | Control | Label | Default |
|-----|---------|-------|---------|
| 1 | Checkbox | "Process subdirectory contents" (recurse) | off; only enabled when the set contains a folder [?] |
| 2 | Group "Change attributes" — 3-state checkboxes | "Archive", "Read only", "Hidden", "System" (NTFS also "Compressed", "Encrypted" when supported [?]) | Single file: checked/unchecked from the file. Several files: grey (indeterminate) = leave unchanged, unless all files agree. |
| 3 | Checkbox | "Change date/time:" | off; enables the next three controls |
| 4 | Date edit (with drop-down calendar) | "Date:" | date of the cursor/first file |
| 5 | Time edit | "Time:" | time of that file, seconds included |
| 6 | Button | "Current" | sets date and time to now |
| 7 | Group "Plugin attributes" (TC 9+) | "Add attribute" button, rows with plugin combo / property combo / value edit / "Delete" | empty — mormiCommander: built-in fields only (creation date, access date, EXIF date as source "[=exif.date]") [MC] F-317, F-340 |
| 8 | Buttons | "OK" (default), "Cancel", "Help" | |

Semantics: checked = set, unchecked = clear, indeterminate = keep. With recursion, folders themselves are changed too. [K]
Errors per file: dialog "Error changing attributes of <name>!" with "Skip", "Skip all", "Retry", "Cancel" [?]. Successful
files are deselected (1.1 rule 5).

mormiCommander per-platform mapping [MC] (F-315, F-338):
- Windows: as TC.
- macOS/Linux: replace the attribute group with an octal box `Mode:` (e.g. `0644`) plus 3x3 checkboxes "Owner / Group /
  Others" x "Read / Write / Execute", 3-state for multiple files, plus "setuid / setgid / sticky"; "Hidden" checkbox maps
  to `chflags hidden` on macOS and is disabled on Linux (dot-files rule); Owner and Group combos (enabled only when running
  with permission, otherwise greyed with tooltip "Only root can change the owner"). Dates: "Modified", and on macOS
  "Created" (birth time via `setattrlist`), on Linux "Accessed".

### 3.2 Pack (cm_PackFiles, Alt+F5) — dialog owned by Packers (§29), summary only [K]/[?]

Caption "Pack files". "Pack N file(s) to the archive:" + editable combo with target path (`<target panel>\<name>.zip`;
name = the single file/folder name or the source folder name for several) + "Tree" button. Group "Packer": radio
zip / arj / lha / rar / uc2 / ace / tar / tgz / tbz / 7z (unavailable packers greyed; plugin packers in a combo "->"). Checkboxes:
"Also pack path names (only recursed)" (on), "Recursively pack directories" (on), "Multiple disk archive", "Move to
archive", "Create self extracting archive", "Encrypt", "Create separate archives, one per selected file/dir". Buttons "OK",
"Configure...", "Cancel", "Help". Ctrl held while choosing the menu item / Ctrl+Alt+F5 [?]: archive in the source folder;
Shift: "Move to archive" pre-checked. [S] (forum) Last choices in `[Packer]` section. [?]

### 3.3 Unpack Specific Files (cm_UnpackFiles, Alt+F9) [K]/[?]

Caption "Unpack files". "Unpack specific files from archive to:" + combo (target panel path) + "Tree". "Files to unpack:"
edit with `*.*`. Checkboxes "Unpack path names if stored with files" (on), "Overwrite existing files" (off), "Unpack each
archive to a separate subdir (name of the archive)" (off). Buttons OK / Cancel / Help. With several archives selected,
all are unpacked. Cursor on a non-archive: TC shows an error that the file is not an archive of a known type (exact
text [?]). mormiCommander message: "No supported archive selected." [MC]

### 3.4 Test Archive(s) (Alt+Shift+F9) [K]

No dialog. Progress window "Testing archive(s)". Ends with message box: success "No errors found in archive(s)." [?] or a
list window of errors per archive. Non-archives in the set are skipped silently. F-320.

### 3.5 Compare By Content [K]

Picks two files: (a) two selected in source panel; (b) one selected (or cursor) in each panel; (c) cursor file and a file
with the same name in the target panel. If only one file can be determined, an Open dialog asks for the second file
("Compare with..."). [?] Folders in the pair: TC refuses with a message asking for two files (exact text [?]); mormiCommander: "Please select two
files to compare." [MC] Opens the Compare
tool (§26, F-556).

### 3.6 Associate With / Internal Associations

- `cm_Associate` [N/A on macOS/Linux, F-322]: mormiCommander opens the OS "Open with / default app" chooser for the cursor
  file's extension (Windows: shell dialog; macOS: Get Info window via `open -R` + note; Linux: `xdg-mime` hint dialog). [MC]
- `cm_InternalAssociate` (F-323): dialog "Internal associations" with list of file types (masks) and buttons "Add...",
  "Edit...", "Delete", "Up", "Down"; each entry: "File type(s):" mask, "Description", "Icon", and actions "Open", "View",
  "Edit" each with "Command:" and "Parameters:". Stored in `[Associations]` in wincmd.ini. [?]

### 3.7 Properties (Alt+Enter)

Windows: shell Properties dialog of the operation set. macOS/Linux: our own Properties dialog (F-681) with "Show in
Finder/Get Info" button. [MC]

### 3.8 Calculate Occupied Space (Ctrl+L) [K]

Scans the operation set recursively (with progress, Esc cancels). Result in a message box, caption "Calculate occupied
space" [?], text:

```
Total: 1,234,567 bytes in 12 files, 3 directories
Occupied space on drive: 1,265,664 bytes (cluster size 4096 bytes)
```

Numbers with thousands separators. Computed folder sizes are also written into the size column (as Space does). [K]
F-325, F-737.

### 3.9 Edit Comment (Ctrl+Z) [K]

Caption "Edit comment". Label "Comment for file: <name>" [?], multi-line edit (Enter = OK, Ctrl+Enter = new line [?]),
OK / Cancel / Help. Saved to `descript.ion` in the same folder: one line per file, `name comment`; names containing
spaces in double quotes; multi-line comments stored with `\n` and the trailer char `\x04\xC2` (4DOS convention). [K]
Empty comment removes the line; an empty descript.ion is deleted. The file is hidden on Windows. With several files
selected: one dialog per file. [?] F-327.

### 3.10 Print

`cm_PrintDir` / `cm_PrintDirSub`: OS print dialog, prints the list in the current view's columns (with subfolders for the
second). `cm_PrintFile`: prints the cursor file via the associated program ("print" verb on Windows, `lp` on macOS/Linux).
[MC] F-334, F-335.

### 3.11 Split File [K]/[?]

Needs exactly one file (cursor or one selected). Folder or several files: "Please select a single file!" [?]. Caption
"Split file". "Split the file <name> to:" + combo (target panel path) + "Tree". "Bytes per file:" editable combo with
presets: "Automatic", "1457664 (3.5")", "98078k (ZIP 100)", "650 MB CD-R", "700 MB CD-R", "4095 MB (FAT32)", "4.7 GB DVD",
"8.5 GB DVD DL", "25 GB BD" — values accept suffix k/M/G. [?] TC always writes
`name.crc` alongside the parts [K]. Output `name.001`, `name.002`, ... and `name.crc` containing `filename=<name>`, `size=<bytes>`,
`crc32=<hex>`. [K] F-328, F-687.

### 3.12 Combine Files [K]

Cursor on `name.001` (or any part, or several parts selected). Caption "Combine files". "Combine the files to:" combo with
`<target>\name`. Parts are joined in numeric order; if `name.crc` exists, size and CRC are checked; mismatch gives a CRC
error message (exact text [?]); mormiCommander: "The combined file does not match the checksum in <name>.crc." Missing
part: "Missing part <name.00n>". [MC] F-329.

### 3.13 Encode / Decode

Encode: caption "Encode file(s)"; target combo + "Tree"; radio "MIME (Base64)", "UUE", "XXE"; "Split into parts of N lines"
edit (0 = no split) [?]. Decode: caption "Decode file"; target folder combo; format auto-detected (MIME, UUE, XXE, BinHex).
[K] F-330, F-331.

### 3.14 Create / Verify checksums [K]/[?]

Create: caption "Create checksum file(s)". "Create checksum file(s) for N file(s):" target name combo (`<folder>.md5` /
`<file>.md5`). Radio: "CRC32 (SFV format)", "MD5", "SHA1", "SHA256", "SHA512" (TC 11 also "SHA3" and "BLAKE3" [?];
mormiCommander adds BLAKE3 [MC]). Checkboxes "Create separate checksum file for each file" (off), "Unix format (LF line
endings)" [?], "UTF-8" [?]. Lines: `<hex> *<name>` (MD5/SHA, binary marker `*`) and `<name> <crc>` (SFV). Verify:
cursor/selection on checksum files; result window with per-file "OK" / "FAILED" / "FILE NOT FOUND" and summary
"N files OK, M failed, K not found"; button "Show only failed" [?]. F-332, F-333, F-689, F-690.

### 3.15 Quit (Alt+F4) [K]

Closes TC. If background operations run: "Background operations are still running. Abort them and quit?" [?] Yes/No.
Settings and positions saved per "Save on exit" options. macOS: Cmd+Q alias. F-336.

## 4. Mark menu (F-342..F-353)

| # | Label (TC 11 English, 11.01 may read "Mark ..." [?]) | Key | cm_ command | Spec | FEATURES |
|---|-------------------------------------------------------|-----|-------------|------|----------|
| 1 | Select &Group... | Num + | cm_SpreadSelection | 1.5, 1.6 | F-342 |
| 2 | &Unselect Group... | Num - | cm_ShrinkSelection | 1.5, 1.6 | F-343 |
| 3 | Select &All | Ctrl+Num + | cm_SelectAll | 1.5 | F-344 |
| 4 | U&nselect All | Ctrl+Num - | cm_ClearAll | 1.5 | F-345 |
| 5 | &Invert Selection | Num * | cm_ExchangeSelection | 1.5 | F-346 |
| 6 | Select All With Same &Extension | Alt+Num + | cm_SelectCurrentExtension | 1.5 | F-347 |
| 7 | &Restore Selection | Num / | cm_RestoreSelection | 1.8 | F-348 |
| — | separator | | | | |
| 8 | Save Selection | — | cm_SaveSelection | 1.8 | F-349 |
| 9 | Save Selection To File... | — | cm_SaveSelectionToFile | 1.9 | F-349 |
| 10 | Load Selection From File... | — | cm_LoadSelectionFromFile | 1.9 | F-349 |
| 11 | Load Selection From Clipboard | — | cm_LoadSelectionFromClip | 1.9 | F-349 |
| — | separator | | | | |
| 12 | Copy Selected Names To Clipboard | — | cm_CopyNamesToClip | 1.10 | F-350 |
| 13 | Copy Names With Path To Clipboard | — | cm_CopyFullNamesToClip | 1.10 | F-350 |
| 14 | Copy To Clipboard With All Details | — | cm_CopyDetailsToClip | 1.10 | F-350 |
| 15 | Copy To Clipboard With Path+Details | — | cm_CopyFullDetailsToClip | 1.10 | F-350 |
| — | separator | | | | |
| 16 | &Compare Directories | Shift+F2 | cm_CompareDirs | 4.1 | F-351, F-133 |
| 17 | Compare Directories With Subdirs [?] | — | cm_CompareDirsWithSubdirs | 4.1 | F-134 |
| 18 | Mark newer / hide equal (TC 11: via Show > "Only selected files" after compare) [?] | — | cm_ShowOnlySelected | 4.1 | F-352 |
| 19 | Feed search results back / select from list [?] | — | (see 4.2) | 4.2 | F-353 |

Items 1–16 [K] for presence; item order [?]; items 17–19 [?] (they exist as commands; whether TC 11 lists them in the
default Mark menu needs checking — mormiCommander lists them). [MC]

### 4.1 Compare Directories (Shift+F2) — algorithm [S] core, [?] details

TC help text (via forum): "Compares the source and target directories; newer files are selected, files missing in one
directory are also selected in the other." [S]

```
clear selection in both panels                                    // [K]
for each name N in union(left, right) (case-insensitive match; skip ".."):
    L, R = entry in left / right
    if only L:  select L         (folders too — "missing" folders are marked, TC 11 fixed RC3 bug)  [S]
    if only R:  select R
    if both are files:
        if L.mtime > R.mtime + tol: select L
        elif R.mtime > L.mtime + tol: select R
        elif L.size != R.size: select L and R                     // same time, different size [?]
        else: equal → nothing
    if both are folders: nothing (cm_CompareDirs); with cm_CompareDirsWithSubdirs compare recursively
                         and select the folder on each side whose subtree has a newer/missing file [K]
    if one is file and other folder: select both [?]
tol: 2 s when either side is FAT/exFAT/FTP/archive, else 0; plus exactly 3600 s ignored when
     [Configuration] 1hourdif=1 [S]
```

- Afterwards the cursor stays where it was; the status bar shows counts. [K]
- If no entry got selected: TC shows a short information message when
  nothing differs (exact text [?]); mormiCommander text: **"No differences found."** (Information icon, OK). [MC]
- Works with archives, FTP, search results and branch view listings (compares the displayed relative names). [K]
- After compare, Num * selects the equal files. [S] (wiki: invert is "useful after comparing directories")
- "Hide equal": `cm_ShowOnlySelected` (Show menu) then shows only the differing files; Ctrl+F10 or reread restores. [S]
- Both panels must show different folders; if they show the same folder every file is equal → the message above. [K]

### 4.2 Selecting from search results

Find Files > "Feed to listbox" puts results into a panel (search results view). Selecting there and pressing F5 works on
the original files. There is no separate Mark command for "select by search results" in TC; F-353 is satisfied by: in the
Find Files results list, the button "Go to file" and "Feed to listbox", and in the panel by `cm_LoadSelectionFromClip`
after "copy results". [MC] [?]

## 5. Commands menu (F-354..F-376)

| # | Label | Key | cm_ command | FEATURES |
|---|-------|-----|-------------|----------|
| 1 | Directory &Hotlist | Ctrl+D | cm_DirHotList | F-354, F-651 |
| 2 | Directory History [?] | Alt+Down | cm_DirectoryHistory | F-660 |
| 3 | &Search... | Alt+F7 | cm_SearchFor | F-356, F-524 |
| 4 | Search in separate process... | Alt+Shift+F7 | cm_SearchStandalone | F-357 |
| 5 | Volume &Label... | — | cm_VolumeId | F-359 |
| 6 | Synchronize dirs... | — | cm_SyncDirs | F-360, F-546 |
| — | separator | | | |
| 7 | Directory &Tree... (CD tree) | Alt+F10 | cm_CDtree | F-355 |
| 8 | &Branch View (With Subdirs) | Ctrl+B | cm_DirBranch | F-370, F-086 |
| 9 | Branch View (Only Selected) | Ctrl+Shift+B | cm_DirBranchSel | F-087 |
| — | separator | | | |
| 10 | Open Command Prompt Window (TC: "Run DOS") | — | cm_ExecuteDOS | F-363, F-371 |
| 11 | Open Desktop Folder | — | cm_OpenDesktop | F-364 |
| 12 | Control Panel / Printers / Network Neighborhood [Windows only] | — | cm_OpenControls / cm_OpenPrinters / cm_OpenNetwork | F-365 (N/A) |
| — | separator | | | |
| 13 | &Target = Source | — (TC 11: Ctrl+? none) | cm_MatchSrc | F-361 |
| 14 | E&xchange Left <-> Right | Ctrl+U | cm_Exchange | F-362, F-004 |
| 15 | Exchange Left <-> Right with tabs | Ctrl+Shift+U | cm_ExchangeWithTabs | F-005 |
| — | separator | | | |
| 16 | Browse Internal Commands... | — | cm_CommandBrowser | F-373, F-374 |
| 17 | Run Explorer (current folder) | — | cm_Explorer | F-366 |

Structure [K], exact order and labels [?]. Not in TC's default Commands menu but placed here by mormiCommander:
"Show file in other panel" (Ctrl+Left/Right with file under cursor goes to the other panel, F-367), "Refresh tree"
(cm_RescanTree, F-369), "Focus command line" (cm_FocusCmdLine, F-372), "New window" (F-376). [MC]

### 5.1 Behaviour notes

- **Directory Hotlist popup** (Ctrl+D): popup menu at the path bar: user entries (with `&` accelerators, submenus,
  separators), then separator, "Add current dir" (A) and
  "Configure..." (C) [K]. Choosing an entry with Ctrl held opens it in a new tab [?]; Shift held opens it in the target
  panel [K]. Stored in `[DirMenu]`: `menu1=title`, `cmd1=cd C:\path`, `path1=target dir` (2-panel entries), `-Submenu` /
  `--` for submenus. [S] Spec owned by §31 (F-651..F-664).
- **Directory Tree** (Alt+F10): dialog "Change directory" with a tree of the current drive, a search box (typing jumps,
  Ctrl+S next match), buttons "OK", "Rescan (F2)", "Cancel", "Help"; tree cached in `treeinfo.wc` in the drive root (or
  configured dir). [K] mormiCommander caches in the app cache dir, never on the drive. [MC]
- **Volume Label** (`cm_VolumeId`): dialog "Change volume label" — "New volume label for drive X:" edit with current label,
  OK/Cancel/Help; error "Cannot change label!" [?]. macOS: `diskutil rename`; Linux: menu item greyed with tooltip "Not
  supported on this system". [MC] F-359.
- **Search in separate process**: same Find Files dialog in a separate window/process that stays open independently of the
  main window. mormiCommander: separate non-modal window in the same process. [MC]
- **Target = Source**: target panel navigates to the source panel's folder (current tab), keeps its own view settings. [K]
- **Exchange**: swaps paths, view modes and sort of both panels (with tabs: swaps whole tab sets). Selection follows the
  folder. [K]
- **Open command prompt**: starts the shell in the source folder (Windows `cmd.exe` or configured; macOS Terminal.app or
  configured; Linux `$TERMINAL`/x-terminal-emulator). [MC] On archive/FTP folders: message "Not possible in a virtual
  folder." [MC]
- **Browse internal commands**: dialog "Choose command": left list of categories ("All", "File", "Edit", "Mark", ...),
  right list of commands with columns "Command", "Hotkey", "Description"; search edit at top "Search:"; buttons to copy the
  command name to the clipboard and to assign a hotkey (exact labels [?]). mormiCommander: Run, Copy
  name, Assign hotkey..., Close. [MC]

## 6. Net menu (F-377..F-392, F-816..F-818)

| # | Label | Key | cm_ command | FEATURES |
|---|-------|-----|-------------|----------|
| 1 | FTP &Connect... | Ctrl+F | cm_FtpConnect | F-377 |
| 2 | FTP &New Connection... | Ctrl+N | cm_FtpNew | F-378 |
| 3 | FTP &Disconnect | Ctrl+Shift+F | cm_FtpDisconnect | F-379 |
| 4 | FTP Show &Hidden Files | — | cm_FtpHiddenFiles | F-380 |
| 5 | FTP Abort Current Operation [?] | — | cm_FtpAbort | F-817 |
| 6 | FTP Resume Aborted Download [?] | — | cm_FtpResumeDownload | F-388 |
| 7 | FTP Download &List... | — | cm_FtpDownloadList | F-381 |
| 8 | FTP Change Transfer Mode [?] | Ctrl+Shift+M | cm_FtpSelectTransferMode | F-816 |
| 9 | Send FTP Command... [?] | — | cm_FtpSendCommand | F-390 |
| — | separator | | | |
| 10 | Background Transfer Manager [?] | — | cm_BackgroundTransferManager | F-382 |
| — | separator | | | |
| 11 | Connect Network Drive... | — | cm_NetConnect | F-383 (N/A) |
| 12 | Disconnect Network Drive... | — | cm_NetDisconnect | F-383 (N/A) |
| 13 | Share Current Directory... | — | cm_NetShareDir | F-384 (N/A) |
| 14 | Unshare Directory... | — | cm_NetUnshareDir | F-384 (N/A) |
| — | separator | | | |
| 15 | Port Connection (direct cable) [?] | — | cm_DirectCableConnect | F-385 (N/A) |

### 6.1 FTP Connect dialog (Ctrl+F) [K]/[?]

Caption "Connect to ftp server". Left: list (tree) of saved connections and folders, double-click connects. Right column
buttons (tab order): "Connect" (default), "New connection...", "New URL...", "Duplicate entry...", "New folder", "Edit...",
"Delete", "Encrypt" [?], "Cancel", "Help". Storage: `wcx_ftp.ini` `[connections]` `1=<name>` ... and one section per
connection. Delete asks "Do you really want to delete <name>?" Yes/No. [K]

Connection details dialog ("FTP: connection details"): "Session:", "Host name[:Port]:" (default port 21), "[ ] SSL/TLS",
"[ ] Anonymous connection (e-mail address as password)", "User name:", "Password:" with a save-password checkbox and a
warning that ini storage is unsafe [?], "Remote dir:", "Local dir:", "Only show these files:" (mask), "[x] Use passive mode for transfers (like a
WWW browser)", "[ ] Use firewall (proxy server)" + proxy combo + "Define new...", "Server type:" (Auto-detect, Unix,
Windows/IIS, ...), "Send commands:", "Codepage / UTF-8" combo, buttons OK / Cancel / Help. Keys in the connection section:
`host=`, `username=`, `password=` (scrambled), `directory=`, `localdir=`, `pasvmode=1`, `ssl=?`, `proxy=`... [?]
mormiCommander stores passwords in the OS keychain, never in the ini. [MC] Details owned by §30.

### 6.2 New FTP connection (Ctrl+N) [K]/[?]

Caption approximately "New FTP connection" [?]. "Connect to:" editable combo (history), accepts `host`,
`host:port`, `user@host`, `ftp://user:pass@host:port/path`, `sftp://...` (mormiCommander [MC]). Checkboxes "SSL/TLS",
"Anonymous connection", "Use passive mode" (on), "Use firewall". OK / Cancel / Help. Empty host + OK: nothing. Errors
appear in the FTP log and as message "Connection failed: <reason>". [MC]

### 6.3 Disconnect and connected state [K]

- Disconnect closes the connection of the active panel (if both panels are FTP, the active one). The panel returns to the
  local folder it showed before connecting. A running transfer asks "Abort current transfer?" Yes/No. [?]
- While connected, a bar above the panel shows: transfer mode combo ("Binary (archives, doc etc.)", "Text (plain text,
  html etc.)", "Auto (ASCII for: <masks>)" [?]), "Disconnect" button, a log toggle (shows the command/response log), and
  "Abort" during transfers. Ctrl+Shift+M cycles the mode. [K] F-816, F-817, F-391.
- Show hidden files toggles the use of `LIST -a`. [K]
- Download list (`cm_FtpDownloadList`): list of queued transfers; F5 with Shift on FTP files adds them to the list instead
  of copying [?]. Buttons "Start", "Remove", "Up/Down", "Close". [?]

### 6.4 Network items (Windows only) [MC]

Windows: call the native WNetConnectionDialog / WNetDisconnectDialog / sharing dialogs. macOS: "Connect Network Drive..."
opens Finder's Connect to Server (`open smb://`) prompt with our own "Server address:" edit; "Disconnect" lists mounted
network volumes under /Volumes and unmounts with `diskutil unmount`. Linux: `gio mount <uri>` / `gio mount -u`. Share /
unshare open the OS sharing settings. Port connection: item hidden. F-383..F-386.

## 7. Show menu (F-393..F-411, F-819)

| # | Label | Key | cm_ command | Kind | FEATURES |
|---|-------|-----|-------------|------|----------|
| 1 | &Brief | Ctrl+F1 | cm_SrcShort | radio (view) | F-393 |
| 2 | &Full | Ctrl+F2 | cm_SrcLong | radio (view) | F-394 |
| 3 | &Comments | Ctrl+Shift+F2 | cm_SrcComments | radio (view) | F-395, F-085 |
| 4 | Thumbnails | Ctrl+Shift+F1 | cm_SrcThumbs | radio (view) | F-396, F-083 |
| 5 | Custom Columns ▸ (list of column sets + "Configure...") | — (sets: Ctrl+Shift+F? none) [?] | cm_SrcCustomViewMenu / cm_SrcCustomView1..99 | submenu | F-397, F-084 |
| 6 | View Modes ▸ (list + "Configure...") | — | cm_SrcViewModeList / cm_SrcViewMode1..n | submenu | F-819 |
| — | separator | | | | |
| 7 | &Tree | Ctrl+F8 | cm_SrcTree | radio (view) | F-398, F-081 |
| 8 | &Quick View | Ctrl+Q | cm_SrcQuickView | toggle | F-399, F-082 |
| 9 | Separate Tree ▸ Off / 1 Tree / 2 Trees | Ctrl+Shift+F8 cycles | cm_SeparateTreeOff / cm_SeparateTree1 / cm_SeparateTree2 | radio | F-400, F-025 |
| — | separator | | | | |
| 10 | &All Files | Ctrl+F10 | cm_SrcAllFiles | radio (filter) | F-401, F-088 |
| 11 | &Programs | Ctrl+F11 | cm_SrcExecs | radio (filter) | F-402 |
| 12 | `*.wav` — label shows the current user filter | — | cm_SrcUserSpec | radio (filter) | F-403 |
| 13 | Custo&m... | Ctrl+F12 | cm_SrcUserDef | opens dialog | F-404, F-089 |
| — | separator | | | | |
| 14 | &Name | Ctrl+F3 | cm_SrcByName | radio (sort) | F-405, F-107 |
| 15 | &Extension | Ctrl+F4 | cm_SrcByExt | radio (sort) | F-405, F-108 |
| 16 | &Time | Ctrl+F5 | cm_SrcByDateTime | radio (sort) | F-405, F-109 |
| 17 | &Size | Ctrl+F6 | cm_SrcBySize | radio (sort) | F-405, F-110 |
| 18 | &Unsorted | Ctrl+F7 | cm_SrcUnsorted | radio (sort) | F-405, F-111 |
| 19 | &Reverse Order | — | cm_SrcNegOrder | check | F-406, F-112 |
| — | separator | | | | |
| 20 | &Reread Source | Ctrl+R | cm_RereadSource | command | F-407, F-064 |
| 21 | Only Selected Files [?] | — | cm_ShowOnlySelected | command | F-408 |
| 22 | Hide Selected Files (ours) | — | cm_HideSelected | command | F-408 |
| 23 | Show Hidden/System Files [?] | — | cm_SwitchHidSys | check | F-409, F-090 |
| 24 | Horizontal Panels [?] | — | cm_HorizontalPanels | check | F-410, F-003 |
| 25 | Single panel / two panels (ours) | — | cm_ToggleSinglePanel [MC] | check | F-411 |

Structure of groups 1–20 [K]; items 21–25 presence/labels [?].

### 7.1 Behaviour [K]

- View, filter and sort items act on the **active** panel and are remembered per tab (`[left]`/`[right]` and tab
  sections: `show=1|2|3`, `userspec=*.wav`, `sortorder=0 name|1 ext|2 size|3 date|4 unsorted`, `negative Sortorder=0|1`
  [S]; TC 11 stores sort per tab in `[lefttabs]`/`[righttabs]` `<n>_options=...` [?]).
- Choosing the sort that is already active reverses it (Ctrl+F3 twice = name descending). Time and size start descending
  (newest / largest first). [K] F-109, F-110.
- Tree (Ctrl+F8) and Quick View (Ctrl+Q) are toggles: pressing again returns to the previous view mode. Quick View shows
  the cursor file of the **active** panel in the **other** panel using Lister; moving the cursor updates it. [K]
- Separate tree: Ctrl+Shift+F8 cycles Off → 1 → 2 → Off. With 1 tree, the tree belongs to the active panel. [K]
- Programs filter: Windows `*.exe *.com *.bat *.cmd *.pif *.lnk` [?]; mormiCommander on macOS/Linux: files with any
  execute bit, plus `.app` bundles, `.command`, `.sh`, `.AppImage`. [MC] Folders always stay visible. [K]
- User filter: folders always stay visible unless the mask contains a folder mask (`\` suffix). The panel header/tab shows
  the filter (TC shows `*.wav` in the path bar after the path). [K]
- Only Selected Files: removes all unselected entries from the list until reread / Ctrl+F10 / folder change. [S] (forum:
  "completely removes the other entries from the list") Works in FTP, search results and branch view (TC 11.57 fixed bugs
  there). [S]
- Reread: keeps cursor (by name) and selection (1.1 rule 2). [K]
- Show hidden/system: global option (`[Configuration] ShowHiddenSystem=0|1`). [S] On macOS/Linux "hidden" = dot names
  plus macOS `UF_HIDDEN`; "system" not applicable. [MC]

### 7.2 Ctrl+F12 dialog "Custom file type" [K] layout, [?] labels

Same layout as Select files (1.6): caption "Select file type to display" [?]; editable combo (default: current
`userspec`, else `*.*`; history), "Define..." (search templates), OK / Cancel / Help. OK sets `show=3` and `userspec`.
Mask syntax identical to 1.6. Empty mask + OK = `*.*`. [?]

## 8. Configuration menu (F-412..F-447, F-820..F-828)

| # | Label | Key | cm_ command | FEATURES |
|---|-------|-----|-------------|----------|
| 1 | &Options... | — (macOS Cmd+,) | cm_Config | F-412 |
| 2 | Change &Button Bar... | — | cm_ButtonConfig | F-441, F-305 |
| 3 | Change Main &Menu... [?] | — | cm_ConfigChangeMainMenu [?] | F-828 |
| 4 | Configure Directory Hotlist... [?] | — | cm_ConfigDirHotlist | F-654 |
| 5 | Configure Favorite Tabs... [?] | — | cm_ConfigFavoriteTabs | F-775 |
| — | separator | | | |
| 6 | Save &Position | — | cm_ConfigSavePos | F-439 |
| 7 | &Save Settings | — | cm_ConfigSaveSettings | F-440 |
| — | separator | | | |
| 8 | Change Settings Files Directly | — | cm_ConfigChangeIniFiles | F-826 |
| 9 | Dark Mode ▸ Off / On / Follow system [?] | — | cm_SwitchDarkmode / cm_EnableDarkmode / cm_DisableDarkmode | F-417 |

Items 1, 2, 6, 7, 8 [K]; 3–5, 9 [?]. "Change Start Menu..." lives in the Start menu (section 9) in TC 11 [K]; FEATURES
F-442 is satisfied by that item.

### 8.1 Options dialog (cm_Config) [K] structure, [?] labels

Modal dialog, caption "Configuration" [?], left a tree of pages, right the page, bottom buttons "OK", "Cancel", "Apply",
"Help". OK = apply + close; Apply = apply now; Cancel = discard changes since the last Apply. The last opened page is
remembered. TC 11 page tree [?]:

```
Layout
Display
    Icons
    Font
    Colors
    Language
Operation
    Edit/View
    Copy/Delete
    Refresh
    Quick Search
    FTP
    Plugins
    Thumbnails
    Log file
    Ignore list
    Packer
    Zip packer
    Misc.
Folder tabs
    Favorite tabs
Custom columns
View modes
Tooltips
```

Page ↔ FEATURES: Layout F-413, Display F-414, Icons F-415, Font F-416, Colors F-417/F-435, Language F-418, Operation
F-419, Edit/View F-420/F-822, Copy/Delete F-421/F-436, Refresh F-422, Quick Search F-423, FTP F-424/F-437, Plugins F-425
(N/A), Thumbnails F-426, Log file F-427, Ignore list F-428, Packer F-429, Zip packer F-430, Misc F-431, Folder tabs F-432,
Favorite tabs F-434, Custom columns F-433, View modes F-820, Tooltips F-821; ours: Drag & drop and mouse F-823, Tree F-824,
Find defaults F-825, search box F-827.

#### Operation page (the part this area owns) [?] labels, [S] ini keys

| Group | Control | ini key | Default |
|-------|---------|---------|---------|
| Program start | [ ] Allow only 1 copy of Total Commander at a time | `[Configuration] onlyonce` [?] | off |
| Mouse selection mode | ( ) Use right mouse button (Norton Commander style) / ( ) Use left mouse button (Windows style) | `UseRightButton` [S] | left [MC] |
| Selection | [ ] Select/unselect directories too with Num+ / Num- / Num* / Ctrl+Num+ | `MarkDirectories` [S] | off |
| Selection | [x] Calculate space occupied by subdirectories when selecting with Space | `CountSpace` [S] | on |
| Selection | [ ] Space moves cursor down (TC: ini only) | `SpaceMovesDown` [S] | off |
| Command line | [ ] Up/Down keys move in command line history (else move cursor) | `CmdUpDown` [S] | off |
| Copy | [x] Count files in subdirectories before copy/delete (second progress bar) | `CountMarked` [S] | on |
| Single click | [ ] Single click opens files / folders | `SingleClickStart` [S] | off |
| Rename | [x] Rename by clicking twice on the name; [ ] select name only (not extension) | `InplaceRename`, `RenameSelOnlyName` [S] | on / off |
| Tree | [ ] Single click in tree changes other panel | `Autotreechange` [S] | off |

mormiCommander moves "Space moves down" into the dialog (it is ini-only in TC). [MC]

### 8.2 Change Button Bar (cm_ButtonConfig) [K]/[?]

Caption "Change button bar". Top: "Button bar:" combo with the `.bar` file path + "..." browse; preview strip of the bar
(click selects a button, drag reorders). Buttons right: "Append", "Insert" [?], "Delete", "Add subbar" [?]. Fields for the
selected button (tab order): "Command:" + browse "..." + magnifier (command browser), "Parameters:", "Start path:", "Icon
file:" + browse, icon list grid, icon index [?], "Tooltip:", "Run minimized" / "Run maximized" [?]. Bottom: "Icon
size:" (16/20/24/32), "[x] Flat icons", "[ ] Small icons", "Change all...", OK / Cancel / Help. The bar file (`default.bar`)
format: `[Buttonbar] Buttoncount=N`, `button1=<icon file>,<index>`, `cmd1=cm_...`, `param1=`, `path1=`, `menu1=<tooltip>`,
`iconic1=0|1|-1`. [K] F-304..F-314.

### 8.3 Save Position / Save Settings [K]

- Save Position writes window position and size, panel splitter, and the separate tree width to a section named after the
  screen resolution, e.g. `[1920x1080 (8x16)]` with `x=`, `y=`, `dx=`, `dy=`, `Divider=`, `maximized=`. [K]
  mormiCommander: section `[Window <w>x<h>]` in its ini. [MC]
- Save Settings writes all current settings (paths, tabs, views, histories) to wincmd.ini immediately. [K]
- Auto-saving on exit is controlled by Configuration > Options > Misc? / Operation "Save on exit" checkboxes ("Directory
  history", "Command line history", "Settings" ...) [?]; `[Configuration] SaveHistory`, `SaveCommands`, `SavePath` [?].

### 8.4 Change Settings Files Directly [K]/[?]

Saves current settings first, then opens `wincmd.ini` and `wcx_ftp.ini` in the configured editor (F4 editor). TC reloads
nothing automatically; changes take effect after restart — TC shows a message to that effect [?]. mormiCommander: opens the
ini in the editor, watches the file, and on save reloads it and shows a toast "Settings reloaded" (or "Restart needed for:
<keys>"). [MC] F-826.

## 9. Start menu (F-448..F-450, F-375, F-442, F-814)

### 9.1 Menu contents [K]

```
Change Start Menu...          (cm_ConfigStartMenu)   [?] position: first item, then a separator
-------------
<user item 1>        <its hotkey>
<user submenu ▸>
...
```

When `RestrictInterface` has bit 1, the whole Start menu is hidden. [S]

### 9.2 ini format `[user]` [S] (keyN [?])

```
[user]
menu1=Edit hosts
cmd1=notepad.exe
param1=%windir%\system32\drivers\etc\hosts
path1=
key1=CS+E                 ; [?] hotkey; TC may store start-menu hotkeys in [Shortcuts] instead
iconic1=0                 ; 1 = minimized, -1 = maximized, 0/empty = normal
menu2=-Tools              ; "-" prefix starts a submenu titled "Tools"
menu3=Calculator
cmd3=calc.exe
menu4=--                  ; ends the current submenu
menu5=-                   ; a lone "-" is a separator
```

- `cmd` may be a program, a document (opened with its association), a `cm_` command, a `cd <path>` command, or an `em_`
  user command. [K]
- Parameters support the button-bar placeholders `%P %N %T %M %S %R %L %F %D %X ...` and a leading `?` shows the expanded
  parameters in an edit box before running (F-814). [K]
- Ampersand in `menuN` = accelerator letter. [K]

### 9.3 Change Start Menu dialog (cm_ConfigStartMenu) [K] layout, [?] labels

Caption "Change Start menu". Left: list box of items (submenus indented). Buttons next to the list (tab order): "Add item",
"Add submenu", "Delete item", "Delete submenu" [?], "Up" (↑), "Down" (↓), "Sort" [?]. Fields for the selected item:

| Control | Label | Notes |
|---------|-------|-------|
| Edit | "Title:" | `&` for accelerator; "-" alone = separator |
| Edit + "..." + magnifier | "Command:" | browse for program; magnifier opens the command browser for cm_ commands |
| Edit | "Parameters:" | placeholders, leading `?` |
| Edit + "..." | "Start path:" | working folder; empty = source panel folder |
| Checkboxes + key combo | "Shortcut key:" [ ]Ctrl [ ]Alt [ ]Shift + key | conflicts with an existing key: message "This hotkey is already used by: <command>. Replace?" Yes/No [?] |
| Radio / checkbox | "Run minimized" / "Run maximized" | → iconicN |
| Buttons | OK / Cancel / Help | |

Add item inserts after the selected item with title "New item" [?] and focuses Title. Delete asks no confirmation [?].
Changes apply on OK. F-450, F-449, F-375.

## 10. Cross-platform mapping summary [MC]

| TC feature | Windows | macOS | Linux |
|------------|---------|-------|-------|
| Num keys | keypad | keypad if present; Shift+= / Shift+- / Shift+8 / `/` when command line empty | same as macOS on laptops |
| Ins | Ins | Space (+ Fn+Return on some keyboards = Ins) | Ins |
| Ctrl+click toggle | Ctrl | Cmd (Ctrl+click also toggles in TC mode; system context menu disabled on list rows) | Ctrl |
| Right-button mode hold | 500 ms | 500 ms two-finger hold | 500 ms |
| Attributes dialog | ARHS | rwx + hidden flag + owner | rwx + owner |
| Network drive items | native dialogs | Connect to Server / diskutil | gio |
| Programs filter | extensions | x-bit + .app | x-bit |

## 11. Open questions for QA against a real TC 11.x install

1. Exact English captions after 11.01's "select" → "mark" rename (Mark menu items, Select files dialog caption).
2. Whether a plain left click in left-button mode marks the clicked file (and the TC 11 option that controls it).
3. Shift+arrow paint state rule (1.4) and whether the landed-on row is painted for Shift+PgDn.
4. Compare Directories: same time + different size result, and the message when nothing differs.
5. Mark menu items 17–19 and Show menu items 21–25: present in the default TC 11 menu or not.
6. Regex prefix in the Select files dialog (`<` per wiki).
7. Where start-menu hotkeys are stored (`keyN` in `[user]` vs `[Shortcuts]`).
8. Default for `UseRightButton` when the key is missing on TC 11.
9. Exact Options page tree and Operation page labels in TC 11.

## Sources

- ghisler.ch wiki: "Selecting files" (https://www.ghisler.ch/wiki/index.php/Selecting_files) — keys, mouse modes, masks,
  `|` exclusions, save/load selection commands, SpaceMovesDown.
- ghisler.ch wiki: "Wincmd.ini" (https://www.ghisler.ch/wiki/index.php/Wincmd.ini) and "Ini settings" — SpaceMovesDown,
  CountSpace, MarkDirectories, UseRightButton, CmdUpDown, CountMarked, Mainmenu, RestrictInterface, ShowHiddenSystem,
  `[user]`, `[DirMenu]`, `[Selection]`, sort/filter keys in `[left]`/`[right]`, `[Colors]` MarkColor/InverseSelection.
- ghisler.com beginners guide (https://www.ghisler.com/beginners.htm) — Files > Change Attributes, Pack.
- ghisler.ch forum threads (via search results): Compare Directories marks newer and missing files; TC 11 RC3 fix for
  missing folders; `cm_ShowOnlySelected` removes other entries; TC 11.01 select → mark rename; Ctrl/Shift modifiers on
  Pack.
- Total Commander keyboard.txt / long-standing program behaviour (tag [K]).
