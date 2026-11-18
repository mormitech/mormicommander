# Changelog

All notable changes to mormiCommander are listed here, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Feature branches add their lines under "Unreleased"; the
version is changed only when a round of features is merged.

## Unreleased

### Added

- Change start menu: each entry can have an icon (a PNG, JPEG, GIF, BMP or WebP file), shown before its title in the
  Start menu.
- Help menu: Report a problem opens the issues page in the browser, and Help shows its key (F1).
- Files > Copy in the same folder (Shift+F5).
- Replace file? question: a Compare tool button opens both files in the compare-by-content tool, then asks again.
- Options > Copy/Delete > Confirm overwriting hidden and system files (off by default): asks before a copy or move
  replaces a hidden file (a dot name on macOS and Linux, hidden or system on Windows).
- Options > Copy/Delete > Show a summary when a copy or move ends (off by default): the status line says how many
  files and bytes went across, and how many were skipped or failed.
- Files > Go to the trash folder, and Files > Put back from the trash…: items picked in the trash folder go back to
  where they came from (Linux and Windows from the trash's own record; macOS for items mormiCommander put in the
  Trash). Nothing is ever replaced.
- Options > Copy/Delete > Say when a drive has no trash before deleting (off by default): F8 on a network share (or
  a USB stick on Windows) says at once that the files will be deleted permanently, instead of offering the trash.

### Fixed

- The copy dialog's target list (Alt+Down) now also lists the folders of the other panel's tabs.

## 0.14.32 – 2026-11-18

Parity checks of the Mark (#61), Commands (#62), Net (#63), Show (#64) and Configuration (#65) menus found most
items already built; every menu entry with a key now shows it, and docs/FEATURES.md notes what was found.

### Added

- Mark > Select search results: selects in the active panel the files and folders the last Find files search found.
- The Mark menu shows the keys of Select group, Unselect group, Select all, Unselect all, Invert selection, Select and
  unselect all with the same extension, Restore selection and Compare folders.
- Commands > Refresh folder tree: reads the open folders of the tree in the panel (Ctrl+F8) again, so new folders
  show up; open folders stay open.
- The Commands menu shows the keys of Folder hotlist, Folder history, Folder tree, Find files, Swap panels, Swap panels
  with tabs, Show folder under cursor in left/right panel, Branch view, Open terminal here, Go to root folder and
  Command line.
- The Net menu shows the keys of FTP connect, New FTP connection, Disconnect and Transfer mode.
- Show > Show only selected files and Hide selected files: the panel shows only the selected files, or all but them,
  until the folder is read again (Ctrl+R).
- Show > Active panel alone: the active panel takes the whole width; again brings both panels back.
- Show > View mode: the saved view modes as a submenu, to switch the panel to one.
- The Show menu shows the keys of the views, Quick view, the filters, the sort orders, Reread and Panels on top of
  each other.
- Configuration > Favourite tabs settings…: the favourite tab sets can be edited from the Configuration menu.
- Configuration > Options… shows its key (Cmd+, on macOS).
- Show > View modes… > Move a view mode up: puts the view modes in order (the first nine are the View mode submenu).
  Saving a view mode under a name in use now changes that mode in its place.

## 0.14.31 – 2026-11-18

Parity checks of the macOS key rule (#57), the command line (#58), the button bar (#59) and the Files menu (#60)
found most items already built; docs/FEATURES.md notes what was found.

### Added

- The Commands menu lists Command line history, Path to command line, Name to command line and Path and name to
  command line, with their keys (Alt+F8, Ctrl+P, Ctrl+Enter, Ctrl+Shift+Enter).
- Files > Internal associations…: map file masks to a program or an internal command, one "mask = command" per line;
  Enter on a matching file uses it instead of the system's app. Empty by default, so Enter works as before.
- Change attributes can set each file's modified date from its EXIF date (photos) or from a date in its name.
- Files > Print file list with subfolders… prints every file below the folder.
- The Files menu shows the keys of Pack, Unpack, Test archives, Properties, Calculate occupied space, Multi-rename
  tool, Edit comment and Quit.
- Real file drag-out on Windows too (#108): with Options > Drag & drop > "Drag files to other programs as real files
  (macOS, Windows)" on, files dragged out of a panel drop into Explorer and other programs as real files (copied). Off
  by default, as before.

## 0.14.30 – 2026-11-18

### Fixed

- The build checks pass again after 0.14.21–0.14.29: the window layout no longer assumes the app can report the
  active panel (a test stand-in could not), and the Options test no longer expects Layout.MenuBar to be marked as
  having no effect, since every option now works.

## 0.14.29 – 2026-11-17

### Added

- **Colour labels and notes for files and folders** (board issue #147). Right-click > Colour label… gives the
  selection a colour (seven presets, a custom colour, or none), shown as the name's colour; it wins over Colour by file
  type. Right-click > Note… adds a short note, shown as a marker with the note as its tooltip and in the status line.
  Both are in the Files menu and in Properties too. They are kept in mormiCommander's own notes.json in the settings
  folder, so they work on any disk and on servers, and they follow renames and moves made in mormiCommander. Files >
  Clean up notes and colour labels… removes the entries of files that are gone. (Total Commander's descript.ion
  comments, Ctrl+Z, stay as they were.)
- **A second sort key** (F-114): Ctrl+click (Cmd+click on macOS) on a column title sorts by it within the first
  column; the header shows ▲2 or ▼2. A plain click on another column drops it.
- **Left-drag selection** (F-135) in Options > Operation > Mouse selection mode "Commander": dragging up or down from a
  row selects the rows passed; a sideways drag still drags the files.
- **F1 in a dialog** opens the help on a page about that dialog (F-145).
- Show > View with the built-in viewer only (cm_ListInternalOnly) is in the menu (F-177).

## 0.14.28 – 2026-11-17

### Added

- **Folder names complete as they are typed** in the path bar, the copy and move dialog's target and Find files'
  "Search in": the first matching folder is filled in after the caret, selected, so typing on replaces it; Tab or
  Right takes it.
- **Alt+F4 and the close button can minimise** (Options > Operation, off by default); Files > Quit still quits. With
  "Minimise to tray / menu bar" on, they hide the window to the icon.

## 0.14.27 – 2026-11-17

### Changed

- **Every option of the Options dialog now works** (board issue #94): the last ones are wired, each keeping today's
  behaviour at its default, and none is marked "no effect yet" any more.
  - FTP: passive mode, hidden files, timeout, retries and the delay between them, keep-alive, the anonymous e-mail,
    the files sent in text mode, the default download folder (offered when copying from a server to another server or
    an archive) and Resume as the default answer for a shorter file. A saved connection's own settings still win.
  - Proxy: a proxy (HTTP CONNECT, SOCKS4, SOCKS5 or USER@host) for FTP connections that have none of their own.
  - Plugins: the 7-Zip, unrar, exiftool and ffprobe programs (ffmpeg is looked for next to ffprobe), and "Use
    configured external tools" off starts none of them. unrar tests RAR archives the built-in reader can't read;
    exiftool gives picture sizes and dates for the columns and Multi-rename when the built-in readers know nothing.
  - Packer: an external zip program instead of the internal packer, owner and permissions in tar, Move to archive, one
    archive per file and Include subfolders as the Pack dialog's defaults, Treat archives like folders (off: Enter
    opens them with their program) and searching inside archives in new Find files searches.
  - Zip: the compression level of "Default" (0 stores), UTF-8 names, file times when unpacking, the temporary folder,
    AES-256 or ZipCrypto for new encrypted archives, and ZIP64 (off refuses archives over 4 GB or 65535 entries).
  - Language: a language file of "English text=Translated text" lines (optionally in sections like [hu], chosen by
    the interface language) translates the menu bar, the function keys, the popup lists and the dialogs' titles and
    buttons.

## 0.14.26 – 2026-11-17

### Changed

- More options that were kept but had no effect now work (board issue #94). Each keeps today's behaviour at its default:
  - Thumbnails: the height of the box, the cache (on or off, its folder and its maximum size), file names below the
    thumbnails, which file types get one (empty: every type that can), the largest file that gets one, network
    folders, and a picture from inside each folder.
  - Background queue: how many queued jobs run at the same time, whether queued jobs start by themselves (off: they
    wait for Start now), the transfer window opening when a job goes to the background, and finished jobs leaving the
    list.
  - Find files: Use the system index (Spotlight on macOS, locate on Linux) finds names through the index and then
    applies the rest of the search; Text encoding for content search sets the encoding new searches start with.
  - Misc > Custom main menu file: a Total Commander .mnu file replaces the menu bar.

## 0.14.25 – 2026-11-17

### Changed

- More options that were kept but had no effect now work (board issue #94). Each keeps today's behaviour at its default:
  - Operation: remember folders, folder history, the command line history and the panel layout and views on exit,
    save settings on exit (off keeps the session that was saved last), allow only one copy of the program (now read
    from the Options value), and minimise to the menu bar or notification area: the window hides and a click on the
    icon brings it back.
  - Viewer/Editor: use the external viewer for F3 (on: when one is named, as before), choose the viewer by file type
    with Lister > Open these types in the external viewer, wait for the editor and then reread the folder, and ask
    before an edited copy goes back into its archive.
  - Lister: detect the text encoding (off: UTF-8), show images, show PDF files, and open text files bigger than the
    set size in hex.
  - Copy/Delete: copy the file comments (descript.ion) along, big file mode by itself above the size set, and a move
    between drives that deletes the originals only after every file was copied (off by default).
  - Refresh: the delay before a changed folder is read again, and whether removable drives refresh by themselves.
  - Log file: copy and move, delete, rename, new folders, pack and unpack, and network transfers are written to the log
    (when the log is on), or errors only.

## 0.14.24 – 2026-11-17

### Added

- **View modes** (Show > View modes…, cm_SrcViewModeList and its left, right and other-panel versions): save a panel's
  view, column set and sort under a name, and switch to it from the list. With a folder mask, a view mode is taken by
  itself in matching folders (Options > View modes > Switch view modes automatically) and the panel goes back to its
  own view after them.

### Changed

- More options that were kept but had no effect now work (board issue #94). Each keeps today's behaviour at its default:
  - Font: main window font and size (bars, function keys, status lines, menus), the Lister's font, and the interface zoom.
  - Colors > Colour scheme: the name of a built-in scheme (Dark, Classic blue…) replaces the colours of the settings
    file.
  - Operation > Mouse selection mode "Commander": the right button selects (a press toggles, a drag selects the rows it
    passes, holding it opens the menu).
  - Drag & drop: right-drag shows a Copy / Move / Link menu where it is let go (off by default), the drag start
    distance, and a middle click on a folder opens it in a new tab or the other panel (a middle drag still pans).
  - Quick search > Show matching names in a list: the matches appear above the search box.
  - Folder tabs > Ctrl+Up opens folder under cursor in a new tab: off opens it in the current tab.
  - Custom columns: list the column sets in the Show menu, and fit the columns to the panel width.
  - Hotlist: add new entries at the top, and one folder history for all tabs of a side instead of one per tab.
  - Tree: follow the other panel's folder (network folders only with "Read network folders in the tree"), show hidden
    folders, show files, and expand folders on a single click.
  - Misc: beep on errors, a sound when a job that ran 10 seconds or more ends, and "Check for updates", which once a
    week offers to open the releases page in the browser (mormiCommander itself still never goes online).

## 0.14.23 – 2026-11-17

### Added

- **System file icons.** Options > Icons > Show icons can now be set to "All, including file icons": every file and
  folder shows the icon the system gives it (Finder's on macOS, Explorer's on Windows; Linux keeps mormiCommander's
  own icons). Programs and folders with icons of their own show those (Program-specific icons, Folder-specific icons),
  and Load icons from network folders decides about network volumes. Icons are fetched for the rows on screen only and
  kept. The default stays the standard icons, and "None" hides them.
- **Icon size** (Options > Icons): 16, 24, 32 or 48 pixels; the rows grow to fit.
- **Overlay icons:** Options > Icons > Show overlay icons and the new Show > Overlay icons (cm_SwitchOverlayIcons)
  turn the arrow after links on and off.
- **Icons in menus** (Options > Icons, off by default): the right-click menus show the icons of their commands.

## 0.14.22 – 2026-11-17

### Added

- **Tooltips with file facts.** Options > Tooltips now works: the file lists can show the name and the file's size,
  date and kind on hover (for a picture also its size in pixels), after the delay set there. "Tooltips for network
  files" keeps them off in network folders. The default is still the tooltip with the whole name, now shown only when
  the name does not fit.
- **The ignore list.** Options > Ignore list and the new Show > Hide files from the ignore list (cm_SwitchIgnoreList)
  hide the files and folders matching the masks of the ignore file (one mask or full path per line, as in Total
  Commander's ignore.txt) in every panel. Off by default.
- **Drop files on a folder tab.** Files dragged from a panel onto a tab are copied into that tab's folder (Shift
  moves), like dropping them on a folder.

## 0.14.21 – 2026-11-17

### Added

- **Panel sizes by command:** cm_20Percent to cm_80Percent give the left (or top) panel that share of the window;
  cm_100Percent shows the active panel alone, and running it again (or Tab) brings both panels back.
- **Commands for a named panel:** left, right and other-panel versions of All files, Programs, Custom filter, Define
  custom filter, Quick view and Quick view with the built-in viewer only, plus cm_TrgOpenDrives. cm_SrcBrief and
  cm_SrcFull (and their left, right and other-panel versions) are other names of the Brief and Full views.
- **cm_GoToDir:** the other panel shows the file under the cursor; for a link, the place it points to, with the cursor
  on the target.
- **cm_VisTwoDriveButtons and cm_VisBreadCrumbs** toggle the two new Layout options below.

### Changed

- Options that were kept but had no effect now work (board issue #94). Each keeps today's behaviour at its default:
  - Layout > Two drive button bars: off shows only the active panel's drive bar.
  - Layout > Breadcrumb path bar: off makes the path plain text; a click anywhere on it edits it.
  - Layout > Show drive combo box: the volume name above each panel opens the drive list when clicked.
  - Layout > Show main menu bar: off hides the menu bar on Windows and Linux (macOS keeps its menu bar).
  - Layout > Separate folder tree: changing it does what the Show > Separate tree commands do.
  - Display > Show system files: off hides files with the Windows system attribute like hidden files.
  - Display > Size format in status bar: the unit of the sizes in the status line.
  - Display > Show .. in root folders: a ".." in a drive's root that opens the drive list.
  - Display > Cursor bar across all columns: off marks only the name under the cursor.
  - Display > Show long names: off shows names the 8.3 way (PROGRA~1.TXT).

## 0.14.20 – 2026-11-16

### Changed

- **A network folder opens in the panel at once.** Choosing a server, a share or a mapped network drive no longer
  leaves the panel on the folder it was in until the share is connected and read: the panel goes there right away,
  empty, with "Connecting…" and a spinner inside, and fills when the folder is read. If it can't be opened, the
  panel says why in the same place. Going elsewhere meanwhile still lets the opening go on in the background, as in
  0.14.6.

## 0.14.19 – 2026-11-16

### Fixed

- The zero-network test failed on every system after 0.14.15: the Lister's media player listens on 127.0.0.1, and it
  is now one of the allowed loopback listeners. docs/privacy.md says what the player serves.

## 0.14.18 – 2026-11-16

### Fixed

- **Later parts of a split TV recording play in the Lister** (name_003.mpg and the like). Such a part starts in the
  middle of the stream; it showed as hex, and ffmpeg took its pictures for MP3 sound. It is now recognised as a film,
  and ffmpeg starts reading where the stream really begins.
- **Films show at their real shape.** TV recordings store their pictures with non-square pixels (720×576 shown as 4:3
  or 16:9); converted films are now scaled to square pixels, and a film fills the Lister window at its own shape, with
  black bars where the window's shape differs, as a player does.

## 0.14.17 – 2026-11-16

### Changed

- **The drive bar has a divider before the network drives too**, so it reads in three groups: disks and Home, network
  drives (connected shares and the shares you mapped), and the folders you mapped. On macOS and Linux the volumes are
  put in those groups; on Windows the drive letters keep their order.

## 0.14.16 – 2026-11-16

### Changed

- **A network share mapped to the drive bar is a network drive there.** Its button stands with the drives, before
  the divider, with the network drive icon; the folders you mapped stay after the divider. Its menu has Eject instead
  of Move left/right and "Remove from the drive bar": Eject disconnects the share and takes the button off (a share
  that is not connected is just taken off). Ejecting the share any other way takes its button off too. Move left and
  right move a mapped folder among the mapped folders only.

## 0.14.15 – 2026-11-16

### Added

- **The Lister (F3) plays every kind of film and sound, streamed** (board issue #150). A film is no longer read whole
  before it plays: it starts at once, can be sought anywhere, and there is no size limit any more (the 256 MB limit
  is gone for sound and films). Files the web view plays itself (MP4 and MOV with H.264 or HEVC, MP3, AAC, WAV, FLAC)
  are read in pieces as they play. Everything else (MPEG-2 .mpg and .ts TV recordings, .mkv, .avi, .wmv, .flv, MP4
  with AC-3 sound and so on) is converted while it plays by ffmpeg, when it is installed (brew install ffmpeg), in
  6-second pieces made only when they are needed (with the hardware video encoder on macOS); the status line says
  "Converted while it plays". MPEG program and transport streams, M2TS, ASF/WMV and FLV files are now recognised as
  films (a TypeScript .ts file still is text).

## 0.14.14 – 2026-11-16

### Fixed

- **A network share can be ejected from the drive bar** (macOS). Eject asked diskutil to eject a disk, and a share has
  none ("Failed to find disk"); it is now unmounted instead, as Finder's eject button does, and the message says it
  was disconnected. A mapped share's button has Eject too while the share is connected.

## 0.14.13 – 2026-11-16

### Fixed

- **A network share mapped to the drive bar shows one button, not two.** Opening a mapped share (such as
  network://DiskStation/z) mounts it, and the mounted volume added a second button for the same share. While a mapped
  button stands for a share, its mounted volume is left out of the drive bar and the drive list, and the mapped button
  lights up while a panel is inside the share. Disconnecting and ejecting still know the volume (macOS).

## 0.14.12 – 2026-11-16

### Fixed

- **Dragging a dialog window (Options and the others) is smooth again on macOS.** A dialog window is a child window of
  the main window, and macOS moved it against its parent on every step of a drag, at some ten frames a second. It now
  lets go of the main window while it is dragged and is attached again, above it, when the mouse button is up, so it
  still stays above the main window and moves with it.

## 0.14.11 – 2026-11-16

### Fixed

- **No more text selection in menus, lists and other parts of the interface.** Dragging over the Options page list (or
  any label, button or list) painted a text selection; the "no selection" rule was written only in the form WebKit
  ignores. Text fields and the places meant for copying (messages, paths, logs, file contents) can still be selected.
- **List boxes in dialog windows no longer jump under the pointer** (the shortcut, button, user command, colour,
  start menu, hotlist, connection, download, tab set, column set and saved search lists). When a list's choice changed,
  the dialog's copy in the main window scrolled to it and sent that scroll to the dialog window, which moved the list
  between press and release, so a click could choose another row. The dialog window now keeps its lists' scrolling.
- The test of 0.14.7's volume tops failed on Windows (it built its paths with "/" only); the app itself was right.

## 0.14.10 – 2026-11-16

### Fixed

- **Clicking the empty part of the Options page list no longer chooses a random page.** The list was the web view's
  native list box, which drew its rows apart from where it took the clicks once scrolled; it is now a list of
  mormiCommander's own (click, arrow keys, Page Up/Down, Home and End), and a click below the last page does nothing.

## 0.14.9 – 2026-11-16

### Fixed

- **"Panels above each other" can be turned off again in Options.** Once on, unticking it put the panels back above each
  other after OK. The panel text size in Options (Font) had the same flaw after Ctrl+= or Ctrl+- had been used.

## 0.14.8 – 2026-11-16

### Changed

- **The Lister (F3) starts playing a film or sound at once.** Quick View doesn't, so moving the cursor over a folder of
  songs stays quiet. A film or sound stops when the Lister closes, goes to another file or switches to another view.

## 0.14.7 – 2026-11-16

### Changed

- **A mounted volume is a drive of its own in the panels** (macOS): the top folder of a share in the drive bar, such as
  /Volumes/music, or of a USB disk has no ".." row, and Backspace doesn't step out to /Volumes; "Go to root folder"
  goes to the volume's top instead of "/". Folders inside the volume keep their "..". The startup disk and the home
  folder are as before.

## 0.14.6 – 2026-11-16

### Changed

- **Opening a server or share in the Network panel shows a spinner next to it** instead of fading the list, and the open
  goes on in the background when you go elsewhere meanwhile: coming back shows the spinner still turning next to it,
  and when it opens while you are back at its folder, the panel goes into it. A second Enter on the same folder waits
  for the same open instead of starting another. A server's list of shares is kept for a minute, so going back and
  forth between a server and its shares doesn't ask the server every time; mounted shares open at once anyway.

### Fixed

- macOS: after macOS's own window from mounting a share (its sign-in window, or "There was a problem connecting…")
  closes, mormiCommander takes the focus back when it had it before, so the keyboard works again without a click.
- macOS: volumes mounted "nobrowse" are left out of the drive bar, as Finder does, such as the Recovery volume macOS
  mounts at /Volumes/Recovery while it prepares an update (it couldn't be ejected from here anyway).

## 0.14.5 – 2026-11-15

### Added

- **More Total Commander commands work** (#38); before, their keys and menu items said "not ready yet". No key changed
  meaning.
  - cm_CDtree (Alt+F10): a folder popup to walk down and up the tree and go to a folder.
  - cm_SeparateTree1/2/Cycle/Off (Ctrl+Shift+F8 cycles) and cm_FocusTreeToggle (Shift+Tab): mapped onto the panels'
    tree view (none, the active panel, both), each panel coming back to its earlier view.
  - cm_ActivateMenu (F9/F10): the menu bar as a popup over the panel, since the page can't open the native menu.
  - cm_VolumeId: the volume's information (name, mount point, file system, size, free space); renaming the label is
    left to the system.
  - cm_PrintFile (Ctrl+F9) asks, then prints a file with the system's print command; cm_PrintDir previews the file list
    as text and prints it on Print.
  - cm_SearchStandalone: Find files in a window of its own.
  - The command browser assigns keys (F-373): F2 or "Assign key…" opens Keyboard shortcuts with the command picked.
  - cm_InternalAssociate stays open: it needs association storage and a dialog of its own.

## 0.14.4 – 2026-11-15

### Changed

- **33 more Options work** (#94); they are off the "no effect yet" list. With nothing stored, each one does what the
  app did before.
  - Bridged to existing settings: Shift+F6 selects the name (Operation.RenameSelectsName), Confirm overwriting files,
    Confirm drag and drop, Allow dragging to other programs (new "dragToOtherApp", on by default), Lister tab width,
    binary line width, font size, external viewer and image size (new: 2 also enlarges small images), command line
    history size and frequently used folders count (were fixed at 50 and 20), hotlist paths, default packer, tabs
    below the file list, restore tabs at startup, activate tabs opened with Ctrl+Up, the three Favorite tabs options,
    custom column sets switching by folder, thumbnail size, and a size limit for the error log (renamed to .old).
  - Read directly: double click on empty space goes up, go to root when changing drive, Enter on an archive opens it
    with its program, command line Tab completion, the Find Files defaults for the first search (depth, archives,
    case, regular expressions, hidden files, links) and Stop after this many results.
  - Still marked "no effect yet": options whose feature isn't there (FTP and proxy, icons, translations, thumbnail
    cache, ignore list, most log categories) or that can't be honest as labelled (Zip.Level's "0 = store", the refresh
    delay).

## 0.14.3 – 2026-11-15

### Added

- **Overwritten files can go to the trash** (#99), off by default. Copy/Move Options has "Move files that are overwritten
  to the trash" (the dialog remembers the choice), and Synchronize directories has "Also trash files that are
  overwritten" under "Delete to the trash". A copy still writes the new contents to a temporary file first, and only
  when they are complete does the old file (or link) go to the trash and the new one take its name, so a failed copy
  leaves the old file where it was; a failing trash stops that file with an error and changes nothing. A move within
  one disk trashes the old file just before the rename. Folders in the way are never trashed. In Synchronize the
  option works between local folders; on servers and in archives files are replaced as before.

## 0.14.2 – 2026-11-15

Release of 0.14.0 (the 0.14.0 and 0.14.1 builds each stopped on a test timing problem, so neither was published).

### Fixed

- Tests: the edited-file watch test waits for the change to the new size; writing truncates the file first, so a poll
  in between could see it empty, as it did on the Linux runner for v0.14.1.

## 0.14.1 – 2026-11-15

Release of 0.14.0 (its macOS build stopped on a test timing problem, so 0.14.0 was never published).

### Fixed

- Tests: fitpath.test.js and popup-path.test.js no longer load the Wails runtime, whose 50 ms start-up timer could fire
  after the file's test environment was gone and fail the run ("window is not defined"), as it did on the macOS
  runner for v0.14.0.

## 0.14.0 – 2026-11-15

Release of 0.13.1 to 0.13.4: the tech-debt round and fixes from testing the Network panel against a real NAS. Servers
and shares show no made-up attributes; a button, bookmark or typed network://Server works right after the start; a
share macOS mounts under the server's name shows the share's name in the drive bar; the operations log always has
its closing line; and control-mode screenshots with a dialog are drawn once.

## 0.13.4 – 2026-11-15

### Fixed

- Drive bar: a network share that macOS mounted at a folder named after the server (share "z" of DiskStation at
  /Volumes/DiskStation.local) shows the share's name.
- Control mode: a screenshot with an open dialog no longer shows the window twice. Newer macOS takes a dialog window
  together with its parent, so its picture is the whole window group, which the window's own picture already shows;
  such a picture is no longer laid over it.

## 0.13.3 – 2026-11-15

### Fixed

- File operations: a job writes its closing "done" line to the operations log before it counts as finished, so a log
  read right after the job ends always has it (the Linux CI run of 0.13.2 caught it missing once).

## 0.13.2 – 2026-11-15

### Fixed

- Network panel: servers and shares show an empty Attr column instead of a made-up "rwxr-xr-x" (a new entry flag,
  NoAttr, says there is nothing to show; a real folder with no permissions still shows dashes).
- A server name used before the network search has found it, such as a button bar "cd network://DiskStation", a
  bookmark or a typed path right after the start, now waits up to 8 seconds for the server to answer instead of
  failing at once with "The folder … is not there".

## 0.13.1 – 2026-11-15

### Changed

Tech-debt round:

- Tests: the dialog-window theme test waits for the change instead of a single tick, so it no longer fails now and
  then (#145).
- The drive bar's sizes come from the same code as the rest of the app (internal/dirsize Space); internal/disk no
  longer has its own copies of the statfs and GetDiskFreeSpaceEx arithmetic (#102). The other #102 items were already
  done: round-trip quoting in descript.ion, the Linux creation date (statx), descript.ion hidden on Windows, and FTP
  stopping on unsafe server names like SFTP.
- Big folders and start-up measured (#20): 100,000 files listed in 0.36 s and scrolled smoothly; panels shown 0.49 s
  after the start (F-742 and F-757 done).
- The pointer-only features were tried with control mode's pointer gestures (#107): resizing a column by dragging
  its border, and reordering Multi-rename rows by dragging, both work.
- Wails advisories checked again (#116): v3.1.0 is still the newest stable release; the frame advisories that apply
  to v3 are fixed in it.

## 0.13.0 – 2026-11-15

Release of 0.12.1 and 0.12.2: the **Network panel**. The Network button (now at the right end of the drive bar) opens
the network in the panel, servers appear as they answer and the search keeps running in the background. On macOS the
app declares its use of the local network, so the system asks once, and servers are found at any hour.

## 0.12.2 – 2026-11-15

### Changed

- **The Network button opens the network in the panel** (#148), like Total Commander's Network Neighborhood, instead
  of a popup. The panel shows network://: a banner with a spinner while it searches, and servers appear one by one as
  they answer. The search keeps running in the background while the app runs, so coming back to the folder shows at
  once everything found meanwhile, and servers that go away disappear. Enter on a server lists its shares (asking for
  a user name and password when needed); Enter on a share connects it and shows its files, with ".." back to the
  shares. smb://server, nfs://server and afp://server typed in the path bar work the same way. An AFP server that
  also answers SMB is listed once.
- The drive bar shows Home and Network as icons only, with tooltips; Network sits at the right end of the bar.

### Fixed

- macOS: servers were not found before 10 in the morning, because dns-sd pads the hour with a blank.

## 0.12.1 – 2026-11-15

### Fixed

- macOS: the Network button found no servers when the app was started from the Dock or Finder. macOS's Local Network
  privacy shows an app nothing on the local network unless the app declares that it uses it; the app now does
  (NSLocalNetworkUsageDescription, NSBonjourServices), so macOS asks once. When nothing is found, the message points to
  System Settings > Privacy & Security > Local Network.

## 0.12.0 – 2026-11-14

Release of everything since 0.11.0 (0.11.1 to 0.11.7, listed below), and the **network neighbourhood**: browse the
file servers on your local network, open or map their shares, and disconnect them, on macOS, Windows and Linux. Also
in this release: the **GTK 3 Linux build** for Ubuntu 22.04 and Debian 12, dialogs that reuse two windows (no lost
focus or taskbar buttons on Windows), the Full Disk Access helper on macOS, and two rounds of engine review fixes
(23 bugs, then 19 follow-ups).

### Added

- **Network neighbourhood** (#148; TC's cm_OpenNetwork, cm_NetConnect, cm_NetDisconnect):
  - A **Network** button at the end of the drive bar, and Net > Network neighbourhood…, lists the file servers on
    the local network after the ones you used before. It finds them with Bonjour on macOS, Avahi on Linux and NetBIOS
    on Windows. "Type a server…" takes smb://server, \\server, nfs://server or afp://server.
  - Picking a server lists its shares. SMB shares are listed by the app itself, signing in as guest or with the user
    name and password it asks for; "Save the password" keeps it in the system keychain. NFS exports come from
    showmount.
  - **Open** shows a share in the panel. A share Open had to mount is disconnected again when the app ends.
  - **Map** keeps a share connected: a drive letter on Windows (with "Reconnect at sign-in"), /Volumes on macOS like
    Finder's Connect to Server, GVfs on Linux. Mapped shares show in the drive bar; on Linux, GVfs shares now appear
    there too.
  - Net > Connect network drive… types a share and maps it; Net > Disconnect network drive… unmounts one.
  - The network is searched only when you press the button; starting the app never does (docs/privacy.md).
  - Tests: parsers of every tool's output, the service with a fake network, and live tests against a real Samba
    server and NFS export in CI.

## 0.11.7 – 2026-11-14

### Fixed

- Tests: the zip split test from 0.11.6 closes the archive it opens, so Windows can remove its test folder.

## 0.11.6 – 2026-11-14

### Fixed

The review follow-ups (#149): each suspicion from the #91 review was checked with a test first, against the real
7-Zip where it mattered.

- Data safety:
  - 7z: "pack without subfolders" was ignored, because 7-Zip packs a folder named on its command line completely even
    with `-r-`. The files are now listed one by one, and "without paths" works for 7z too (two files with one name are
    refused). "Move to archive" deletes a source only when the tested archive lists it with the same size, and a
    folder only when it is empty afterwards. File names are never read as wildcards. Outside Windows the password goes
    to 7-Zip on its input and no longer shows in the process list.
  - Putting back from the trash, undo and moves on one volume use a rename that never replaces (renamex_np,
    renameat2, MoveFileEx), so a file that appears at the destination in the meantime is no longer overwritten.
  - Undo of a copied folder leaves the folder alone when something was added to it after the copy.
  - Zip split archives: the parts are written to temporary files first, so a split that fails leaves the parts of
    an older set in place.
- Correct results:
  - A rename that only changes the Unicode form of a name (NFC to NFD, as on macOS) is no longer refused as "exists".
  - Renaming a file to the name of another hard link of it (a case-only rename on a case-sensitive disk) is refused,
    in rename and in undo, instead of leaving the file under a temporary name.
  - Multi-rename: search and replace and the duplicate check compare names in NFC, as masks and Find do. A name
    ending in a dot ("file.") keeps it with the default mask.
  - Unpacking without paths keeps two files with one name: zip and 7z write the second under a free name, so the
    overwrite question sees both; tar skips the second and says so.
  - Names close to the 255-byte limit: temporary files (archives, settings, compare) use only the start of the name,
    and "name(2).ext" stays within 255 bytes.
  - macOS: folder sizes calculated earlier are found again whichever Unicode form the path has.
- Security hardening:
  - FTP active mode prefers the data connection from the server's own address, so another machine on the network
    can't take over a transfer by connecting first. A server that connects from another address still works.
  - AI (OpenRouter): a redirect from https to http is refused, so the API key never travels unencrypted.
  - The access tokens of the AI endpoint and the control mode are compared in constant time.
  - A tampered FTP password-store header can no longer ask for billions of key derivation rounds.
  - Windows: when a button's program is a .bat or .cmd file, names with `& | < > ^ ( )` are quoted, so a file name
    can't add commands. Other programs get names as Total Commander passes them.
  - Extended attributes: the size-then-read retries are bounded.

## 0.11.5 – 2026-11-14

### Fixed

A fresh review of the engines (#91) found 23 bugs; each has a test that failed before the fix.

- Data loss and safety:
  - Splitting a file with an absurd part size could empty the source file with its own first part.
  - Combining could append the result to itself until the disk was full (when the last part was missing).
  - "Move to archive" into a zip deleted a file the archive never stored, when two sources had the same name in the
    archive; such a move is now refused before anything is written, as for tar.
  - A folder download from an FTP, SFTP or WebDAV server could write files outside the target folder when the server
    listed names like "../x" (path traversal); such names now fail the copy.
  - Linux: the AI runs' working folder in /tmp could be one another user planted; it is used only when it is the
    user's own private folder. The control-mode file (--control) is always written new and private.
  - Windows Terminal: a folder name with ";" could start another command ("open terminal here").
  - FTP: a server answering PASV with 127.0.0.1 could make the client connect to its own local ports.
  - Copying a link or file over an existing one no longer removes the old one before the new one is in place.
  - Copy or move of a folder into itself through a link or another case of its name is refused clearly.
- Correct results:
  - Regex content search found nothing in long single-line files when a match ran past a 1 MiB read chunk.
  - Unpacking tar and RAR archives without paths could give a hard link another file's content.
  - WebDAV: resuming a download checks where the server's answer starts (a wrong range broke the file); a folder
    listing keeps only its own children.
  - Files with names near 255 bytes can be overwritten, renamed case-only, undone, and replaced as links.
  - The branch view of a selection says when it was cut at the limit.
  - Content search no longer counts links to folders as errors.
  - FTP uploads through an HTTP proxy close cleanly (they could lose their end).
  - Passwords typed in URLs no longer appear in error messages.

## 0.11.4 – 2026-11-14

### Added

- A Linux build for older distributions (#40): GTK 3 with WebKitGTK 4.1, for Ubuntu 22.04, Debian 12 and others
  without GTK 4 and WebKitGTK 6.0. Built on Ubuntu 22.04 with Wails' gtk3 tag, it comes as
  `mormicommander-gtk3_X.Y.Z_amd64.deb` (package mormicommander-gtk3, which takes the place of mormicommander) and
  `mormicommander-X.Y.Z-linux-x64-gtk3.tar.gz`. CI installs the .deb on clean Ubuntu 22.04 and Debian 12 and checks the
  app starts and keeps running; releases include both files.

## 0.11.3 – 2026-11-14

### Changed

- Dialog windows are reused for as long as the app runs: the two hidden windows made at start show every dialog. When
  a dialog closes, its window is hidden, emptied and gets the next dialog; it is no longer destroyed and replaced by a
  newly made spare. A new window is made only when all are in use (a dialog opened from a dialog), and it is kept for
  later too. So no window is ever made while a dialog is open, which is what took the focus from a just-opened dialog
  on Windows (replaces 0.11.2's Windows-only change).

## 0.11.2 – 2026-11-14

### Fixed

- Windows: dialog windows (Copy, Properties and the rest) no longer get a taskbar button of their own. They are real
  dialogs, owned by the main window, as in Total Commander: Wails gives every window the "taskbar button" style
  (WS_EX_APPWINDOW), which beat the owner; dialog windows now lose it before they are shown.
- Windows: a dialog window no longer loses the focus about half a second after it opened. Making the next spare dialog
  window in the background (Wails hides and shows its WebView2 once its page has loaded) took the focus; on Windows the
  spare is now made when the dialog closes instead.

## 0.11.1 – 2026-11-14

### Fixed

- macOS: "Open Full Disk Access settings" on the first-run page now puts mormiCommander in the Full Disk Access list,
  ready to be switched on. macOS lists an app there only after it tried to read data the permission guards, so the
  button first tries to read a few such files (TCC, Safari, Mail, Messages folders), which simply fails without the
  permission and reads nothing else. The page also says to reopen the app after switching it on, and to use + if the
  app is still missing.

## 0.11.0 – 2026-11-14

Release of everything since 0.10.0 (0.10.1 to 0.10.27, listed below): **AI search and Ask AI** through
mormiCommander's own non-destructive tools (Claude Code CLI over a local MCP endpoint, or OpenRouter with the key in
the system keychain); the **branch view** (Ctrl+B) and Multi-rename across subfolders; **Synchronize folders with an
archive or a server** as one side; saved passwords in the **system keychain**; **undo of delete to Trash**; terminal
words (cmd, terminal, shell names) and Ctrl+Alt+T; Tab completion and Ctrl+E history in the command line;
middle-button panning; a faster key repeat option; big dialogs keeping their size; universal macOS build and signing
ready in CI; and two new CI guards, the **zero-network** test of the whole app and **live tests** against real FTP,
SFTP, WebDAV servers and 7-Zip, which found and fixed an FTPS upload bug and a WebDAV redirect bug.

## 0.10.27 – 2026-11-14

### Fixed

- CI: the live archive tests search each archive in a folder of its own, so the files it was packed from (lying next
  to it) are not found as well.

## 0.10.26 – 2026-11-14

### Fixed

- WebDAV on Apache (mod_dav) and other servers that redirect a folder named without its trailing slash: the folder
  could not be opened ("PROPFIND /dav: 200 OK"), because the redirect was followed as a plain web page request. The
  client now sends the same WebDAV request again to the folder's address and follows no other redirect (found by the
  live tests, #106).

## 0.10.25 – 2026-11-14

### Fixed

- CI: the live tests' server script makes vsftpd's config file root's (vsftpd refuses it otherwise) and reports why a
  server did not start; the live-tag check reads Windows CRLF checkouts; a trash test no longer trips over Linux
  reusing a freed inode number.
- Undo of delete to Trash also checks size and date when it looks for the trashed item, so a new file that happens to
  get a freed inode number is never taken for it.

## 0.10.24 – 2026-11-14

### Added

- Live tests against real servers and archive tools (#106): `go test -tags live` (skipped by default) drives the app's
  own code against real FTP (passive, active, explicit FTPS), SFTP (password, key, host key checks) and WebDAV servers:
  F5/F6/F8 jobs, rename, new folder, resume, the download list, copies between servers, and Synchronize folders with
  each kind as a side. Archives with the real 7-Zip and unrar: pack, read, test, unpack, search inside, Feed to listbox
  and F5/F8 through it. A CI job (`live`, Linux) starts vsftpd, OpenSSH and Apache mod_dav on the runner and runs them.

### Fixed

- FTPS uploads could lose the end of the file while reporting success (the data connection was closed while the
  server still had unread data, so the system reset it). Uploads now close the data connection cleanly.

## 0.10.23 – 2026-11-14

### Added

- Zero-network test of the whole app (#14): a CI job (`zero-network`, Linux) starts the built app under a virtual
  display in control mode with temporary folders, drives a session (browsing, the Lister on text, Markdown, HTML and SVG
  files full of remote content, quick view, thumbnails, copy, Find files, the Options and Ask AI dialogs) and watches
  every process of the app, the web view's included, with strace and ss. Any connection that leaves the machine, or a
  DNS lookup, fails the build; a self-test first proves the watcher catches a planted connection. Releases wait for it.
  docs/privacy.md and the README describe it.

## 0.10.22 – 2026-11-14

### Added

- Undo of delete to Trash (#13): Cmd+Z (cm_UndoFileOp) and the undo list now also bring back files and folders moved
  to the Trash / Recycle Bin, to their old place; undo refuses when something else is there now. The item is found in
  the trash of its own volume (macOS ~/.Trash and .Trashes, the freedesktop Trash on Linux, $Recycle.Bin on Windows).
  Items that can't be found (for example macOS without Full Disk Access for the app, or network shares) are named, with
  a hint to use Put Back / Restore. Permanent deletes are still not undoable.
- Options > Operation > "Keep an undo journal of file operations" (on by default, as before).
- The copy/move dialog says that files replaced with "Overwrite all" can't be undone.

## 0.10.21 – 2026-11-14

### Added

- AI step 3 (#146): **Ask AI…** (Ctrl+Shift+A, also Shift+Cmd+A on a Mac; Commands menu), a modeless dialog for
  requests like "swap the panels", "sort by date, newest first", "show hidden files", "turn on dark mode", "make up a
  dark colour scheme" or "pack the selected files into test.zip". It uses the provider chosen in Options > AI, keeps a
  prompt history (Up/Down) and shows the tool calls and the answer; Stop or closing ends the run.
- New AI tools, all non-destructive:
  - `list_commands` / `run_command`: only commands on a reviewed allow-list (navigation, cursor, panels, tabs, views,
    sorting, hidden files, quick filter, branch view, compare marks, selection, bars, zoom); everything else, and any
    command added later, is refused.
  - `open_folder`, `set_panel_paths`.
  - `list_settings` / `get_setting` / `set_setting`: look and view options only, values checked; nothing about
    programs, paths, confirmations, the trash, the network or the AI.
  - `get_colors` / `set_colors` / `restore_colors`: colour schemes with a minimum contrast, and every AI change can be
    put back for the session.
  - `pack_files`: a new archive only; it refuses when one of that name already exists.
- Nothing in the toolset deletes, moves, renames, overwrites or runs programs.

## 0.10.20 – 2026-11-14

### Added

- AI search step 2 (#146): OpenRouter, or any OpenAI-compatible API, as a second AI provider. Configuration > AI
  settings… (also a button in the Find files AI tab) chooses the provider, takes the API key (kept only in the system
  keychain, never in a settings file), lists the models with a filter, prices and context length, and sets the API
  address. mormiCommander runs the tool loop itself with the same tools Claude Code gets (up to 25 rounds). Claude Code
  stays the default and works as before.
- Privacy: the OpenRouter request goes out only when you start an AI search with it chosen, or press Show models; it
  carries your prompt, the panels' folders and the tool results (file names, sizes, dates). The zero-network test
  allows exactly this one function.

## 0.10.19 – 2026-11-14

### Added

- Saved connection passwords can be kept in the system keychain (#10, F-634): "Keep saved passwords in the system
  keychain" in the connection manager (Ctrl+F), off by default, so the master password stays the default. macOS uses
  the login keychain, Windows the Credential Manager, Linux the Secret Service (secret-tool); the connection list then
  holds only a reference, never the password. Turning it on or off moves the saved passwords both ways. A password
  missing from the keychain is asked for when connecting. The secret never appears on a command line.
- A server URL typed alone on the command line (sftp://…, ftp://…) opens in the active panel, like cd.

### Fixed

- A password in a typed URL (sftp://user:pass@host/) is used instead of being asked for again.
- The command line history no longer keeps passwords typed in URLs.

## 0.10.18 – 2026-11-14

### Added

- Synchronize folders with an archive or a server as one side, or both (#9, F-555): either field takes a local folder,
  an archive (/x/a.zip or /x/a.zip/sub) or a server URL (ftp://, ftps://, sftp://, webdav://), typed or taken from a
  panel. Compare (also by content, which downloads from a server) and sync both ways go through the same connections
  and archive engines as the panels. Uploads replace files safely (a temporary file renamed over the target). Writable
  archives: the zip family and tar variants; 7z, RAR, single-file gz/bz2/xz and archives inside archives or on servers
  are read-only, and the window says so and only offers copies out of them. Deletes on a server or in an archive are
  permanent, and the Synchronize dialog says so. Archive and server sides compare dates with a 1-second tolerance, since
  they keep whole seconds at best; local pairs compare as before. WebDAV, and FTP without MFMT, can't keep file dates,
  and a note after the sync says so.

## 0.10.17 – 2026-11-14

### Added

- Branch view (#7, F-086, F-087): Ctrl+B shows every file of the folder and all its subfolders in one flat list, each
  with its path relative to the folder; Ctrl+Shift+B does the same for the selected folders and files only. Copy, move,
  delete, view (with N/P), edit, sorting, selection, quick search and Multi-rename work on the real files. Big trees
  are read in the background with a count, Stop and Esc, up to 200,000 files. Ctrl+B again, Ctrl+Shift+B or ".."
  returns, with the cursor on the subfolder the file came from. A "BRANCH" badge marks the path bar.
- Multi-rename in subfolders (F-511): Ctrl+M in a branch view renames each file in its own folder; [P]/[G] and the
  conflict check are per folder, and Undo restores every file.
- More tag formats for Multi-rename fields and the Pictures/Music columns: EXIF date from PNG, WebP, TIFF, camera raw
  (DNG, CR2, NEF…) and HEIC; image size for WebP, TIFF, BMP and HEIC; audio tags from FLAC and Ogg Vorbis/Opus.

### Fixed

- Shift+F6 in a Feed to listbox list edits only the file's own name (it used to start with "sub/a.txt" and fail).

## 0.10.16 – 2026-11-14

### Added

- AI search, step 1 (#146): Find files has an **AI** tab. Describe what you are looking for ("PDFs over 5 MB changed
  this month that mention invoice") and the Claude Code command (`claude`, installed and signed in by you) searches by
  calling mormiCommander's own tools; the results land in the normal results list, so Go to file, Feed to listbox and
  the rest work on them. A progress log shows the tool calls and the answer; Stop ends the run.
- The tools the AI can use: `find_files`, `get_state`, `select_files` (by pattern or names, in either panel) and
  `set_panel_path`. None of them deletes, moves or overwrites anything, and the AI gets no shell or file access: Claude
  runs with all its own tools turned off and reaches mormiCommander through a local endpoint (127.0.0.1, random token,
  one run at a time) with only these tools. It runs in an empty folder of its own, so no project's CLAUDE.md or
  settings steer it.
- Options > AI: which AI to use (Claude Code for now), the path of the `claude` command (empty: found automatically,
  also in the login shell's PATH and the usual install places) and the model (empty: Claude's default).

## 0.10.15 – 2026-11-14

### Changed

- Server panels (FTP, SFTP, WebDAV) no longer have a bar of their own above the path bar: the transfer mode, protocol
  log and disconnect buttons sit in the path bar next to the "FTP" badge, as icons with tooltips (the mode icon shows
  Automatic, Binary or ASCII). "not connected" shows there too when the connection is gone.

## 0.10.14 – 2026-11-14

### Changed

- Big dialogs in windows of their own (Options, the button bar editor, Find files and the like) keep the size they
  opened with: switching to a page with more or less on it no longer resizes the window; only the user does. Small
  dialogs still fit their content.

## 0.10.13 – 2026-11-14

### Changed

- Held Up/Down key repeat: Faster and Fastest also start repeating sooner, after 300 ms and 230 ms (the Mac's default
  wait is about 375 ms); Fast keeps the system's wait. A system set to repeat even sooner still does.

## 0.10.12 – 2026-11-14

### Added

- Opening a terminal in the current folder from the command line: "terminal" or "term" on every system, and "cmd"
  on macOS and Linux (Windows runs cmd.exe itself, as before). A shell's name alone (zsh, bash, fish...) on macOS and
  Linux starts that shell in a terminal in the current folder.
- Ctrl+Alt+T (Ctrl+Option+T on a Mac) opens a terminal in the current folder (cm_ExecuteDOS), the usual terminal key
  of Linux desktops.

## 0.10.11 – 2026-11-14

### Added

- Options > Operation > "Held Up/Down key repeat speed": System setting (the default, as before), Fast, Faster or
  Fastest. With a faster speed, a held Up, Down, Page Up or Page Down key (also with Shift) repeats at
  mormiCommander's own pace once the system starts repeating it, instead of the system's repeat rate.

## 0.10.10 – 2026-11-14

### Added

- Tab completion in the command line: Tab completes the file or folder name at the caret (from the active panel's
  folder, or the folder the word names); with several matches each further Tab goes to the next one, Shift+Tab back.
  A folder gets a separator after it, names with spaces are quoted. With nothing to complete, Tab gives the keyboard
  back to the panel as before.

### Changed

- Ctrl+E (and Ctrl+X) bring back earlier command lines with the line focused and the caret at the end, and show the
  command line for the time being when it is turned off; each further Ctrl+E goes further back.

## 0.10.9 – 2026-11-14

### Changed

- With the command line turned off (Options > Layout > Show command line), typing in a panel shows it for the time
  being, as Total Commander does; Esc, or running the line with Enter, hides it again.

## 0.10.8 – 2026-11-14

### Changed

- macOS: while the window is not full screen, the main window keeps a strip of room at the bottom, so its rounded
  corners no longer cut into the command line or the function key buttons. In full screen the strip goes away.

## 0.10.7 – 2026-11-14

### Changed

- Typing in a panel moves the focus to the command line: its caret shows and the field is marked with an accent
  border. Keys that do not edit text (Up, Down, F-keys...) still act on the panel; once the line is empty again
  (Backspace, Enter, Esc) the focus goes back to the panel.

## 0.10.6 – 2026-11-14

### Changed

- Options > Layout > "Flat interface" now has a visible effect: turned off, the bars, function key buttons, button
  bar buttons, sort headers and status lines get a raised, bevelled look. On (the default) the look is as before.

## 0.10.5 – 2026-11-14

### Changed

- With no button bar above the panels, the drive buttons get more room above them instead of touching the top of the
  window.

## 0.10.4 – 2026-11-14

### Changed

- Options > Layout: "Show button bar" and "Show vertical button bar" say "bar is empty" while their bar has no
  buttons (the vertical bar has none at first), since turning them on shows nothing then; Configuration > Change button
  bar adds buttons. A review of the other options found none that is read but does nothing, so no new "no effect yet"
  marks were needed.

## 0.10.3 – 2026-11-14

### Added

- Middle-button panning in the file panels on every system: hold the middle mouse button and move the mouse to drag
  the list up and down (and across when it is wider than the panel). The pointer shows a grab hand while the button is
  down; the browser's own middle-click autoscroll is off in the panels.

## 0.10.2 – 2026-11-14

### Added

- Windows signing ready in CI: release tags send the .exe and then the installer to SignPath (free for open source) and
  use the signed files, once the SignPath secrets are added (docs/signing.md). Without them the builds stay unsigned as
  before (#45).

## 0.10.1 – 2026-11-14

### Added

- The macOS build is universal: one app for Apple silicon and Intel Macs (`GOARCH=universal scripts/build.sh`; the
  release file is now `mormicommander-X.Y.Z-macos-universal.zip`).
- Developer ID signing with the hardened runtime and notarization, ready in CI: they switch on by themselves once the
  signing secrets are added to the repository (docs/signing.md, scripts/notarize.sh). Without them the app is signed ad
  hoc as before (#44).

## 0.10.0 – 2026-11-14

Release of everything since 0.9.0 (0.9.1 to 0.9.103, listed below): Wails v3 with all dialogs as real windows, the
Home drive and mapped folders, column resizing, live Options, and about 100 board items, among them cut and paste,
undo journal, transfer error list, command palette, F4 editing inside archives, table preview, folder size treemap,
zoxide jump, Marta import and many fixes found in real-app testing.

### Fixed

- scripts/build.sh builds the Windows .exe on a Mac too (the Wails resource tool is built for the machine it runs on).

## 0.9.103 – 2026-11-14

### Fixed

- CI: gofmt clean again; the zoxide database path is worked out by the asked platform's rules (Windows test runs no longer get backslashes in Linux and macOS paths); the go.mod version check accepts Windows line endings; the Marta import tests are skipped on Windows, where Marta does not exist.

## 0.9.102 – 2026-11-14

### Added

- Control mode (testing): `pointer_drag` and `hover` play real pointer gestures inside the page, so features you can only reach with the mouse (drag and drop, hover states) can be tested. `filedrop` can take a point. The app itself works as before (#107).

## 0.9.101 – 2026-11-14

### Fixed

- Issue #91 (partly): engine review findings. Move to archive (tar and zip) no longer deletes files outside the selection when a folder link is packed: only the link itself is removed. Files that changed or kept growing while being packed are kept and named in the error. With "pack path names" off, move to archive refuses when two files would get the same name. The tar packer stores a file that grew after the scan whole. Extracting tar.gz and tar.xz now reports a bad gzip CRC or xz trailer. 7z and RAR extraction sends a different entry landing on a file written earlier in the same run through the conflict rules. Extraction applies the process umask. Find: regex matches across a chunk boundary and line break are no longer dropped, and duplicate search no longer lists a file as its own duplicate. Not done: the remaining "rest of the review findings" list, which is not in the issue or repository.

## 0.9.100 – 2026-11-14

### Added

- Issue #26 (partly): Table view in the Lister and quick view for CSV, TSV, JSONL and SQLite files. Press T or pick Options > Table to see the file as a scrollable grid with column names and types in the header. Only the first 10,000 rows are read, and rows are fetched in blocks of 200 as you scroll. SQLite files show a list of their tables to switch between. Cursor keys, Page Up/Down, Home/End, Left/Right move around; Ctrl+G goes to a row, Ctrl+C copies the row as tab-separated text, F2 reloads. New setting "Open CSV, TSV, JSONL and SQLite files as a table" (lister.openTables), off by default, so files still open as text or hex unless it is on. Not done: Parquet, SQLite tables declared WITHOUT ROWID, searching inside the grid, counting rows beyond 10,000, and a check in the real app.

## 0.9.99 – 2026-11-14

### Changed

- Issue #2 (partly): The Space key now reads the "SpaceCalcSize" option from a wincmd.ini copied in as the settings file. When the page has no stored choice of its own (set in the Options dialog), that value decides whether Space counts folder sizes; a stored choice still wins, and with neither the default is unchanged (count sizes). Not done: running the key script against the real app, checking AltGr on a real Windows or Linux machine, and confirming the exact key name Total Commander writes for this option.

## 0.9.98 – 2026-11-14

### Added

- Issue #24 (F-872, partly): Folder size treemap. A new command "Folder size treemap" (cm_ShowTreemap) in the Show menu, also in the command line and command palette, with no default key. It shows the active panel's folder as blocks sized by the space each file or folder takes, in place of the file list. Folder sizes are counted in the background by the same code Space and Alt+Shift+Enter use, so blocks grow while counting and folders still being counted are striped. Click a folder block to open its treemap, click a file block to go back to the list with the cursor on it, Backspace or the Up button goes to the parent. Arrow keys, Home and End move between blocks, Enter opens one, Esc returns to the list and stops any running count. Sizes already counted by Space or Alt+Shift+Enter are reused, and sizes counted by the treemap go into the panel's size cache. Not done: sharing the engine with mormiDiskFree (the treemap logic and the counting code are kept free of the UI, ready to move), keeping sizes cached for good, and a check in the real app (mouse, keys, both themes). Nothing existing changes behaviour.

## 0.9.97 – 2026-11-14

### Added

- Issue #32 (F-862): Duplicate actions in Find files. After a duplicate search, a new "Duplicates..." button in the results bar (greyed out unless a group has at least two usable files) opens a dialog. Pick which file to keep in each group (newest, oldest, or first path A to Z), then either mark the other copies in the list, move them to the Trash (asks first, never deletes permanently, does nothing if there is no Trash), or replace them with hard links (asks first; a copy is only replaced when both are plain local files of the same size with identical bytes). A live summary shows copies, groups and space saved, and warns when the search did not compare contents. Nothing existing changes behaviour.

## 0.9.96 – 2026-11-14

### Added

- Issue #29 (partly): Extended attributes. The Properties dialog now shows an "Extended attributes" row when a file has any, with plain words for common names. On macOS, a file with the quarantine flag also gets a "Remove quarantine flag" button, and there is a new command "Remove quarantine flag" (cm_RemoveQuarantine) in the Files menu after "Change attributes" that clears it on the selection, including folders, with progress and cancel. Before a copy to a FAT, exFAT, UDF or CD target, a note says that extended attributes worth keeping will be lost; it only warns and never stops the copy. A new internal package (internal/xattr) does the work on macOS and Linux and does nothing on Windows. Not built yet: an opt-in "Extended attributes" panel column and a warning for cloud targets (iCloud/CloudStorage), because there is no reliable file-system signal for it. Nothing existing changes behaviour.

## 0.9.95 – 2026-11-14

### Added

- Issue #27 (F-867): Frecency jump. A new command, "Frecency jump" (cm_FrecencyJump, Ctrl+Alt+J when that key is free, and in the Commands menu after "Frequently used folders"), opens a popup of folders ranked by how often and how recently they were used. Typing narrows the list the way zoxide does: the words must appear in the path in the order typed, and the last word must be in the folder's own name (it may be fuzzy, so "dwnl" finds Downloads). Enter opens the folder in the active panel, Shift+Enter in a new tab. The last row switches zoxide's folders on or off (setting frecencyZoxide, on by default). zoxide's database is only read, never written or copied. Nothing existing changes behaviour.

## 0.9.94 – 2026-11-14

### Added

- Issue #23: Git status column and branch indicator, off by default (nothing changes until you turn it on). "Show > Git status" switches it on, and "Configuration > Git status settings..." picks the parts. In a folder inside a git work tree, each name gets a status letter (M changed, A added, D deleted, R renamed, U conflict, ? untracked, ! ignored); a subfolder with changes inside gets M, or U if one of them is a conflict. Ignored files can be shown, dimmed, or treated as hidden files. The path bar shows a badge with the branch (or commit when detached), a dot when something is uncommitted, and ahead/behind counts; its tooltip says the same in words. Outside a git repository no space is used. The control command "git" reports each panel's answer for QA.

## 0.9.93 – 2026-11-14

### Added

- Issue #22 (F-855): Marta onboarding. When Marta's settings folder is found on the Mac, the first-run setup page shows a "Coming from Marta" box with an "Import from Marta" button. Pressing it copies Marta's favourite folders into a "Marta" submenu of the folder hotlist; folders already in the hotlist are skipped, so pressing it twice adds nothing, and Marta's files are only read, never changed. Nothing is imported unless you press the button. There is also a new opt-in "Marta-like" keyboard preset (the default preset is unchanged) and a docs/marta.md page that maps everyday Marta actions to mormiCommander keys.

## 0.9.92 – 2026-11-14

### Added

- Issue #19 (F-863): native macOS Quick Look. The new command cm_QuickLook (Cmd+Y on macOS, in the Show menu after Quick view, greyed out on other systems) opens the system Quick Look panel on the file or folder under the cursor. While it is open it follows the active panel's cursor, the main window keeps the keyboard, and Cmd+Y or Esc closes it. Files inside archives or on servers show a message that Quick Look works on files on this computer only. Nothing else changes.

## 0.9.91 – 2026-11-14

### Changed

- Issue #43: control mode fixes for testing. The `dom` command now takes a `limit` (default 50 as before; 0 or less returns every match) and answers `more` to say whether matches were left out; `count` is still the total. The `about` command now waits until the About window is really visible (or gone, with `close`) instead of answering after a fixed pause, and also answers `visible` and `settled`. scripts/qa_nav.py now expects the cursor on ".." when re-entering a folder (the default) and has a new suite that checks the remember-cursor setting with its own temporary settings folder. No change to normal app behaviour.

## 0.9.90 – 2026-11-14

### Changed

- Issue #121: stale documentation fixed after the Wails v3 move. docs/ENGINES.md now describes internal/trash and links its README, and its integration-status paragraph no longer mentions "ui/* feature branches" or "about 1000 commands". docs/FEATURES.md F-761 says the Linux CI build uses GTK 4 and WebKitGTK 6.0 with no build tags. README.md and docs/DOWNLOADS.md state the minimum systems (macOS 13 on Apple silicon, Windows x64 with WebView2, Linux x64 with GTK 4 and WebKitGTK 6.0). scripts/make-cask.mjs (the Homebrew cask generator) now asks for macOS 13 (Ventura) and names the GTK 4 / WebKitGTK 6.0 packages in its Linux note. No change to the app itself.

## 0.9.89 – 2026-11-14

### Added

- Issue #120: new scripts/prune_merged.sh lists (and with --apply deletes) local eng/, spec/ and ui/ branches that are fully merged into main, together with their clean worktrees under .worktrees/. It never touches unmerged branches, dirty worktrees, ow/ or wip/ branches, main or remote branches. Without --apply it only shows what it would do. No change to the app itself.

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
