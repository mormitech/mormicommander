# Area 6 spec: Multi-Rename tool, Change attributes, Split/Combine, Checksums, Pack/Unpack dialogs

This document describes how Total Commander 11 (TC 11.0x to 11.5x) behaves in the five
"bulk file tool" areas, so that mormiCommander can reproduce that behaviour faithfully on
macOS, Windows and Linux. It is written for the people and agents who build the Go back-end
packages and wire the Wails UI: every section lists the dialog layout (controls, labels,
defaults, tab order), the hotkeys and mouse actions, edge cases and error messages, the
wincmd.ini keys TC uses to remember state, and the FEATURES.md IDs the section backs.
Where TC is Windows-specific (attribute bits, SFX stubs, external DLL packers) the
"mormi adaptation" notes say what we do instead.

## How to read this spec

Every behaviour carries a confidence tag, because parts of TC's UI are only documented in
the offline help file and in forum posts:

- **[V]** verified against a public source (TC history.txt, TotalcmdWiki, author forum posts).
- **[K]** well-established TC behaviour from long-term use; not re-verified for this pass.
- **[?]** plausible but unverified detail (exact label wording, default value); implement
  as described but treat as a candidate for correction when someone checks a real TC 11.
- **[M]** mormiCommander adaptation or extension; not TC behaviour.

Labels are given in TC's English UI wording. `&` marks the accelerator letter where known
(`&Archive` = Alt+A); exact accelerators are [?] unless stated. All
dialogs are modal to the main window unless stated otherwise; Esc = Cancel/Close,
Enter = default button, F1 = Help in every dialog [K].

General conventions that apply to every dialog in this area:

1. Dialogs act on the **selection** of the active panel; if nothing is selected they act on
   the item under the cursor. `..` is never part of the selection. [K]
2. The window title reflects what is acted on: one item shows its name, several show a
   count (TC 11.58 change for Change attributes, consistent elsewhere). [V]
3. Long operations (pack, unpack, split, combine, checksum creation) show TC's standard
   progress dialog with **Background** (moves the job to the background queue),
   **Pause**, and **Cancel** buttons [K]. In mormi this is the shared job engine progress
   UI owned by the architect; the tools below only emit progress events.
4. On Windows, paths over 259 characters are supported via `\\?\` prefixing; several TC 11
   fixes concern this (7z packer/unpacker, TAR aborts for >259 char archive names). [V]
   mormi: Go handles long paths natively on Windows since 1.17 with the manifest
   `longPathAware`; on Unix there is no such limit. [M]

---

## 1. Multi-Rename Tool (MRT)

FEATURES.md IDs: F-197, F-245, F-326, F-504, F-505, F-506, F-507, F-508, F-509, F-510,
F-511, F-512, F-513, F-514, F-515, F-516, F-517, F-518, F-519, F-834, F-835, F-836.

### 1.1 Invocation

| Trigger | Behaviour |
|---|---|
| `Ctrl+M` | `cm_MultiRenameFiles` on the selection (or item under cursor) [V] |
| Files menu > "Multi-&Rename Tool..." | same [K] |
| Default button bar "Multi-Rename Tool" button | same [K] (F-304) |
| `MULTIRENAME` internal command / `cm_MultiRenameFiles` with parameter `<UNDO>` | undoes the last MRT rename after asking for confirmation; `==<UNDO>` undoes without confirmation (TC 11.5x) [V] |
| Branch view (Ctrl+B) or search results ("Feed to listbox") as source | files from different folders are listed with their location; renames happen in place in each folder [V]-ish (TC fixed false duplicate warnings in branch view) |
| Invoked from tree view | base path is the tree folder (fixed 01.02.24) [V] |

If nothing renamable is selected (only `..`), the command does nothing [K]. Folders may be
selected together with files; MRT renames folders too (F-516) [K]. Inside archives and on
FTP, MRT works where the file system supports rename [K] (mormi: only where the VFS
reports rename capability; otherwise the menu item is disabled).

### 1.2 Window layout

The MRT is a **resizable, non-modal-feeling but modal** dialog (TC blocks the main window
while it is open) [K]. Size and position are remembered [K]. Layout top to bottom, left to
right; tab order follows this list:

**Row 1 – mask area (three group boxes side by side)**

1. Group box **"Rename mask: file name"** [K]
   - Combo-edit (editable, with history dropdown) — default `[N]` [V].
   - Buttons below the edit, each inserting a placeholder **at the caret** (TC 11.58: insert
     at cursor, previously appended) [V]:
     - `[N] Name` — inserts `[N]`.
     - `[N#-#] Range` — opens the *range picker* (see 1.5) for the name.
     - `[P] ...`/`[G] ...` are **not** separate buttons; they live in the "+" menu [?].
     - `[C] Counter` — inserts `[C]`.
     - `[YMD] Date` — inserts `[Y][M][D]` [K].
     - `[hms] Time` — inserts `[h][m][s]` [K].
     - `[=?] Plugin` — opens the field picker menu (built-in `tc.*` fields and content
       plugins) [V].
     - `+` (more) — menu with all other placeholders: `[P]`, `[G]`, `[d]`, `[t]`, `[U]`,
       `[L]`, `[F]`, `[n]`, `[X]`, `[I]`, `[v]`, literal `[[]`/`[]]` etc. [V] (the `[I]`
       placeholder "can be added via the + button", 02.06.23).
2. Group box **"Extension"** [K]
   - Combo-edit — default `[E]` [V].
   - Buttons: `[E] Extension`, `[E#-#] Range`, `[C] Counter`, `[=?] Plugin` [K].
3. Group box **"Search & Replace"** [K]
   - Label "&Search for:" + combo-edit (history) — default empty [K].
   - Label "R&eplace with:" + combo-edit (history) — default empty [K].
   - Checkboxes on one line [K]:
     - `^Aa` — case sensitive search (default off; search is case-insensitive) [K]
     - `1x` — replace only the first occurrence (default off) [K]
     - `[E]` — search/replace also acts on the extension (default off: only the name part
       is searched) [K]
     - `RegEx` — regular expressions (default off) [V]
     - `Subst.` — with RegEx: replace the **whole name** by the replace text, which may
       reference subexpressions `$1`..`$9`; unchecked replaces only the matched part [V]
4. Group box **"Upper/lowercase"** — dropdown, default "Unchanged" [K]. Items, in order:
   - `Unchanged`
   - `lowercase`
   - `UPPERCASE`
   - `First letter uppercase` (rest lowercase) [V]
   - `First letter of each word uppercase` [K]
   - `First of each word uppercase (English)` (TC 10.50+: keeps short English words like
     "a", "of", "the" lowercase, list in wincmd.ini `LowercaseStrings`; a trailing `!` in
     that list lets the last word stay lowercase) [V]
5. Group box **"Define counter [C]"** [V]
   - "Start at:" edit — default `1` [K]
   - "Step by:" edit — default `1`; negative allowed (counts down) [V]
   - "Digits:" dropdown — default `1` (no padding); values 1..10 [K]; values > 1 pad with
     leading zeros [V]
   - (TC 10+) "Interval" / "Reset at each folder" are **not** TC options [?]; F-507's
     "reset per folder" is an [M] extension: checkbox "Restart counter in each folder",
     default off.

**Row 2 – file list** (fills remaining space) [V]

- Columns: `Old name`, `New name`, `Size`, `Date`, and (branch view / search results)
  `Path` [K]. Clicking a column header sorts by it (name, extension, size, date) [V];
  the counter follows the **displayed order** (F-519) [V].
- Rows can be reordered by **drag and drop** or **Shift+Up/Down** (moves the focused
  row(s)) [V]. Moving the last item up by drag was fixed in 11.5x [V].
- Invalid results show `<Error!>` in the New name column [V]; conflicting or
  invalid-character names are highlighted (mormi: red text + tooltip with the reason) [M].
- New names update **live** on every keystroke in any mask/option control (F-505) [V].
- Clicking below the last row does not focus it (11.5x fix) [V].
- Ctrl+A selects all rows; Ctrl+C copies "old<TAB>new" lines [?].

**Row 3 – bottom buttons** (left to right) [V]

| Button | Hotkey | Behaviour |
|---|---|---|
| `F2 Load/save settings` (shows a dropdown menu) | `F2` | menu: saved setting names (click loads), "Save settings", "Save settings as...", "Delete entry", "<Default>" (reset all fields, F-518), "Load names from file...", "Load names from file with encoding...", "Edit names..." (opens new names in the editor, F-512), "Load names from clipboard" [V partially; exact item list ?] |
| `Start!` | Enter (default button) | performs the renames; warns on conflicts first (see 1.7) [V] |
| `Undo` | — | reverts the last rename batch in reverse order; works even after the dialog was closed and reopened (uses the undo log) [V] |
| `Result list` | — | shows/creates a log of old -> new names (opened in Lister/editor) [V] |
| `Close` | Esc | closes without renaming; mask/option state is remembered for the next opening (wincmd.ini) [K] |

Other MRT hotkeys:

| Key | Action | Tag |
|---|---|---|
| F2 | open load/save settings menu | [V] |
| F3 | "Edit names" — writes current new names to a temp text file and opens it in the configured editor; on save+close the edited lines become the new names | [K] (F-512) |
| F5 | "next step": applies the current result as the new input, so another rename pass can be chained before Start | [V] (wiki) |
| F10 | opens the same settings menu as F2 | [?] |
| Shift+Up/Down | move row | [V] |
| Alt+underlined letter | jump to control | [K] |
| Enter | Start! | [K] |
| Esc | Close | [K] |

### 1.3 Placeholders (exact semantics)

Placeholder names are **case-sensitive** [V]. Everything outside brackets is copied
literally. The final new name = `eval(nameMask)` + `"."` + `eval(extMask)`; if the
extension result is empty, **no trailing dot** is written [K].

Character positions are 1-based and counted in **characters (runes), not bytes**; TC 11.58
fixed multi-unit characters (emoji, surrogate pairs) in the range picker [V]. mormi counts
Unicode code points after NFC normalisation [M].

| Placeholder | Meaning | Tag |
|---|---|---|
| `[N]` | old name without extension | V |
| `[N5]` | character 5 only | V |
| `[N2-5]` | characters 2..5 | V |
| `[N2,5]` | 5 characters starting at 2 | V |
| `[N2-]` | from 2 to end | V |
| `[N-8,5]` | 5 characters starting at the 8th from the end | V |
| `[N-8-5]` | 8th-last to 5th-last | V |
| `[N2--5]` | from 2 to the 5th-last | V |
| `[N-5-]` | 5th-last to end | V |
| `[N-5]` | single character, 5th from end (fixed 12.03.26) | V |
| `[2-5]` (no letter) | characters 2..5 of the full name **including** extension | V |
| `[E]`, `[E1-2]`, all range forms | extension, same range syntax | V |
| `[P]`, `[P2-5]`, ... | parent folder name, same range syntax | V |
| `[G]`, ... | grandparent folder name | V |
| `[I]` / `[I1]` | like `[N]` but treats dots in **folder** names as part of the name (folder "v1.2" -> `[I]`="v1.2"); `[I0]` = normal behaviour | V |
| `[C]` | counter using the Define counter fields | V |
| `[C10+5:3]` | inline counter: start 10, step 5, 3 digits; any part optional: `[C10]`, `[C+5]`, `[C:3]` | V |
| `[C:a]` | auto digits: width = digits of the largest value in this batch | ? |
| `[Caa+1]` | letter counter starting at "aa" (aa, ab, ..., az, ba), width from start string | V |
| `[d]` | date in locale format, `/` replaced by `-` | V |
| `[Y]` `[M]` `[D]` | 4-digit year, 2-digit month, 2-digit day of the **modification** time | V |
| `[y]` | 2-digit year | ? |
| `[t]` | time in locale format, `:` replaced by `.` | V |
| `[h]` `[m]` `[s]` | 2-digit 24h hour, minute, second | V |
| `[v]` | milliseconds (3 digits), TC 11.5x | V |
| `[U]` | uppercase from here on | V |
| `[L]` | lowercase from here on | V |
| `[F]` | first letter of each word uppercase from here on | V |
| `[n]` | restore original case from here on | V |
| `[X]` | clipboard text; forbidden filename characters replaced with `_`; error if clipboard holds a zero-length string | V |
| `[X+]` | like `[X]` but empty clipboard inserts empty string (no error) | V |
| `[[]` / `[]]` | literal `[` / `]`; cannot be combined with other commands inside the same brackets | V |
| `[=plugin.field.unit]` | content field; `:4-7` suffix takes a substring; `[=tc.writedate.YMD]` style formatters | V |
| `[T4]` + date fields | take date from a field (e.g. recording date of MP4/HEIC) | V (06.05.24) |

mormi plugin fields (F-506, F-317): `[=tc.size]`, `[=tc.writedate]`,
`[=tc.creationdate]`, `[=exif.DateTimeOriginal.YMD]`, `[=image.width]`,
`[=image.height]`, `[=audio.artist]`, `[=audio.title]`, `[=audio.track]` — implemented in
Go, no DLL plugins [M]. Plugin field names are stored in English (TC 11.58) [V].

**Error cases for masks** (result shows `<Error!>` and Start is refused with a message):

- unknown placeholder letter, unbalanced `[`, range outside the string is **not** an error
  (yields the existing part / empty) [K];
- `[X]` with empty clipboard -> error [V];
- resulting name empty, `.` or `..` -> error [K];
- resulting name contains characters invalid on the target FS (Windows: `\/:*?"<>|`,
  control chars, trailing space/dot; macOS: `:` and `/`; Linux: `/` and NUL) -> error [K/M].

### 1.4 Processing order

For each file, in list order [V]:

1. Evaluate the name and extension masks (placeholders, case-switch placeholders).
2. Apply Search & Replace (on the name, or on name+ext when `[E]` is checked).
   - Plain mode: case-insensitive unless `^Aa`; `*` matches any run, `?` one character
     [V]; `|` separates several search terms and the matching replace terms positionally
     ("a|b" with "x|y" replaces a->x and b->y; one replace term with several search terms
     uses it for all) [V]. Enter `|` via the keyboard; the wiki notes Alt+124 [V].
   - RegEx mode: TC uses its own Perl-like engine; `$1`..`$9` in replace; `\U`, `\L`,
     `\F` in the replace text uppercase/lowercase/capitalize **only the next parameter**
     (TC 11.0 fix) [V]. mormi uses Go `regexp` (RE2) — lookbehind/backreferences in the
     pattern are not supported; the UI shows the regexp compile error under the field [M].
   - `1x` limits to the first occurrence per term [K].
3. Apply the Upper/lowercase dropdown [V].
4. [M] Optional "Remove accents" checkbox (F-836): NFD + strip combining marks
   (`árvíztűrő` -> `arvizturo`), applied last.

### 1.5 Range picker (`[N#-#]`, `[E#-#]`)

Opens a small dialog showing the **name of the focused file** as selectable characters
[K]. The user drags/selects a range with the mouse; the dialog offers radio buttons
"count from start" / "count from end" and generates `[N3-7]` / `[N-5-3]` accordingly [K].
If nothing is selected and measuring from the end, the preview was fixed in 11.58 [V].
OK inserts at caret; Cancel inserts nothing [K].

### 1.6 Saved settings (F-513)

- "Save settings" asks for a name (max 252 chars, TC 11.58) [V]; stores name mask,
  ext mask, search, replace, the four checkboxes, case mode, counter start/step/digits,
  and plugin parameters [K].
- Stored in wincmd.ini section **`[rename]`** as `name=...` style serialized entries
  (TC: one key per saved preset, 32767-character limit per entry as of 11.5x) [V for limit,
  ? for section name — wiki lists `[RenameTemplates]` for the last used strings].
- History of each combo-edit kept in `[RenameTemplates]` (or `[RenameHistory]`) [?].
- `<Default>` resets every field to its default; with defaults, every new name equals the
  old name [V].
- [M] mormi stores presets as JSON in the app config dir, not an ini file.

### 1.7 Executing: Start!, conflicts, errors (F-515, F-517)

1. Before renaming, TC checks for duplicate target names within the batch and for
   targets that already exist on disk and are not themselves part of the batch [V].
   Message (approx.) **"The following file names are duplicates: ..." / "File exists"**
   with options to continue or cancel [?].
2. Swaps and cycles (1.txt <-> 2.txt) are handled via temporary names, so they succeed
   (TC 11.0 fix) [V].
3. Case-only renames (`a.txt` -> `A.txt`) work on case-insensitive volumes (via temp name
   on Windows/macOS) [K] (F-487).
4. Per-file errors (access denied, file in use, name too long) bring up a dialog
   "Error renaming <old> to <new>!" with **Retry / Skip / Skip all / Cancel** [K].
5. After the batch, the list shows the new names as the new "old" names (so pressing
   Start again does nothing new) [K]; the panels reread when MRT closes [K].
6. Every successful rename is written to the **undo log** (TC: `%TEMP%\` file, survives
   dialog close; one level of undo) [V]. Undo renames back in reverse order; files that
   were changed afterwards or no longer exist are reported and skipped [K].
7. Renaming a file also renames its **descript.ion** comment entry (F-745) and, if
   `SetArchiveBitOnRename=1` (default), sets the archive bit on Windows [V].

### 1.8 Load names from file / clipboard (F-835)

- Each line is one new **full** name (name + extension) for the row at the same index [K].
  Unix LF and Windows CRLF both accepted (TC 11.0x fix 28.07.24) [V]; names may exceed
  259 chars when converted to UTF-8 (fix 25.06.23) [V].
- Encoding: "Load names from file" assumes ANSI or UTF-8 with BOM; the "with encoding"
  variant asks (ANSI, OEM/DOS, UTF-8, UTF-16) [K]. mormi: UTF-8 default with
  auto-detection of BOM/UTF-16; Latin-2/CP852 selectable [M].
- If the line count differs from the row count, TC shows an error and loads as many as
  match [K]; mormi shows a warning banner "N names loaded for M files" (F-835) [M].
- Loaded names become the name mask *per file*; the mask fields show `[=list]`-style
  read-only state until "<Default>" [?].

### 1.9 wincmd.ini keys used by MRT

| Section / key | Meaning | Tag |
|---|---|---|
| `[Configuration] LowercaseStrings` | words kept lowercase by "First of each word uppercase (English)" | V |
| `[Configuration] SetArchiveBitOnRename=1` | set archive bit on renamed files (Windows) | V |
| `[RenameTemplates]` | last used masks / history | V (name), ? (format) |
| `[rename]` | saved presets | ? |
| `[Configuration] MultiRenameUndoFile` / undo log in TEMP | undo storage | ? |
| window position keys (`MultiRenameWin`, ... per resolution) | size/position | ? |

### 1.10 mormi implementation notes

- Back-end package suggestion: `internal/mrt` (pure functions: `Plan(files, Options) []Result`,
  `Apply(plan) (UndoLog, []error)`, `Undo(log)`); UI-free and table-tested.
- The evaluation must be deterministic and fully previewable: `Plan` performs no I/O
  besides reading metadata passed in; `Apply` does the renames through the VFS.
- Undo log: JSON file in the app's data dir, one batch, containing absolute old/new paths
  and mtimes, so undo can detect files changed since.

---

## 2. Change attributes (cm_SetAttrib)

FEATURES.md IDs: F-194, F-315, F-316, F-317 (N/A plugins, built-in fields instead),
F-338, F-340, F-683, F-684, F-685, F-686, F-815.

### 2.1 Invocation

| Trigger | Behaviour |
|---|---|
| Files menu > "Change &Attributes..." | `cm_SetAttrib` [K] |
| No default hotkey in TC (assignable) | [K] |
| Button bar / context menu | via command [K] |

Acts on the selection (files and folders). In archives: not available except for a few
plugin FS; disabled in mormi for archives and FTP (except SFTP chmod later) [K/M].

### 2.2 Dialog layout (Windows TC 11)

Title: "Change attributes" + item name or "N files" (11.58) [V]. Tab order:

1. Checkbox **"Process &subdirectories"** — default off [K]. Only enabled if a folder is
   in the selection [K]. When checked, applies to all files below the selected folders
   (and, depending on the next checkbox, to the folders themselves).
2. (TC 9+) Checkbox **"Change attributes of folders too"** / "Process also folders" [?].
3. Group box **"Change attributes"** with tri-state checkboxes [K]:
   - `&Archive`
   - `&Read only`
   - `&Hidden`
   - `S&ystem`
   - `&Compressed` (NTFS; greyed on other FS) [K]
   - `&Encrypted` (NTFS EFS; greyed if unsupported) [?]
   Tri-state semantics: **checked = set**, **unchecked = clear**, **grey (indeterminate) =
   leave unchanged** [K]. Initial state: with one file, reflects its attributes; with
   several files, each box is grey unless all files agree [K] (TC had bug reports about
   "not properly initialized", forum p=335052).
4. Checkbox **"Change date/time:"** — default off [K]; when checked enables:
   - "Date:" edit (locale format, 4-digit year as of 11.0x) + date picker button [V]
   - "Time:" edit `hh:mm:ss`, fractions up to 3 digits allowed (11.5x) [V]
   - `Current` button — fills current date and time [K]
   - Prefill: with one file, that file's modification time; with several, the last used
     date (only if the date format did not change) [V]
5. Group box **"Load from file under cursor"** with button **"From active panel"** (and in
   older builds "From inactive panel") — copies the date/time (and attributes) of the file
   under the cursor in the other panel into the fields [V] (forum). Clicking the group box
   title no longer triggers the button (12.12.24) [V].
6. Group box **"Plugin attributes"** / checkbox **"Change plugin attributes:"** [V]:
   - Row: Plugin dropdown (e.g. `tc`), Property dropdown (e.g. `writedate`,
     `creationdate`, `accessdate`, `comment`), Value edit, `>>` button (load value from
     file under cursor), `+` button (add another row), `Delete` button [V/?].
   - Date fields use the same picker; invalid data shows current date instead of 1601 [V].
   - Value may contain MRT-like placeholders (e.g. `[=tc.writedate]`, `[N1-8]`) so a date
     can be set from the file name or EXIF (F-340) [V] (forum "set date from filename").
   - `tc.comment` sets descript.ion comments, Unicode capable [V].
7. Buttons: **OK**, **Cancel**, **Help** [K].

Errors: invalid date or time -> message **"Invalid input!"** and focus returns to the
field [V] (forum TC 11.02 report). Per-file failures (access denied, file in use) ->
"Error changing attributes of <file>" with Retry / Skip / Skip all / Cancel [?].

Order of application per file: attributes, then dates, then plugin fields [?]. Read-only
files: TC clears read-only temporarily if needed to set the date, then restores [K].

### 2.3 mormi adaptation (macOS/Linux) [M]

The dialog keeps TC's structure but swaps the attribute group by platform:

- **Windows**: exactly TC's group (Archive, Read only, Hidden, System, Compressed
  read-only display).
- **macOS/Linux**: group **"Permissions"**: a 3x3 tri-state grid Owner/Group/Others x
  Read/Write/Execute, plus tri-state `setuid`, `setgid`, `sticky`; an octal edit
  ("Octal:") kept in sync (shows `----` digits for mixed state); on macOS additionally
  tri-state `Hidden` (chflags `UF_HIDDEN`) and `Locked` (`UF_IMMUTABLE`) checkboxes.
- **Owner / Group** dropdowns (F-338), enabled only when running with permission to chown
  (root, or group change to a group the user belongs to); default "(unchanged)".
- Recursion options for permissions: "Process subdirectories" plus a dropdown
  "Apply to: files and folders / files only / folders only" (common need: `X` semantics).
- Dates: Modified (all platforms), Created (Windows; macOS via setattrlist; disabled on
  Linux), Accessed. Each has its own enable checkbox.
- Built-in "plugin" fields: tc.writedate, tc.creationdate, tc.accessdate, tc.comment,
  exif.DateTimeOriginal (source only), with the value template using MRT placeholders.

### 2.4 wincmd.ini keys

| Key | Meaning | Tag |
|---|---|---|
| `[Configuration] SetAttribLastDate` (last used date) | prefill for multiple files | ? |
| `[Configuration] SetArchiveBitOnRename` | see MRT | V |
| `[Configuration] ShowHiddenSystem` | affects whether files hidden by this dialog stay visible | V |
| `[Confirmation] OverwriteReadonly`, `OverwriteHidSys` | interplay with read-only/hidden flags set here | V |

---

## 3. Split file / Combine files

FEATURES.md IDs: F-328, F-329, F-687, F-688, F-763.

### 3.1 Split (cm_FileSplit)

Invocation: Files menu > "Spl&it File..." [K]; only for **one file** under the cursor (a
selection of several files or a folder -> message "Please select a file first" /
the command splits only the file under cursor) [?]. Not available inside archives [K].

Dialog **"Split"** [K], tab order:

1. Label "Split `<filename>` to:" + edit with target directory — default: the path of the
   **opposite panel** [K]; `>>`/Tree button to browse [K].
2. Label "Bytes per file:" + editable combobox [K], items:
   - `Automatic` (only meaningful for removable media: fills the free space of each disk,
     asks for the next disk) [K]
   - `1457664 (3.5")` [K]
   - `1213952 (5.25")`, `730112`, `362496` (legacy floppy sizes) [K]
   - `100 MB`, `250 MB` (ZIP disks) [?]
   - `650 MB CD-R`, `700 MB CD-R` [K]
   - `4482 MB DVD` [K] (4.7 GB DVD-R), `8152 MB DVD DL` [?]
   - `25 GB BD-R` [?]
   - custom value typed by the user; suffixes `k`, `M`, `G` (powers of 1024) accepted, a
     plain number = bytes [K].
   Default: the last used value [K].
3. Checkbox **"Create CRC32 file"** / "Create .CRC file" — default on [?]. (TC always
   writes the `.crc` file in older versions [K].)
4. (TC 9+) Checkbox "Split in background" is not present; Background is available from the
   progress dialog [K].
5. Buttons: **OK**, **Cancel**, **Help**.

Output: `<name>.001`, `<name>.002`, ... (three digits; beyond 999 continues `.1000`) [K]
where `<name>` is the full original file name **without** its extension (TC: `file.iso`
-> `file.001`; the `.crc` file restores the original name) [K]. Companion file
`<name>.crc` with exactly these lines (CRLF) [K]:

```
filename=file.iso
size=4700000000
crc32=1A2B3C4D
```

CRC32 is uppercase hex, 8 digits [K]. Errors:

- target exists -> standard overwrite dialog per part [K];
- part size larger than file -> message "File is smaller than the requested part size"
  and nothing is created [?] (TC actually creates a single `.001` [?]); mormi: warn and
  ask "Create one part anyway?" [M];
- part size 0 or unparsable -> "Invalid input!" [K];
- disk full / Automatic mode on removable media: prompt "Insert next disk" [K]; mormi
  does not support media swapping — "Automatic" is dropped, replaced by
  "Fit to FAT32 (4 GiB - 1)" preset [M].

wincmd.ini: `[Configuration] SplitBytesPerFile` (last split size) [?];
`SplitBytesPerFileZip` holds the last ZIP multi-disk part size (TC 11.0) [V].

### 3.2 Combine (cm_FileCombine)

Invocation: Files menu > "Com&bine Files..." [K]. Selection rules [K]:

- cursor on (or selection of) `name.001`: TC finds `.002`, `.003`, ... automatically.
- If several parts are selected, they are combined **in name order**; if the `.crc` file
  is present it determines the target name and expected size/CRC.
- Selecting files that are not numbered parts: TC combines the selected files in their
  current **sort order** (works as a generic concatenation tool) [K]; it asks for the
  target name.

Dialog **"Combine"**: "Combine to:" edit — default `<opposite panel path>\<original
filename from .crc or name without .001>` [K]; OK / Cancel / Help.

Behaviour:

1. Parts are appended in order; a missing part in the sequence (e.g. `.003` absent while
   `.004` exists, or last part missing per `.crc` size) -> prompt asking for the next part
   location ("Please insert disk with file ... / Cannot find file ...") with Retry /
   Cancel [K].
2. After combining, if a `.crc` file was found: size and CRC32 are compared. Mismatch ->
   error **"CRC error! The combined file is damaged!"** (approx. wording) and the target
   file is kept for inspection [?]. Match -> no message (silent success) [K].
3. Without `.crc`: no verification; combining stops at the first missing number [K].

mormi: also recognise `name.ext.001` (7-Zip/HJSplit style) and `.sfv`/`.md5` alongside
parts for verification [M].

---

## 4. Checksums (create / verify)

FEATURES.md IDs: F-332, F-333, F-689, F-690, F-691, F-693.

### 4.1 Create (cm_CRCCreate)

Invocation: Files menu > "Create Checksum File(s) (CRC32, MD5, SHA)..." [K]. Acts on the
selection; folders are processed recursively [K].

Dialog **"Create CRC checksums"** [K], tab order:

1. Label "Create checksum file(s) for N files in:" + edit with **target file name** —
   default `<current dir>\<name>.<ext>` where name is the single file's name, or the
   directory name for several files [K].
2. Checkbox **"Create separate checksum file for each file"** [V] — default off.
3. Checkbox **"Separate checksum file for each directory"** [V] (11.5x: unchecking it no
   longer loses the base name for one file).
4. Group box with radio buttons for the method [K/V] (default last used):
   - `CRC32 (SFV format)` -> `.sfv`
   - `MD5` -> `.md5`
   - `SHA1` -> `.sha`
   - `SHA256` -> `.sha256`
   - `SHA512` -> `.sha512`
   - `SHA3-224`, `SHA3-256`, `SHA3-384`, `SHA3-512` -> `.sha3-224` ... (TC 11.0x) [V]
   - `BLAKE3` -> `.blake3` (TC 10.50+) [V]
   (SHA224/SHA384 are verify-only, accepted from clipboard [V].)
5. Checkbox **"Unix format"** — LF line endings and `/` path separators [K].
6. Checkbox **"UTF-8"** — write names in UTF-8 (default on in TC 11) [?].
7. Buttons: **OK**, **Cancel**, **Help**.

File format (matches md5sum/sha*sum, F-332) [K]:

```
d41d8cd98f00b204e9800998ecf8427e *empty.txt
```

- Hash lowercase hex, one space, `*` (binary marker), relative path from the checksum
  file's folder (`subdir\file` or `subdir/file` in Unix format).
- SFV: `; Generated by ...` comment lines, then `filename<space>CRC32HEX` (uppercase) [K].
- `CrcAppendExt=1`: append extension to the per-file checksum name
  (`file.txt.md5` instead of `file.md5`), also when no separate option is checked (11.5x) [V].

Errors: unreadable file -> dialog with **Retry / Skip / Skip all / Cancel** (11.5x) [V];
cannot write checksum file (disk full) -> error (TC 11.0) [V].

Performance notes [V]: TC reads via memory mapping, multi-threaded BLAKE3 on >1 MB files
on local SSDs with >= 8 cores; `CrcBlake3BlockSize` tunes it (0 = 256k blocks no mapping,
1 = no multi-threading, max 2047 MB). mormi: Go `crypto/*`, `hash/crc32`,
`lukechampine.com/blake3` or `github.com/zeebo/blake3`; streaming 1 MiB buffers;
parallelise across files, not within a file [M].

### 4.2 Verify (cm_CRCCheck)

Invocation: Files menu > "Verify Checksums (CRC32, MD5, SHA)..." [K], or **double-click /
Enter on a checksum file** (checks only that file; if it is part of the selection, all
selected checksum files) [V]. Recognised extensions: `.sfv .md5 .sha .sha1 .sha256
.sha512 .sha224 .sha384 .sha3-224 .sha3-256 .sha3-384 .sha3-512 .blake3 .bk3` [V].
Method detection: by extension, then by hash length; when several methods share a length
(SHA256 vs SHA3-256 vs BLAKE3-256), the extension decides; for clipboard input the user
chooses (11.0x) [V].

Verify from clipboard: if the clipboard contains a bare hash and one file is selected,
TC compares that file against it [V].

Result window **"Verify checksums"** [K/V]:

- List: one line per entry with status `OK`, `FAILED` (checksum mismatch),
  `Error reading file` / `File not found` (missing) [K].
- Summary line: "OK: n, Failed: n, Missing: n" [?].
- Checkbox **"Only errors"** filters the list (11.0x) [V].
- Ctrl+A selects all, Ctrl+C / "Copy" copies the selected lines [V].
- Buttons: **Close**, "Copy to clipboard"; "Feed to listbox" of failed files [?].
- Parsing rules [V]: two spaces or space+`*` separate hash and name (only two spaces
  skipped unless the name starts with `*`); names >259 chars and leading `./` or `.\`
  handled; checksum files > 2 GB rejected with an error.

### 4.3 Checksum to clipboard / single file (F-691, F-693)

TC 11 has no dedicated "copy checksum" command; users run Create with target "clipboard"
[?]. mormi [M]: context submenu "Copy checksum" -> MD5 / SHA1 / SHA256 / BLAKE3 copies
`hash  name` lines for the selection; Properties dialog shows a "Checksum" row computed on
demand.

### 4.4 wincmd.ini keys

| Key | Meaning | Tag |
|---|---|---|
| `[Configuration] CrcAppendExt=0/1` | keep original extension in per-file checksum names | V |
| `[Configuration] CrcBlake3BlockSize` | BLAKE3 threading block size | V |
| `[Configuration] lastmd5` / `CRCmethod` | last used method | V (lastmd5, legacy), ? |
| `[Configuration] CRCUnix`, `CRCUtf8` | last used checkbox states | ? |

---

## 5. Pack files dialog (Alt+F5)

FEATURES.md IDs: F-156, F-222, F-223, F-278, F-318, F-598, F-599, F-604, F-605, F-616,
F-624, F-625, F-627 (N/A), F-429, F-430.

### 5.1 Invocation

| Trigger | Behaviour |
|---|---|
| `Alt+F5` | `cm_PackFiles` — pack selection [V] |
| `Alt+Shift+F5` | `cm_PackFiles` with **Move to archive** pre-checked [K] |
| Files menu > "&Pack..." | same [K] |
| Drag & drop onto an archive / F5 into a panel showing an archive | adds files without the dialog (F-615) [K] |
| `cm_PackFiles` with parameters (TC 11.0) | `/T="target"` (`""` or `"."` = source dir + default name; `c:\path\.` = path + default name) [V]; `/E="zip"` packer/extension [V]; `/C=n` encryption method (-1 default, 0 ZipCrypto, 1 AES128, 2 AES192, 3 AES256) [V]; `/F="..."` extra packer options [V]; `/D` `/R` `/A` etc. flag letters + 0/1 for the checkboxes (example `/D1R1A0`), `/G` start without showing the dialog [V-ish] |

### 5.2 Dialog layout

Title **"Pack files"** [K]. Tab order:

1. Label "Pack N file(s) to the archive:" (or "Pack `name` to the archive:") [K]
   + combo-edit with the target archive path — default
   `<opposite panel path>\<name>.<ext>` where name = the single selected item's name
   (without extension for files) or the **current folder name** for several items [K].
   The extension follows the selected packer and updates when the packer changes [K];
   `ExtNeverUppercase=1` prevents an uppercase extension (11.5x) [V].
2. Group box **"Packer"** with radio buttons [K]: `ZIP`, `ARJ`, `LHA`, `RAR`, `UC2`,
   `ACE`, `TAR`, `TGZ`, `TBZ`, `7Z` (TC 11 has internal 7z via its DLL [V]), and a
   dropdown `->` listing packer plugins (gz, bz2, xz, zst, cab, ...) [K]. Default:
   `LastUsedPacker` [V]. **Configure...** button opens the packer settings for the chosen
   packer (Configuration > Packer page, or plugin options) [K].
3. Compression rate control (TC 11.0): a dropdown/edit "Rate" next to ZIP/GZ/TGZ, 0..9
   (GZ/TGZ use 1 as minimum), changed value saved [V].
4. Checkboxes, in order [K]:
   - **"Also pack &path names (only recursed)"** — default `ZIPDirectoryOfFile=1` [V]
   - **"&Recursively pack subdirectories"** — default `ZIPRecurseSubdirs=1` [V]
   - **"&Multiple disk archive"** — default off; enables part size prompt
     (`SplitBytesPerFileZip` remembers it) [V]
   - **"Mo&ve to archive"** — default off (on via Alt+Shift+F5) [K]
   - **"Create self extracting archive (.EXE)"** — ZIP/ARJ/ACE/RAR/7Z only; mutually
     constrained with Multiple disk (switching to ZIP/ARJ clears SFX when multi-disk is
     checked, 11.58) [V]
   - **"Create separate archives, one per selected file/dir"** — default off; target name
     may then contain `*` meaning each item's name (`*.zip`) [V]
   - **"&Encrypt"** — ZIP (internal), RAR, 7z, ACE; prompts for the password after OK
     (twice for confirmation); ZIP method choice ZipCrypto / AES-128/192/256 [V via /C]
5. Buttons: **OK**, **Tree** (choose target folder), **Cancel**, **Help**,
   **Configure...** [K].

### 5.3 Behaviour and edge cases

- Target archive exists and is a supported, writable format -> files are **added**
  (updated); TC does not ask first [K]. For TAR/TGZ (non-updatable streams) TC rebuilds
  [K]. Different-format file with the same name -> error "Not a valid archive" [?].
- "Move to archive": sources deleted only after the archive was written and (for ZIP,
  `VerifyZIP=1` default) **tested** successfully [V]. mormi: always verify before deleting
  (F-616) [M].
- Invalid characters in the archive name -> warning offering to replace with `_`
  (11.0) [V].
- Packing a folder into itself (target inside a source folder) -> the archive being
  written is skipped [K].
- External packer not found (RAR/ARJ/ACE path empty) -> message asking to configure the
  path in Configuration > Packer [K] (F-624).
- Empty selection (only `..`) -> nothing happens [K].
- Progress: standard dialog with Background [K]; `AlwaysPackInBackground` (0/1/2/3)
  selects default [V].
- Packing to zip64 when > 4 GB or > 65535 entries, automatically [K]; Windows <10
  warning for > 2 GB zips disabled by `WarnTooBig=0` default [V].
- UTF-8 names in ZIP: `[Packer] PreferUtf8ForZip=1` sets the UTF-8 flag (bit 11) [V].
- Archive date: option on Configuration > Packer "Set archive date to newest file"
  (`ZipSetDateToNewest`, applies to 7z/RAR/ACE/LHA since 11.0) [V].

### 5.4 mormi packer set [M]

Radio list replaced by a dropdown "Packer:" with: `zip` (internal, default), `tar`,
`tar.gz (tgz)`, `tar.bz2 (tbz)` (write via external bzip2 or Go lib), `tar.xz`,
`tar.zst`, `gz`, `bz2`, `xz`, `zst` (single-file packers: disabled when more than one
file or any folder is selected, matching TC's GZ plugin behaviour), `7z` (create only if
external `7z`/`7zz` is configured), `rar` (only if external `rar` is configured).
SFX is N/A (F-627): checkbox hidden. Compression level slider 0..9 for zip/gz/zst/xz.

### 5.5 wincmd.ini keys ([Packer] unless noted)

| Key | Default | Meaning | Tag |
|---|---|---|---|
| `ZIPDirectoryOfFile` | 1 | pack path names | V |
| `ZIPRecurseSubdirs` | 1 | recurse subdirectories | V |
| `LastUsedPacker` | 1 | last packer radio | V |
| `DefPlugin` | — | last used packer plugin | V |
| `InternalZIP` | 1 | use internal ZIP packer | V |
| `InternalZipRate` | 6 | ZIP rate 0..9 | V |
| `VerifyZIP` | 1 | test after packing | V |
| `ZipSetDateToNewest` | 0 | archive date = newest file | V |
| `PreferUtf8ForZip` | 1 | UTF-8 names | V |
| `ExtNeverUppercase` | 0 | lowercase extensions | V |
| `7zSolidSize` | — | 7z solid block MB | V |
| `KeepPackerPassword` | — | password memory (4 = forget on leaving header-encrypted archives) | V |
| `ZIP`, `RAR`, `ARJ`, `ACE`, `LHA`, `UC2` | — | external packer paths | V |
| `LinuxCompatible` | 1 | TAR flavour | V |
| `[Configuration] AlwaysPackInBackground` | 0 | background default | V |
| `[Configuration] SplitBytesPerFileZip` | — | last multi-disk part size | V |
| `[Configuration] WarnTooBig` | 0 | >2 GB zip warning | V |

---

## 6. Unpack files dialog (Alt+F9) and Test archives

FEATURES.md IDs: F-159, F-169, F-224, F-227, F-228, F-160, F-319, F-320, F-612, F-613,
F-614.

### 6.1 Invocation

| Trigger | Behaviour |
|---|---|
| `Alt+F9` | `cm_UnpackFiles` on the selected archives (or archive under cursor); a `.lnk` whose target is an archive unpacks the target (11.5x) [V] |
| `Alt+F6` | TC 11 default keymap also maps unpack [?] (FEATURES F-224 says so) |
| Files menu > "&Unpack..." | same [K] |
| `Alt+Shift+F9` | `cm_TestArchive` [K] |
| Inside an open archive, F5 | copy-out dialog (not this dialog) [K] |
| `cm_UnpackFiles` parameters (TC 11.0) | target, `/S` (separate subdirs, `/S1`/`/S0`), `/P="password"` (also in background), flags for the checkboxes, start immediately [V] |

### 6.2 Dialog layout

Title **"Unpack files"** [K]. Tab order:

1. Label "Unpack specific files from archive to:" (several archives: "Unpack N archives
   to:") + combo-edit — default: **opposite panel path** [K].
2. Label "Files to unpack:" + combo-edit — default `*.*` [K]; standard TC mask syntax
   (`*.txt *.doc | old*`) [K].
3. Checkbox **"Unpack path names if stored with files"** — default `ExtractPathInZIP=1` [V].
4. Checkbox **"Overwrite existing files"** — default `UnZIPOverwrite=0` [V]; when off, the
   standard overwrite dialog appears per conflict (incl. "Auto-rename target files") [K].
5. Checkbox **"Unpack each archive to a separate subdir (name of the archive)"** — default
   off, remembered as `UnZIPSeparateSubdirs` (11.5x) [V].
6. Buttons: **OK**, **Tree**, **Cancel**, **Help** [K].

### 6.3 Behaviour and edge cases

- Several archives selected: each is unpacked in turn; with "separate subdir" each goes to
  `target/<archive name without extension>/` [K]. Non-archives in the selection are
  skipped (or error "Not an archive" at the end) [?].
- Multi-volume: select the first part (`.zip`/`.z01`, `.7z.001`, `.part1.rar`); later parts
  are ignored if selected (testing `.7z.002+` is skipped, 11.0x) [V]. Missing part ->
  "Insert next disk / file not found" with Retry/Cancel [K]; tc7z reports missing RAR
  volumes as a warning [V].
- Encrypted archives: password prompt; wrong password -> "CRC error / wrong password",
  prompt again [V]. Bad CRC on extracted file: deleted unless `NoDelete=1` [V].
- Path traversal (`..\`) in archive names is neutralised (11.5x) [V]. mormi: refuse any
  entry resolving outside the target and absolute paths; symlink entries never followed
  during extraction [M].
- Progress with Background; `AlwaysUnpackInBackground` default [V].

### 6.4 Test archives (Alt+Shift+F9)

Tests every selected archive; shows a per-archive result list: "OK" or the error ("CRC
error in file x", "Unexpected end of archive", "Wrong password") [K]. Continues past
failures for multiple 7z archives (11.5x) [V]. Final summary message box
"N archives OK, M with errors" [?]. `ZipTestWarnExplorer=0` (default) suppresses the
warning about ZIPs Explorer cannot open [V].

### 6.5 wincmd.ini keys

| Key | Default | Meaning | Tag |
|---|---|---|---|
| `[Packer] ExtractPathInZIP` | 1 | unpack path names | V |
| `[Packer] UnZIPOverwrite` | 0 | overwrite without asking | V |
| `[Packer] UnZIPSeparateSubdirs` | 0 | one subfolder per archive | V |
| `[Packer] NoDelete` | 0 | keep files with bad CRC | V |
| `[Packer] OpenPartial` | 0 | open partial (truncated) ZIPs | V |
| `[Packer] InternalUnzip`, `InternalUnrar`, ... | 1 | use internal unpacker | V |
| `[Packer] ZipTestWarnExplorer` | 0 | Explorer-compat warning on test | V |
| `[Configuration] AlwaysUnpackInBackground` | 0 | background default | V |
| `[Configuration] FirstTimeUnpack` | 0 | first-use hint | V |

---

## 7. Cross-area acceptance checklist (for test writers)

1. MRT with mask `[N]_[C]`, counter 1/1/1 on `a.txt b.txt` -> `a_1.txt b_2.txt` (F-505).
2. `[N2-3]` on `photo.jpg` -> `ho.jpg`; `[N-3-]` -> `oto.jpg`; `[2-5]` on `ab.cdef` uses the
   full name `ab.c` (F-834).
3. `[C10+5:3]` on three files -> `010 015 020`.
4. Search `a|b`, replace `x|y`, on `abc` -> `xyc` (F-509).
5. RegEx `(\d+)-(\d+)` Subst. replace `$2-$1` on `12-34` -> `34-12` (F-508).
6. Swap `1.txt`/`2.txt` succeeds; Undo restores (F-514).
7. Duplicate result names are refused before any rename happens (F-515).
8. Change attributes tri-state grey leaves bits unchanged on mixed selection (F-315).
9. Split 10 MiB file at 4 MiB -> `.001` 4 MiB, `.002` 4 MiB, `.003` 2 MiB, `.crc` with
   size and CRC; Combine reproduces the file, corrupting a byte gives the CRC error (F-328,
   F-329).
10. Create MD5 for a folder -> lines readable by `md5sum -c`; verify reports OK / FAILED /
    missing correctly (F-332, F-333).
11. Pack with "Move to archive" deletes sources only after a passing test (F-616).
12. Unpack with "separate subdir" on two archives creates two folders named after them
    (F-612).

## Sources

- TC history.txt (author changelog): https://www.ghisler.com/history.txt
- TotalcmdWiki, Multi-rename tool: https://www.ghisler.ch/wiki/index.php?title=Multi-rename_tool
- TotalcmdWiki, wincmd.ini: https://www.ghisler.ch/wiki/index.php?title=Wincmd.ini
- Forum threads on Change attributes and cm_PackFiles parameters (ghisler.ch/board,
  e.g. viewtopic.php?p=335052, p=366747, p=433268, p=465949).
