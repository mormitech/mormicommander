# Changelog

All notable changes to mormiCommander are listed here, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Feature branches add their lines under "Unreleased"; the
version is changed only when a round of features is merged.

## Unreleased

## 0.9.88 – 2026-11-14

### Changed

- Issue #118: the CI build now uses the Go version named in go.mod instead of "stable", caches the Go modules by go.sum, caches npm downloads by package-lock.json, and on Windows builds the wails3 tool once and caches it instead of rebuilding it every run. scripts/build.sh accepts an optional WAILS3 variable naming a prebuilt wails3; without it nothing changes for local builds. A new Go test fails if CI drifts back to these problems. No change to the app itself.

## 0.9.87 – 2026-11-14

### Changed

- Issue #119: five Go files (args.go, feature_find.go, feature_find_feed.go, feature_net.go, internal/launch/params.go) were not gofmt-formatted; they are now (whitespace and alignment only, no change in behaviour). The CI build gains a "gofmt" step, run on Linux before go vet, that fails if any tracked Go file needs formatting.

## 0.9.86 – 2026-11-14

### Security

- Issue #116 (advisory review, no change in behaviour): checked the Wails v3.1.0 fix for GHSA-2p5f-2gx8-mh8m. Reserved messages from child frames are dropped, but bound Go calls (fetch to /wails/runtime) are not frame-checked, so any script-capable frame with the app's origin could still reach Go. Our exposure is low (no page creates frames, only bundled pages are loaded, viewer HTML is sanitized into a shadow root). As hardening, the viewer now draws an <embed> only for PDF; any other type shows a "can't be shown here" message. No current format is affected. Tests added on both sides. The Wails upgrade itself is not done: Wails stays at v3.1.0 until a later v3 release can be tried by hand on every platform.

## 0.9.85 – 2026-11-14

### Added

- Issue #110: Find Files can now be a modeless window, like Total Commander's. New switch FindFiles.Modeless (ini key SearchDefaults:Modeless), off by default, so Find Files stays modal as before. When on (and dialogs show in their own windows), the main window is not dimmed and keeps working, keys not meant for Find go to the panels, activating the main window puts focus back on the panel, and Alt+F7 while Find is open brings its window to the front. Dialogs opened from Find (save search, save list, delete, overwrite) appear over the Find window; a modal dialog opened from the main window blocks only the main window. Dialogs that must stay inside the page remain modal.

## 0.9.84 – 2026-11-14

### Added

- Issue #111: the hotlist popup now understands Left and Right. Right opens a submenu, Left goes back up one level; Enter and Esc work as before, and other popups are unchanged.
- Issue #111: new switch Options > Hotlist > "Typing in the hotlist filters it", off by default (the old letter jumps stay). When on, typed text shows under the title and only entries whose name or folder contain every typed word stay, including entries inside submenus. Backspace removes a letter; Esc clears the filter first, then closes.

### Fixed

- Issue #111: hotlist entries with a file mask (such as cd folder/*.txt) now test each file name again whenever the list is shown.

## 0.9.83 – 2026-11-14

### Fixed

- Issue #112: only the active tab's folder history was saved, so the other tabs came back after a restart with no history. Now every tab keeps its own history in folders.ini (extra keys in the tab sections that Total Commander ignores), cut to the history size setting, and Alt+Left works in every tab after a restart. The active tab is saved in [LeftHistory]/[RightHistory] exactly as before; older folders.ini files load as they always did.

## 0.9.82 – 2026-11-14

### Added

- Options dialog search (issue #109): a new "Whole words" checkbox under the search box. It is off by default, so searching works exactly as before. When ticked, each search word only matches as a word of its own: "two" finds "Two drive button bars" but no longer matches "network". The choice lasts until the app is closed.

## 0.9.81 – 2026-11-14

### Fixed

- Engine leftovers from issue #102. File comments (descript.ion): a name containing a quote, such as `a" b`, now survives a save and reload; names that already read back correctly are written exactly as before, and older files still read the same. On Linux the creation date column is now filled from the file system birth time when the kernel and file system record one, and stays empty otherwise. On Windows descript.ion is marked hidden again after each save, as Total Commander keeps it. FTP and SFTP transfers now refuse remote names that are unsafe as local file names. Not done yet: merging the volume code in internal/disk and internal/dirsize is left as its own follow-up change.

## 0.9.80 – 2026-11-14

### Added

- Synchronize directories (issue #99): a new checkbox, "Also trash emptied folders and replaced links", under "Delete to the trash". It is off by default, so a sync behaves exactly as before. When it is on (and "Delete to the trash" is on too), folders a sync empties and links it replaces go to the trash instead of being removed or overwritten directly. A folder that still holds anything is never removed, and if trashing fails the old link is left in place. Not covered: plain file overwrites still replace the old file in place.

## 0.9.79 – 2026-11-14

### Fixed

- Properties on Windows (issue #100): the Explorer properties sheet (Alt+Enter style "system properties") is now opened from the program's main UI thread, so it reliably appears. Before, it was opened from a background thread that had no message loop and the sheet could fail to show. macOS and Linux are unchanged.

## 0.9.78 – 2026-11-14

### Added

- Real file drag-out on macOS (issue #108, first part): a new option under Options > Drag & drop, "Drag files to other programs as real files (macOS)", lets you drag the selected files out of the window and drop them into Finder or other programs as real files. It is off by default; with it off, dragging works exactly as before. Windows and Linux, dragging out of archives and remote servers, and highlighting the drop target are not done yet.

## 0.9.77 – 2026-11-14

### Fixed

- Regular expression errors (issue #96): a broken pattern such as "<(" no longer shows the program's internal case-flag prefix (for example "missing closing ): (?i)("). The message now quotes only what you typed. This applies to the Mask field, Find Files (name and content), the Lister search and Multi-Rename. Matching itself is unchanged.

## 0.9.76 – 2026-11-14

### Fixed

- Folder history (Alt+Down) and frequently used folders (issue #95): long paths are no longer cut at the right edge with a horizontal scrollbar. The start of the path is shortened with an ellipsis, the last folder name always stays whole, and hovering shows the full path.

## 0.9.75 – 2026-11-14

### Added

- Window layout (issue #37): right-clicking the bar between the two panels lists fixed shares (50/50, 60/40, 70/30, 80/20 and the other way round), with the current one marked; the choice is applied and saved with the session. The bar's direction and tooltip now follow Ctrl+Shift+H. Ctrl+Shift+= (Ctrl++) and, on macOS, Shift+Cmd+= also make the text larger.

### Fixed

- A maximised window is now remembered even when no size was saved before, and is maximised again once the window is shown. Control mode's `window` command now maximises after setting the size instead of before (setting a size had un-maximised the window).

## 0.9.74 – 2026-11-14

### Fixed

- Control mode: a right-click (`click` with `button: 2`) on a file row now opens that row's context menu (issue #42). Before, the click moved the cursor, the list redrew, and the follow-up events went to a row that was no longer on the page. Control mode now sends each event to the live element (or the one at the same point) after a redraw, as a real mouse does. Nothing changes for normal use of the app.

## 0.9.73 – 2026-11-14

### Fixed

- Control mode: the `screenshot` command now waits for the page to finish painting and for any open dialog windows to show their dialog, then lays those dialog windows over the main window's picture, so the PNG shows what is on screen (issue #41). The reply says how many dialogs were laid over (`dialogs`); passing `dialogs: false` gives the window alone as before. Nothing changes for normal use of the app.

## 0.9.72 – 2026-11-14

### Added

- Seven reserved Total Commander commands now work instead of saying "not ready yet" (issue #38, first slice): the command line history popup (Alt+F8, Ctrl+Down) puts the chosen line into the command line; Show > Programs (Ctrl+F11) shows only folders and runnable files; Show > Define custom filter... asks for a mask such as "*.txt *.md | *.bak" and keeps it in the new setting srcUserMask; Show > Custom filter (Ctrl+F12) applies the saved mask (asking the first time); Alt+F11 and Alt+F12 show the left or right panel's path as a popup list from the root down. Folders always stay visible under a custom filter. cm_CommandBrowser keeps the command palette version. Still open in issue #38: the folder tree panel commands, directory branch listing, F9/F10 menu activation, volume label, printing, standalone find and the internal associations editor.

## 0.9.71 – 2026-11-14

### Added

- Command palette: Ctrl+Shift+P (Cmd+Shift+P on macOS) opens one search box over every command (with its keys and cm_* name), the folder hotlist, the folder history of both panels, the open tabs and the Options pages. Matching is fuzzy and items you chose before rank higher (saved under the settings key commandPalette). Type, Up/Down/PgUp/PgDn, Enter runs, Esc closes. "All commands..." in the Commands menu now opens the same box with only commands and a "Keyboard shortcuts..." button that opens Options at the Hotkeys page (issue #21).

## 0.9.70 – 2026-11-14

### Added

- View options has two new settings, "Sort names" and "Language for sorting". Sort names can be the usual language rules (the default, same order as before), case-sensitive (upper case first) or by character code (case ignored, natural number order still works). The language (for example hu, de or sv) is empty by default, which means the system's language. They apply to the panels, the tree view and custom-column sorting, and TC's SortUpper option now maps to the case-sensitive choice. Names Finder stores in decomposed form now sort like composed ones. Still open in issue #20: streaming big folder listings, the 1 second startup timing check, and Wayland drag and drop (issue #20).

## 0.9.69 – 2026-11-14

### Fixed

- Copying or moving a folder to an FTP, SFTP or WebDAV server now shows one clear error when a plain file with the same name already sits where the folder should go, instead of a confusing failure further on. Merging into an existing same-named folder on a server works as before. The rest of issue #17 (queue, background transfer window, progress box, merge on local copies) was already in the app and is covered by tests (issue #17).

## 0.9.68 – 2026-11-14

### Changed

- Docs only: the 22 items issue #103 names in FEATURES.md were checked one by one against the code. Each now has the mark that fits what really works and an "audit #103" note saying what exists and what is missing. A new test keeps those marks and notes from drifting back. The other partly-done marks still need the same check (issue #103 stays open). App behaviour is unchanged.

## 0.9.67 – 2026-11-14

### Added

- The quick view panel (Ctrl+Q) now has a right-click menu, so with the mouse you can change its view mode (1-5), copy, Find, Find next, Go to, use image commands, pick the encoding and text options, and Reload or Follow. It uses the Lister's own menus; keys and the Lister window work as before (issue #11).

## 0.9.66 – 2026-11-14

### Fixed

- Restoring tabs at startup now also remembers the folder a "locked, but folder changes allowed" tab is locked to. Before, a locked tab that showed a different folder at exit came back locked to that other folder, so "go back to the locked folder" went to the wrong place. The lock folder is saved as a new `<n>_lockpath` key (ignored by Total Commander) and also travels in .tab files and favourite tab sets. Older files without the key behave as before (issue #12).

## 0.9.65 – 2026-11-14

### Changed

- The feature list (docs/FEATURES.md) now includes the 22 new items from the market research (F-854 to F-875) with their current status, a ranked "Priorities from the market research" section, and a note that copying a folder onto a same-named folder merges them (F-460). A new test, TestCopyMergesSameNamedFolder, checks that merge rule. No change to how the app behaves (issue #104).

## 0.9.64 – 2026-11-14

### Fixed

- Viewing files on FTP, SFTP and WebDAV now works from the Lister and quick view instead of saying "not ready yet". The file is copied to a temporary local file first (shown as "Downloading..."), and F3 on a server folder counts its size from the server's listing. F3 from a menu, the button bar or control mode on an archive, server or search-results panel now behaves like the F3 key, and N/P step through the files (issue #101).

## 0.9.63 – 2026-11-14

### Changed

- Options audit: 18 options in the Options dialog that were saved but never used now control the features they describe (refresh: auto refresh, foreground only, network, excluded folders; drag and drop: confirm drop, spring-loaded folders and delay; hotlist history size; log: enabled and file; folder tabs: show, multi-line, close button, double-click open, new tab next to current, confirm close all, locked icon, title width). The dialog shows each feature's real value, and a value is written only when you change it, so behaviour stays as it was until then.
- The other 161 options that still have no effect are listed in the code and marked "no effect yet" in the dialog. A new setting, "Hide options that have no effect yet", can hide them; it is off by default. Remaining work stays on issue #94.
- Folder tabs redraw at once when changed in Options.

## 0.9.62 – 2026-11-14

### Changed

- Internal clean-up, no change in behaviour: features are now handed to Wails through its official application.NewService helper instead of copying the service structure through an unsafe pointer, so a Wails upgrade can no longer silently break the bindings. The same objects are bound in the same order, and a test now checks them (issue #114).

## 0.9.61 – 2026-11-14

### Fixed

- Updating a zip or tar archive no longer loses the archive's hard links, owner and group, or extended attributes. If the archive has other hard names, the new content is written into the original file so every name sees it (a backup is taken first and restored if the copy fails); otherwise the old owner, group and extended attributes are put on the new file before it replaces the old one. Archives that cannot be opened for writing, new archives and split volumes behave as before (issue #97).

## 0.9.60 – 2026-11-14

### Fixed

- Compare directories (Shift+F2) now treats a name in the decomposed form macOS uses and its composed twin as the same name, as Find and Synchronize directories already did, so one file is no longer shown as two one-sided entries. Each side keeps its own spelling in the lists, and no panel names or paths change (issue #98).

## 0.9.59 – 2026-11-14

### Fixed

- Typing an sftp:// address without a password in the path bar, or after "cd" on the command line, now asks for the password when the server accepts nothing else (it used to just fail; only quick connect asked). The password is tried last, after the agent, key files and keyboard-interactive, is asked again up to three times if refused, and is remembered for the session so a reconnect does not ask again.
- Still open on issue #10: saved passwords have no OS keychain storage yet (the master password is the only option).

## 0.9.58 – 2026-11-14

### Fixed

- The Configuration > Display options now also apply outside the core Full view: folder brackets show in Brief, Comments, custom column sets and the views-engine Full view, and the Full column view drops its Ext column when "separate extension" is off.
- The column views (custom sets, and Full with a views format) use the Display size unit, thousands separator, date format, 12/24-hour clock and seconds whenever the views' own format is left at its default; a format set in the views still wins. With Display at its defaults nothing changes.
- Changing a Display option makes the panel ask for fresh cells (issue #93).

## 0.9.57 – 2026-11-14

### Fixed

- Ctrl+F10 (Show > All files) now works: it removes any filter on the active panel, so "Hide equal files" after a folder compare can be undone. That filter is also dropped by a reread or the next plain Compare folders.
- The Synchronize window's date columns now fit the chosen date format instead of cutting dates off.

### Added

- New Mark menu command "Compare folders, mark one-sided folders", which also marks folders that exist on one side only. Plain Shift+F2 behaves as before unless the new setting compareDirs.oneSidedFolders is switched on (off by default).
- Docs: F-546 to F-552 are marked as built and waiting for QA (issue #9). Syncing with an archive or FTP as one side (F-555) is still open.

## 0.9.56 – 2026-11-14

### Added

- F4 on a file inside an archive now works (issue #8): the file is unpacked to a temporary copy that opens in your editor. Each time you save a real change, mormiCommander asks whether to pack the new version back into the archive; a save that changes nothing is ignored, later saves ask again, and a failed update is offered again. Archives that cannot be changed (7z, RAR, an archive inside another archive) still open the copy, with a note that the changes stay in the copy. Files on servers still say the editor is not ready. Nothing else changes.

## 0.9.55 – 2026-11-14

### Added

- Multi-Rename can now use values read from the file itself (issue #7): `[=img.exifdate]` (and exifyear, exifmonth, exifday, exifhour, exifminute, exifsecond) for the photo date, `[=img.width]`, `[=img.height]`, `[=img.size]` for the image size, and `[=audio.artist]`, title, album, track, year, genre for ID3 tags. `exif.*` and `id3.*` work as aliases so masks from Total Commander carry over. A file is read only when the mask uses one of these fields; values are cached and re-read if the file changes. Folders and files without the data give empty text. Nothing existing changes.

## 0.9.54 – 2026-11-14

### Fixed

- No more hangs on network and unreachable volumes (issue #16): a folder read that is stuck inside the system (for example an unplugged network share) no longer freezes the panel. Esc now stops the read at once, the panel stays on the folder it showed before, and after 4 stuck reads of the same folder the next one answers "not responding" right away; the other panel keeps working meanwhile. On macOS and Linux the drive list and the drive info line check each mount in parallel with a 1.5 second limit; a mount that does not answer is still listed, marked as not responding. Nothing else changes.

## 0.9.53 – 2026-11-14

### Added

- Error list for file jobs (issue #15): every file a copy, move or delete could not handle is now recorded with its target and a plain-English reason (up to 10,000 entries; the error count has no limit). The progress box and the transfer window show the error count, and the transfer window's "Errors..." button opens the list of a finished job with Retry failed, Copy list and Save log. New setting "Skip errors in background jobs (list them at the end)" on Configuration > Copy/Delete, off by default: when on, background copies and moves skip errors without asking, a job sent to the background stops asking, and a job that skipped errors opens the error list when it ends. With the setting off nothing changes.

## 0.9.52 – 2026-11-14

### Added

- Undo journal for file operations (issue #13, first slice): renames in place (Shift+F6), new folders (F7) and copy/move jobs on this computer's disks (F5, F6, drag and drop, paste) are now recorded in undo.json in the settings folder (up to 100 steps, kept across restarts). The new command cm_UndoFileOp (Cmd+Z on macOS only; Ctrl+Z stays Edit comment) asks first and then takes back the newest step; pressing it again goes one step further back. cm_UndoList opens a popup to pick one step. An undo never overwrites anything: items that were changed, moved or put back since are left alone and named in the message. Undoing a delete to the Trash is not done yet. Nothing changes unless you use the new command.

## 0.9.51 – 2026-11-14

### Added

- Zero-network checks (issue #14): a new run-time test builds every feature in a temporary settings folder and runs a scripted session (list folders, view text, Markdown and HTML files with remote images and styles, search by name and text, copy, thumbnails) while it refuses and records HTTP requests, DNS lookups and any internet socket beyond loopback. A second test feeds remote-heavy Markdown and HTML through the Lister and checks nothing remote is loaded. docs/privacy.md describes the guarantees; a CI job that drives the built app and watches its sockets is still planned. No change to app behaviour.

## 0.9.50 – 2026-11-14

### Added

- First-run setup page (issue #6): on the very first start on a computer (no settings.json and no saved window) one page offers a keyboard preset, a "press F5" check that says whether the fn key is needed on this Mac, and buttons that open System Settings for Keyboard and Full Disk Access. Existing installations never see it by themselves; Configuration > "First-run setup..." reopens it.
- Keyboard presets: "Total Commander" (the default, adds no keys, behaviour unchanged), "Mac-native" (Finder-style keys and Shift+arrows/Home/End/PgUp/PgDn selection) and "Vim" (J/K/H/L, G, /, : and more). Your own key changes still win over a preset. The choice is saved as "keyPreset" in settings.json.

## 0.9.49 – 2026-11-14

### Fixed

- Move to Trash (issue #36): the job no longer sits at 0% forever when Finder does not answer (for example while macOS waits for permission to control Finder). The Finder call now gives up after a minute plus a second per item (at most ten minutes), and Cancel stops a stuck trash call on every platform.
- The two common Finder failures, "not allowed" and "no answer in time", now show plain messages.

### Added

- When trashing through Finder fails on macOS, the error question also offers "Trash without Finder" (this item) and "All without Finder" (the rest of the job). These use the system trash call, which needs no permission but loses Finder's Put Back. The normal route is unchanged.

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
