# Control mode

mormiCommander can be driven from another program: the QA harness, scripted tests, demo screenshots. It is off unless
you ask for it. Commands act inside the app through the same code a key press or a click runs; nothing moves the
system's pointer or presses real keys, so it works with the window in the background.

```
mormicommander --control 0                    # 0 takes a free port; or MORMICOMMANDER_CONTROL=0 in the environment
mormicommander --control 0 --control-file /tmp/mc.json -L=/tmp/a -R=/tmp/b
mormicommander --demo                         # fictional files, settings of its own, control mode on a free port
```

On start the app writes the control file (`--control-file`, else `MORMICOMMANDER_CONTROL_FILE`, else
`mormicommander-control.json` in the temporary folder; readable by you only, removed when the app quits):

```json
{"url": "http://127.0.0.1:53211", "token": "…", "pid": 4711}
```

Every request needs `Authorization: Bearer <token>`. Requests with an `Origin` header (a web page) or for another host are
refused, and the server only listens on 127.0.0.1.

**Settings.** `MORMICOMMANDER_CONFIG_DIR=/some/folder` makes the app keep its settings (settings.json, window.json) there
instead of your real settings folder. `scripts/mc_ctl.py` gives every app it starts a new empty one, so tests never read
or change your settings. `--demo` does the same by itself.

## Requests

| | |
|---|---|
| `POST /cmd` | one command, a JSON object with `"cmd"`; the answer is JSON, `{"error": "…"}` when it failed |
| `POST /batch` | a JSON list of commands, done one after the other; stops at the first error (`?continue=1` goes on) |
| `GET /info` | version, `bundleId`, pid, platform, window (`windowId` on macOS, `x`, `y`, `w`, `h`, `maximised`), `title`, `demo`, Go `features`, `windows` (the open windows by name: `main`, `about`) |

Commands that change something wait until the app is idle (folders read, the screen painted) and answer with the same
object as `state`, so a script can check the result at once.

## Commands

### Window (done in Go)

| cmd | arguments | does |
|---|---|---|
| `info` | | see above |
| `window` | `w`, `h`, `x`, `y`, `max`, `window` | size and place of the window (centred without `x`); `max: true/false` maximises or restores; `window: "lister-2"` sizes another window and answers its `windows` entry |
| `windows` | | every window of the app: `{windows: [{name, kind ("main", "about", "lister", "dialog"), visible, focused, x, y, w, h, title, path, side, spare, ready}]}`; a hidden Lister window with `spare: true` is the one made in advance. A dialog window (`dialog-<n>`) adds `dialogKind`, `title`, `shown`, `innerW`, `innerH`, `owner` (the window whose page owns the dialog), `parent` (the window it stays above: the owner, or the dialog window below it), `openMs` and `woken` (how many times the watchdog woke a window whose page had not drawn the dialog within half a second; left out when 0); the two hidden ones made in advance have `spare: true` |
| `screenshot` | `path`, `window` | the window as a PNG (macOS: `screencapture -l`; elsewhere `mc_ctl.py shot()` makes it); `window: "lister-2"` takes another window of the app |
| `menu` | `name` | does what choosing the menu item of that cm_* command does (the native menu can't be clicked from outside) |
| `nativemenu` | | the native menu bar as built from the page's tree: `{menus: [{label, items, checked, disabled, sep}]}` (`menus`, a page command, is the tree the page sent) |
| `about` | `close` | opens the About window (a second window of the same process since Wails v3; `close: true` closes it); answers `windows` and the About window's `windowId` (macOS) |
| `answer` / `asked` | `kind`, `value` / `values` | queue answers for system dialogs (kind `folder`: the folder picker); `asked` lists the questions asked |
| `jobs` | | the file jobs (copy, move, delete) and their progress |
| `sleep` | `ms` | a pause |
| `quit` | | quits at once, without asking |

### State

| cmd | arguments | answer |
|---|---|---|
| `state` | | `active` ("left"/"right"), `left` and `right` panel states (below), `cmdline`, `focus`, `dialog`, `popup`, `message` (the last message shown), `showHidden`, `busy`, `demo`, `platform`, `title` |
| `list` | `side`, `from`, `count` | the entries of a panel in display order: `name`, `ext`, `size`, `mtime`, `dir`, `up` (".."), `link`, `hidden`, `selected`, `dirSize` |
| `commands` | | every registered command: `name`, `label`, `keys`, `enabled`, `checked` |
| `keymap` / `keyfor` | — / `keys` | the whole keymap (combo → command), or the command of one combo |
| `keyref` | | Total Commander's key reference (FEATURES.md section 8, `src/core/tckeys.js`): `id`, `key`, `command`, `how` (`press` or `map`); `scripts/qa_keys.py` checks it end to end |
| `runs` | `clear` | the commands asked for lately, newest last: `name`, `how` (`ran`, `disabled`, `notready`); `clear: true` empties the list |
| `menus` | | the menu tree as sent to the native menu (labels, commands, disabled, check marks) |
| `settings` | | all settings |
| `layout` | | boxes of the panels and the splitter, `horizontal`, `rowHeight`, `theme`, `split` |
| `dom` | `selector` | count, texts, visibility and classes of matching elements (for checks of what is drawn) |

A panel state has: `path`, `parent`, `cursor` (index, ".." is 0 when shown), `cursorName`, `count` (entries without
".."), `files`, `folders`, `totalBytes`, `selected` (names), `selectedBytes`, `sort` (`{key, desc}`), `filter`,
`loading`, `error`, `back`, `forward` (history), `status` (the status line text), `driveInfo`, `volume`, `scrollTop`,
`renderedRows`, `firstRenderedRow` (the virtual list: rows in the page).

### Acting

| cmd | arguments | does |
|---|---|---|
| `key` | `keys` (one combo or a list), `delay`, `hold` | presses keys through the app's key handler: `"Down"`, `"Ctrl+Right"`, `"Alt+F1"`, `"Shift+Cmd+."`, `"Num+"`. A character typed into a focused text field goes in. `hold: true` sends no key-up (to hold Shift) |
| `release` | | lets go of held keys |
| `type` | `text` | types characters one by one through the key handler (in a panel they go to the command line) |
| `run` | `name`, `args` | runs a registered cm_* command; `result` is what it returned |
| `cd` | `side`, `path` | the panel shows that folder (as if typed in its path bar) |
| `focus` | `side` | makes a panel the active one |
| `cursor` | `side`, `name` or `index` | puts the cursor on an entry |
| `click` | `selector` or `side` + `name`/`index`; `button` (0 left, 2 right, 3/4 back/forward), `double`, `ctrl`, `shift`, `meta`, `alt` | mouse events on an element or a row (scrolled into view first) |
| `drag` | `selector`, `dx`, `dy` | press, move, let go (the splitter) |
| `scroll` | `side`, `row` or `top` | scrolls a panel's list |
| `dialog` | | the open page dialog: `kind`, `title`, `message`, `buttons`, `fields` (by id), `error`, `window` (the dialog window that shows it: `name`, `shown`, `openMs`, `wakes` (times the watchdog woke the window before it drew the dialog), `title`, `resized`, `layers`; `{layerOf}` for a floating box drawn in the dialog window below it; `null` when it is drawn in the page) |
| `dialog_answer` | `button`, `fields` | fills fields and presses a button by its text; `"Esc"` cancels, no button submits |
| `dialog_fill` | `fields` | fills fields of the open dialog (input and change events, as typing does) without pressing a button |
| `dialog_ping` | `n` | `n` input round trips with the top dialog window (window → Go → page → Go → window): `count`, `min`, `median`, `p90`, `max` in ms |
| `popup` / `popup_choose` | — / `index` or `label`, `shift` | the open popup list (drives, history, hotlist); choose an item (`null` closes it) |
| `wait` / `idle` | `ms`, `timeout` | waits, then until the app is idle |

Feature modules add their own commands: page side with `addControlCommand(name, fn)` (src/core/control.js), Go side with
`registerControl(name, fn)` (features.go). Each feature documents its commands in its own section below.

### Dialog kinds

Page dialogs report a `kind`, so a script knows what it is answering: `message`, `input`, `form`, `info`, `job` (the
progress of a file job: button Cancel), `overwrite` (buttons Overwrite, Skip, Overwrite all, Skip all, Overwrite all
older, Rename…, Cancel), `rename-target`, `quit`.

## mc_ctl.py

`scripts/mc_ctl.py` is the client and a library:

```python
from mc_ctl import MC
mc = MC.start("build/bin/mormicommander.app", "-L=/tmp/a", "-R=/tmp/b")   # own control file and settings folder
mc.key("Down", "Enter")
assert mc.panel("left")["path"] == "/tmp/a/sub"
mc.run("cm_GoToParent")
mc.cursor("left", name="x.txt"); mc.key("Ins")
mc.menu("cm_MatchSrc")
mc.shot("/tmp/shot.png")
mc.quit()          # quits the app it started (and only that one), removes its control file and settings folder
```

From the shell, with an app already running in control mode: `scripts/mc_ctl.py state`, `scripts/mc_ctl.py key Down
Enter`, `scripts/mc_ctl.py run cm_GoToParent`, `scripts/mc_ctl.py cd left /tmp`, `scripts/mc_ctl.py '{"cmd": "list",
"side": "left"}'`.

`scripts/qa_nav.py` is the QA suite of the core (it names the FEATURES.md IDs it checks); `scripts/qa_smoke.sh` builds
everything and runs it.

## File operations (F5–F8, background transfers)

Every box of the file operations is a page dialog, so `dialog` reads it and `dialog_answer` answers it. Kinds:
`copy` / `move` (fields `target`, `filter`, `overwrite`, `verify`, `dates`, `perms`, `owner`, `attrs`, `links`,
`bigfile`, `ignoreErrors`; buttons OK, F2 Queue, Tree, Options, Cancel), `make-target` (Make it / Cancel), `mkdir`
(field `name`), `delete` / `wipe` (the confirmation), `job` (progress: Background, Pause/Continue, Cancel),
`overwrite` (Overwrite, Overwrite all, Skip, Skip all, Cancel, Overwrite all older, Rename…, Compare, and under More
options: Auto-rename, Auto-rename all, Rename target, Rename all targets, Copy all larger, Copy all smaller, Append,
Resume — only those the engine offers), `rename-target` (field `mc-input`), `overwrite-readonly`, `job-error`
(Retry, Skip, Skip all, Delete permanently, Cancel), `no-space`, `delete-folder` and `delete-readonly` (Yes, All,
Skip, Skip all, Cancel), `transfers` (the background transfer window: fields `limitOn`, `limit` in KB/s; buttons
Pause/Continue, Start now, Stop job, Clear finished, Close).

| Command | Answer |
|---|---|
| `{"cmd": "rename_editor"}` | the inline rename editor of Shift+F6: `open`, `name`, `value`, `selection` [start, end], `mode` (0 name, 1 extension, 2 all), `side`. Type into it with `key` / `type`; Enter renames, Esc cancels |
| `{"cmd": "transfers"}` | every job the page knows (Progress fields plus `title`, `background`, `sources`) |
| `{"cmd": "jobs"}` | (Go side) the engine's list of jobs |

## Archives and server folders (features/vfs.js)

| Command | Answer |
|---|---|
| `{"cmd": "vfs"}` | `left` / `right`: what each panel shows (`kind`: local, archive, ftp, sftp; `format`, `archive`, `inner`, `readOnly`, `remote`, `nested`), and `packers` (the Pack dialog's list with what each can do) |

Dialog kinds: `pack` (fields `mc-pack-target`, `mc-pack-format`, `mc-pack-level`, `mc-pack-paths`, `mc-pack-recurse`,
`mc-pack-split`, `mc-pack-size`, `mc-pack-move`, `mc-pack-separate`, `mc-pack-encrypt`, `mc-pack-cipher`,
`mc-pack-password`, `mc-pack-password2`), `unpack` (`mc-unpack-target`, `mc-unpack-mask`, `mc-unpack-paths`,
`mc-unpack-overwrite`, `mc-unpack-separate`), `password` (`mc-password`, asked by encrypted archives), `test-report`.
`cm_PackFiles`, `cm_UnpackFiles` also take their options as run arguments with `"noDialog": true` (for example
`{"cmd": "run", "name": "cm_PackFiles", "args": {"noDialog": true, "format": "tar.gz", "target": "/tmp/x/a.tgz"}}`).

## Net (FTP, SFTP, WebDAV)

| command | does |
|---|---|
| `{"cmd": "net"}` | open sessions (`url`, `kind`, `name`, `mode`), the saved connections (`sites`, `folders`, `hasMaster`) and the FTP options |
| `{"cmd": "net_job", "req": {"kind": "copy", "sources": [...], "target": "...", "overwrite": ""}}` | starts a transfer to, from or between servers (progress dialog `net-job`, question `net-overwrite` with Resume) |

Dialog kinds: `net-connect` (the connection manager: field `site` = `s:<id>` or `f:<folder>`, buttons Connect, New
connection…, New URL…, Duplicate…, New folder…, Edit…, Delete, Master password…), `net-site` (connection details),
`net-quick` (Ctrl+N, field `url`), `net-connecting`, `net-password` / `net-master` / `net-interactive` (field `value`),
`net-master-new` (`value`, `confirm`), `net-hostkey` (buttons Accept and save, Accept once, Cancel), `net-command`,
`net-reply`, `net-log`. Run commands with args: `cm_FtpNew {"url": "ftp://…"}`, `cm_FtpTransferMode {"mode": "ascii"}`,
`cm_FtpSendCommand {"command": "SITE HELP"}`, `cm_FtpDisconnect {"path": "ftp://…"}`.

## Viewer: Lister, quick view, editor (features/viewer.js)

### Lister windows (F3, feature_viewer_window.go, src/viewer-window.js)

F3 opens the Lister in a real window of its own (setting `viewerInWindow`, on by default), named `lister-<n>`. The file
manager's page can't see into another window, so these commands go through Go to the Lister window's page. `window`
names the Lister window; without it the one opened last is meant. Each answer has `window`, `title` ("Lister – <name>")
and `shown`.

| cmd | arguments | does |
|---|---|---|
| `viewer` | `window` | the window's state: `open`, `list` (the N/P names), `dir`, `side`, `lister` (as `lister` below: path, mode, rows, media, mediaLoaded, selection, match, status…), `dialog`, `popup`, `documentTitle`, `theme` |
| `viewer_key` | `window`, `keys` | key presses in that window (1-7, N, P, F7, W, Esc…); a key that closes it answers `{closed: true}` |
| `viewer_menu` | `window`, `menu`, `item` | chooses a Lister menu item by its label |
| `viewer_select` | `window`, `start`, `end` | sets the selection by bytes |
| `viewer_dialog` | `window`, `button`, `fields` | answers a dialog of that window (`lister-find`, `lister-goto`…; button `Esc` cancels) |
| `viewer_popup` | `window`, `index` / `label` | chooses an item of that window's open popup (a Lister menu) |
| `viewer_close` | `window` | closes it like its close button (answers the remaining `windows`) |

```
python3 -c 'import sys; sys.path.insert(0, "scripts"); from mc_ctl import MC
mc = MC.start("build/bin/mormicommander.app", "-L=/tmp/a", "-R=/tmp/b")
mc.cursor("left", name="notes.txt"); mc.key("F3")
print(mc.cmd("windows")["windows"]); print(mc.cmd("viewer")["lister"]["rows"][:3])
mc.cmd("screenshot", path="/tmp/lister.png", window=mc.cmd("viewer")["window"])
mc.cmd("viewer_key", keys="Esc"); mc.quit()'
```

### The Lister over the panels (`viewerInWindow: false`)

With `viewerInWindow` off (or a build without Lister windows) the Lister window is a page dialog of kind `lister`: `dialog` shows its title (`Lister – <path>`) and its menu buttons
(File, Edit, Options, Image, Encoding; pressing one opens a popup of kind `lister-menu`, or `lister-encoding`), and
`dialog_answer` with button `Esc` closes it. Keys go to it through `key` like any key press (1-7, A, S, U, V, W, N, P, F2,
F3, Shift+F3, F7, Ctrl+A, Ctrl+C, Ctrl+G, Ctrl+S, +, -, L, R, F, I, Q, Esc). Its dialogs: `lister-find` (fields
`mc-viewer-find`, `mc-viewer-find-words`, `-case`, `-hex`, `-back`, `-regex`, `-enc`; button Find), `lister-notfound`,
`lister-goto` (field `mc-input`, button Go), `lister-saveas` (field `mc-input`, button Save), `lister-replace`,
`lister-info`, `lister-error`, `lister-newfile` (Shift+F4: field `mc-input`, button Edit), `lister-options`.

| Command | Answer |
| --- | --- |
| `{"cmd": "lister"}` | `open`, `lister` (path, name, mode, asText, encoding, forcedEncoding, wrap, top, line, percent, atEnd, rows: the visible rows as text, selection, match, media, mediaShown, mediaLoaded, docShown, fit, zoom, rotate, follow, status, list: the N/P names), `quickView` (the same plus side, folder, note), `sessions` (files open in Go), `settings` |
| `{"cmd": "lister_select", "start": 0, "end": 10, "quick": false}` | sets the selection by bytes (for checks of copy and save) |
| `{"cmd": "lister_menu", "menu": "File", "item": "Follow file (auto-reload)"}` | chooses a Lister menu item by its label |

`cm_List` (and `cm_ListInternalOnly`, `cm_ListExternal`) also take `{"path": "/full/path"}` from `run`, to view one file
directly; `cm_EditNewFile` takes `{"name": "x.txt"}` and then asks nothing.

## Find files, quick search and quick filter

The Find files dialog (`cm_SearchFor`, Alt+F7) is a page dialog of kind `find`. Its tabs (General, Advanced, Plugins,
Load/Save) and actions (Start search, Stop/Cancel, New search, Go to file, View, Edit, Feed to listbox, Copy list,
Save list…, Load, Save…, Delete…) are buttons; its fields have ids: `ff-for`, `ff-regex`, `ff-in`, `ff-selected`,
`ff-results`, `ff-depth` (-1 all, 0 current folder, n levels), `ff-archives`, `ff-texton`, `ff-text`, `ff-whole`,
`ff-case`, `ff-not`, `ff-hex`, `ff-tregex`, `ff-ansi`, `ff-oem`, `ff-utf8`, `ff-utf16le`, `ff-utf16be`, `ff-ansicp`,
`ff-oemcp`, `ff-dateon`, `ff-from`, `ff-to` (YYYY-MM-DD), `ff-ageon`, `ff-agemode` (newer/older), `ff-agen`, `ff-ageunit`,
`ff-sizeon`, `ff-sizeop` (= > <), `ff-size`, `ff-sizeunit` (B KB MB GB TB), `ff-attr-r|h|x|l|a|s|c|e` ("", "+", "-"),
`ff-perms`, `ff-kind` ("", files, dirs), `ff-links`, `ff-skiphidden`, `ff-exclude`, `ff-dupon`, `ff-dupname`,
`ff-dupsize`, `ff-dupcontent`, `ff-saved`. Answering with no button starts the search. Other kinds: `find-save`
(field `ff-save-name`, `ff-save-in`), `find-delete`, `find-savelist`, `find-overwrite`.

| Command | Answer |
|---|---|
| `{"cmd": "find", "wait": true}` | `open`, `running`, `status` (the status line), `count`, `files`, `folders`, `results` (paths; duplicates as `{path, group}`), `cursor`, `selected`, `feeds` (fed result lists and their base folders). `wait` waits for the running search to end. |
| `{"cmd": "run", "name": "cm_SearchFor", "args": {"fields": {"ff-for": "*.txt"}, "start": true}}` | opens the dialog with those fields and starts at once |
| `{"cmd": "quicksearch"}` | the quick search box: `open`, `side`, `text`, `filter` (filter mode), `mode`, `filterText` per panel |

Feed to listbox shows the results in the active panel as the folder `find://results-N` (names relative to the folder
all results share); file commands work on the real files.

## Synchronize folders and Compare by content

- The Synchronize folders window (`cm_SyncDirs`, args `{left, right, options, show, profile, compare: true}`) is a
  dialog of kind `syncdirs`. Fields: `sync-left`, `sync-right`, `sync-filter`, `sync-subdirs`, `sync-content`,
  `sync-ignoredate`, `sync-asym`, `sync-emptydirs`, `sync-fat`, `sync-hour`, `sync-onlysel`, and the Show buttons
  `sync-show-leftToRight`, `-equal`, `-unequal`, `-rightToLeft`, `-duplicates`, `-singles`. Buttons: `Compare` (`Stop`
  while comparing), `Synchronize…`, `Close`. `{"cmd": "syncdirs"}` returns the rows (`path`, `category`, `action`,
  `unselected`…), the summary, the cursor and the selected rows; `{"cmd": "syncdirs", "op": "cursor", "path": "a.txt"}`
  moves the cursor to a row and focuses the list, so keys (Ctrl+Right, Delete, Enter, Shift+F10…) act on it. A row's
  arrow is `.mc-sync-row[data-path="a.txt"] .mc-sync-act` for `click`. The Synchronize dialog has kind
  `syncdirs-confirm` (fields `sync-do-lr`, `sync-do-rl`, `sync-do-del`, `sync-confirm`, `sync-trash`), the progress
  `syncdirs-progress`, the overwrite question `overwrite`, right-click menus `menu` (buttons are the menu items).
- The Compare by content window (`cm_CompareFilesByContent`, args `{left, right, internal: true}`) is a dialog of kind
  `compare`; fields `cmp-case`, `cmp-ws`, `cmp-rep`, `cmp-bin`, `cmp-edit`, `cmp-left`, `cmp-right`, `cmp-enc-left`,
  `cmp-enc-right`, and `cmp-line` while a line is edited. `{"cmd": "compare"}` returns its state (current difference,
  row, side, modified sides, encodings, the rows around the cursor).
- Compare folders (`cm_CompareDirs`, `cm_CompareDirsWithSubdirs`, `cm_CompareDirsHideEqual`) selects in both panels;
  "No differences found." is a dialog of kind `comparedirs`.

## Multi-Rename tool

The window (Ctrl+M) is a page dialog of kind `multirename`: `dialog` lists its fields (`mr-name`, `mr-ext`,
`mr-search`, `mr-replace`, `mr-case`, `mr-first`, `mr-inext`, `mr-regex`, `mr-subst`, `mr-casemode`, `mr-accents`,
`mr-cstart`, `mr-cstep`, `mr-cdigits`, `mr-creset`, `mr-dirext`) and buttons (placeholder buttons, `Start!`, `Undo`,
`Close`…); `dialog_answer` presses them (with no button it presses `Start!`). F2 opens the settings menu, whose items are
buttons of the same dialog while it is open.

| command | arguments | does |
|---|---|---|
| `multirename` | `set` {field: value}, `sort` (name/ext/size/date/path), `order` [old indices], `select` [indices], `refresh` | fills fields without pressing a button and waits for the preview; returns `open`, `rows` [{old, new, dir, isDir, problems, warnings}], `changes`, `errors`, `field`/`fieldError` (mask or regex mistake), `warnings`, `names` (loaded names, -1 none), `steps`, `status`, `presets`, `undo`, `menu` (items of an open menu) |

`MORMICOMMANDER_NO_EDITOR=1` makes "Edit names" (F3) write the temporary file without opening an editor, so a test
can change the file itself. `cm_MultiRenameFiles` with `{"param": "<UNDO>"}` undoes the last batch after asking,
`{"param": "==<UNDO>"}` without asking.

## Views (features/views.js, feature_views.go)

| command | answer |
|---|---|
| `{"cmd": "views"}` | per panel (`left`, `right`): `mode` (`brief`, `full`, `comments`, `custom`, `thumbs`, `tree`), `set` (the column set of a custom view), `auto` (chosen by a folder rule), `columns` (header titles), `rows` (the drawn rows: `cells` in column views, `left`/`top` in Brief, `thumb`/`icon`/`text` in Thumbnails), `brief` / `thumbs` geometry, `tree` (`at`, `path`, `nodes`); and `settings` (the "views" settings) |

Dialog kinds: `edit-comment` (field `comment`, OK / Cancel), `view-options` (fields `sizeFormat`, `dateFormat`,
`datePicture`, `attrFormat`, `separateExt`, `naturalSort`, `dirsByName`, `thumbSize`), `custom-columns` (list `sets`;
buttons New…, Copy…, Edit…, Delete, Up, Down, OK, Cancel), `edit-column-set` (fields `name`, `auto`, `title-N`,
`template-N`, `width-N`, `align-N`; buttons + Add column, −, …), `thumbnails-config` (field `thumbSize`; button Delete
thumbnail cache…), `clear-thumbs`. Popups: `column-sets` (Shift+F1, header right-click), `column-fields`.

## Folder tabs, hotlist and history (features/tabs.js, features/hotlist.js)

| command | fields | result |
|---|---|---|
| `tabs` | — | `left` / `right`: `{active, hidden, tabs: [{path, caption, shown, lock, lockPath, sort, view, set}]}` (lock 0 none, 1 locked, 2 folder changes allowed; `shown` has the `*` and the width limit; `view`/`set`: the tab's own view and custom column set, absent for a tab that saved none), `closed`: tabs that `cm_ReopenClosedTab` can bring back |
| `tab_menu` | `side`, `index` | opens the right-click menu of a tab (a popup of kind `tab-menu`; answer it with `popup_choose`) |
| `hotlist` | — | the hotlist tree: `[{kind: "cmd"/"menu"/"sep", title, label, key, cmd, target, items}]` |
| `frequent` | — | the frequently used folders, most used first |

Popups have the kinds `hotlist`, `hotlist-search-results`, `history`, `frequent`, `tab-menu`, `favorite-tabs`; dialogs
`hotlist-config` (fields `hl-list`, `hl-cmd`, `hl-target`), `hotlist-add`, `hotlist-add-tabs`, `hotlist-search`,
`tab-rename` (fields `tab-name`, `lock-none`, `lock-full`, `lock-dirs`), `tab-close-locked`, `tab-close-all`,
`tabs-config`, `history-settings`, `favorite-tabs-config`, `favorite-add`. The system Save and Open dialogs of tab files
and the TC import take queued `answer`s of kind `savefile` and `openfile`. Commands with arguments: `cm_LoadTabs`
`{file, append}`, `cm_SaveTabs` `{file}`, `cm_SaveFavoriteTabs` / `cm_LoadFavoriteTabs` `{name}`, `cm_OpenDirInNewTab`
`{path}`, `cm_DirectoryHotlist` `{panel, search}`, `cm_DirectoryHistory` `{panel}`. Everything is stored in
`folders.ini` in the settings folder (TC's `[DirMenu]`, `[lefttabs]`, `[righttabs]`, `[LeftHistory]`, `[RightHistory]`,
`[FrequentDirs]`, `[FavoriteTab <name>]`).

## Files-menu tools

The tools of features/tools.js (attributes, properties, occupied space, checksums, split/combine, encode/decode, links)
use page dialogs, so `dialog` / `dialog_answer` read and answer them. Dialog kinds: `attributes`, `properties`,
`tools-space` (Ctrl+L result), `crc-create`, `crc-verify`, `crc-clip`, `split`, `split-small`, `combine`, `encode`,
`decode`, `decode-warnings`, `create-link` (cm_CreateSymbolicLink / cm_CreateHardLink), `create-links` (Ctrl+Shift+F5),
`tools-progress` (a long operation: button Cancel), `tools-overwrite` (Overwrite, Cancel), `tools-error`, `tools-failed`.

In `attributes`, every three-state box `perm-<octal bit>` (perm-400 … perm-4000), `attr-<bit>` (Windows) and `hidden`
(macOS) has a twin field `<id>-state` with `on`, `off` or `mixed`; answering the box with true/false sets on/off,
answering the twin can also set `mixed` (leave alone). Dates: `set-modified` + `modified` (`2026-10-14T09:30:00`), the
same for `created` and `accessed`.

| command | answer |
|---|---|
| `{"cmd": "tools"}` | `{counting: [{side, path, names}]}`: folder sizes being counted (Space on a folder, Alt+Shift+Enter) |

Settings: `countSpace` (default true: Space on a folder counts its size, TC's CountSpace); the dialogs remember their
choices under `tools`.

## Desktop integration (features/system*.js, feature_system*.go)

| Command | What it does |
|---|---|
| `{"cmd": "fswatch"}` | Automatic refresh: `{watch: {running, panels: {left, right}, watched: [...]}}`. |
| `{"cmd": "errorlog"}` | The error log window's entries: `{errors: [{time, text, details}]}`. |
| `{"cmd": "clipboard_content"}` | What the system clipboard holds: `{content: {files, text, image}}` (reads only). |
| `{"cmd": "cut_state"}` | The files of a pending cut, drawn dimmed until pasted or cancelled with Esc: `{paths: [...]}` (empty when there is none). |
| `{"cmd": "drop", "from": "left", "names": ["a.txt"], "to": "right", "name": "sub", "op": "copy"}` | Does what dropping those files on the folder `sub` of the right panel does (no `name`: its folder; `"target": "cmdline"`: the command line). The drop dialog comes up (kind `drop`, field `drop-target`, buttons Cancel, Copy, Move, Link) unless the setting `confirmDrop` is off. Answers `{dropped: false}` where a drop is not allowed (own folder, folder into itself). |
| `{"cmd": "filedrop", "paths": ["/abs/x"], "to": "left", "name": "sub"}` | As if the files were dropped from Finder / Explorer. |
| `{"cmd": "drive_menu", "side": "left", "name": "USB stick"}` | Opens a drive button's context menu (popup kind `context-drive`). |

Popup kinds: `context-file`, `context-folder`, `context-sort`, `context-drive`, `context-path`, `open-with`. Dialog kinds:
`drop`, `error-log` (buttons Copy all, Clear, Close), `confirmations`, `paste-data`, `open-with-program`, `volume-props`,
`folder-props`, `quit-confirm`.

## Dialogs in windows of their own (core/mirror.js, feature_dialog_window.go)

With the setting `dialogsInWindows` (on by default; Options > Misc > "Open dialogs in windows of their own") every dialog
opens in a native window named `dialog-<n>`. The dialog itself is still built in the page that asked for it, hidden
there, and the window shows a live copy of it, so `dialog`, `dialog_answer` and `dialog_fill` work on it exactly as on a
dialog in the page; `dialog` tells which window shows it (`window`), and `windows` lists the dialog windows. A popup list
or a menu drawn as a floating box over a dialog window is drawn inside that window (`popup` and `popup_choose` work as
always). Kinds that always stay in the page: `lister` (the Lister over the panels, `viewerInWindow: false`); more kinds
can be kept in the page with the setting `dialogsInPage` (a list of kinds, e.g. `["options", "find"]`).
