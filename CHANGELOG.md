# Changelog

All notable changes to mormiCommander are listed here, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Feature branches add their lines under "Unreleased"; the
version is changed only when a round of features is merged.

## Unreleased

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
