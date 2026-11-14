# How mormiCommander is built, and how to add a feature

mormiCommander is one small native program: Go + [Wails v3](https://v3.wails.io) (pinned to v3.1.0) for the windows
and everything that touches the system, and a plain JavaScript page (Vite, no framework) that draws the panels and owns
the keyboard. It is built with `scripts/build.sh` (frontend, then `go build`, then the macOS bundle / Windows resources);
there is no Wails CLI in the build.

The whole design serves one rule: **features are modules**. A feature adds files; it does not rewrite shared ones. Many
agents can build features at the same time, and their branches merge without conflicts.

## The map

```
main.go               start-up: flags, application.New with every bound object as a v3 service — never edited for a feature
app.go                the App object: version, platform, start folders, title, quit, About window
window.go             the windows: "main" (the file manager) and "about" (a second window of the same process)
feature_viewer_window.go  Lister windows (F3): "lister-<n>", one made hidden in advance; control commands "windows", "viewer…"
feature_dialog_window.go  dialog windows: "dialog-<n>", a pool of two made hidden in advance (dialogwin_logic.go: sizes,
                      places, pool; dialogwin_<os>.go: kept above their window — macOS child window, Windows owner, Linux transient)
shell.go              helpers the features use instead of Wails: emit, clipboard, file and folder dialogs
features.go           where Go features register (bound objects, start-up/shut-down hooks, control commands)
args.go               the command line (TC's /L= /R= /P= too)
menu.go               builds the native menu from the tree the page sends
control.go            control mode server (docs/control.md)
settings.go           settings.json: one key per feature
feature_fs.go         FS: listing, volumes, free space, open with the system      (a feature, registered like any other)
feature_jobs.go       Jobs: copy / move / delete / trash jobs with progress and questions
winstate.go, configdir.go, reexec*.go, hide_*.go, windowid_*.go, version.go   window and platform plumbing
internal/vfs          file systems for the panels: Local, Mock (demo); Register(scheme, fs) for archives, FTP…
internal/disk         volumes and free space, per platform (volumes_darwin.go, _linux, _windows, _other)
internal/fileops      the job engine: queue, progress events, cancel, overwrite questions (no UI)
internal/trash        Trash / Recycle Bin per platform

frontend/index.html   the layout: #toolbar, two .panel sections, #splitter, #vbar, #cmdline, #fkeys, #flash
frontend/src/main.js  start-up of the page; imports every file of src/features/ automatically
frontend/viewer.html  a Lister window's page (a second Vite entry); src/viewer-window.js starts it with the viewer feature
frontend/dialog.html  a dialog window's page (a third entry); src/dialog-window.js draws the live copy of a dialog (below)
frontend/src/core/    the core (see the list below) — shared, change carefully, add rather than rewrite
frontend/src/features/ one file per feature: navigation.js, selection.js, sorting.js, drives.js, cmdline.js, window.js…
frontend/test/        vitest unit tests of the pure core modules

scripts/build.sh      the build: build/bin/mormicommander.app (macOS), .exe (Windows), binary (Linux)
scripts/mc_ctl.py     control-mode client = the QA harness
scripts/qa_nav.py     QA of the core through control mode;  scripts/qa_smoke.sh builds and runs everything
scripts/prune_merged.sh  removes merged eng/spec/ui branches and their clean worktrees (dry run unless --apply)
docs/FEATURES.md      the parity checklist (F-IDs);  docs/spec/ TC behaviour specs (on spec/* branches);  docs/control.md
```

## Adding a feature

Say the feature is the directory hotlist (Ctrl+D, F-651…).

### 1. The page side: `frontend/src/features/hotlist.js`

Create the file. That is all it takes to load it: `main.js` imports every file in `src/features/` with
`import.meta.glob`, so there is no list to edit.

```js
// The directory hotlist (Ctrl+D, FEATURES.md section 31): favourite folders in a popup.
import { command } from '../core/registry.js';
import { showList } from '../core/popup.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';

command({
  name: 'cm_DirectoryHotlist',          // TC's name; Ctrl+D is already mapped to it in core/keymap.js
  label: 'Folder hotlist',              // menu / command browser text; "…" when a dialog follows
  run: async (app, args) => {           // app: see "The app object"; args: {} from keys, anything from control mode
    const list = app.setting('hotlist', []);
    const r = await showList({ title: 'Hotlist', items: list.map((p) => ({ label: p, value: p })), anchor: app.cur.$.pathbar, kind: 'hotlist' });
    if (r) await app.cur.go(r.item.value);
  },
});

export function init(app) {             // optional: runs once, after the panels exist
  const star = document.createElement('button');
  star.textContent = '★';
  star.title = 'Folder hotlist (Ctrl+D)';
  star.onclick = () => app.run('cm_DirectoryHotlist');
  for (const p of app.panels) p.$.pbtns.append(star.cloneNode(true));   // .pbtns: room for buttons in the path bar
}

addControlCommand('hotlist', async () => ({ ok: true, hotlist: theApp.setting('hotlist', []) }));   // optional
```

What you get for free once `cm_DirectoryHotlist` is registered:

- its **key** works: every Total Commander default key is already in `core/keymap.js` (key → cm_* name), also for
  commands nobody wrote yet (pressing them says "… is not ready yet"). Don't add TC keys anywhere else. A command of our
  own (no TC key) may declare `keys: ['Ctrl+Alt+K']` in `command({...})`; it is used when the key is free.
- its **menu item** turns on: `core/menus.js` lays out Total Commander's menus with their cm_* names, greyed out until
  registered. A command that is not in that layout can put itself in a menu with `menu: 'Show'` (and `menuAfter:
  'cm_SrcLong'`). A toggle gives `checked: (app) => bool` and gets a check mark; `enabled: (app) => bool` greys it out.
- its **function key button** (if it is on F3–F8, also with Shift/Alt/Ctrl) is live; `short: 'Hotlist'` sets the text.
- the **command line** runs it when the user types `cm_DirectoryHotlist`, and control mode runs it with
  `{"cmd": "run", "name": "cm_DirectoryHotlist"}`.

### 2. The Go side (only if the feature needs the system): `feature_hotlist.go` + `internal/hotlist/`

Put the logic in a UI-free package under `internal/` (it cross-compiles and tests alone), and a thin bound object in
`feature_<name>.go` in the root package:

```go
package main

// The directory hotlist: reading and writing the list, importing TC's [DirMenu].
import "mormicommander/internal/hotlist"

// Hotlist is what the page calls for the hotlist (window.go.main.Hotlist).
type Hotlist struct{ app *App }

func init() {
	registerFeature("hotlist", func(a *App) []exposed { return []exposed{expose(&Hotlist{app: a})} })
	registerControl("hotlist-import", func(a *App, m map[string]any) (map[string]any, error) { ... }) // optional
	onStartup(func(a *App) { ... })                                                                  // optional
}

// Import reads a TC wincmd.ini and returns its hotlist entries.
func (h *Hotlist) Import(path string) ([]hotlist.Entry, error) { return hotlist.ImportINI(path) }
```

The page calls it as `go.Hotlist.Import(path)` (`import { go } from '../core/go.js'`), which is
`window.go.main.Hotlist.Import`: go.js calls the Go method by name through `@wailsio/runtime` (`Call.ByName
("main.Hotlist.Import", ...)`), so no binding files are generated and two features adding bound objects never conflict.
go.js is the only file of the page that knows about Wails; events (`on`), files dropped from other apps (`onFileDrop`)
and the clipboard (`setClipboardText`) go through it too. On the Go side a feature never calls Wails either: it uses
`a.emit`, `a.clipboardSetText`, `a.openFileDialog` and the other helpers of shell.go. A method whose first parameter is a
`context.Context` gets the calling window from Wails (`App.Mode` uses that); the page does not pass it. Rules: type names unique; every exported method has a one-line doc comment in
plain words; errors are plain sentences ("The folder /x is not there."), they reach the user as they are.

Platform splits go by file name: `x_windows.go`, `x_darwin.go`, `x_linux.go`, `x_unix.go` (`//go:build !windows`),
`x_other.go`. Helper programs on Windows get `hideWindow(cmd)` so no console flashes. Paths always travel as separate
arguments or environment variables, never inside script text.

A file system for the panels (archives, FTP, SFTP) implements `vfs.FS` and registers a scheme in an `init` function:
`vfs.Register("ftp", myFS)`. The panels then show `ftp://host/path` like any folder (`FS.List` picks the FS by scheme).

### 3. Tests

- **Go:** `go test ./...` — one `_test.go` per file, plain `testing`, `t.TempDir()` for every file test. Never the user's
  files, never the network beyond servers you start on 127.0.0.1.
- **Page:** pure logic (parsers, masks, formatting) goes in a module without DOM use and gets a test in
  `frontend/test/<name>.test.js` (vitest: `cd frontend && npm test`).
- **The real app:** through control mode, in fresh temporary folders. Start from `scripts/qa_nav.py` (copy a suite
  function): `MC.start(app, "-L=" + tmp_left, "-R=" + tmp_right)` gives the app its own control file and settings folder;
  `mc.key(...)`, `mc.run(...)`, `mc.state()`, `mc.dialog()`, `mc.answer(...)`, `mc.shot(...)`; always `mc.quit()` in a
  `finally`. Name the F-IDs in each check.
- In a fresh checkout or worktree, build the page once before `go vet`/`go test` (`main.go` embeds it):
  `cd frontend && npm install && npm run build`. The app itself: `scripts/build.sh` (Linux needs libgtk-4-dev and
  libwebkitgtk-6.0-dev: Wails v3 is GTK 4 with WebKitGTK 6.0 there).

### 4. Docs and status

- Add your control commands to `docs/control.md` (a row in the table, or a short section of your own at the end).
- CHANGELOG.md: a line under `## Unreleased` (feature branches never change the version; the integrator does).
- docs/FEATURES.md: set your items to `[~]`. Only QA sets `[x]`, after checking the acceptance criterion in the app.

## The core, for features

Import from `../core/…`. These are stable interfaces; extend them by adding, not by changing what exists.

### The app object (`core/app.js`, passed to every command)

| | |
|---|---|
| `app.cur`, `app.other`, `app.left`, `app.right`, `app.panel('left'/'right'/'active'/'other')` | the panels; `cur` is the active (source) one |
| `app.setActive(panel)` | makes a panel active |
| `app.run(name, args)` | runs a command; failures become a message, never an exception |
| `app.flash(text, 'err'?)` | a short message above the function keys (errors stay longer) |
| `app.on(event, fn)` / `app.emit(event, …)` | events: `panel` (folder read or re-sorted: `(panel, {moved})`), `cursor`, `selection`, `active`, `exchange`, `space-select`, `ready` |
| `app.setting(key, default)` / `app.setSetting(key, value)` | settings.json; use one top-level key per feature |
| `app.openHandlers.push(async (panel, entry, path) => handled)` | what Enter on a file does before the system opens it (archives, .url files) |
| `app.platform`, `app.sep`, `app.home`, `app.demo`, `app.showHidden`, `app.volumes` | facts |
| `app.resolvePath(text, from)`, `app.sepFor(path)`, `app.volumeFor(path)`, `app.loadVolumes()` | path helpers |

### A panel (`core/panel.js`)

| | |
|---|---|
| `p.path`, `p.parent`, `p.entries` (shown, sorted, ".." first), `p.all` (as read), `p.cur`, `p.cursor`, `p.side` | state; an entry is `{name, base, ext, size, mtime, flags, mode, dir, link, hidden, exec, broken, up, dirSize}` |
| `await p.go(path, {focus, history})` → true/false | shows a folder (false: it couldn't be read; the reason was shown) |
| `p.reload()`, `p.goBack()`, `p.goForward()` | reread, history |
| `p.setCursor(i)`, `p.moveCursor(d)`, `p.focusName(name)`, `p.pageSize()` | cursor |
| `p.targets()` | what a file command works on: the selection, or the entry under the cursor (never "..") |
| `p.selectedEntries()`, `p.isSelected(e)`, `p.select(e, on)`, `p.toggle(e)`, `p.selectWhere(fn, on)`, `p.clearSelection()`, `p.selectionChanged()` | selection (call `selectionChanged()` after `select`/`toggle`) |
| `p.saveSelection()`, `p.saved` | kept names for Num / (file-job features call `saveSelection()` before a job) |
| `p.sortBy(key, desc)`, `p.sort` | sort (`name`, `ext`, `size`, `date`, `none`) |
| `p.setFilter(fn, label)` | show only entries `fn` accepts (quick filter, Ctrl+F11, Ctrl+F12) |
| `p.dirSizes.set(name, bytes); p.rebuild()` | show a calculated folder size instead of `<DIR>` |
| `p.fullPath(name)`, `p.editPath()` | helpers |
| `p.$.pathbar`, `p.$.pbtns`, `p.$.tabbar`, `p.$.drives`, `p.$.list`, `p.$.status`, `p.el` | places in the panel for features (`.tabbar` is empty and hidden: the tabs feature fills it) |
| `Panel.STATE_KEYS` | fields swapped by Ctrl+U; a feature with more per-panel state (tabs) pushes its field names |
| `Panel.rowClassHooks.push((panel, entry) => cls)` | an extra class on a row (cut files are drawn dimmed with `cut`); `''` adds none |

### Other core modules

| module | for |
|---|---|
| `registry.js` | `command({...})`, `run(name, args)`, `hasCommand`, `getCommand`, `commandList`, `keysFor(name)`, `bindKey(combo, name)` |
| `keymap.js` | the default TC keys and the macOS extras — owned by the core: ask before changing a default key |
| `keys.js` | combos as text ("Ctrl+Shift+F5"): `comboFromEvent`, `normalizeCombo`, `label` (⌃⌥⇧⌘ on macOS) |
| `keyboard.js` | `addKeyHandler((ev, combo) => handled)`: keys before the keymap (quick search, inline rename) |
| `dialogs.js` | `messageDialog`, `inputDialog`, `formDialog`, `infoDialog`, `modal` — page dialogs, readable and answerable by control mode (give them a `kind`) |
| `popup.js` | `showList({title, items, anchor, index, kind})` — keyboard-driven popup lists |
| `jobs.js` | `runJob({kind, sources, target, rename, overwrite}, {title})` — copy/move/delete with progress and the overwrite question |
| `cmdline.js` | `cmdline.text/insert/add/clear/focus`, `addCmdlineHandler(async (text, {shift}) => handled)` |
| `go.js` | `go.<Type>.<Method>(…)`, `on(event, fn)`, `onFileDrop(fn)`, `setClipboardText(text)`, `errText(e)` |
| `format.js` | `size`, `bytes`, `date`, `attr`, `splitExt`, `plural`, `F` (entry flags) |
| `control.js` | `addControlCommand(name, async (m) => result)` |
| `settings.js`, `busy.js`, `menus.js`, `fkeys.js`, `splitter.js`, `tooltip.js`, `about.js` | core plumbing |

### Dialogs in windows of their own

Every dialog goes through `modal()` in `core/dialogs.js` (and `messageDialog`, `inputDialog`, `formDialog`,
`infoDialog`, the progress boxes of `core/jobs.js`). With the setting `dialogsInWindows` (on by default) the dialog is
still built there, in the page that asked for it, so a feature's code, its closures and control mode keep working on the
real elements; the box stays invisible in that page and a native window shows a **mirror** of it:

- `core/mirror.js` (the owning page) describes the dialog once (elements numbered, field values included) and then
  sends every change as a patch: a MutationObserver plus hooks on the `value`/`checked`/`selectedIndex` setters (code
  that fills a field makes no mutation), focus, selection and scrolling; the look of the page (theme, fonts, the colour
  style Options applies live) goes along. It replays what comes back from the window on the real elements: field values
  with input/change events, clicks and other mouse events at the same place in the dialog, keys (so `meta.onKey`, Esc
  and Enter work as in the page), focus, scrolling, drags, resizing, and closing the window (= cancel).
- `core/mirror-copy.js` (the window, started by `src/dialog-window.js`) builds the copy and applies the patches as a
  morph: elements it has are moved, never rebuilt, so focus, caret and scrolling survive. `core/mirror-events.js`
  builds what it sends back; `core/mirror-dom.js` holds what both sides share (attribute filter, message order).
- `feature_dialog_window.go` keeps two hidden windows ready, sizes a window to the dialog (big dialogs are resizable),
  centres it over the window below it or puts it where that kind was left (setting `dialogWindows`), titles it with the
  dialog's heading, keeps it above its window, stacks nested dialogs, and carries the messages ("dlg-open",
  "dlg-patch", "dlg-input", "dlg-closed", each numbered so the order is kept).

Popup lists and menus drawn as floating boxes (`position: fixed`) are not windows: over a dialog window they are drawn
inside it as a layer, otherwise in the page. Kinds that cannot be mirrored stay in the page (`PAGE_ONLY_KINDS` in
`core/mirror-dom.js`, with the reason for each; today only `lister`), and so do the kinds in the setting `dialogsInPage`.
`meta.modeless` (today only Find files, with the option FindFiles.Modeless, off by default) makes a dialog in a window
leave its page usable: no dimming, the page keeps the keys not aimed at the dialog (`keyOwner`), and the dialogs opened
from it go over its window (Go skips modeless windows in `topOf`). In the page it stays modal.
A feature needs nothing special for its dialog to open in a window; it should only not keep references to elements
of other windows, and not rely on `getBoundingClientRect` of its box being on the screen.

### Shared files

These belong to the core. A feature should not need to change them; when it really must, keep the change small and
additive (a new export, a new event), and say so in the commit message: `main.go`, `app.go`, `features.go`, `menu.go`,
`control.go`, `frontend/index.html`, `frontend/src/main.js`, `frontend/src/style.css` (tokens at the top: use them,
don't add colours), everything in `frontend/src/core/`. Styles of a feature go in a CSS file next to its module
(`import './hotlist.css'`), with classes prefixed by the feature name.

## Web view security

Every page of the app (main window, Lister windows, dialog windows) can call every bound Go method, so the rule is
simple: **nothing from a file, an archive or the network may ever run as script inside one of our pages, or inside a
frame of one.**

**The Wails iframe advisory (GHSA-2p5f-2gx8-mh8m).** The advisory is that a frame inside a Wails page can reach the
bound Go methods. Wails v3.1.0 fixes only part of it: reserved framework messages posted by a child frame are dropped
(`handleWindowMessage` checks the frame on macOS; on Linux an injected guard script does it, and by its own note a
same-origin child frame can still get around it; Windows passes the sender's address instead of a frame flag), but
bound method calls travel as `fetch` requests
to `/wails/runtime`, and the asset server does not ask which frame sent them. Any frame or embedded document that
shares the app's origin and can run script therefore still reaches Go. Frames from another origin are held back by
the web view's same-origin rules, but we do not rely on that alone.

**Why the exposure is low here (checked for issue #116):**

- No page creates an `<iframe>`, `<frame>`, `<object>` or `<portal>`, and no window loads anything but our own
  bundled pages (`/`, `/viewer.html`, `/dialog.html`; see window.go, feature_viewer_window.go,
  feature_dialog_window.go). Nothing is loaded from the network.
- The Lister's rendered HTML and Markdown views (`viewer/sanitize.js`) parse the file without running it, drop
  `script`, `iframe`, `frame`, `frameset`, `object`, `embed`, `portal`, `template` and the rest, remove every `on…`
  attribute and `srcdoc`, and keep addresses only when they stay inside the page or are inline images. The result goes
  into a shadow root of our page, not into a frame. `frontend/test/viewer-noremote.test.js` checks this.
- Media files are drawn from a `blob:` address, which has the app's own origin. Images go in `<img>` (no scripts, SVG
  included), sound and film in `<audio>`/`<video>`. The one `<embed>` is for PDF, and since issue #116 it is allowed
  only for the types in `viewer/mediael.js` (just `application/pdf`); any other type gets no element at all. Go's
  `TestMediaKindsAreSafeToEmbed` (internal/lister) checks the other end: the only "document" kind is PDF and SVG stays
  an "image".

When adding a feature that shows file content: draw it through `sanitize.js` or as plain text, never into an
`<iframe>`, an `srcdoc`, or an `<embed>`/`<object>` of a `blob:` or `data:` address holding HTML, SVG or XML.

**Upgrading Wails.** Wails stays pinned at v3.1.0 (go.mod, and `@wailsio/runtime` in frontend/package.json). The plan
is to move to a later v3 weekly release once v3 settles (the research pointed at about 3.2–3.4) and, when doing so,
check its notes for a full fix of the advisory (a frame check on `/wails/runtime` calls). An upgrade changes the
runtime under every window, so it needs its own change: update go.mod and the npm runtime together, run all checks,
build for every platform and try the app by hand on macOS, Windows and Linux before merging.

## Conventions

- **Words:** plain, short, everyday English in the UI, messages and comments ("image", not "picture"; "folder", not
  "directory" in the UI). Sentence case, "…" on items that open a dialog. Errors say what happened and what to do.
- **Comments** explain why, in full sentences, wrapped at about 120 columns; every Go file opens with a paragraph.
- **Names:** cm_* commands use Total Commander's names (docs/FEATURES.md); our own get a cm_* name too and are marked
  "ours" in FEATURES.md.
- **Safety:** deleting goes to the Trash unless the user chose permanent delete; nothing is overwritten without asking
  (or a rule the user set); tests run in temporary folders only.
- **Version:** only in wails.json (`info.productVersion`); `--version`, About and control mode read it from there, and
  scripts/build.sh puts it in the macOS Info.plist and the Windows file properties. (Wails v3 itself does not read
  wails.json; the name stayed.)
- **Commits:** clear messages ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never push, never
  publish.
