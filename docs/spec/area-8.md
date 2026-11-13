# Area 8 spec: hotlist, history, FTP connect, command line, button bar editing, drag and drop, context menus

This file describes how Total Commander 11 behaves in seven areas. The UI agents can build mormiCommander from it without opening TC. It covers dialog layouts (controls, labels, defaults, tab order), hotkeys, mouse actions, edge cases, error messages and the wincmd.ini keys TC uses to save each setting. Every section ends with the FEATURES.md IDs it backs. Some TC details only show up on a real Windows install, and not every claim could be checked against a primary source. So each claim carries a source tag, which tells the implementer how far to trust it and what to recheck on a real TC 11 before calling a feature "done".

Source tags:

- **[W]**: checked against the ghisler.ch TC wiki (pages "Directory hotlist", "Button bar" and "Wincmd.ini") while this spec was written.
- **[K]**: long-standing, well-known TC behaviour (help file, years of forum usage). Treat it as correct unless a real TC 11 says otherwise.
- **[?]**: likely but not confirmed. The exact label, default or key name must be checked on a real TC 11 before the matching FEATURES.md item is ticked.

Where TC is Windows-only (shell menus, `.exe` icons, UAC), the "mormi" notes say what we do on macOS/Linux instead. Keys follow the FEATURES.md hotkey rule: Ctrl means Cmd on macOS.

---

## 1. Directory hotlist

### 1.1 Opening the hotlist

| Trigger | Result | Src |
|---|---|---|
| Ctrl+D (`cm_DirectoryHotlist`) | Popup menu opens at the file cursor's position in the active panel | [W] |
| Click the star button (history/hotlist buttons at the right end of the current-dir bar, `HistoryHotlistButtons=1`) | Same popup, anchored under the button; it acts on *that* panel | [W] |
| Double-click the current-directory (path/title) bar of a panel | Same popup for that panel | [W] |
| Menu Commands > Directory hotlist | Same popup | [W] |
| Typing `cm_DirectoryHotlist` in the command line + Enter | Same popup | [W] |
| Button bar button with command `cm_DirectoryHotlist` | Popup appears under the button | [K] |

The hotlist is a standard Win32 popup menu, not a dialog. It closes on Esc, a click outside, or a choice.

### 1.2 Popup structure

```
&Desktop                    <- entries from [DirMenu], in file order
&Projects            >      <- submenu
---------------------       <- separator
&Add current dir            <- only if the current dir is NOT already in the list
&Remove current dir         <- only if it IS in the list (replaces "Add")
&Configure...
```

- **Add current dir** is hidden when the folder is already in the list; **Remove current dir** is hidden when it is not [W]. They never appear together.
- The bottom block is separated from the user entries by a separator [K].
- An `&` in a title makes the next letter the accelerator (underlined). Pressing it picks the entry right away when the letter is unique in that menu level; when it is not unique, repeated presses cycle and Enter confirms (standard Win32 menu behaviour) [K]. `&&` shows a literal `&` [K].
- Entries without `&`: the first letter still works as a Win32 menu mnemonic only if no other item has an explicit accelerator [K].
- Keyboard in the popup: Up/Down move, Right/Enter open a submenu, Left/Esc close a submenu, Home/End go to first/last [K].
- **Shift+Enter** on an entry, or **Ctrl+click** [?], opens the folder in a **new tab** instead of changing the current tab (TC 7.5+) [K].
- Choosing an entry while the current tab is **locked** opens a new tab (the lock rule from F-035) [K].
- Type-to-filter (TC 10+): typing more than one letter does **not** filter in TC 11; only Win32 mnemonics exist [?]. mormi: F-658 asks for type-to-filter, so we add it as our own extension (a filter box at the top of the popup appears on the first typed letter that is not an accelerator) and keep plain mnemonics working.

### 1.3 "Add current dir"

1. Choosing it opens a small input dialog [K]:
   - Title: `Total Commander`.
   - Label: `Name of new menu entry:` [?] (TC 11 wording; older versions used `Title of new menu entry:`).
   - Edit field prefilled with the **last path component** of the current folder (e.g. `Projects` for `C:\Work\Projects`), fully selected [K].
   - Buttons: `OK` (default), `Cancel`.
2. OK appends the entry at the **end of the top level** (not in a submenu), with `cmd = cd <full path>` [K].
3. **Holding Ctrl** [?] (TC help: "hold down Ctrl to also store the target dir") while choosing Add stores the other panel's path too (`path<n>=`); see F-653.
4. An empty name: OK is accepted and the entry shows the bare path [?]. mormi: if the name is empty, use the path as the title.
5. Virtual locations (archives, FTP, plugin roots) are added as written, e.g. `cd ftp://host/dir` or `cd C:\a.zip\sub` [K].

### 1.4 "Remove current dir"

Removes the **first** entry (at any depth) whose `cd` target equals the current folder (case-insensitive on Windows, ignoring a trailing backslash) and asks no confirmation [K]. mormi: match case-sensitively on Linux, case-insensitively on macOS/Windows.

### 1.5 Configure dialog (`cm_ConfigDirHotlist`, also "Configure..." in the popup)

Since TC 8 this is a page of the Options dialog: **Configuration > Options > "Directory hotlist"** [?]. When opened from the popup it shows as a stand-alone dialog with the same controls. Title: `Change directory menu` (older) / `Directory hotlist` (11) [?].

Layout, top to bottom, left to right (tab order follows this order) [K, labels ?]:

| # | Control | Label / content | Behaviour |
|---|---|---|---|
| 1 | Tree/list box | All entries; submenus shown with `-` prefix and children indented; separators shown as `--` lines | Single selection. Drag with the mouse to reorder [?] |
| 2 | Button | `Add item...` | Asks `Title of new menu entry:` (input box), inserts **after** the current item, at the current depth |
| 3 | Button | `Add submenu...` | Asks for the title, inserts a submenu header plus its end marker |
| 4 | Button | `Add separator` [?] | Inserts a separator after the current item |
| 5 | Button | `Delete item` | Removes the item; deleting a submenu asks `Delete whole submenu?` [?] Yes/No |
| 6 | Button | `Rename...` [?] | Edits the title |
| 7 | Buttons | `↑` / `↓` (move up/down) | Move the selected item; it cannot leave its submenu with these buttons. **Ctrl+Up/Down** in the list does the same [?] |
| 8 | Button | `Cut` / `Paste` [?] | Move an item (with its subtree) into another submenu |
| 9 | Button | `Sort submenu` [?] | Sorts the current level alphabetically |
| 10 | Edit + `>>` | `Command:` | `cd <path>` or any command (see 1.6). `>>` opens a folder browser and writes `cd <chosen>` |
| 11 | Edit + `>>` | `Path:` (target path) | Optional. When set, choosing the entry *also* changes the **other** panel to this path |
| 12 | Edit / checkbox | `Hotkey` [?] | TC 11 lets an entry get a global hotkey through `cm_` mapping; not a field here. mormi: leave out |
| 13 | Buttons | `OK`, `Cancel`, `Help` | OK writes [DirMenu] to wincmd.ini at once |

The controls for items 10 and 11 apply to the current list item and update live. They are disabled for separators and submenu headers.

Tool buttons in the Command field's `>>` menu (TC 9+) [?]: `Current directory` (inserts the active panel's path), `Directory of other panel`, `Browse...`. mormi: give `>>` a dropdown with exactly these three.

### 1.6 Commands an entry may contain (F-655)

| Command | Meaning | Src |
|---|---|---|
| `cd C:\path` | Change the active panel; with `path<n>` also change the other panel | [K] |
| `cd ftp://user@host/dir` | Open an FTP folder | [W] |
| `cd \\server\share` | UNC / network folder | [W] |
| `cd C:\a.zip\inner` | Open inside an archive | [K] |
| `cd \\\Plugin name\...` | File-system plugin root | [W] |
| `cm_Xxx` | Run an internal command | [W] |
| `em_Xxx` | Run a user command from usercmd.ini | [K] |
| `http://...` | Open in the default browser | [W] |
| program or document path | Run it (ShellExecute) | [W] |
| `OPENTABS <file.tab>` | Replace the panel's tabs with a saved tab set (F-045, F-664) | [K] |
| `APPENDTABS <file.tab>` | Add the tabs of a tab set | [K] |
| `cd %COMMANDER_PATH%\...`, `%USERPROFILE%` | Environment variables are expanded when the entry runs (F-849) | [K] |

mormi variables: expand `$HOME`, `${VAR}`, `%VAR%` and a leading `~` on every platform. `%COMMANDER_PATH%` maps to the mormiCommander install folder.

### 1.7 wincmd.ini: `[DirMenu]` (F-663, F-446)

Same format as the Start menu's `[user]` section [W]:

```ini
[DirMenu]
menu1=&Desktop
cmd1=cd %USERPROFILE%\Desktop
menu2=-&Projects          ; leading "-" + text = submenu start
menu3=&Alpha
cmd3=cd C:\Work\Alpha
path3=D:\Backup\Alpha     ; optional: other panel goes here
menu4=--                  ; "--" = end of submenu
menu5=-                   ; lone "-" = separator
menu6=Both panels
cmd6=cd C:\src
path6=C:\build
```

- Numbering starts at 1 and must have no gaps. TC stops reading at the first missing `menuN` [K].
- `paramN` and `iconicN` are allowed (shared parser) but only matter for program entries [W].
- A missing `--` at the end of the file implicitly closes open submenus [K].
- Importing (F-663): read `[DirMenu]` from a user-picked wincmd.ini copy. Map `cd X` to a path entry and keep `cm_*`/program commands as command entries. Convert Windows paths only when a drive letter maps to a known volume, otherwise keep them verbatim and mark them "unreachable" (shown greyed).

### 1.8 Edge cases

- Path no longer exists: TC tries to go there and shows its normal error, `Path not found!` / `The system cannot find the path specified.` [K]. The entry is **not** removed.
- Hotlist with 0 entries: the popup still shows `Add current dir` and `Configure...` [K].
- Very long lists: the Win32 menu splits into columns [K]. mormi: a scrollable popup is fine.
- `RestrictInterface` has no bit for the hotlist [W].

**FEATURES.md IDs:** F-013, F-190, F-238, F-354, F-438, F-045, F-651, F-652, F-653, F-654, F-655, F-656, F-657, F-658, F-659, F-663, F-664, F-777, F-849.

---

## 2. History (directory and frequently used)

### 2.1 Back/forward

| Key / mouse | Command | Behaviour | Src |
|---|---|---|---|
| Alt+Left | `cm_GoToPreviousDir` | Go to the previous folder in this tab's history | [K] |
| Alt+Right | `cm_GoToNextDir` | Go forward | [K] |
| Mouse button 4 / 5 | same | Back / forward | [K] |
| Button bar "Back"/"Forward" | same | Right-click on these buttons shows the history list [?] | |

- History is **per tab** and is saved with the tabs (F-047) [K].
- At the start or end of the history the command does nothing (no beep) [K].
- Folders inside archives and FTP are history entries like any other [K].

### 2.2 History list (Alt+Down, `cm_DirectoryHistory`)

- A popup menu at the cursor (or under the history arrow button `▼` next to the star) lists recent folders of **the active tab** [K].
- Order: the **oldest at top** and the current folder at the bottom, unless `ReverseHistory=1` ([Configuration], default 0) puts the newest at top [W].
- The current folder is marked (radio/check bullet) [K].
- Accelerators: entries are numbered `&1`, `&2`, ... `&9`, `&0`, then `&A`... [?].
- Picking an entry: with `HistoryAppend=1` (default) the folder is **appended** to the end of the history. With `0` the history pointer just moves to that entry, so Alt+Right still works [W].
- Shift+Enter: open in a new tab (F-777) [K].
- Size: up to 30 entries per tab [?] (old TC used 20). mormi: default 30, setting `HistoryLength` (ours), range 5..200 (F-662).
- Duplicates: when the same folder is visited again, the older entry is removed so each path appears once [?].
- Clearing (F-662): TC has no "Clear" menu item. History is cleared by deleting `[LeftHistory]`/`[RightHistory]`. mormi: add `Clear history` at the bottom of the popup, after a separator.

### 2.3 Frequently used folders (F-661)

- Command `cm_DirectoryHistory` with a parameter, shown as "Frequently used directories" (TC 9.5+). Default hotkey: **Alt+Shift+Down** [?].
- The popup lists the most-visited folders (by visit count, highest first), up to 20 [?], across **all** tabs of that side.
- Shift+Enter: open in a new tab.

### 2.4 Path bar dropdown (F-056, F-013)

The `▼` button at the right of the current-directory bar opens the same list as Alt+Down for *that* panel. The `*` button opens the hotlist [K] (`HistoryHotlistButtons=1` in [Layout] [W]).

### 2.5 wincmd.ini keys

| Section / key | Meaning | Default | Src |
|---|---|---|---|
| `[LeftHistory]` / `[RightHistory]` `0=`, `1=`, ... | Directory history of the active tab per side, newest = `0` | — | [K] |
| `[Configuration] HistoryAppend` | See 2.2 | 1 | [W] |
| `[Configuration] ReverseHistory` | See 2.2 | 0 | [W] |
| `[Layout] HistoryHotlistButtons` | Show `▼`/`*` buttons | 1 | [W] |
| Tab files `.tab` / `[lefttabs]` | `N_path`, `N_options`, plus per-tab history (TC 9+) | — | [?] |

**FEATURES.md IDs:** F-013, F-047, F-054, F-055, F-056, F-073, F-266, F-267, F-660, F-661, F-662, F-777.

---

## 3. FTP connect dialog

TC's built-in client speaks FTP and FTPS. SFTP in TC needs the SFTP plugin; mormi builds it in (F-632). Connection data lives in **wcx_ftp.ini** (path from `[Configuration] FtpIniName` [?], or the `/F=` start parameter [W]).

### 3.1 Commands

| Key | Command | Result | Src |
|---|---|---|---|
| Ctrl+F | `cm_FtpConnect` | `Connect to ftp server` dialog (connection manager) | [K] |
| Ctrl+N | `cm_FtpNew` | `FTP: Connect to server` quick dialog (URL) [?] | [K] |
| Ctrl+Shift+F | `cm_FtpDisconnect` | Close the connection in the active panel; the panel returns to the last local folder | [K] |
| Ctrl+Shift+M | `cm_FtpSwitchMode`? (transfer mode) | Cycle Binary → Text → Auto [?] | [K] |
| — | `cm_FtpHiddenFiles` | Toggle "Show hidden files" (LIST -a) | [K] |
| — | `cm_FtpAbort` | Abort the current transfer | [K] |
| — | `cm_FtpResumeDownload` | Resume a broken download | [K] |
| — | `cm_FtpSelectTransferMode` | Choose a mode | [K] |
| — | `cm_FtpAddToList` / `cm_FtpDownloadList` | Download queue | [K] |
| — | `cm_FtpSendCommand` [?] | Raw command to the server | [?] |

`RestrictInterface` bit **64** disables Ctrl+N and Ctrl+F [W].

### 3.2 Connection manager: `Connect to ftp server` (Ctrl+F)

```
+-----------------------------------------------------------+
| Connect to ftp server                                  [x]|
| +-------------------------------------+  [ Connect      ] |
| | ▸ Folder A                          |  [ New connection...] |
| |   myserver                          |  [ New URL...   ] |
| |   ftp.example.com                   |  [ Duplicate entry...] |
| |   ...                               |  [ New folder   ] |
| |                                     |  [ Edit...      ] |
| |                                     |  [ Delete       ] |
| +-------------------------------------+  [ Encrypt      ] |
|                                          [ Cancel       ] |
|                                          [ Help         ] |
+-----------------------------------------------------------+
```

- Labels as shown [K]; `Encrypt` (master password, TC 7+) [K]; `New URL...` (TC 7+) [?].
- Default button: **Connect**. Double-click on an entry connects. Enter connects. Esc cancels [K].
- The list is sorted in **user order**, not alphabetically, and entries can be dragged into folders [?]. Folders expand on double-click.
- The last used entry is preselected (`[General] DefaultConnection?` [?]); TC saves it as the `listname` key in wincmd.ini [W].
- `Delete` asks `Really delete the connection "<name>"?` Yes/No [?]. Deleting a non-empty folder asks once for the whole folder.
- `Duplicate entry...` asks for the new name (prefilled `<name> (2)` [?]) and then opens the details dialog.
- Keyboard: Ins = New connection, Del = Delete, F2 = Edit (rename?) [?].
- With master password protection on, the first connect in a session asks `Master password:` (input, masked) [K]. Wrong password: `Wrong master password!` [?] retry; Cancel aborts the connect.

### 3.3 Connection details dialog (`FTP: connection details`)

Tab order = row order. Defaults are for a new entry [K labels; exact wording ?]:

| # | Control | Label | Default | Notes |
|---|---|---|---|---|
| 1 | Edit | `Session:` | empty | Name shown in the list. Required. Error if empty: `Please enter a session name!` [?] |
| 2 | Edit | `Host name[:Port]:` | empty | `host`, `host:2121`, `[::1]:21`. A pasted `ftp://user:pw@host:port/dir` is split into the fields on leaving the field [?] |
| 3 | Checkbox | `SSL/TLS` | off | Explicit TLS (AUTH TLS). `ftps://` in the host field means implicit TLS on port 990 [?] |
| 4 | Checkbox | `Anonymous connection` | off | Sets `User name` to `anonymous` and the password to the e-mail from the FTP options page (`anonymous@` default) |
| 5 | Edit | `User name:` | empty | |
| 6 | Edit (masked) | `Password:` | empty | Below it a red note: `Warning: Storing the password is insecure!` [K]. An empty password means TC asks at connect time |
| 7 | Checkbox | `Use master password to protect the password` | on if a master password exists | |
| 8 | Edit | `Remote dir:` | empty | Folder to open after login |
| 9 | Edit + `>>` | `Local dir:` | empty | Other panel changes to this folder on connect |
| 10 | Checkbox + edit | `Send commands:` | off | Raw commands sent after login, separated by `;`? [?] (e.g. `SITE UMASK 022`) |
| 11 | Checkbox | `Use passive mode for transfers (like a WWW browser)` | **on** for new entries [K] | TC FAQ: the last used mode becomes the default |
| 12 | Checkbox + combo + `Define new...` | `Use firewall (proxy server)` | off | Proxy type from the proxy list (F-637, F-437) |
| 13 | Combo | `Server type:` | `Auto detect` | Picks the LIST parser (Unix, Windows/IIS, VMS, ...) [?] |
| 14 | Combo | `Character set:` / `Codepage:` | `Auto (UTF-8 if supported)` | F-640 [?] |
| 15 | Edit | `Time zone offset` (hours) | 0 | F-639 [?] |
| 16 | Checkbox | `Scan for hidden files (ls -la)` / `Show hidden files` | off | F-380 [?] |
| 17 | Buttons | `OK` `Cancel` `Help` | | OK saves to wcx_ftp.ini at once |

### 3.4 Quick connect (Ctrl+N)

- Title `FTP: Connect to server` [?]. Label `Connect to:` with a combo of recent URLs (history, ~20 entries) [K].
- Accepted forms: `host`, `host:port`, `ftp://[user[:pass]@]host[:port][/path]`, `ftps://...`, `http(s)://...` for an HTTP download (F-644) [K].
- Checkboxes [K/?]: `Anonymous connection`, `SSL/TLS`, `Use passive mode`, `Use firewall (proxy server)`.
- After a successful connect, TC offers to save the connection: `Save connection details?` [?]. mormi: no prompt; add a `Save as...` item to the FTP status bar context menu.
- The same URL typed in the command line (`cd ftp://...`) or the path bar connects too (F-636).

### 3.5 While connected

- A thin **FTP bar** shows above the panel [K]. From left: a transfer-mode combo `Binary (archives, doc etc)` / `Text (plain text, html etc)` / `Auto (based on extension)` [K wording ~], a log/`Disconnect` button [K], and `Abort` (enabled only during a transfer) [K].
- Clicking the log area opens the **log window** with all commands and replies (F-391) [K].
- The drive combobox shows the connection name. The panel path reads `ftp://name/dir` (F-071).
- Connection lost: `Connection to server lost! Reconnect?` [?] Yes/No.
- Login failed: a message box with the server's reply line, e.g. `530 Login incorrect.` [K], and Retry/Cancel [?].
- Timeouts and retries come from the Configuration > FTP page (F-424): `Retry count`, `Wait time between retries (s)`, `Timeout (s)`, `Send keepalive (NOOP) every n s` (F-642) [?].

### 3.6 wcx_ftp.ini

```ini
[General]
ConnectionCount=?            ; [?] newer TC just lists sections
[connections]
1=myserver
2=ftp.example.com
[myserver]
host=ftp.example.com:21
username=peter
password=<encrypted hex>     ; TC obfuscates; with master password "!" prefix [?]
directory=/pub
localdir=C:\Download
pasvmode=1                   ; 0 = active (PORT)  [W-search: ghisler FAQ]
ssl=0
firewall=0
anonymous=0
sendcommands=
servertype=0
```

mormi: never store passwords in plain text. Use the OS keychain or the master password (F-634, F-635). Import from wcx_ftp.ini takes everything except the password, which the user enters again.

**FEATURES.md IDs:** F-071, F-191, F-192, F-193, F-240, F-241, F-246, F-377, F-378, F-379, F-380, F-381, F-387, F-389, F-390, F-391, F-424, F-437, F-445, F-629, F-630, F-631, F-632, F-634, F-635, F-636, F-637, F-639, F-640, F-642, F-644, F-650, F-802, F-816, F-817.

---

## 4. Command line

### 4.1 Layout

- A single-line combobox under the panels and above the function key bar [K]. On the left a **prompt label** shows the active panel's path followed by `>` (e.g. `C:\Work>`). It is shortened in the middle with `...` when too long [K].
- Shown or hidden with `[Layout] CmdLine=1` [W] or `cm_VisCmdLine` (F-767).
- `RestrictInterface` bit 2 disables it [W].
- For virtual folders (archive, FTP) the prompt shows the virtual path, but programs run in the archive's or the last local folder [?].

### 4.2 Getting focus / typing

| Action | Result | Src |
|---|---|---|
| Typing a printable character in the panel | Goes to the command line when quick search is on Ctrl+Alt+letter or Alt+letter (Configuration > Quick search) | [K] |
| Click into the field | Focus | [K] |
| `cm_FocusCmdLine` | Focus, caret at the end | [K] |
| Up / Down while the command line has text and focus | `CmdUpDown=0` (default): move the panel cursor, focus stays visually in the command line. `CmdUpDown=1`: leave the command line [W] | |
| Left/Right/Home/End | Edit the command line **only when it has text**; when empty, they go to the panel [K] | |
| Esc | Clear the command line (F-273) | [K] |
| Tab | Switch panels; the command line text stays [K] | |

### 4.3 Executing

| Key | Behaviour | Src |
|---|---|---|
| Enter (text present) | Runs the text with the active panel's folder as working directory (F-812). Console programs run in a new console window that closes when done | [K] |
| Shift+Enter | Runs it in a console window that **stays open** (TC uses `noclose.exe` / `cmd /k`) (F-294) | [K] |
| Ctrl+Shift+Enter? | TC: run as administrator (TC 8+ in the command line) [?] — mormi N/A (F-303) | |
| Enter (empty) | Normal panel Enter (open the file under the cursor) | [K] |

Parsing rules, applied in this order [K unless marked]:

1. `cd` with no argument shows the current dir? [?] (Windows cmd behaviour). In TC, `cd` alone does nothing.
2. `cd <path>`: change the **active panel**. Relative paths resolve against the panel folder. `cd ..`, `cd \` (root), `cd C:`, `cd -`? [?] (no), `cd ftp://...`, `cd \\server\share`, `cd archive.zip` (opens the archive) all work. A path that does not exist shows `Path not found!`/`Directory not found` [?] and the text **stays** in the command line.
3. Just a drive, e.g. `d:`: switch to that drive [K].
4. `cm_<name>` or `em_<name>` (case-insensitive): run the internal or user command (F-301) [W].
5. An existing directory typed alone (`C:\Temp`, `..`)? [?]. TC runs it as a program, which opens Explorer. mormi: change into it, the friendlier behaviour.
6. Anything else: pass it to the shell (`ShellExecute`, so documents open in their app) [K]. Environment variables `%VAR%` are expanded before passing (F-302) [K]. mormi: run through the user's `$SHELL -c` on Unix, which expands `$VAR` itself, and expand `%VAR%` ourselves.
7. On success the text is added to the history (newest first, duplicates moved to the top) and the field is cleared [K].
8. Program not found: TC shows the Windows error `Windows cannot find '<name>'. Make sure you typed the name correctly, and then try again.` [K]. mormi: `Command not found: <name>`, and the text stays.

### 4.4 History (F-167, F-188, F-295)

| Key | Behaviour | Src |
|---|---|---|
| Ctrl+E | Previous command into the field (cycles toward older) | [K] |
| Ctrl+X (command line has focus/text) | Next (newer) command; in an empty list, Ctrl+X = cut files | [K] |
| Alt+F8 (`cm_ShowCmdLineHistory`) | Drop the combobox list open | [K] |
| Ctrl+Down | Same as Alt+F8 | [K] |
| In the open list: Del / Shift+Del | Remove that history entry (TC 8+) [?] | |

- Storage: `[Command line history]` section with entries `0=newest`, `1=...` [W]. It is saved on exit when `[Configuration] SaveCommands=1` (default) [W].
- Size: 20? [?]. mormi: 50, setting `CmdLineHistoryLength`.

### 4.5 Inserting names (F-185, F-186, F-187, F-262, F-298, F-299, F-813)

| Key | Command | Inserts | Src |
|---|---|---|---|
| Ctrl+Enter | `cm_AddFilenameToCmdline` | Name under the cursor + a trailing space; quoted `"..."` if it contains spaces | [K] |
| Ctrl+Shift+Enter | `cm_AddPathAndNameToCmdline` | Full path + name, quoted as needed | [K] |
| Ctrl+P | `cm_AddPathToCmdline` | Current panel path with a trailing separator | [K] |
| Repeated Ctrl+Enter | Appends, each name separated by a space (F-813) | | [K] |
| Drop a file on the command line | Inserts the full path, quoted as needed (F-699) | | [K] |

When the cursor is on `..`, Ctrl+Enter inserts `..`? [?]. mormi: insert nothing on `..`.

### 4.6 Autocomplete (F-296)

- TC 8+: `Configuration > Options > Operation`? [?]. The option `Autocomplete paths in command line / dialogs` exists in `[Configuration] AutoComplete=` with bit flags (1 = dirs, 2 = files, 4 = also in dialogs?) [?].
- Behaviour: after typing a path fragment, a dropdown of matching entries appears. Up/Down select, Tab/Enter accept, Esc closes.
- mormi: implement as described, dirs and files on, toggle in settings.

### 4.7 Num-key substitutes

Num+, Num- and Num* act on selection only while the command line is **empty**. Once it has text, they type `+`, `-`, `*` (F-288) [K].

**FEATURES.md IDs:** F-062, F-063, F-167, F-185, F-186, F-187, F-188, F-226, F-239, F-256, F-262, F-273, F-275, F-288, F-292, F-293, F-294, F-295, F-296, F-297, F-298, F-299, F-300, F-301, F-302, F-303, F-372, F-636, F-699, F-767, F-804, F-812, F-813.

---

## 5. Button bar editing

### 5.1 Ways to edit

| Action | Result | Src |
|---|---|---|
| Configuration > Button bar... (`cm_ButtonConfig`) | `Change button bar` dialog | [W] |
| Right-click a button | Local menu: `Change...`, `Delete`, `Cut`, `Copy`, `Paste` [K], plus on sub-bars `Change subbar...`? [?] | [W] |
| Right-click an empty bar area | `Change...` (opens the dialog), `Paste` [K] | |
| **Shift**+drag a file/program from a panel onto the bar | Adds a button for it at the drop position | [W] |
| Drag a button with the mouse (Shift held? [?]) | Reorders | [W] |
| Shift+Left/Right in the dialog's icon row | Moves the selected button | [W] |
| Plain drag of files **onto an existing button** | Runs that button's program with the dropped files as parameters (F-307) | [K] |
| Alt+Shift+F11 | Keyboard focus on the bar (F-768) | [K] |

`RestrictInterface` bit 4 disables changes [W].

### 5.2 `Change button bar` dialog

Top to bottom, in tab order [W labels; layout K]:

| # | Control | Label | Behaviour |
|---|---|---|---|
| 1 | Edit + `>>` | `Button bar:` | Path of the .bar file being edited. `>>` loads another bar. The name can only be changed for the default bar [W] |
| 2 | Spin edit | `Size:` | Button size in pixels; 32 = normal. Default stored in `Buttonheight` (TC default 29? the wiki example shows 29) [W] |
| 3 | Checkbox | `Flat icons` | No 3D frame [W] |
| 4 | Checkbox | `Small icons` | 16×16 icons inside the button [W] |
| 5 | Icon strip | (preview of all buttons) | Click selects; Shift+arrows move [W] |
| 6 | Button | `Add` | Inserts an empty button **after** the selected one [W] |
| 7 | Button | `Delete` | Removes the selected button, no confirmation [W] |
| 8 | Edit + `>>` + `Add subbar >>` | `Command:` | Program, `cm_*`, `em_*`, `cd <path>`, `ftp://`, `ftpopen <session>`, `zipfromlist`, `OPENTABS`, `APPENDTABS` [W]. `>>` = browse for a file or pick from the command list (`cm_` browser dialog with category tree and filter) [K] |
| 9 | Edit + `>>` | `Parameters:` | See 5.3 [W] |
| 10 | Edit + `>>` | `Start path:` | Working dir. For a `cd` command: path for the **other** panel [W] |
| 11 | Edit + `>>` | `Icon file:` | .exe/.dll/.ico; internal commands default to `wcmicons.dll` [W] |
| 12 | Icon grid | `Icon:` | Icons in that file; click selects index [W] |
| 13 | Edit | `Tooltip:` | Hover text, shown after `ToolTipDelay` ms (default 500 [W]; the wiki's button page says "over one second") |
| 14 | Radio/checkboxes | `Run minimized` / `Run maximized` | Window state [W] |
| 15 | Checkbox | `Run as administrator` | TC 9+ [?]. mormi: hide |
| 16 | Buttons | `OK` `Cancel` `Help` | OK writes the .bar file |

Selecting a separator disables fields 8–15. A separator is added with `Add` and then an empty Command and Icon file [K].

### 5.3 Parameter placeholders (F-310, F-814)

Applies to buttons, the start menu and user commands [W/K]:

| Placeholder | Expands to |
|---|---|
| `?` (first char) | Show the expanded parameters in an edit box before running; Cancel runs nothing (F-814) |
| `%P` | Source path (active panel), with trailing separator |
| `%N` | Name under the cursor (source) |
| `%T` | Target path (other panel), with trailing separator |
| `%M` | Name under the cursor in the target panel |
| `%O` | Current name without extension |
| `%E` | Extension without the dot |
| `%S` | Selected names in source, space-separated, quoted as needed (`%S`), `%R` = same for target? [?] |
| `%L` / `%l` | Temp list file with full paths / names only (ANSI) |
| `%F` / `%f` | Temp list file, names only / full? [K: F = names, f = names short?] [?] |
| `%D` / `%d` | List file with short (8.3) names [W]. mormi: same as `%L` |
| `%UL`, `%WL` | List file in UTF-8 / UTF-16 (TC 7.5+) [K] |
| `%Z` | UTF-8 list (FEATURES wording) [?] |
| `%%` | A literal `%` [W] |
| `%X` prefix | Lets `%N` etc. expand without quotes [?] |
| `%Q` | Turn off auto quoting [?] |
| `%$VAR%` | Environment variable |

List files are deleted when the started program exits, or by TC at the next start [K].

### 5.4 .bar file format (F-309)

```ini
[Buttonbar]
Buttoncount=3
button1=%COMMANDER_PATH%\WCMICONS.DLL,3
cmd1=cm_RereadSource
menu1=Reread source window
button2=                      ; separator: empty button and cmd
cmd2=
button3=C:\Tools\notepad++.exe
cmd3=C:\Tools\notepad++.exe
param3=%P%N
path3=%P
iconic3=1                     ; 1 = minimized, -1 = maximized, 0 = normal
menu3=Edit with Notepad++
```

- A sub-bar is a button whose `cmd` is the path of another `.bar` file. Clicking it shows that bar as a dropdown under the button. Circular references are allowed [W]. Do not edit a bar while its dropdown is open [W].
- wincmd.ini `[Buttonbar]`: `Buttonbar=<path of default .bar>`, `Buttonheight=`, `FlatIcons=1`, `SmallIcons=1` [W]. `[Layout] ButtonBar=1` (visible), `InterfaceFlat=0` [W].
- Vertical bar (TC 9+): `[Layout] ButtonBarVertical=1` and `[ButtonbarVertical] Buttonbar=<path>` [?] (F-314, F-024).

**FEATURES.md IDs:** F-024, F-304, F-305, F-306, F-307, F-308, F-309, F-310, F-311, F-312, F-313, F-314, F-441, F-698, F-768, F-799, F-814.

---

## 6. Drag and drop rules

### 6.1 Starting a drag

- Left button down on a file, then move more than the system drag threshold (Windows: 4 px). TC starts the drag only when the press was on a file name, not on empty space [K]. With "Windows-style" selection, empty space starts a rubber band? TC: no rubber band (F-700 is ours).
- With `UseRightButton=1` (TC classic default [W]), **right-drag selects** files instead of dragging. A right-drag drop menu is available only in left-button (Windows) selection mode [K].
- What gets dragged: the selected files if the pressed file is selected, otherwise just the pressed file [K].
- Esc during the drag cancels it [K].

### 6.2 Modifiers at drop time (between panels / onto folders)

| Modifier when releasing | Operation | Src |
|---|---|---|
| none | **Copy**: the Copy dialog (F5) opens with the target prefilled, unless drag confirmations are off | [K] |
| Shift | **Move** (F6 dialog) | [K] |
| Ctrl+Shift or Alt | Create **link/shortcut**: `.lnk` (Windows), symlink choice (TC 8+) [?] | [K] |
| Right button drag | Popup: `Copy`, `Move`, `Create link(s) here`? , `Cancel` [?] | [K] |
| macOS (mormi) | Also Finder rules: Option = copy, Cmd = move, Cmd+Option = link (F-851) | — |

- Confirmation: `[Confirmation] MouseActions=0` (default) means **no** extra "Really drop?" question, but the copy/move dialog still shows. `1` adds `Do you really want to copy/move the dropped files?` [W/?].
- Option "Drag&drop: show copy/move dialog" (Configuration > Operation?) [?]. mormi: one setting, `confirmDrop`, default on (F-823).
- The cursor shows the OS feedback (`+` badge for copy, arrow for link) [K].

### 6.3 Drop targets

| Drop on | Result | Src |
|---|---|---|
| Empty area / any non-folder file of the other panel | Into that panel's **current folder** | [K] |
| A folder in either panel | Into that folder (it is highlighted while hovering) (F-696) | [K] |
| `..` | Into the parent folder | [K] |
| Same panel, empty area / onto a file | Nothing (no-op) | [K] |
| Same panel, onto a subfolder | Copy (Shift = move) into it | [K] |
| Onto itself (folder into itself) | Error `Cannot copy a folder into itself!` / `You cannot copy a file to itself!` [?] | |
| An archive file (.zip etc.) | Opens the **Pack** dialog to add to it [K] | |
| A program/.exe in the list | Runs it with the dropped files as parameters [K] | |
| A drive button | Into the current folder **of that drive** (TC remembers one per drive) (F-697) | [K] |
| A folder tab | Into that tab's folder (F-040) | [K] |
| A breadcrumb segment of the path bar | Into that folder (TC 10+) [?] (F-697) | |
| A button bar button | Runs its program with the files (F-307); Shift = add a button (F-306) | [W/K] |
| The command line | Inserts the full path(s) (F-699) | [K] |
| Panel of FTP / archive | Upload / pack through the VFS (F-484) | [K] |
| Recycle bin / desktop / Explorer (outside TC) | OLE drag with CF_HDROP; the receiver decides (F-695) | [K] |
| From Explorer into a panel | Copy into the target (Shift = move), same dialog rules [K] | |

### 6.4 During the drag

- Auto-scroll: hovering in the top or bottom ~1 row of a list scrolls it (F-701) [K].
- Hovering over a folder tab for ~1 s activates that tab [?].
- Spring-loaded folders: TC has none [?]. mormi option (F-702), default off, 1 s delay.
- Hovering over the other panel's drive button does not switch drives [K].

### 6.5 Tabs and the splitter

- Dragging a tab reorders it. Dropping it on the other panel's tab bar moves the tab; Ctrl copies it [?] (F-039).
- Dragging the splitter resizes the panels; double-click gives 50/50 (F-008).

**FEATURES.md IDs:** F-008, F-039, F-040, F-135, F-136, F-306, F-307, F-480, F-481, F-484, F-694, F-695, F-696, F-697, F-698, F-699, F-700, F-701, F-702, F-719, F-823, F-851.

---

## 7. Context menus

### 7.1 File context menu (F-172, F-218, F-250, F-703, F-708, F-811)

| Trigger | Src |
|---|---|
| Right-click a file (left-button selection mode) | [K] |
| Right button press and **hold** ~0.5 s in right-button selection mode (`UseRightButton=1`); a short right click only selects | [K] |
| Shift+F10 (`cm_ContextMenu`) or the Menu/Apps key: menu opens at the cursor row | [K] |
| `cm_ContextMenu` from a button | [K] |

- Windows TC shows the **Explorer shell menu** for the selection (or the item under the cursor when nothing is selected) [K].
- TC adds, depending on version and settings, `View` / `Edit` items? [?]. In TC 11 the menu is pure shell for local files [K].
- In archives, FTP and plugins TC shows its **own** menu: `View`, `Edit`, `Copy`, `Move`, `Delete`, `Properties` [K].
- mormi menu (macOS/Linux, and the fallback everywhere): `Open`, `Open with ▸` (system apps for the type, then `Other...`), `View (F3)`, `Edit (F4)`, sep, `Cut`, `Copy`, `Paste`, sep, `Copy path`, `Copy name`, sep, `Rename (Shift+F6)`, `Delete (F8)`, sep, `Pack... (Alt+F5)`, `Encode/Decode ▸`, `Split...` (F-763), sep, `Reveal in Finder / Show in file manager`, `Properties (Alt+Enter)`. Items that do not apply are disabled, not hidden.
- Multiple selection: the commands apply to all selected; `Open with` lists only apps common to all types.
- `..` under the cursor: the menu for the current folder [K].

### 7.2 Background (empty area) menu (F-704)

Right-click below the last file (left-button mode), or Shift+F10 on an empty folder [K]. TC shows the shell menu of the current folder (`View`, `Sort by`, `Paste`, `New ▸`, `Properties`) [K]. mormi: `New folder (F7)`, `New file (Shift+F4)`, `Paste`, sep, `Refresh (Ctrl+R)`, `Sort by ▸`, `View ▸`, sep, `Open terminal here`, `Properties of folder`.

### 7.3 Drive button / drive combobox menu (F-059, F-705, F-710)

Right-click a drive button: the shell menu of that drive root (Eject, Format, Properties) [K]. `RestrictInterface` bit 8 disables it [W] (bit 16 disables the file menu [W]). mormi: `Open`, `Open in new tab`, `Eject / Unmount` (removable only), `Properties`.

### 7.4 Tab menu (F-034, F-706)

Right-click a folder tab [K]: `Close tab`, `Close all tabs`? (`Close all but this`?), `Lock tab`, `Lock tab with dir changes allowed`, `Rename/Lock tab...`, `Duplicate tab`?, `New tab`, `Tab to other side`? [?] (exact TC 11 list to recheck), sep, `Save tabs...`, `Load tabs...`, `Favorite tabs ▸` (F-775).

### 7.5 Column header menu (F-707, F-671)

Right-click the sort header: the list of custom column sets for this folder type, `Configure custom columns...`, and the standard views [K].

### 7.6 Button bar menu (F-313)

See 5.1.

### 7.7 Path bar menu

Right-click the current-directory bar: `Copy path to clipboard`? [?], `Edit path`. mormi: `Copy path`, `Edit path (Cmd+L)`, `Open in new tab`.

### 7.8 Splitter menu (F-008)

Right-click the splitter between the panels: `50/50`, `60/40`, `70/30`, ..., `Other ratio...` [K].

### 7.9 Keyboard in all menus

Up/Down, Enter, Esc, Right/Left for submenus, accelerators via `&` [K]. The menu opens at the mouse position for a mouse trigger and at the cursor row (left side of the name column) for a keyboard trigger [K].

**FEATURES.md IDs:** F-008, F-034, F-059, F-172, F-200, F-218, F-250, F-313, F-341, F-671, F-703, F-704, F-705, F-706, F-707, F-708, F-710, F-763, F-811.

---

## 8. Summary of wincmd.ini keys in this area

| Section | Key | Default | Purpose | Src |
|---|---|---|---|---|
| `[DirMenu]` | `menuN`, `cmdN`, `pathN`, `paramN`, `iconicN` | — | Hotlist | [W] |
| `[LeftHistory]` / `[RightHistory]` | `0..n` | — | Directory history | [K] |
| `[Command line history]` | `0..n` | — | Command history | [W] |
| `[Configuration]` | `HistoryAppend` | 1 | History pick appends | [W] |
| `[Configuration]` | `ReverseHistory` | 0 | Newest at top | [W] |
| `[Configuration]` | `CmdUpDown` | 0 | Up/Down from the command line | [W] |
| `[Configuration]` | `SaveCommands` | 1 | Save cmd history | [W] |
| `[Configuration]` | `UseRightButton` | 1 | Right-button selection | [W] |
| `[Configuration]` | `ToolTipDelay` | 500 | Tooltip delay ms | [W] |
| `[Configuration]` | `RestrictInterface` | 0 | Bits: 2 cmdline, 4 button bar edit, 8 drive menu, 16 file menu, 64 FTP | [W] |
| `[Configuration]` | `listname` | — | Last FTP connection list | [W] |
| `[Configuration]` | `FirstFTPSync` | 0 | FTP sync time warning | [W] |
| `[Confirmation]` | `MouseActions` | 0 | Confirm drag & drop | [W] |
| `[Layout]` | `CmdLine`, `ButtonBar`, `HistoryHotlistButtons`, `InterfaceFlat` | 1,1,1,0 | Visibility | [W] |
| `[Buttonbar]` | `Buttonbar`, `Buttonheight`, `FlatIcons`, `SmallIcons` | path, 29?, 1, 1 | Bar | [W] |
| `[Configuration]` | `AutoComplete` | ? | Path autocomplete | [?] |
| `[Layout]` / `[ButtonbarVertical]` | `ButtonBarVertical`, `Buttonbar` | ? | Vertical bar | [?] |

mormi stores its own settings in its own config, not in wincmd.ini. The table exists for the importer (F-446, F-663) and for parity checks.

## 9. Open questions to check on a real TC 11

1. Exact title and button labels of the hotlist configuration page (1.5) and the Ctrl modifier for "add with target" (1.3).
2. The frequently-used-folders hotkey (2.3) and the history length (2.2).
3. Field order and exact wording in the FTP details dialog (3.3) and the quick connect dialog (3.4).
4. The right-drag drop menu items (6.2) and the self-copy error text (6.3).
5. The TC 11 tab context menu item list (7.4).
6. The `AutoComplete` key and its bits (4.6).
7. The `%F`/`%f`, `%R`, `%X`, `%Q`, `%Z` placeholder meanings (5.3).
