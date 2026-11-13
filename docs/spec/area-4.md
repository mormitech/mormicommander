# Area 4 spec: Lister, quick view, editor launching, file associations, compare by content

This document describes how Total Commander 11 behaves in five related areas: the built-in viewer Lister (F3), the quick view panel (Ctrl+Q), launching the editor (F4, Shift+F4), file associations (Enter, Alt+F3, internal associations), and the compare-by-content tool. It is written so the UI and backend agents can build mormiCommander without owning a copy of TC. Each section lists the matching FEATURES.md IDs. Where TC behaviour is Windows-only or cannot be confirmed from the help, the wiki or the forum, the text says so with **[unverified]** or **[ours]**, and gives the mormiCommander behaviour to build instead. Key names follow TC; on macOS, Ctrl in a Lister or compare window may also be offered as Cmd (see docs/keyboard.md once it exists).

Sources: the TC help (lister.htm, configuration pages), the TotalcmdWiki pages "Lister" and "Wincmd.ini", forum threads on ghisler.ch, and knowledge of TC 11 behaviour.

---

## 1. Lister (built-in viewer)

FEATURES: F-148, F-149, F-151, F-176, F-177, F-203, F-213, F-220, F-798, F-564 to F-590 (F-585 is N/A), F-840 to F-846, F-852, F-540, F-648.

### 1.1 Opening Lister

| Trigger | Command | Behaviour |
|---|---|---|
| F3 | cm_List | Opens Lister on the file under the cursor. If files are marked, Lister opens the **first marked file** and the marked set becomes the N/P list (F-151). On a folder, F3 calculates the folder size, shows it in the status bar and does not open Lister (TC behaviour); on `..` it does nothing. |
| Shift+F3 | cm_List with "current file only" | Opens only the file under the cursor, ignoring the selection (F-149, F-213). |
| Alt+F3 | cm_ListExternal | Starts the "other" viewer: the external viewer if F3 is internal, or Lister if F3 is set to external (F-176, F-220). With `Viewer=none`, Alt+F3 falls back to the `[Extensions]` mapping and finally to Lister. |
| Alt+Shift+F3 | cm_ListInternalOnly | Opens Lister with plugins and the image/multimedia viewer turned off, so the file shows as text, binary or hex (F-177, F-798). |
| Double click with Shift (option) | — | **[unverified]** Not standard; do not bind. |
| Command line `TOTALCMD.EXE /S=L file` | — | Starts only Lister for that file, without the main window (F-852). Ours: `mormicommander /S=L <file>`. |
| Find Files results, F3 | — | Opens Lister on the found file; N/P walks the result list (F-540). |
| F3 on a file in an archive or on FTP/SFTP | — | The file is unpacked or downloaded to a temp folder, Lister opens the temp copy, and the temp copy is deleted when Lister closes (F-648). A progress dialog appears for large or remote files; Cancel aborts and Lister does not open. |

Each F3 opens a **new, independent Lister window** (TC default), so several Listers can be open at once (F-846). The option "Lister: Only one instance" **[unverified, TC has 'Use single Lister window' in recent versions]** makes F3 reuse the open window; we add it as an option, default off.

### 1.2 Window

- Title bar: `Lister - [full path of file]`. When the mode is forced (e.g. hex) the title does not change.
- Size and position are stored in `[Lister]` `x`, `y`, `dx`, `dy`, and `Maximized=1` opens it maximised. Every new Lister opens at the stored geometry; closing a Lister stores its geometry (F-846). Ours: store in our config under `lister.window`.
- Layout, top to bottom: menu bar (File, Edit, Options, Plugins/Image, Encoding, Help), content area, no toolbar, no status bar. The current position is shown in the title bar as a percentage at the right end in TC 9+ (`Lister - [file] 34%`) **[unverified wording]**; ours shows a slim status line with mode, encoding and percent instead.
- Esc closes the window. Q is **not** a TC close key (FEATURES F-844 says "also Q"; keep Q as our extra, but only when no text field has focus). Alt+F4 also closes. Focus returns to the main window and the panel cursor stays where it was.
- Lister keeps the file open read-only with sharing allowed for read and write, so other programs can still write to the file while it is viewed.

### 1.3 View modes and keys

The mode keys work in the main area (not in the search dialog). The Options menu shows the same items with a radio mark.

| Key | Mode | Menu text (Options) | Notes |
|---|---|---|---|
| 1 | Text | `Text` | Line-oriented text, wrap per `Wrap`. (F-568) |
| 2 | Binary | `Binary` | Fixed line width `binwidth` (default 75) characters, no line breaks interpreted. (F-566) |
| 3 | Hex | `Hex` | `OFFSET: XX XX ... XX  ascii` 16 bytes per row, offset as 8 hex digits (16 for files over 4 GB). (F-567) |
| 4 | Image/Multimedia/Plugins | `Image/Multimedia, Lister plugins, RTF` | Images, sound, video, RTF, plugins. (F-569) |
| 5 | HTML as text | `HTML text` | Tags stripped, links shown in braces when `LinkBraces=1`. Ours: F-571 adds a rendered HTML/Markdown mode on the same key, with scripts and remote loads off. |
| 6 | Unicode (UTF-16 LE) | `Unicode` | FEATURES F-841 says U; TC uses 6 (and U as shortcut in some builds **[unverified]**). Bind both. |
| 7 | UTF-8 | `UTF-8` | FEATURES F-841 says 8; TC uses 7. Bind both 7 and 8. In HTML mode, 7 forces UTF-8 decoding of the HTML. |
| A | ANSI charset (Windows code page) | `ANSI (Windows charset)` | Font/charset sub-mode, combined with 1/2. (F-840) |
| S | ASCII / OEM (DOS charset) | `ASCII (DOS charset)` | (F-840) |
| V | Variable char width | `Variable char width` | Uses the variable-width font `Font3`. (F-840) |
| W | Wrap lines on/off | `Wrap text` | Toggles and saves `Wrap`. (F-572) |
| F2 | Reload file | `File > Reload` **[unverified menu entry]** | Re-reads from disk, keeps position if the file is still that long. (F-842) |
| N / P | Next / previous file | `File > Next / Previous` | (F-583) See 1.6. |
| F7 or Ctrl+F | Find | `Edit > Find` | (F-575) |
| F3 | Find next | `Edit > Find next` | (F-576) |
| Shift+F3 | Find previous | — | (F-576) |
| Ctrl+C / Ctrl+Insert | Copy selection | `Edit > Copy as text` | (F-577) |
| Ctrl+A | Select all | `Edit > Select all` | (F-577) |
| Ctrl+P | Print | `File > Print` | (F-581, F-843) |
| Ctrl+S | **[ours]** Save selection as | `File > Save as` | TC has `File > Save as...` without a key (F-582). |
| Ctrl+G | **[ours]** Go to line / offset | `Edit > Go to` | TC has no go-to dialog in text mode; hex mode jump is ours (F-578). |
| + / - | Image zoom in / out | — | (F-845) |
| F or `Fit image to window` | Toggle fit | `Image > Fit image to window` | ShrinkBitmap key (F-573). |
| L / R | Rotate left / right **[unverified keys]** | `Image > Rotate` | (F-574) |
| Alt or F10 | Open menu bar | — | |
| Esc | Close (or cancel a running search) | — | (F-844) |

Auto mode (`Startup=0`) picks the mode per file: images and media go to mode 4 if `BmpStartup=1`/`Multimedia=1`; files with a high share of non-text bytes go to binary... in TC they still open in text mode with control chars shown; ours: if more than 10 % of the first 8 KB are NUL or C0 control bytes (excluding tab, CR, LF, FF), open in hex. Text encoding in auto mode: BOM first (UTF-8, UTF-16 LE/BE), then UTF-8 validity check of the first 64 KB, then the default charset (F-565). Hungarian `áéíóöőúüű` in UTF-8 without BOM must show correctly.

`Startup` values: 0 auto, 1 text, 2 binary, 3 hex, plus charset flag 16 ANSI, 32 ASCII, 48 variable (added). Ours: `lister.defaultMode` and `lister.defaultCharset`.

### 1.4 Scrolling and mouse

- Up/Down: one line. PgUp/PgDn and Space/Shift+Space: one screen. Home/End or Ctrl+Home/Ctrl+End: start/end of file. Left/Right: horizontal scroll when wrap is off (in text mode with wrap on, they do nothing).
- Mouse wheel scrolls 3 lines; Ctrl+wheel **[ours]** changes font size.
- Left drag selects text; double click selects a word; Shift+click extends. Selection is shown with `FgSelColor`/`BgSelColor`.
- Right click: context menu with Copy, Select all, mode items **[unverified exact list]**.
- Large files (F-579): Lister never loads the whole file. It maps or reads windows around the visible position. A 10 GB file must open in under a second and the scroll thumb maps to the byte offset (in text mode, scroll is by byte position snapped to the next line start, as TC does).

### 1.5 Find dialog (F7 / Ctrl+F)

Title: `Find`. Controls in tab order:

1. `Search for:` combo box with history (last 20 entries, stored in `[SearchText]` section, shared with other TC searches **[unverified section name]**).
2. `[ ] Whole words only`
3. `[ ] Case sensitive`
4. `[ ] Hex` (the search text is hex bytes, e.g. `4D 5A`)
5. `[ ] Search backwards`
6. `[ ] Regular expressions` (TC 9+)
7. `[ ] Unicode` / `[ ] UTF-8` — search the text encoded that way (TC shows these on the right) **[unverified exact labels]**
8. Buttons: `OK` (default), `Cancel`, `Help`.

Behaviour: OK starts a search from the current position (just after the current selection). The match is selected and scrolled to with `SearchGoBack` (default 3) lines of context above it. During a long search a small window `Searching...` with a Cancel button appears; Esc cancels. Not found: message box `Search string not found!` with OK only. F3 repeats forward, Shift+F3 backwards, using the last options.

### 1.6 Next / previous file (N / P)

- When Lister was opened with a selection, N/P walk the marked files in panel order. Otherwise they walk all files of the panel's folder (folders skipped) in panel sort order (F-583).
- At the end: no wrap; TC beeps and stays (**[unverified]** whether a message is shown). Ours: show a short status line `Last file` / `First file`.
- The panel cursor follows the file shown in Lister (TC moves the source panel cursor) **[unverified]**; ours: yes, the cursor follows.

### 1.7 Options menu and Lister configuration

Options menu items (in order): Text, Binary, Hex, Image/Multimedia/Plugins, HTML text, Unicode, UTF-8 (radio group); separator; ANSI (Windows charset), ASCII (DOS charset), Variable char width (radio group); separator; Wrap text, Fit image to window, Center images **[unverified]**; separator; `Configure...`.

`Options > Configure...` opens the Configuration dialog on the Lister page (also reachable from Configuration > Edit/View > `Configure internal viewer...`). Page layout (F-822, F-588, F-580):

- Group `Startup mode`: radio `Auto-detect` (default), `Text`, `Binary`, `Hex`; checkbox `Load bitmaps on startup` (`BmpStartup`), `Load multimedia files on startup` (`Multimedia`), `Use HTML/RTF detection` (`HTMLAsText`, `RTF`).
- Group `Font`: radio `ANSI`, `ASCII (DOS)`, `Variable`; buttons `Font 1...`, `Font 2...`, `Font 3...` (keys `Font1`..`Font3`, format `name,size,b`).
- `Text width:` edit, default 80 (`TextWidth`). `Binary width:` default 75 (`binwidth`). `Tab width:` default 8 (`TabWidth`).
- `[ ] Auto-detect ANSI/OEM charset` (weak heuristic in TC).
- Colours: `Text color`, `Background color`, `Selection`, keys `FgColor`, `BgColor`, `FgSelColor`, `BgSelColor`.
- Print margins in 1/10 mm: `LeftMargin=200`, `RightMargin=0`, `TopMargin=200`, `BottomMargin=200`, printer fonts `Font4` (fixed), `Font5` (variable).
- External image viewer: `[ ] Use IrfanView/XnView for other image types` (`IView`), path (`IViewPath`), extra types (`IviewAdditionalTypes`). N/A for us; our image viewing is built-in via the webview.

### 1.8 Image / multimedia mode

- Images: shown centered, scaled down to fit if `ShrinkBitmap=1` (TC "Fit image to window"), never scaled up unless "Fit larger and smaller" **[unverified option]**. +/- zoom, rotate, `Image > Show EXIF` **[ours, F-586]**.
- Audio/video: an embedded player with play/pause. Ours: HTML5 media in the webview; Space toggles play only when the media has focus.
- Animated GIF plays.
- Unknown binary files in mode 4 fall back to text mode.

### 1.9 Print (Ctrl+P)

Opens the system print dialog. Printed text uses `Font4` (fixed) or `Font5` (variable) and the four margins. In image mode prints the image fitted to the page. Ours: print through the webview; on Linux the GTK dialog.

### 1.10 Edge cases and errors

| Case | TC behaviour | Ours |
|---|---|---|
| File cannot be opened (locked, no permission) | Message box `Cannot open file <path>!` (exact text **[unverified]**), Lister does not open. | Same text, OK button. |
| File deleted while open | Lister keeps showing the old content (handle still open on Windows). F2 then reports the error. | Keep the last shown buffer; F2 shows the error. |
| Empty file | Opens, empty window. | Same. |
| Zero files selected but cursor on `..` | Nothing happens. | Same. |
| File grows (log) | No auto-reload; F2 reloads. TC 10+ has "Auto-reload" via Ctrl+R? **[unverified]**. | F-584: `Options > Follow file (auto-reload)`, polls every second and keeps the view at the end when it was at the end. |
| Lines over 64 K characters | Shown wrapped/cut at internal limit. | Wrap at 64 K. |
| Mixed line endings | CR, LF and CRLF each end a line. | Same. |

---

## 2. Quick view (Ctrl+Q)

FEATURES: F-082, F-150, F-248, F-399, F-589, F-784.

- Ctrl+Q (cm_SrcQuickView) turns the **other** panel into a viewer of the file under the cursor in the active panel. Pressing Ctrl+Q again (from either panel) restores the panel exactly as it was (path, cursor, selection, view mode).
- Content: the Lister engine in a panel-sized frame, no menu bar. Mode keys 1-7, A/S/V, W are **not** available while focus stays in the file panel; focus never moves into quick view by default. Ctrl+Tab or a click into the viewer **[unverified in TC]** gives it focus so keys work there; Tab returns.
- Updates as the cursor moves, with a short delay (TC loads after the cursor stops; ours: 150 ms debounce) so fast scrolling does not load every file.
- Folders: shows the folder size calculation result **[unverified]**; ours: shows name, number of files/folders and total size once computed in the background.
- Archives: the file is unpacked to temp only if small; TC shows nothing for files inside archives over a size limit **[unverified]**. Ours: preview files in archives up to 10 MB.
- cm_SrcQuickInternalOnly (F-784): same, but only the built-in text/binary/hex viewer, no images, media or plugins.
- The divider in quick view mode has its own position, `DividerQuickView` (permille, default 500) in the resolution section, separate from `Divider`. Changing it with the mouse and then `Save position` keeps it.
- Quick view and thumbnails/tree in the same panel are mutually exclusive: switching the target panel to tree (Ctrl+F8) ends quick view.
- Closing quick view while a large file is loading cancels the load.

---

## 3. Editor launching

FEATURES: F-152, F-153, F-204, F-214, F-591 to F-595, F-512, F-540, F-648, F-826.

### 3.1 F4 (cm_Edit)

- Starts the program in `[Configuration] Editor=` (default `notepad.exe`) with the file name appended. If the setting contains `%1`, the file name replaces `%1` instead of being appended. The file name is quoted when it contains spaces. The working directory is the file's folder.
- With several marked files, TC opens **only the file under the cursor** (F4 ignores the selection, unlike F3) **[TC 11 behaviour; since 10.x a flag in Editor setting allows passing all files — unverified]**. Ours: file under cursor only.
- On a folder: nothing (TC beeps) **[unverified]**.
- Our defaults (F-591): macOS `open -e` style (TextEdit) — better: `open -t <file>` (default text editor); Linux `xdg-open` with a text MIME fallback (`$VISUAL`, `$EDITOR` in a terminal if set); Windows `notepad.exe`.
- F4 in Find Files results (F-540) and in the Multi-Rename tool (F-512, our addition) uses the same editor.

### 3.2 Files in archives and on remote file systems

- F4 on a file inside an archive (F-592): the file is unpacked to a temp folder, the editor starts, and TC **waits for the editor process to end** (or, if it returns at once, shows a dialog). When the editor closes and the file changed, TC asks:
  `The file <name> was changed. Update archive?` — buttons `Yes` / `No` **[unverified exact text]**. Yes repacks the file; No discards. If the archive type cannot be written (RAR without WinRAR, 7z read-only), the message `This archive type cannot be modified` appears and the changed temp copy is kept only until the dialog closes.
- If the editor returns immediately (single-instance editors such as TextEdit via `open`), TC shows a waiting dialog: `Wait for editor to close, then click OK to update the file` with `OK` / `Cancel` **[unverified]**. Ours: watch the temp file modification time, and show a non-modal bar `Editing <name> - changes will be uploaded` with `Upload now` and `Stop watching`; on each save ask once (with "don't ask again for this file").
- FTP/SFTP (F-593, F-648): same flow — download, edit, upload on change. Upload errors keep the temp copy and show its path.

### 3.3 Shift+F4 (cm_EditNewFile)

Dialog title `Total Commander` (ours: `New file`), single edit field with prompt `Edit new file:` **[unverified label]**. Default text: the name of the file under the cursor (TC fills the current file name, selected, so typing replaces it). Buttons `OK` (default), `Cancel`.
- Name without path: created in the active panel folder. A relative or absolute path is honoured.
- If the file exists, it is opened as is (no overwrite prompt).
- If the file does not exist, TC creates an empty file (0 bytes) and starts the editor; the panel rereads and puts the cursor on the new file.
- Invalid name (characters not allowed by the file system): error `Cannot create file <path>!` **[unverified text]**.
- With no extension TC creates `name.txt`? — **no**, TC creates the name as typed **[unverified]**; ours: as typed.

### 3.4 Configuration > Edit/View page (F-420)

Layout (TC 11, page `Edit/View`):

- Group `Viewer for F3`: radio `Internal viewer (Lister)` (default) / `External viewer:` with path edit and `>>` browse button. The other one is used for Alt+F3. Keys: `Viewer=` (path or `none`), `Viewertype=` (0 current file, 1 marked files, 2 external).
- Button `Configure internal viewer...` opens the Lister page (1.7).
- Group `Editor for F4`: path edit (`Editor=`), `>>` browse. Hint text: `%1 = file name` **[unverified]**.
- `[ ] Use internal associations for F4/F3 if defined` **[unverified, TC 9+]**.
- Ours: add `[ ] Use built-in text editor` (F-594) and `Compare tool:` path with `%1 %2` placeholders (F-563).

---

## 4. File associations

FEATURES: F-322 (N/A), F-323, F-095, F-232, F-335, F-445.

### 4.1 Enter / double click on a file

Order of resolution in TC 11:
1. If the file is an archive and "Enter opens archive" applies, it is entered as a folder (area of packers, F-596).
2. **Internal associations** (cm_InternalAssociate) matching the file: the `Open` command is run.
3. If the file is executable (`.exe .com .bat .cmd .pif` and `PATHEXT`), it is run.
4. The Windows shell association (ShellExecute "open").
5. No association: Windows "Open with" dialog. Ours: the OS "Open with" chooser (macOS) or a simple app chooser (Linux).

Shift+Enter runs the file in a console window that stays open (`noclose.exe`) for executables; ours: in the configured terminal **[area of command line]**.

### 4.2 Internal associations dialog (cm_InternalAssociate, F-323)

Opened from `Files > Internal associations (Total Commander only)...`. Layout:

- List of association entries, each shown as file type description plus mask, e.g. `Text files (*.txt;*.log)`.
- Buttons beside the list: `Add...`, `Edit...`, `Remove`, `Up`, `Down`.
- Lower group for the selected entry: `Open:`, `View (F3):`, `Edit (F4):` command fields with `>>` browse buttons, plus `Additional commands:` list for the right-click menu.
- `[ ] Use internal associations also for F3/F4` **[unverified label]** — when on, F3 and F4 use the View/Edit commands of the matching entry instead of Lister/the editor.
- `OK`, `Cancel`, `Help`.

Add/Edit asks first for the file mask (`File type (e.g. *.txt):`, masks separated by `;`, `|` exclusions allowed, as everywhere in TC), then a description and icon. Masks are matched top-down; the first match wins, so Up/Down matter.

Commands accept parameters like the button bar: `%1` or `%N` for the file name (with path), `%P` source path, `%T` target path, `%O` name without extension, `%E` extension, `?` before the parameters to ask. Without a placeholder the file name is appended in quotes.

Storage: wincmd.ini section `[Associations]` **[unverified exact key layout]**, roughly `1=*.txt`, `1_Open=...`, `1_View=...`, `1_Edit=...`. Ours: `associations[]` in our config with `masks`, `description`, `open`, `view`, `edit`, `extra[]`.

### 4.3 [Extensions] (legacy, Alt+F3)

`[Extensions]` maps extensions to a viewer for Alt+F3, format `ext=program ^.ext` (the `^` stands for the file name), e.g. `pif=c:\windows\pifedit.exe ^.pif`. Ours: read it if we import a wincmd.ini, otherwise use internal associations' View command.

### 4.4 System associations (cm_Associate)

`Files > Associate...` opens a dialog to change Windows associations. N/A for us (F-322): instead the internal associations dialog has a button `Open system settings...` and the context menu has `Open With`.

### 4.5 Icons and print

- Icons for associated files come from the OS (F-095).
- Ctrl+F9 / cm_PrintFile prints through the associated program's print verb (F-232, F-335). Ours: macOS `lp`/NSWorkspace print where available, Linux `lp` for text and PDF, otherwise message `Printing is not supported for this file type`.

---

## 5. Compare by content

FEATURES: F-181, F-321, F-466, F-552, F-553, F-556 to F-563, F-839, F-614 (dialog in copy area F-463).

### 5.1 Starting

`Files > Compare By Content...` (cm_CompareFilesByContent). Which two files:
1. Exactly two files marked in the active panel: those two.
2. One file marked in each panel: those two.
3. One file marked in the active panel, or none: the file under the cursor and the file **with the same name** in the other panel (F-561); if there is none, the file under the cursor in the other panel.
4. Otherwise: message `Please select two files to compare, or one file in each panel!` **[unverified text]**.

Also from: the overwrite dialog `Compare` button (F-466; compare tool opens modal over the dialog), the Synchronize dirs result list (right click `Compare by content...`, F-553), and Find Files results feed **[unverified]**.

If `[Configuration]` has an external compare tool (TC: in Configuration > Edit/View? actually `Configuration > Misc > Compare tool` **[unverified page]**, key `CompareTool=` **[unverified]**), that program is started with both file names and the built-in tool is not shown (F-563).

Both files the same: TC still opens the tool and shows the message `The two files have identical content!` **[TC shows a message box and does not open the window — unverified which]**. Ours: message box `Files are identical`, with a button `Show anyway`.

Files in archives or on FTP are unpacked/downloaded to temp first.

### 5.2 Window layout

Title `Compare <file1> <file2>` **[unverified]**. Top-down:

- Toolbar row of buttons: `Next difference`, `Previous difference`, `Recompare` (`Compare`), `Binary` (toggle text/binary), `Ignore case`? (as options), `Edit` (toggle edit mode), `Save left`, `Save right`, `Undo`, `Copy to left` (`<<`), `Copy to right` (`>>`), `Encoding` drop-down (ANSI/ASCII/UTF-8/UTF-16), `Font`, `Help`, `Close` **[layout from TC 7.5+ compare; labels unverified]**.
- Two path edit fields above the two panes (editable: change a path and press Recompare to compare another file).
- Two side-by-side text panes (or stacked when `CompareVertical=1`) with line numbers in the left margin (F-839), synced vertical scroll (F-839).
- A status line with the count of differences (`n differences`) and the current position.

Colours: differing lines get `[Colors] CompareText` background, the current difference `CompareTextSel`; in binary mode `CompareBinary`, `CompareBinarySel`. Missing lines on one side are shown as empty placeholder lines so lines stay aligned.

### 5.3 Text compare rules (F-556, F-560)

- Line-based diff. Options: `Case sensitive` (`CompareCaseSensitive=0` default = ignore case **is off**? the key means case-sensitive when 1; default 0 means ignore case), `Ignore repeated spaces` (`CompareIgnoreRepSpace=1`), `Ignore leading/trailing whitespace` **[ours]**, `Ignore line ending style` **[ours, always on]**.
- After a file is longer than a limit, TC auto-switches to binary **[unverified]**. Ours: text diff up to 50 MB per side or 1 M lines; above that, binary mode with a note.
- Resync: TC uses a fixed number of matching lines (default 3? key `CompareMinMatch` **[unverified]**) to resynchronise after a difference. Ours: Myers diff, no resync parameter.
- Binary files (detected NUL bytes) open directly in binary mode.

### 5.4 Binary compare (F-557)

Hex rows of 16 bytes per side with offsets, differing bytes highlighted. Different lengths: the extra tail of the longer file is highlighted as a difference. For files of the same length only byte replacements are detected (no insert/delete resync), like TC.

### 5.5 Keys and mouse (F-558, F-559)

| Key | Action |
|---|---|
| F8 or Ctrl+Down **[unverified]** | Next difference (also toolbar button) |
| Shift+F8 or Ctrl+Up **[unverified]** | Previous difference |
| F2 | Recompare |
| Ctrl+F | Find in the focused pane; F3 find next |
| Tab | Switch focus between the panes |
| Ctrl+E | Toggle edit mode **[ours]** |
| Alt+Left / Alt+Right | Copy current difference block to left / right **[ours]** |
| Ctrl+S | Save the focused side (edit mode) |
| Ctrl+Z | Undo in edit mode |
| Esc | Close; if there are unsaved edits ask `Save changes to <file>?` Yes / No / Cancel |

Mouse: wheel scrolls both panes together; drag selects text in one pane; click on a highlighted block makes it the current difference; double click on a line in edit mode puts the caret there.

### 5.6 Edit mode (F-559)

`Edit` toggles both panes to editable text. Typing in one pane recomputes the diff after a pause (ours: 500 ms). Copying a block left/right replaces the block on the other side. Save writes in the original encoding and line-ending style; for files from archives or remote locations, saving triggers the same "Update archive?" / upload flow as F4 (3.2). Read-only files: save shows `Cannot write file <path>!` and keeps edits.

### 5.7 Window state

Size, position and options are stored per resolution as `Compare*` keys **[unverified exact names, e.g. CompareX, CompareY, CompareDX, CompareDY]**. Ours: `compare.window`, `compare.options`.

---

## 6. wincmd.ini key summary

| Section | Key | Default | Meaning |
|---|---|---|---|
| Lister | Maximized | 0 | Open Lister maximised |
| Lister | x, y, dx, dy | — | Lister window geometry |
| Lister | TextWidth | 80 | Text width in chars |
| Lister | binwidth | 75 | Binary mode width |
| Lister | TabWidth | 8 | Tab width |
| Lister | Startup | 0 | Mode 0 auto,1 text,2 bin,3 hex; +16 ANSI, +32 ASCII, +48 variable |
| Lister | BmpStartup | 1 | Show images on open |
| Lister | Multimedia | 1 | Play media on open |
| Lister | HTMLAsText | 1 | HTML detection |
| Lister | LinkBraces | 1 | Braces around links |
| Lister | RTF | 1 | RTF detection |
| Lister | Wrap | 1 | Line wrap |
| Lister | ShrinkBitmap | 0 | Fit images to window |
| Lister | IView, IViewPath, IviewAdditionalTypes | — | External image viewer (N/A) |
| Lister | SearchGoBack | 3 | Context lines above match |
| Lister | FgColor, BgColor, FgSelColor, BgSelColor | — | Colours |
| Lister | Font1..Font3, Font4, Font5 | — | Screen fonts, printer fonts |
| Lister | LeftMargin, RightMargin, TopMargin, BottomMargin | 200,0,200,200 | Print margins (1/10 mm) |
| Configuration | Viewer | none | External/Alt+F3 viewer |
| Configuration | Viewertype | 1 | Alt+F3 behaviour |
| Configuration | Editor | notepad.exe | F4 editor, `%1` placeholder |
| Configuration | CompareCaseSensitive | 0 | Case-sensitive compare |
| Configuration | CompareIgnoreRepSpace | 1 | Ignore repeated spaces |
| Configuration | CompareVertical | 0 | Stack compare panes vertically |
| <resolution> | DividerQuickView | 500 | Quick view divider (permille) |
| <resolution> | Compare* | — | Compare window geometry |
| Colors | CompareText, CompareTextSel, CompareBinary, CompareBinarySel | — | Compare colours |
| Extensions | ext | — | `ext=program ^.ext` for Alt+F3 |
| Associations | (see 4.2) | — | Internal associations **[unverified layout]** |

## 7. Open points for the implementers

- FEATURES.md lists `U`/`8` for Unicode/UTF-8 and `Q` for close; TC uses `6`/`7` and Esc. Bind both sets.
- The **[unverified]** labels above should be checked against a running TC 11 when possible; until then, use the given text.
- Settings live in our own config (docs/briefs conventions); importing a real wincmd.ini is optional and maps the keys in section 6.
