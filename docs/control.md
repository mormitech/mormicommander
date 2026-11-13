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
| `GET /info` | version, pid, platform, window (`windowId` on macOS, `x`, `y`, `w`, `h`), `title`, `demo`, Go `features` |

Commands that change something wait until the app is idle (folders read, the screen painted) and answer with the same
object as `state`, so a script can check the result at once.

## Commands

### Window (done in Go)

| cmd | arguments | does |
|---|---|---|
| `info` | | see above |
| `window` | `w`, `h`, `x`, `y`, `max` | size and place of the window (centred without `x`); `max: true/false` maximises or restores |
| `screenshot` | `path` | the window as a PNG (macOS: `screencapture -l`; elsewhere `mc_ctl.py shot()` makes it) |
| `menu` | `name` | does what choosing the menu item of that cm_* command does (the native menu can't be clicked from outside) |
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
| `dialog` | | the open page dialog: `kind`, `title`, `message`, `buttons`, `fields` (by id), `error` |
| `dialog_answer` | `button`, `fields` | fills fields and presses a button by its text; `"Esc"` cancels, no button submits |
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
