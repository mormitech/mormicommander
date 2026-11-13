# Area 5 spec: Find files and Synchronize directories

This document describes how Total Commander 11 behaves in two of its largest
dialogs: Find files (Alt+F7) with its results list and "Feed to listbox", and
Synchronize directories (Commands > Synchronize dirs). It is written so that a
UI agent can build the mormiCommander equivalent without owning a copy of
Total Commander. Where TC is Windows-specific we say what the macOS/Linux
equivalent should be. Every section lists the FEATURES.md IDs it backs.

**Sources and confidence.** The official help pages and the ghisler.ch forum
were consulted, but most of the help is only shipped inside `TOTALCMD.CHM`, so
large parts come from knowledge of the product. Statements marked
**[verify]** are believed correct but should be checked against a real TC 11
install before being treated as binding. Unmarked statements are well
established (stable since TC 7-9 and unchanged in 11). Where TC is ambiguous,
the section "mormiCommander decision" says what we do.

Notation: `Alt+S` means the underlined accelerator letter in a label. On macOS
the Alt accelerators use Option, matched by physical key code (F-286).

---

## 1. Opening the Find files dialog

| Trigger | Command | Result |
|---|---|---|
| Alt+F7 | `cm_SearchFor` (internal 501) | Opens the dialog modeless, inside the TC process (F-163, F-225, F-524) |
| Alt+Shift+F7 | `cm_SearchStandalone` (545) | Opens the dialog in a separate process; main window stays responsive (F-164, F-543) [verify default key] |
| Commands > Search | `cm_SearchFor` | Same as Alt+F7 (F-356) |
| Commands > Search in separate process | `cm_SearchStandalone` | (F-357) |
| Button bar "Find files" (binoculars) | `cm_SearchFor` | (F-304) |
| macOS Cmd+F | `cm_SearchFor` | ours only (F-285) |

Behaviour on open:

1. The dialog is **modeless** and has its own taskbar entry. The main window
   remains usable; several Find dialogs can be open at the same time (each
   Alt+F7 opens a new one) [verify: TC 9+ opens a new instance each time; older
   versions re-focused the existing one].
2. "Search in" is prefilled with the **active panel's current path**. If the
   panel is inside an archive the archive path is used; if it is an FTP/SFTP
   connection the remote path is used (F-545).
3. If files/folders are **selected** in the active panel, the checkbox
   "Only search in selected directories/files" is enabled (otherwise greyed).
   It is unchecked by default [verify: TC remembers its last state].
4. "Search for" is prefilled with the last used value from history, fully
   selected so typing replaces it. If the cursor sits on a file and
   `[Configuration] SearchFilenameOnOpen` style behaviour is active, TC does
   **not** insert the file name by default (no such option in 11) [verify].
5. Focus is in "Search for". The General tab is active.
6. The settings of the last search (depth, archives, text options, Advanced
   tab values) are restored if `[Configuration] SearchFlags` remembers them;
   by default TC keeps them for the lifetime of the process and loads the
   special template "last search" on request (F-542).

Window geometry is stored in `wincmd.ini` under `[Configuration]` (or the
resolution-specific `[1920x1080 (8x16)]` style section) as `SearchX`,
`SearchY`, `SearchDX`, `SearchDY`, `SearchMax` [verify key names]. The dialog
is resizable; the results list grows with the window, the controls above stay
fixed height.

mormiCommander decision: the Find window is a separate Wails window (or an
in-app floating panel) that does not block the panels. "Separate process"
(F-164) maps to a second window running its own search goroutine; we do not
spawn a second OS process.

---

## 2. Dialog skeleton

```
+--------------------------------------------------------------------+
| Find Files                                                  [_][X] |
+--------------------------------------------------------------------+
| [General] [Advanced] [Plugins] [Load/Save]                         |
| +----------------------------------------------+  [ Start search ] |
| |  (tab page, ~170 px high)                    |  [   Cancel     ] |
| |                                              |  [    Help      ] |
| +----------------------------------------------+                   |
| Search results:                                                    |
| +----------------------------------------------------------------+ |
| | C:\Work\a.txt                                                  | |
| | C:\Work\sub\b.txt                                              | |
| | [C:\Work\sub2]                                                 | |
| +----------------------------------------------------------------+ |
| [x files and y directories found]                                  |
| [New search] [Go to file] [View] [Feed to listbox]                 |
+--------------------------------------------------------------------+
```

- The tab control holds four tabs: **General**, **Advanced**, **Plugins**,
  **Load/Save**. FEATURES.md F-524 mentions a "Results" tab; in TC 11 the
  results are **not a tab**, they are the list in the lower half of the same
  window. mormiCommander shows the list below the tabs too.
- Right-hand buttons: **Start search** (default button, Enter), **Cancel**
  (Esc), **Help** (F1). While a search runs, "Start search" is disabled and
  "Cancel" stops the search instead of closing the window (F-538).
- Under the list: a status line, then buttons **New search**, **Go to file**,
  **View**, **Feed to listbox**. Buttons other than New search are disabled
  while the list is empty. TC 10+ also has **Edit** reachable via F4 only
  (no button) [verify].

Tab order (Tab key): tab control > controls of the current page top-to-bottom,
left-to-right > Start search > Cancel > Help > results list > New search >
Go to file > View > Feed to listbox. Ctrl+Tab / Ctrl+Shift+Tab switch tabs
from anywhere in the dialog. Ctrl+PgDn / Ctrl+PgUp also switch tabs (standard
Windows tab control behaviour).

---

## 3. General tab

```
Search for:  [*.txt                                    v] [ ] RegEx
Search in:   [C:\Work                                  v] [>>] [Drives v]
             [ ] Only search in selected directories/files
Search subdirectories: [all (unlimited depth)   v]
[ ] Search archives (all except for uncheck)
[ ] Find text: [                                       v]
    [ ] Whole words only     [ ] Case sensitive     [ ] Not containing
    [ ] ASCII (DOS charset)  [x] ANSI (Windows charset)
    [x] Unicode (UTF-16)     [x] UTF-8              [ ] Hex
    [ ] RegEx (2)            [ ] Office XML (docx, xlsx, odt ...)
    [ ] Text from plugins (Find text in files via content plugins)
```

Exact label strings in the English language file (`WCMD_ENG`):

| Control | Label | Accelerator | Default |
|---|---|---|---|
| Combo | `Search for:` | Alt+S | last history item, or `*.*` |
| Checkbox | `RegEx` (applies to file name) | Alt+E [verify] | off |
| Combo | `Search in:` | Alt+I | active panel path |
| Button | `>>` | — | opens folder browse dialog |
| Button | `Drives` (drop-down) | Alt+D [verify] | — |
| Checkbox | `Only search in selected directories/files` | Alt+O | off, greyed if nothing selected |
| Combo (drop-down list, editable) | `Search subdirectories:` | Alt+B [verify] | `all (unlimited depth)` |
| Checkbox | `Search archives (all except for uncheck)` | Alt+A | off |
| Checkbox + combo | `Find text:` | Alt+F | off; combo disabled until checked |
| Checkbox | `Whole words only` | Alt+W | off |
| Checkbox | `Case sensitive` | Alt+C | off |
| Checkbox | `Not containing` | Alt+N | off |
| Checkbox | `ASCII (DOS charset)` | | off |
| Checkbox | `ANSI (Windows charset)` | | on |
| Checkbox | `Unicode (UTF-16)` | | on |
| Checkbox | `UTF-8` | | on |
| Checkbox | `Hex` | Alt+H | off |
| Checkbox | `RegEx (2)` | Alt+X [verify] | off |
| Checkbox | `Office XML (docx, xlsx, odt etc.)` | | off |

### 3.1 Search for (F-525, F-530, F-837)

Semantics of the pattern string when RegEx is **off**:

1. Several patterns are separated by **spaces or semicolons**:
   `*.txt *.doc` and `*.txt;*.doc` are equivalent. A pattern containing a
   space must be put in double quotes: `"my file*.txt"`.
2. `|` starts the **exclude list**. Everything after the first `|` is a list
   of patterns to exclude, again separated by spaces/semicolons:
   `*.txt|old*` = all txt files whose name does not start with `old`.
   `|*.bak` alone = everything except *.bak.
3. An exclude pattern ending with a backslash (TC) excludes **directories**
   with that name and everything below: `*.go|vendor\ .git\`. Our
   implementation must accept both `\` and `/` as the trailing separator on
   every OS (F-837).
4. A pattern **without** any `*` or `?` and without a dot is treated as
   `*pattern*` (substring search): typing `report` finds `annual_report.pdf`.
   A pattern with a dot but no wildcard matches exactly (`readme.txt`)
   [verify: TC turns `abc` into `*abc*` only if the option
   `Configuration > Options > Quick search/Find`... is default; default is on].
5. `*.*` matches every name **including names without a dot** (DOS
   semantics). `*` alone matches everything too. `*.` matches names without an
   extension.
6. Matching is case-insensitive on all platforms (TC is Windows); on macOS
   and Linux we keep case-insensitive matching and compare in NFC (F-754).
7. Empty "Search for" = `*` (everything). This is how you search only by
   date/size/text.
8. A pattern containing a path separator (`sub\*.txt`) is matched against
   the path relative to the search root [verify; low priority].
9. Prefixes `ev:` and `ed:` send the term to the Everything search engine
   (TC 9.5+). F-358 maps this to Spotlight `mdfind` on macOS and `plocate`/
   `locate` on Linux; if neither exists we ignore the prefix and do a normal
   walk.

When the **RegEx** checkbox next to Search for is on (F-530): the whole field
is one regular expression (TC uses Perl-compatible syntax), matched against
the file name only (not the path), case-insensitive. `|` keeps its regex
meaning (alternation) and no longer means "exclude". mormiCommander uses Go
`regexp` with `(?i)`; we document that lookarounds are unsupported.

History: the combo keeps the last 20 entries [verify count] in
`wincmd.ini [SearchName]` as keys `0=`, `1=`, ... (0 = most recent).
Entries are added when a search starts, deduplicated case-insensitively.
Down arrow / Alt+Down opens the history list. Del inside the open drop-down
does **not** delete items in TC [verify].

### 3.2 Search in (F-526, F-782)

1. One or more folders separated by **semicolons**: `C:\Work;D:\Backup`.
   Paths with spaces may be quoted but do not need to be.
2. Empty field = error, see 3.10.
3. Drive roots (`C:\`), UNC paths (`\\server\share`), archive paths
   (`C:\a.zip\sub`) and FTP connections (current connection only) are
   accepted. On macOS/Linux: absolute paths, `~` expands to home [ours].
4. `>>` opens the OS folder picker starting in the first path of the field;
   the chosen folder **replaces** the field contents [verify: TC appends with
   `;` when Shift is held — not documented, do not implement].
5. **Drives** drop-down menu: `All local drives` (fixed disks),
   then one item per drive letter with volume label. Choosing an item puts
   `C:\;D:\` etc. in the field. mormiCommander: `All local volumes` (macOS
   `/` plus `/Volumes/*`; Linux `/` plus mounted non-pseudo filesystems from
   internal/disk), then each volume.
6. Typing offers folder auto-complete (F-782): matching subfolders appear in a
   drop-down after a short delay; Tab or Right accepts.
7. History in `wincmd.ini [SearchIn]` (same format as `[SearchName]`).
8. **Only search in selected directories/files**: the search roots become the
   items selected in the panel at the moment the dialog opened. Selected
   folders are searched (with depth applied below them), selected files are
   tested themselves. The "Search in" field is ignored (greyed) while this is
   checked [verify: field stays editable but is ignored].

### 3.3 Search subdirectories (F-527)

Editable drop-down with items:

- `all (unlimited depth)` (default)
- `current dir only`
- `1 level`, `2 levels`, ... `99 levels` (the user may also type a number)

`current dir only` = depth 0 (only direct children of each root). `1 level` =
children plus grandchildren. Depth is counted per root.

### 3.4 Search archives (F-528, F-626)

When on, every file whose extension is handled by an internal packer (ZIP,
7Z, TAR, GZ, BZ2, XZ, RAR, CAB, ARJ, LZH, UC2, ACE) or by a packer plugin
is opened and its entries are matched like files in a folder. Nested archives
are not searched. Results show as `C:\path\archive.zip\inner\file.txt`.

The label text "(all except for uncheck)" refers to a sub-dialog [verify]: in
TC 9.10+ right-clicking the checkbox or using a small button lets you
untick archive types to skip. mormiCommander: plain checkbox; the set of
types is whatever internal/vfs supports.

Content search ("Find text") inside archives is supported in TC (it
extracts each candidate to memory/temp) [verify for RAR]. Ours: supported for
formats internal/vfs can stream.

### 3.5 Find text (F-529)

- Ticking `Find text` enables the combo and the option checkboxes. Typing in
  the combo ticks the checkbox automatically [verify].
- History in `wincmd.ini [SearchText]`.
- Only **files** are content-searched; folders never match a text search
  (they are dropped from results when Find text is on).
- `Whole words only`: match must be bounded by non-word characters
  (letters, digits and `_` are word characters; Unicode letters count).
- `Case sensitive`: off = case-insensitive in all selected encodings
  (Unicode-aware lower-casing for UTF-8/UTF-16).
- `Not containing`: the result list contains files that do **not** contain
  the text (respecting all other options). Combining with Hex works.
- Encodings: the file is searched once per ticked encoding; a hit in any of
  them is a hit. At least one encoding must be ticked, otherwise TC shows
  error 3.10 (c). `ASCII (DOS charset)` = OEM codepage (CP437/852 for HU);
  `ANSI` = Windows codepage (CP1252/1250). mormiCommander: ANSI = the user's
  configured legacy codepage (default Windows-1252, Windows-1250 for HU
  locale); ASCII = CP437/CP852 accordingly (F-825 "encoding" default).
- `Hex`: the text is a hex byte sequence, spaces allowed: `4D 5A 90 00`.
  Encoding/case/whole-word boxes are greyed. Invalid hex = error 3.10 (d).
- `RegEx (2)`: the text is a regex applied per line (TC: to the decoded text
  of each selected encoding). The "(2)" distinguishes it from the file-name
  RegEx box. Multi-line matches are not supported.
- `Office XML`: for .docx/.xlsx/.pptx/.odt/.ods/.odp, the XML inside the ZIP
  is unpacked and tags stripped before matching, so visible text matches.
- `Text from plugins` [verify presence in 11]: uses content-plugin full text.
  Not implemented (WDX, F-536 N/A).

Performance rule (ours): files larger than 4 GiB are still searched in
streamed chunks with an overlap of `len(pattern)-1` bytes so matches across
chunk borders are found.

### 3.6 Everything / Spotlight

TC 10+ adds the ev:/ed: prefix only; there is no checkbox in TC 11's General
tab for it [verify]. Our F-825 option "Use Spotlight/locate when available"
lives in Configuration > Find files, not in this dialog.

### 3.7 Start search

Enter anywhere on a page (except inside an opened drop-down) or clicking
**Start search**:

1. Validates (3.10). On error a message box appears and focus goes to the
   offending control.
2. Saves the current settings as the template `last search` (see 7).
3. Clears the result list, sets the status line to the folder currently being
   scanned, starts the walk.
4. Results are appended **live** as found (F-538). The list scrolls only if
   the user has not moved the cursor.
5. When finished, the status line shows
   `[N files and M directories found]` (singular forms: `1 file`,
   `1 directory`) [verify exact wording]. With no hits the list shows a single
   grey line `[No files found!]` and Go to file / View / Feed are disabled.
6. Focus moves to the results list (first item focused) so arrows and F3
   work immediately.

### 3.8 Cancel / stop

- While searching: Esc or **Cancel** stops the walk. The results found so far
  stay; the status line shows `[Search aborted] N files and M directories
  found` [verify wording].
- When idle: Esc or **Cancel** closes the window. The results are lost
  unless they were fed to a listbox. Closing the window via [X] while
  searching first stops the search.

### 3.9 New search

Clears the result list and status, switches to the General tab, puts focus in
Search for with its text selected. Field values are **kept** (not reset to
defaults) [verify]. The accelerator letters of "New search" and "Not
containing" collide on N in a naive translation; the TC 11 assignment is
unverified. mormiCommander: assign Alt+N to New search and give "Not
containing" Alt+T ("No&t containing").

### 3.10 Error messages

| Case | TC message (English) | Focus |
|---|---|---|
| (a) Search in empty | `Please enter a directory to search in!` [verify] | Search in |
| (b) Search in path does not exist | `Directory not found: <path>` / Windows error text; search continues with the other roots, the missing root is skipped with a beep [verify] | Search in |
| (c) Find text on but no encoding ticked | `Please choose at least one character set!` [verify] | ANSI checkbox |
| (d) Hex text invalid | `Invalid hex string!` [verify] | Find text combo |
| (e) Invalid regular expression | `Error in regular expression:` + detail [verify] | the RegEx field |
| (f) Date between with invalid date | `Invalid date!` [verify] | the date field |
| (g) Access denied in a subfolder | silently skipped, not reported | — |

mormiCommander uses the same wording (our English strings) and shows them
inline under the field rather than as a modal box when possible.

---

## 4. Advanced tab (F-531, F-532, F-533, F-534, F-535)

```
[ ] Date between: [2026-01-01 v] and: [2026-11-13 v]
[ ] Not older than: [ 1 ] [days      v]
[ ] File size:  [=  v] [ 0       ] [kbytes  v]
[ ] Attributes:  [?] Archive  [?] Read only  [?] Hidden  [?] System
                 [?] Directory [?] Compressed [?] Encrypted
[ ] Find duplicate files: [x] same name  [x] same size  [ ] same contents
                          [ ] Search for unique files (no duplicates) [verify]
[ ] Search for: files and folders / only files / only folders  [verify]
[ ] Follow symlinks/junctions (TC: "Search in symbolic links/junctions") [verify]
```

| Control | Behaviour |
|---|---|
| `Date between:` + `and:` | Inclusive range on **modification time**, both ends date-only (whole days, local time). The two date pickers default to today. Unchecked = ignored. If the first date is after the second they are swapped silently [verify]. |
| `Not older than:` + number + unit | Units: `minutes`, `hours`, `days`, `weeks`, `months`, `years`. Number 1..999999. Mutually exclusive with Date between (ticking one unticks the other). "1 days" means within the last 24 h, not "since midnight" [verify]. |
| `File size:` op + number + unit | Op: `=`, `>`, `<` (TC 9+ also `>=`? no — only these three [verify]). Units: `bytes`, `kbytes`, `Mbytes`, `Gbytes`. `=` with kbytes compares **rounded** sizes (a 1500-byte file equals 1 kbyte) [verify]. Folders are never matched by a size filter. |
| `Attributes:` | Each box is **tri-state**: checked = must have, unchecked = must not have, grey = don't care (default grey). On macOS/Linux: Read only = no write bit for owner; Hidden = name starts with `.` or macOS UF_HIDDEN; System/Compressed/Encrypted are greyed; Directory = is a folder. Ticking Directory=checked yields folders only, unchecked yields files only (this is how TC does "folders only", F-535). |
| `Find duplicate files` | Results are grouped: duplicates are listed consecutively, each group separated by a line `----------------` [verify]. `same name` compares names case-insensitively; `same size` compares byte length; `same contents` byte-compares (only files with equal size are compared; TC uses a hash first). At least one sub-option must be ticked. Groups with one member are not shown. Folders are ignored. |
| Symlinks | Off by default in TC: junctions/symlinks to folders are not followed to avoid loops. On: followed, with loop detection by (dev, inode). [verify label] |

Plugins-based conditions ("Search in plugins") belong on the Plugins tab.

mormiCommander simplification: implement explicit radio buttons
`Files and folders / Files only / Folders only` in addition to the Directory
attribute, because tri-state checkboxes are unfamiliar to Mac users. Both map
to the same internal filter.

---

## 5. Plugins tab (F-536 — N/A as WDX, replaced by built-in fields)

TC layout:

```
[ ] Search in plugins:   (o) AND (all conditions)  ( ) OR (any condition)
Plugin       Property       OP     Value        Unit
[tc     v]   [size     v]   [> v]  [1000    ]   [bytes v]
[       v]   [         v]   [  v]  [        ]   [      v]
...  (up to 8 rows by default) [More rules] [Fewer rules]
```

- Plugin combo lists `tc` (internal fields: name, size, date, attributes,
  path, ...) plus installed content plugins.
- OP list depends on field type: numbers `= != < > <= >=`; strings
  `contains`, `!contains`, `=`, `!=`, `= (case)`, `!= (case)`, `regex`,
  `!regex`; booleans `=`.
- Unit only for fields that have units.

mormiCommander: same grid with Plugin replaced by a "Field group" combo:
`File` (name, size, dates, permissions), `Image` (width, height, EXIF date,
camera), `Audio` (artist, album, title, duration), `Text` (line count,
encoding). Same AND/OR semantics.

---

## 6. Results list (F-538 .. F-541, F-544, F-838)

### 6.1 Display

- One line per hit: full path. Folders in square brackets: `[C:\Work\sub]`
  [verify: TC 11 shows folders with brackets, files plain].
- Inside archives: `C:\a.zip\dir\file.txt`.
- No columns, no icons [verify: TC 10+ shows icons]. mormiCommander may show
  a file-type icon and greyed path + bold name, but copying the text must
  give the plain full path.
- Multi-selection supported (Shift/Ctrl+click, Shift+arrows, Ctrl+A).

### 6.2 Keys and mouse in the list

| Input | Action |
|---|---|
| Enter or double-click | **Go to file**: the active panel changes to the file's folder and the cursor is put on the file; the Find window stays open but the main window is activated (F-539). For a folder hit, the panel shows the folder's parent with the cursor on it. For a hit inside an archive, the archive is opened as a folder. |
| F3 | View file in Lister (F-540). Several selected files open Lister with all of them (navigate with N/P). |
| Shift+F3 | Lister with only the focused file [verify] |
| F4 | Edit file in the configured editor (F-540). |
| Ctrl+C / Ctrl+Ins | Copy selected names (full paths, one per line) to clipboard as text (F-838) [verify: TC copies files as file objects in 10+; ours copies text when the Find window has focus]. |
| Del / F8 | Delete selected files after confirmation; deleted items are removed from the list [verify: TC 9+]. |
| Shift+F10 / right-click | OS context menu for the file(s) (Windows shell menu). Ours: own menu with View, Edit, Go to file, Copy path, Reveal in Finder/Explorer, Delete. |
| Alt+F7 | Opens another Find window. |
| Ctrl+F / F7 | (ours) Search in results (F-544). |
| Esc | Close the window (when idle). |
| Tab | Moves to the buttons under the list. |
| Drag from list | Drag the files out to a panel or other app (copy). |

### 6.3 Go to file button
Same as Enter. If the file's folder no longer exists, TC shows the panel at
the nearest existing parent and beeps.

### 6.4 View button
Same as F3.

### 6.5 Feed to listbox (F-541, F-353)

Clicking **Feed to listbox** (or `cm_SearchFor` result shortcut Alt+L
[verify]):

1. The Find window closes (TC 9+ keeps it open only when started as a
   separate process; then it feeds the running main instance) [verify].
2. The **active panel** switches to a virtual folder whose path line reads
   `Search results:` [verify; earlier versions showed `<Search results>`].
   The panel shows every result as an item; the name column contains the
   path **relative to the common search root** (e.g. `sub\b.txt`), the other
   columns (size, date, attributes) are real.
3. All normal operations work on it (copy F5, move F6, delete F8, pack
   Alt+F5, view F3, edit F4, multi-rename Ctrl+M, attributes, selection,
   compare). Deleting removes the item from the list. Renaming changes the
   real file.
4. Ctrl+PgUp, Backspace, or entering `..` leaves the result list and returns
   to the folder the panel showed before. Enter on a folder entry enters the
   **real** folder (leaving the list). TC's command for jumping from a
   result-list entry to its real folder is unverified. Ours: Ctrl+J "Jump to
   file location" (final choice belongs to the panel spec).
5. The result list survives tab switches and is restorable through the
   Back (Alt+Left) history while the session lasts. It is not persisted
   across restarts.
6. Archives: if results come from several archives or mix archive entries
   with normal files, only the archive files themselves are fed (TC cannot
   operate on a mix). If all hits are inside one archive, the archive entries
   are fed. ours: same rule; show a one-line note in the status bar.
7. FTP results cannot be fed [verify]; button disabled.
8. Reread (Ctrl+R) on a result list re-stats every entry and drops entries
   whose file vanished.

The command-line counterpart: TC accepts a list file via `cm_LoadSelection`
style param `/L=` — out of scope.

### 6.6 Search in results (F-544)

TC has no button, but you get it by Feed to listbox and then Alt+F7 with
"Search in" = the result list (TC 9+ fills `Search in:` with
`<Search results>`... [verify]) or by "Only search in selected directories/
files" after selecting all in the list. mormiCommander: when Alt+F7 is pressed
on a result-list panel, prefill Search in with the special token
`::results::` labelled "Search results" and walk only those entries
(depth 0 on files, configured depth on folders).

### 6.7 Save results (F-838, ours)
Context menu item `Save list to file...` writes UTF-8 text, one full path per
line, LF on macOS/Linux and CRLF on Windows.

---

## 7. Load/Save tab (F-537, F-542)

```
Stored searches:
+------------------------------+   [ Load  ]
| last search                  |   [ Save  ]
| Images                       |   [ Delete]
| Big files                    |
+------------------------------+
[ ] Save also "Search in" folder with template     [verify placement: asked in Save dialog]
[ ] Save also plugin conditions                    [verify]
```

- **Load** (or double-click/Enter on a list item): applies every saved field
  to all tabs and switches to General. Does not start the search.
- **Save**: prompts `Save search as:` with an edit box (default = selected
  name). In the same prompt TC asks whether to store the "Search in" folder
  (checkbox `Save directory "Search in"` [verify]) — without it the template
  is path-independent and usable in filters.
- **Delete**: asks `Really delete template "<name>"?` Yes/No [verify text].
- `last search` is a pseudo-entry always at the top: it holds the settings of
  the previous search in this process (and since TC 9, across restarts)
  [verify].
- Templates are used elsewhere: in `cm_SrcUserDef` custom filter (F-089,
  F-404), "Select group" (Num +) and the Attributes/selection dialogs
  (F-685), in Synchronize dirs' file mask field (section 9.3) and in
  colour-by-file-type definitions. The reference syntax is `>TemplateName`.

wincmd.ini storage (section `[searches]`, keys prefixed with the template
name followed by `_`):

```
[searches]
Images_SearchFor=*.jpg;*.png;*.gif
Images_SearchIn=
Images_SearchText=
Images_SearchFlags=0|002002000020|||||||||0000|
Big files_SearchFor=*
Big files_SearchFlags=0|002002000020|||1|100|3|||||0000|
```

`SearchFlags` is a `|`-separated record: the first field is the text options
bit-string (case, whole word, hex, not containing, regex, encodings ...),
followed by depth, archive flag, date-between from/to, not-older-than
number/unit, size op/value/unit, attribute masks, duplicate options. The exact
positional layout is undocumented [verify]; mormiCommander stores templates
in its own INI as named keys instead (F-443) and offers an import of the TC
`[searches]` section as best effort.

Plugin conditions are stored as `Name_plugin=` lines [verify].

---

## 8. Find options in Configuration (F-825)

TC `Configuration > Options > Operation`/`Find` relevant keys under
`[Configuration]` [verify names]:

| Key | Meaning | Default |
|---|---|---|
| `SearchDepth` [verify] | default depth | unlimited |
| `SearchArchives` | default for Search archives | 0 |
| `FindDialogFont` | font for results | system |
| `SearchFoundFont` | — | — |
| `ShowFoundAsRelative` [verify] | Feed to listbox shows relative paths | 1 |
| `Everything` / `EverythingPath` | Everything integration | — |

mormiCommander Configuration > Find files page: default depth, search
archives default, follow symlinks default, use Spotlight/locate for name-only
searches, legacy ANSI/OEM codepage choice, maximum history entries.

---

## 9. Synchronize directories (F-198, F-360, F-546 .. F-555, F-485, F-562)

### 9.1 Opening

| Trigger | Command |
|---|---|
| Commands > Synchronize dirs... | `cm_SyncDirs` (2020) |
| Button bar "Synchronize" | `cm_SyncDirs` |
| No default hotkey | — |

The window is **modeless** and resizable, with its own taskbar entry. Left
path = left panel's path, right path = right panel's path (not
source/target). If either panel is a result list/plugin root without a real
path, its field is empty.

### 9.2 Layout

```
+-----------------------------------------------------------------------+
| Synchronize directories                                        [_][X] |
| [C:\Work                         v][>>]  [<->]  [D:\Backup\Work   v][>>] |
| Search for: [*.*                       v] [>>?]                        |
| [x] Subdirs  [ ] by content  [ ] ignore date  [ ] Asymmetric  [Compare] |
| Show: [->][=][!=][<-][duplicates][singles]                             |
+-----------------------------------------------------------------------+
| Name      Size   Date      | = | Date     Size   Name                 |
| a.txt     1 200  2026-01-02| ->|                    (empty)            |
| b.txt     1 300  2026-01-03| = | 2026-01-03 1 300  b.txt              |
| c.txt     1 400  2026-01-04| <-| 2026-02-01 1 410  c.txt              |
| \sub                       |   |                    \sub              |
+-----------------------------------------------------------------------+
| Total: 120 Files (123 MB)  Equal: 100 ...  Different: ... left/right   |
| [Synchronize...]                                      [Close] [Help]   |
+-----------------------------------------------------------------------+
```

Exact controls (top to bottom; tab order follows):

| Control | Label | Default |
|---|---|---|
| Combo + `>>` | left path (history `[SyncLeft]` [verify]) | left panel path |
| Button | `<->` swap left and right [verify presence: present since TC 7.5? yes: "Swap sides"] | — |
| Combo + `>>` | right path (history `[SyncRight]` [verify]) | right panel path |
| Combo | `Search for:` file mask, same syntax as Find files 3.1 including `|` excludes and `>template` | `*.*` |
| Button | `>>` next to mask: picks a Find files template | — |
| Checkbox | `Subdirs` (recurse) | on [verify: TC default off, remembers last] |
| Checkbox | `by content` | off |
| Checkbox | `ignore date` | off |
| Checkbox | `Asymmetric` (`asymmetric` in TC 11 lowercase [verify]) | off |
| Button | `Compare` (default button, Enter) | — |
| Toggle buttons | `Show:` `->` `=` `≠` `<-` `duplicates` `singles` | `->`, `≠`, `<-`, `singles`, `duplicates` on; `=` off [verify] |
| List | comparison result | empty |
| Status line | totals | — |
| Buttons | `Synchronize...`, `Close`, `Help` | Synchronize disabled until a compare ran |

TC 8+ also offers a small drop-down arrow / menu with options (F-547)
[verify placement: some are in Configuration > Options > Operation,
"Synchronize dirs" section]:

- `Ignore difference of exactly 1 hour (timezone/DST)` — `[Configuration] SyncIgnoreHour` [verify]
- `Ignore time difference of up to 2 seconds (FAT)` — always on in TC (FAT
  resolution) [verify]; ours: option, default on.
- `Compare by content` also for files with different dates
- `Ignore empty folders` / `Show empty folders` (F-548) [verify].
- `Show only selected`.
- `Ignore case of names` (ours, needed on Linux where `A.txt` and `a.txt`
  may coexist; default: case-sensitive on Linux, insensitive elsewhere).

### 9.3 Compare semantics

1. Walk both trees (respecting Subdirs and Search for). Pair entries by
   relative path, case-insensitively (TC/Windows).
2. For each pair, decide the symbol in the middle column:
   - `=` equal: same size and same modification time (within 2 s tolerance,
     and within exactly 1 h if that option is on). With `by content` on,
     files with equal size are byte-compared; equal content gives `=` even if
     dates differ **only if ignore date is also on**; otherwise dates decide
     direction and content is only used to confirm equality of same-date
     files [verify subtleties].
   - `≠` different: same date but different size or content; or (with
     ignore date) different size/content.
   - `->` left newer (or left only) → copy left to right.
   - `<-` right newer (or right only) → copy right to left.
   - singles: present on one side only; shown with `->` or `<-` pointing to
     the missing side.
   - duplicates: present on both sides (equal or not) — the "duplicates" show
     button controls all pairs, "singles" all one-sided entries.
3. **Asymmetric**: the right side is a mirror of the left. Right-only files
   get the action **delete on right** (shown as `<-` with a red X / "delete"
   marker, but initially **not** selected for deletion — the user must mark
   them) [verify: in TC 11 they are shown and must be ticked for deletion in
   the Synchronize dialog]. How TC 11 marks files that are newer on the right
   in asymmetric mode is unverified. mormiCommander decision:
   asymmetric => right-newer pairs get action "copy left to right"
   (overwrite) but unselected; right-only entries get "delete right",
   unselected.
4. Folders: each folder row appears before its contents with its relative
   path (`\sub`). Folder rows have a symbol only if the folder is missing on
   one side.
5. While comparing, the status line shows the current folder and a
   `Cancel`/Esc stops; partial results remain.
6. Errors (unreadable folder): a message `Error reading directory <path>`
   with Retry/Skip/Cancel [verify]; Skip continues.

Content compare runs in a background thread; files being compared show a
`?` [verify] until done. Large-file content compare can be cancelled with
Esc.

### 9.4 Show filter buttons (F-549)

Toggle buttons; pressing one re-filters the list immediately without
re-comparing. Hidden categories are also excluded from Synchronize. Ours adds
hotkeys Ctrl+1..Ctrl+6 [ours].

### 9.5 List columns

Left half: `Name`, `Size`, `Date` (left file). Middle: action symbol
column (width ~ 30 px, coloured: green `->`/`<-`, red `≠`, grey `=`).
Right half: `Date`, `Size`, `Name` (right file), mirrored. Names appear
on both sides when present. Folder headers span the row. Newer date is shown
in the newer-colour (blue) [verify]. Column headers can be clicked to sort
[verify: TC 11 sorts by name only within folders].

### 9.6 Changing actions (F-550, F-552, F-553)

| Input | Effect |
|---|---|
| Click on middle column | cycles action for that row: `->` / `<-` / none [verify order] |
| Space / Ins | select row (selection highlight like panels) |
| Shift+click / Ctrl+click / Shift+arrows | multi-select |
| Num + / Num - | select / unselect group by mask [verify] |
| Right-click / Shift+F10 | context menu (below) |
| Double-click / Enter on a pair | open Compare by content (F-553) for the two files |
| Double-click on single | open file in Lister [verify] |
| F3 | View left file (or the single file) [verify: F3 left, Shift+F3 right] |
| F4 | Edit [verify] |
| Del | remove the action from the selected rows (set to none) [verify] |
| Ctrl+F | not used |
| Esc | stop compare; when idle, close window |
| Alt+F4 | close |

Context menu items (TC 11 English) [verify exact strings]:

```
Compare by content...
-----
Copy from left to right           (->)
Copy from right to left           (<-)
Remove selection                  (no action)
Reverse copy direction
-----
Delete on both sides              (TC 9+)
Delete left
Delete right
-----
Select for copying (group)        -> submenu: all, ->, <-
Unselect all
-----
View left / View right
Edit left / Edit right
Show in main window (left) / (right)   -- jumps the panel to the file
-----
Copy file names to clipboard
```

### 9.7 Synchronize... dialog (F-551)

Button `Synchronize...` opens a modal dialog:

```
Synchronize directories
  [x] left to right:  copy  12 files,  3.4 MB
  [x] right to left:  copy   2 files,  0.1 MB
  [ ] Delete on left:   0 files  [verify]
  [ ] Delete on right:  5 files  [verify, asymmetric only]
  [x] Confirm overwrite
  [ ] Copy NTFS permissions (Windows only)   [verify]
  [ ] Verify after copy                       [verify: TC 11 yes]
            [ OK ]  [ Cancel ]
```

- Counts reflect only **visible** (Show filter) rows that have an action.
- OK runs a file-operation job (internal job engine): progress dialog shows
  overall and per-file progress; `Background` button available [verify].
- Confirm overwrite on = the normal overwrite dialog (F-463) appears for each
  target that exists; off = overwrite silently.
- Deletions honour the "Delete to trash" setting.
- When finished, TC re-compares automatically and the processed rows become
  `=` (F-551 acceptance test). Rows that failed keep their action and a
  message lists errors.

### 9.8 Status line

`Total: N files (size)`, then counts per category:
`Equal: n`, `Different: n`, `Newer left: n`, `Newer right: n`,
`Only left: n`, `Only right: n` [verify wording; TC shows per-side
file/dir counts and bytes for the selected actions].

### 9.9 Save / load settings (F-554)

TC 7.5+ lets you store sync settings (both paths, mask, all checkboxes) as a
named **sync template** via a drop-down next to the paths or a menu
[verify]. Stored in `wincmd.ini` section `[SyncSettings]` (or `[SyncDirs]`)
as `Name_left=`, `Name_right=`, `Name_mask=`, `Name_flags=` [verify].
Command `cm_SyncDirs` accepts a parameter with the template name from button
bar/start menu [verify].

mormiCommander: a `Settings` drop-down button with `Save as...`, `Load >`
(list), `Delete >`. Stored in our INI section `[SyncTemplates]`.

### 9.10 Archives / FTP (F-555)

Either side may be an archive path (`C:\a.zip\dir`) or an FTP/SFTP
connection (the connection must already be open in a panel). `by content` on
remote/archive sides downloads to temp. FTP date resolution: TC ignores
seconds when one side is FTP (minute precision) [verify]. Ours: when either
side reports only minute precision (internal/vfs capability flag), compare
dates truncated to minutes.

### 9.11 wincmd.ini keys (sync)

| Section/Key | Meaning |
|---|---|
| `[Configuration] SyncFlags` [verify] | bitmask of last checkbox states |
| `[Configuration] SyncX/SyncY/SyncDX/SyncDY` [verify] | window geometry |
| `[SyncLeft]`, `[SyncRight]` [verify] | path histories |
| `[SyncMask]` [verify] | mask history |
| `[SyncSettings]` [verify] | saved templates |

---

## 10. Edge cases checklist (for tests)

Find files:
1. `*.*` matches `Makefile` (no extension).
2. `report` (no wildcard) matches `2026_report_final.pdf`.
3. `*.txt|old*` excludes `old.txt` and `Old_notes.txt` (case-insensitive).
4. `*.go|vendor\` and `*.go|vendor/` skip `vendor/x/a.go` but keep
   `myvendor.go`.
5. Quoted pattern `"a b*.txt"` matches `a bc.txt`.
6. Depth `current dir only` does not return `sub/a.txt`.
7. Not containing + Hex: binary files lacking `4D 5A` are listed.
8. UTF-16LE file with BOM containing "árvíztűrő" is found with UTF-16 on and
   ANSI off; not found when UTF-16 and UTF-8 are off.
9. NFD name from macOS Finder (`a\u0301rok.txt`) matches search `árok*`.
10. Find text never lists folders.
11. Size `= 1 kbytes` vs 1500-byte file (rounding rule).
12. Duplicate search with only `same size` groups unrelated equal-size
    files; with `same contents` they split.
13. Symlink loop `a/link -> ..` terminates with follow-symlinks on.
14. Cancel mid-search leaves partial results and status `[Search aborted]`.
15. Feed to listbox, delete one entry with F8: item disappears from list and
    from disk; Backspace returns to the previous folder.
16. Feed with results from two archives feeds the two archive files.
17. Unreadable subfolder is skipped silently.

Synchronize:
1. Identical trees: all `=`, Synchronize button shows 0 actions.
2. FAT 2-second difference gives `=`.
3. Exact 1 h difference gives `=` only with the timezone option.
4. Same date, different size gives `≠` and no default action.
5. Asymmetric: right-only file gets unselected delete-right action.
6. After Synchronize, auto re-compare shows all handled rows `=`.
7. Show filter `=` off hides equal rows and they are not counted.
8. Linux case-only difference (`A.txt` vs `a.txt`) with ignore-case off is
   two singles.

---

## 11. FEATURES.md ID map

| ID | Section |
|---|---|
| F-163, F-225, F-356 | 1 |
| F-164, F-357, F-543 | 1 |
| F-524 | 2 (note: results are not a tab) |
| F-525, F-530, F-837 | 3.1 |
| F-358 | 3.1 item 9, 3.6 |
| F-526, F-782 | 3.2 |
| F-527 | 3.3 |
| F-528, F-626 | 3.4 |
| F-529 | 3.5 |
| F-538 | 3.7, 3.8 |
| F-542 | 3.9, 7 |
| F-531 .. F-535 | 4 |
| F-536 | 5 |
| F-539, F-540 | 6.2 .. 6.4 |
| F-541, F-353 | 6.5 |
| F-544 | 6.6 |
| F-838 | 6.2, 6.7 |
| F-545 | 1, 3.2 |
| F-537, F-089, F-404, F-685 | 7 |
| F-443 | 7, 9.11 |
| F-825 | 8 |
| F-198, F-360, F-546 | 9.1, 9.2 |
| F-547, F-548 | 9.2, 9.3 |
| F-549 | 9.4 |
| F-550, F-552, F-553 | 9.6 |
| F-551, F-485 | 9.7 |
| F-554 | 9.9 |
| F-555 | 9.10 |
| F-562 | 9.3 (by content) |
| F-754 | 3.1, 10 |
