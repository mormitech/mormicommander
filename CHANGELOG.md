# Changelog

All notable changes to mormiCommander are listed here, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Feature branches add their lines under "Unreleased"; the
version is changed only when a round of features is merged.

## Unreleased

## 0.9.48 – 2026-11-14

### Added

- README (issue #46): says plainly that mormiCommander is inspired by Total Commander and not affiliated with Ghisler Software GmbH, and has a key table, a comparison with other file managers, and a no-telemetry section.
- docs/DOWNLOADS.md: which download to pick, how to check it on each system, and package-manager commands. The release job now also writes SHA256SUMS.txt.
- Tests: a README test checks every listed key and command against the real keymap, and a zero-network test fails if the code reaches the network outside the FTP/SFTP/WebDAV clients and the two local listeners.

## 0.9.47 – 2026-11-14

### Fixed

- Tests: the function key bar test no longer fails CI now and then with "window is not defined" after it ends (the Wails runtime's timer outlived the test page).

## 0.9.46 – 2026-11-14

### Fixed

- Read-only archives now say why, before anything starts (issue #143). A panel showing an archive inside another
  archive was described as the outer archive, so copying into it said "is a file, not a folder" and writes failed
  with a vague read-only error. It is now treated as the inner archive's root, and F5/F6/F7/F8 into or inside a
  read-only archive (7Z, or one nested in another archive or on a server) stop up front with a plain sentence that
  names the reason and suggests unpacking it or packing a new archive with Alt+F5.

## 0.9.45 – 2026-11-14

### Fixed

- The first password question for a WebDAV address like webdav://bob@host/ no longer says "The server did not accept
  the login" (issue #144). That address is tried without a password first, so nothing had been refused yet. The
  refused-login text now appears only after a password was actually sent and turned down.

## 0.9.44 – 2026-11-14

### Fixed

- In the Lister, N and P now step through the files inside archives, Find Files results and fed lists, not only
  through local folders (issue #142). Each file is unpacked once when reached. Local folders step as before.

## 0.9.43 – 2026-11-14

### Fixed

- Quick view (Ctrl+Q) now previews files inside zip and tar.gz archives and shows the size of folders inside them,
  instead of a stat error (issue #141). A file in an archive is unpacked to a temporary copy once per quick view
  session; files over 10 MB are not unpacked and quick view says F3 can view them. Servers and URLs still show the
  "not ready yet" note. The folder size also fixes F3 on a folder inside an archive.

## 0.9.42 – 2026-11-14

### Fixed

- A very quick network copy no longer leaves the "Copying" box open and the app busy for good (issue #140). A transfer
  that finished within a few milliseconds could report "done" before the page knew the job number, so the page never
  noticed it had ended. The page now remembers such early "done" reports and uses them as soon as it starts waiting.

## 0.9.41 – 2026-11-14

### Fixed

- Dialog windows can no longer go full screen (issue #138). On macOS they are kept out of full-screen Spaces, and the
  green button is greyed out on dialogs that have a fixed size (New folder, Delete, Copy and so on). On dialogs that can
  be resized, such as Find files, the green button now zooms the window instead of opening a full-screen Space. On
  Windows the maximise box is off for fixed-size dialogs. The main window and the other windows are not changed.

## 0.9.40 – 2026-11-14

### Fixed

- The "Replace file?" question shown while unpacking an archive or copying out of one now names the file inside the
  archive (for example `f1.zip/f1.txt`) instead of the hidden temporary staging file (issue #139). Only the path text
  in the question changed; nothing else behaves differently.

## 0.9.39 – 2026-11-14

### Fixed

- The close button of a dialog window now goes the way Esc goes in that dialog (issue #136). Before, it closed the
  window and cancelled the dialog at once, so Compare by content threw away unsaved edits without asking; now it asks
  whether to save, like Esc and its Close button. If a dialog's page does not answer within 3 seconds, the next click
  on the close button closes the window as before, so a stuck page cannot keep a window open. Closing after an answer,
  together with the owner window, and at quit work as before.

## 0.9.38 – 2026-11-14

### Fixed

- A dialog window that has another dialog window open above it (for example Multi-Rename with its "Save settings as…"
  window) can no longer be closed with its close button (issue #137). The close is refused and the focus goes to the
  top window instead, the way Total Commander and macOS sheets behave. Closing after an answer, closing together with
  the owner window and quitting the app work as before.

## 0.9.37 – 2026-11-14

### Fixed

- A dialog window that was shown but stayed an empty box (issue #134) is now woken up. On macOS the dialog windows no
  longer let WebKit guess that they are hidden behind other windows, so the page draws whenever the window is on
  screen. If a page has not reported that it is drawn 500 ms after its window is shown, the window is woken again
  (at 0.5, 1.5, 3.5 and 7.5 seconds): on macOS the window is put back on screen and its web view and size are nudged,
  on Windows and Linux only the page is told to repaint. Control mode shows the wake counts in `windows` and
  `dialog` -> `window`.

## 0.9.36 – 2026-11-14

### Fixed

- On macOS, a dialog window (for example Options) no longer grows by about 32 px every time it is opened and closed.
  The app now asks the window itself for its title bar and border size instead of relying on a value that came out
  as zero, so an unresized window is no longer saved as if you had resized it.

## 0.9.35 – 2026-11-14

### Fixed

- After F5/F6 copy or move, only the files and folders that really got across are unselected. Sources that were
  skipped (for example with "copy all larger") or failed, or that the job never reached, stay selected so you can see
  what is left. A cancelled job still keeps the whole selection, and server and archive jobs behave as before.

## 0.9.34 – 2026-11-14

### Fixed

- Shift+F5 on a single folder, with the name in the dialog left as it is, no longer says "Can't copy the folder into
  itself". It now makes a copy with a free name such as dx(2), the same as it already did for files. Typing the name
  with a trailing slash (dx/) still gives the error, and moving a folder in the same place still says there is nothing
  to move.

## 0.9.33 – 2026-11-14

### Fixed

- A job's progress box no longer opens on top of a question the job just asked and hides its buttons. The box appears
  after a short delay, and that delay only waited for archive passwords, not for the job's own questions (overwrite,
  folder not empty, read-only, job error). Now the box waits until the question is answered. Server transfers (FTP,
  SFTP, WebDAV) had the same problem and are fixed too. When nothing is asked, the box still opens after 400 ms.

## 0.9.32 – 2026-11-14

### Fixed

- The copy summary now counts only the bytes that were really written. Files that were skipped or failed used to be
  added to the total, so the summary showed more than was copied, and after stopping a copy the done size could jump to
  the full file size. The progress bar still reaches the end as before.

## 0.9.31 – 2026-11-14

### Fixed

- Changes made just before quitting are no longer lost. The folders, sorting and active panel were saved a moment after
  each change, so quitting right away (Alt+F4, Cmd+Q, the window close button) could skip the last change. The session
  is now written before the app exits, and a copy is also kept on the Go side that is written at shutdown if the page
  never got to it. Nothing else about saving the session changes.

## 0.9.30 – 2026-11-14

### Fixed

- Each tab now keeps its own view (Ctrl+F2 and the other view choices). Before, a new tab shared the view of the tab it
  was made from, so changing the view in one tab changed it in the other. Duplicated tabs and tabs copied to the other
  side start with a copy of the view, and the view is saved with the tab: in the tab lists, .tab files, favourite tabs,
  closed tabs and across restarts. Tabs saved by older versions open as before.

## 0.9.29 – 2026-11-14

### Fixed

- A long path in the path bar is now shortened at the start instead of the end, so the current folder's name stays
  visible: leading folders are hidden behind a "…", and clicking the "…" opens the deepest hidden folder. If even the
  current folder's name is too wide, its own text is shortened at the start. The command-line prompt likewise keeps the
  trailing part of the path that fits, which also removes the wide empty gap after it. Both are fitted again when the
  window is resized.

## 0.9.28 – 2026-11-14

### Fixed

- The command line's own `cd` now expands environment variables: `cd $HOME`, `cd ${HOME}/Documents` and, on Windows,
  `cd %HOME%` go to the folder the variable names. Variables that are not set stay as typed. `cd\`, `cd/` and `cd..`
  written without a space are also understood as `cd` (as in Total Commander) instead of being started as a program.
  Words such as `cdrecord` or `cd.sh` are still run as programs.

## 0.9.27 – 2026-11-14

### Added

- Shift+Left and Shift+Right now mark files, as Shift+Up and Shift+Down do, but a side step at a time. In the Brief view
  they move one column and mark everything passed over; in the Thumbnails view they move one tile. In the Full view,
  where there are no side columns, they act like Shift+PgUp and Shift+PgDn. Turning round takes the marks back. The
  keys are the new commands cm_SelLeft and cm_SelRight, so they can be rebound.

## 0.9.26 – 2026-11-14

### Fixed

- Moving through the folder history (Alt+Left / Alt+Right, Cmd+[ / Cmd+], mouse buttons 4 and 5, or choosing a folder
  from the Alt+Down list) now puts the cursor back on the entry it was on when you last left that folder, instead of on
  "..". If that entry is not known and the folder you came from is inside the target, the cursor goes on the subfolder
  that leads to it, the same rule Backspace uses.

## 0.9.25 – 2026-11-14

### Fixed

- Clicking the drive that is already lit on a panel's own drive bar (or picking it in the Alt+F1 / Alt+F2 drive list)
  now goes to that drive's root again (/ for Macintosh HD, ~ for Home). It used to jump to the other panel's folder
  whenever the other panel was on the same drive. The other panel's folder is only taken when switching to a
  different drive.

## 0.9.24 – 2026-11-13

### Fixed

- Select files (Num+ / Num-) mask box: Up and Down, and Alt+Down, work again. Alt+Down opens or closes the list of
  earlier masks, Up and Down move through it (or step through earlier masks when it is closed), and Enter takes the
  highlighted mask. They did nothing before because the dialog swallowed the keys before the box saw them.

## 0.9.23 – 2026-11-13

### Fixed

- Function key bar: the shifted, Alt and Ctrl labels no longer stay stuck after the key is released. Modifiers are now
  tracked even while a dialog is open, are corrected from mouse and wheel events, and are cleared when the window
  loses focus, is hidden or is minimised (for example after Cmd+Tab). Cmd now shows as the ⌘ symbol on macOS.

## 0.9.22 – 2026-11-13

### Changed

- The macOS bundle identifier is now com.mormitech.mormicommander (it was com.wails.mormicommander). It is kept in one
  place, wails.json, and filled into the macOS Info.plist and the Windows manifest by scripts/build.sh. The control-mode
  `info` command also reports it as `bundleId`. `brew uninstall --zap` now also removes the Library folders named after
  the ID. Settings stay where they are. macOS ties permissions such as Full Disk Access to the bundle ID, so they must be
  granted again after updating.

## 0.9.21 – 2026-11-13

### Fixed

- Find: a huge size filter value in TB no longer overflows and matches the wrong files.
- Packing a file whose name has characters outside Latin-1 to .gz no longer fails.
- Multi-rename: counter width is capped at 255 digits, so a mask like [C:2000000000] is refused instead of hanging.
- Multi-rename: the error count is recomputed before running, so hand edits can no longer slip past the check.

## 0.9.20 – 2026-11-13

### Added

- Copy path: on macOS, Option+Shift+Cmd+C copies the source folder's path (the key had no binding before).
- New command "Copy names with full path, quoted" (Mark menu, no default key): copies each full path in double quotes,
  one per line. On Windows only the quotes are added; elsewhere backslash, double quote, $ and backquote are escaped so
  the path pastes into a shell exactly.
- The copy-path items (F-139, F-141, F-142, F-796) are ticked in docs/FEATURES.md.

## 0.9.19 – 2026-11-13

### Added

- The key reference of the Total Commander manual as data (118 keys, each with its command and whether it is safe to
  press in a test), with a test that checks it against the keymap on Linux, Windows and macOS.
- Control mode has two new commands: "keyref" returns that list and "runs" returns the commands run lately (the last
  100). New script scripts/qa_keys.py presses the safe keys in the real app (not yet run against it).

### Fixed

- The Options checkbox "Space calculates folder size" now really controls whether Space counts a folder's size
  (it was not linked to the setting the Space key reads). The default stays on.

## 0.9.18 – 2026-11-13

### Added

- Files that were cut (Ctrl+X) now show dimmed in both panels and every view mode until they are pasted, replaced by
  a copy, or the cut is cancelled. Esc with an empty command line cancels the cut and empties the system clipboard
  only if it still holds exactly that cut. The dimming also clears when the window regains focus and the clipboard
  holds something else. Option dimCutFiles (on by default) turns the dimming off.
- Tests for the clipboard shortcuts on macOS (Cmd keys) and for their absence on Windows and Linux.

## 0.9.17 – 2026-11-13

### Added

- While hidden files are shown, each panel's status line ends with "Hidden: shown". Option Layout.HiddenMarker
  (Options > Layout, on by default) turns the marker off.
- Find files (Alt+F7) has an "Include hidden files" checkbox on the General tab, kept in sync with "Skip hidden
  files" on the Advanced tab. It starts as the last search had it; setting FindFiles.HiddenFromPanel (off by
  default) makes new searches follow the panel's hidden-files state instead.
- The Show menu entry reads "Hidden files (Ctrl+H)": commands can now show their key in the native menu label.

## 0.9.16 – 2026-11-13

### Added

- Every dialog opens in a native window of its own: it can be moved anywhere, also outside the main window, it is
  titled with the dialog's heading, stays above the window it belongs to, and opens where that kind of dialog was last
  left (big ones such as Options and Find also keep their size). Closing the window cancels the dialog; a dialog opened
  from a dialog stacks on top of it; popup lists and menus of a dialog show inside its window. Two windows are made in
  advance, so a dialog appears at once.
- Setting "Open dialogs in windows of their own" (Options > Misc, `dialogsInWindows`, on by default): off brings back
  the boxes inside the main window. `dialogsInPage` (a list of dialog kinds in settings.json) keeps chosen kinds in
  the page; the Lister over the panels always stays there.
- Control mode: `windows` lists the dialog windows (kind `dialog`), `dialog` tells which window shows the dialog, and
  `dialog_ping` measures the round trip of input through a dialog window.

## 0.9.15 – 2026-11-13

### Changed

- Options apply as you change them, so you see the effect at once; there is no Apply button any more. OK keeps the
  changes, Cancel (or Esc, or closing) puts back what was there when the dialog opened.

## 0.9.14 – 2026-11-13

### Fixed

- Folder sizes no longer count anything twice: the count stays on the folder's own volume (other volumes mounted
  inside it are left out, like "du -x"), links are not followed, and on macOS /System/Volumes/Data is skipped because
  its folders are the same ones you see as /Users, /Applications and so on. /System went from 310 GB to its real 34 GB.

## 0.9.13 – 2026-11-13

### Changed

- A drive button opens the other panel's folder when the other panel is on that same drive button, as in Total
  Commander: with the left panel in ~/Downloads, Home on the right opens ~/Downloads (Macintosh HD still opens /); on
  Windows, with the left panel in d:\games, d opens d:\games and c opens c:\. The drive list (Alt+F1/F2) does the same.

## 0.9.12 – 2026-11-13

### Changed

- Searching in Options filters the dialog: only the matching options are shown, under their page titles, and the page
  list keeps only the pages with a match. A page whose name matches is shown whole.

## 0.9.11 – 2026-11-13

### Fixed

- A calculated folder size is shown in full: folders have no extension (a dot in a folder name is part of the name),
  so the size also uses the empty Ext column.

## 0.9.10 – 2026-11-13

### Added

- Map any folder to the drive bar as a quick jump: "Map to the drive bar…" in the context menu of a folder, of the
  folder's background and of the path bar asks for a name. Mapped buttons come after the real drives and Home, behind a
  thin divider, appear in the drive list (Alt+F1/F2), light up while you are inside them, and their own context menu
  renames, moves or removes them.

## 0.9.9 – 2026-11-13

### Fixed

- A right-click in the window while another app was active opened a context menu that ignored the mouse (no
  highlight) until you clicked: the window now comes to the front on the right-click.

## 0.9.8 – 2026-11-13

### Changed

- CI builds pushes to main again only (the temporary v3 branch trigger is gone).

## 0.9.7 – 2026-11-13

### Added

- F3 opens the Lister in a real window of its own, like Total Commander's Lister: the title is "Lister – <name>", the
  size is kept, Esc or the close button closes it, N and P walk the files and move the panel's cursor, and several can
  be open at once. One window is made in advance, so it shows in about 50 ms. The old Lister over the panels stays as a
  setting ("Open the Lister in a window of its own" in the viewer options, `viewerInWindow`). Quick view (Ctrl+Q) stays
  in the panel.

### Changed

- The window is now Wails v3 (v3.1.0) instead of Wails v2: one process with real windows, so the About box opens as a
  window of the app instead of a second copy of it. Linux builds use GTK 4 and WebKitGTK 6.0 (the .deb depends on
  them). The build is `scripts/build.sh`; the Wails CLI is no longer needed.

## 0.9.6 – 2026-11-13

### Fixed

- Resized column widths were lost when a panel changed folder.

## 0.9.5 – 2026-11-13

### Added

- Columns can be resized: drag the line between two column titles (double-click it for the default width). The
  widths are saved in the settings right away, per set of columns, and both panels share them.

### Fixed

- The Insert key selects on Mac with a PC keyboard (macOS reports it as Help) and as numpad 0 with Num Lock off.

## 0.9.4 – 2026-11-13

### Changed

- Rows are no longer highlighted under the mouse. Options > Display > "Highlight the row under the mouse" turns it
  back on (off by default).

## 0.9.3 – 2026-11-13

### Changed

- Entering a folder always puts the cursor on ".." at the top, also when you were in that folder before. Going up
  still puts it on the folder you came from. The old behaviour (back to the last file) is the setting
  "rememberCursor".

## 0.9.2 – 2026-11-13

### Fixed

- Double-clicking a file or folder opens it, like Enter (Ctrl/Cmd+double-click opens a folder in a new tab). The
  double-click was lost because the first click redraws the rows.

## 0.9.1 – 2026-11-13

### Added

- Dialogs and tool windows can be moved: drag the title (or an empty part of the box). Each kind of dialog opens
  where you left it during the session; double-click the title to put it back in the middle.
- A Home drive button for your home folder (~): after the startup disk on macOS and Linux, after the drive letters
  on Windows. It is highlighted while a panel is anywhere inside your home folder, and ~ picks it in the drive list.

## 0.9.0 – 2026-11-13

### Added

- The engines are wired into the app (one feature module per area, all built on the command registry): copy, move,
  delete, wipe, new folder and rename with background transfers and the transfer window; archives and FTP/SFTP
  servers as folders with Pack and Unpack; viewer and editor; Find files; Synchronize folders and Compare by content;
  Multi-rename; selection by mask, Compare folders; panel views and sorting; tabs and the folder hotlist; tools
  (checksums, split and combine, properties, command line and launching); the Options dialog; system features.
  Items stay "partly done" in docs/FEATURES.md until QA checks them.
- Engine for starting programs (internal/launch): open with the default app, "Open with…" lists, running programs
  with Total Commander parameter placeholders, command line parsing and history, external viewer/editor, terminal
  here, reveal in Finder/Explorer, internal associations and shortcut files (.lnk, .url, .webloc, .desktop).

## 0.1.0 – 2026-11-13

### Added

- Two file panels with a drive bar, the free space of the drive, a path bar (click to type a path, click a part to go
  there, an arrow for the folders you visited), column titles that sort, and a status line with the selection.
- Total Commander's keys for moving around: arrows, PgUp/PgDn, Home/End, Enter, Backspace, Ctrl+PgUp/Ctrl+PgDn,
  Ctrl+\\ and Ctrl+<, Alt+Left/Alt+Right, Alt+Down, Alt+F1/Alt+F2, Tab and Ctrl+I, Ctrl+Left/Ctrl+Right, Ctrl+U,
  Ctrl+R and F2, Ctrl+H (Cmd+Shift+. on a Mac), mouse buttons 4 and 5.
- Selecting with Ins, Space, Shift and the arrows, Ctrl+A, Ctrl+Num+/Num-, Num * and Shift+Num *, Num /; Shift+8 works
  as Num * on keyboards without a numpad.
- Sorting by name, extension, date and size (Ctrl+F3 to Ctrl+F7), natural order (file2 before file10).
- Command line: letters typed in a panel go there, Enter runs it, `cd` changes folder, cm_* names run commands,
  Ctrl+Enter, Ctrl+Shift+Enter and Ctrl+P add names and paths.
- The menus of Total Commander (Files, Mark, Commands, Net, Show, Configuration, Start, Help) and the function key bar;
  what is not built yet is greyed out and says so.
- Panels side by side or on top of each other, a bar between them to drag, larger and smaller text.
- The window, the folders, the sort and hidden files come back as they were at the next start; light and dark theme.
- Big folders (100,000 files) scroll smoothly.
- The engine for copy, move, delete and the Trash: jobs with progress, cancel and the overwrite question.
- Control mode (`--control`) and `scripts/mc_ctl.py` to drive the app from scripts; `--demo` with made-up files;
  Total Commander's start options /L= /R= /P=.
- About window, --version.
