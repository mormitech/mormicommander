<h1 align="center">mormiCommander</h1>

<p align="center">
  <b>Two panels, all the keys you know.</b><br>
  A free, open-source two-panel file manager for macOS, Windows and Linux, inspired by Total Commander.
</p>

<p align="center">
  <a href="https://github.com/mormitech/mormicommander/actions/workflows/build.yml"><img alt="build" src="https://github.com/mormitech/mormicommander/actions/workflows/build.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="MIT license" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <img alt="platforms" src="https://img.shields.io/badge/platforms-macOS%20%7C%20Windows%20%7C%20Linux-lightgrey.svg">
  <img alt="status" src="https://img.shields.io/badge/status-early%20work-orange.svg">
</p>

> **Inspired by Total Commander, not affiliated with Ghisler Software GmbH.** Total Commander is a registered trademark
> of Christian Ghisler. mormiCommander is an independent project with its own name, icon and code; it uses none of Total
> Commander's code or assets.

---

## Our goal

> **A two-panel file manager that is free, forever, on every platform, and installed the way you install everything else.**

Total Commander has been the file manager of choice for decades, but it is paid and made for Windows. On macOS and Linux
there is nothing with the same keys and the same speed. mormiCommander is an attempt to build that experience as a small
native app that costs nothing, has no accounts, no ads, no telemetry and no upsell, and uses Total Commander's keys on
every system: F5 copies, F6 moves, F7 makes a folder, F8 deletes, Tab switches panels, everywhere.

The full list of what Total Commander does, and how far mormiCommander has come, is in
[docs/FEATURES.md](docs/FEATURES.md).

## What it does today

mormiCommander is early work, but most of the daily tools are in:

- **Two panels** side by side (or on top of each other with Ctrl+Shift+H), with tabs, a drive bar, the free space of
  the drive, a path bar you can click into, column titles to sort by, and a status line with what is selected.
- **Copy, move, rename, delete** (to the Trash, or for good), new folders, with progress, pause and a list of transfers running in the background.
- **The keyboard first**: Total Commander's keys for moving around, selecting (Ins, Space, Num+ / Num- / Num*), sorting
  and every tool below. Typing goes into the command line under the panels; `cd` changes folder.
- **Tools**: find files (Alt+F7), quick filter (Ctrl+S), multi-rename (Ctrl+M), compare folders (Shift+F2), compare
  files by content, synchronize folders, the directory hotlist (Ctrl+D) and folder history, folder sizes.
- **Lister** (F3) for text, images, Markdown and HTML, and quick view (Ctrl+Q) in the other panel.
- **Archives**: open ZIP, TAR, 7z and RAR like folders, pack with Alt+F5, unpack with Alt+F9.
- **FTP, SFTP and WebDAV** panels (Ctrl+F), only when you connect.
- **Big folders**: 100,000 files scroll as smoothly as ten.
- **Native window, one small binary.** Light and dark theme follow the system. The window comes back where you left it,
  with the same folders.

## Keys

The same keys on macOS, Windows and Linux, by the physical key: Ctrl is the Control key, Alt is Alt (Option on a Mac).
These are the defaults; every key can be changed (Configuration > Keyboard shortcuts). The rules, and the keys macOS
keeps for itself, are in [docs/keyboard.md](docs/keyboard.md); the full list is in
[frontend/src/core/keymap.js](frontend/src/core/keymap.js). On a Mac laptop the function keys need fn unless "Use F1,
F2, etc. keys as standard function keys" is on.

<!-- key-table:start -->
| Key | What it does | Command |
| --- | --- | --- |
| `F3` | View the file (Lister) | `cm_List` |
| `F4` | Edit the file | `cm_Edit` |
| `F5` | Copy to the other panel | `cm_Copy` |
| `Shift+F5` | Copy in the same folder | `cm_CopySamepanel` |
| `F6` | Move or rename to the other panel | `cm_RenMov` |
| `Shift+F6` | Rename in place | `cm_RenameOnly` |
| `F7` | New folder | `cm_MkDir` |
| `F8` / `Delete` | Delete (to the Trash) | `cm_Delete` |
| `Shift+F8` / `Shift+Delete` | Delete for good | `cm_DeletePermanent` |
| `Tab` | Switch panel | `cm_FocusTrg` |
| `Enter` | Open the folder or file | `cm_Enter` |
| `Backspace` | Parent folder | `cm_GoToParent` |
| `Ctrl+\` | Root folder | `cm_GoToRoot` |
| `Alt+Left` / `Alt+Right` | Back / forward through the folders you visited | `cm_GoToPreviousDir` / `cm_GoToNextDir` |
| `Alt+Down` | Folder history | `cm_DirectoryHistory` |
| `Alt+F1` / `Alt+F2` | Drive list, left / right | `cm_LeftOpenDrives` / `cm_RightOpenDrives` |
| `Ctrl+Left` / `Ctrl+Right` | Show the folder in the left / right panel | `cm_TransferLeft` / `cm_TransferRight` |
| `Ctrl+U` | Swap the panels | `cm_Exchange` |
| `Ctrl+R` / `F2` | Reread the folder | `cm_RereadSource` |
| `Ctrl+H` | Show or hide hidden files | `cm_SwitchHidSys` |
| `Ins` | Select and move down | `cm_SelectToggleDown` |
| `Space` | Select (and show a folder's size) | `cm_SelectToggle` |
| `Num+` / `Num-` | Select / unselect a group (Shift+= / Shift+- without a numpad) | `cm_SpreadSelection` / `cm_ShrinkSelection` |
| `Num*` | Invert the selection (Shift+8 without a numpad) | `cm_ExchangeSelection` |
| `Num/` | Bring the last selection back | `cm_RestoreSelection` |
| `Ctrl+A` | Select all | `cm_SelectAll` |
| `Ctrl+F3` / `Ctrl+F4` / `Ctrl+F5` / `Ctrl+F6` | Sort by name / extension / date / size | `cm_SrcByName` / `cm_SrcByExt` / `cm_SrcByDateTime` / `cm_SrcBySize` |
| `Ctrl+F7` | Unsorted | `cm_SrcUnsorted` |
| `Alt+F7` | Find files | `cm_SearchFor` |
| `Ctrl+S` | Quick filter | `cm_QuickFilter` |
| `Ctrl+M` | Multi-rename | `cm_MultiRenameFiles` |
| `Shift+F2` | Compare folders | `cm_CompareDirs` |
| `Ctrl+Q` | Quick view in the other panel | `cm_SrcQuickView` |
| `Ctrl+D` | Directory hotlist | `cm_DirectoryHotlist` |
| `Ctrl+T` / `Ctrl+W` | New tab / close tab | `cm_OpenNewTab` / `cm_CloseCurrentTab` |
| `Ctrl+Tab` | Next tab | `cm_SwitchToNextTab` |
| `Alt+F5` | Pack into an archive | `cm_PackFiles` |
| `Alt+F9` | Unpack | `cm_UnpackFiles` |
| `Ctrl+F` | Connect to an FTP, SFTP or WebDAV server | `cm_FtpConnect` |
| `Alt+Enter` | Properties | `cm_Properties` |
| `Ctrl+L` | Size of the selected folders | `cm_GetFileSpace` |
| `Ctrl+P` | Put the folder path into the command line | `cm_AddPathToCmdline` |
| `Ctrl+C` / `Ctrl+X` / `Ctrl+V` | Copy / cut / paste files through the clipboard | `cm_CopyToClipboard` / `cm_CutToClipboard` / `cm_PasteFromClipboard` |
| `Shift+F10` | Context menu | `cm_ContextMenu` |
| `Esc` | Clear the command line | `cm_ClearCmdLine` |
| `Alt+F4` | Quit | `cm_Exit` |
| `Cmd+C` / `Cmd+X` / `Cmd+V` | macOS: copy / cut / paste files | `cm_CopyToClipboard` / `cm_CutToClipboard` / `cm_PasteFromClipboard` |
| `Cmd+Backspace` | macOS: delete to the Trash | `cm_Delete` |
| `Cmd+I` | macOS: properties | `cm_Properties` |
| `Cmd+F` | macOS: find files | `cm_SearchFor` |
| `Cmd+L` / `Shift+Cmd+G` | macOS: type a path | `cm_EditPath` |
| `Cmd+,` | macOS: options | `cm_Config` |
<!-- key-table:end -->

## How it compares

The honest short version: Double Commander is the mature free commander on all three systems; the polished Mac apps are
paid or Mac-only. mormiCommander aims at Total Commander's keys and behaviour, a native feel on every system, and the
same free app everywhere. It does not run Total Commander plugins and does not claim to.

| | mormiCommander | Double Commander | Commander One | ForkLift 4 | Nimble Commander | Marta |
| --- | --- | --- | --- | --- | --- | --- |
| Price | Free | Free | Free; PRO $29.99 | Paid (about $20–30) | Free | Free (beta) |
| Systems | macOS, Windows, Linux | macOS, Windows, Linux | macOS | macOS | macOS | macOS |
| Open source | Yes (MIT) | Yes (GPL-2) | No | No | Yes (GPL-3) | No |
| Total Commander keys by default | Yes, all of them | Mostly | Some (Tab, F5, F6, F8) | Some; no Ins/Space selection | Function keys | No |
| Synchronize folders | Yes | Yes | No | Yes | No | No |
| Multi-rename | Yes | Yes | No | Yes | No | No |
| TC plugins (WCX/WDX/WFX/WLX) | No | Yes | No | No | No | No |
| Look on macOS | Native window, web UI | Lazarus, Windows-like | Native | Native | Native (AppKit) | Native |
| Development (Nov 2026) | Active, early | Very active, 2 main authors | Active | Very active | About one release a year | No release since July 2024 |

Compiled from public sources (release notes, trackers, reviews) in November 2026, not from hands-on testing; the
details are in [docs/research/market.md](docs/research/market.md). Spotted a mistake? Please
[open an issue](https://github.com/mormitech/mormicommander/issues).

## Download

**The only official downloads are the
[GitHub releases of mormitech/mormicommander](https://github.com/mormitech/mormicommander/releases)** and the package
managers fed from them (the Mormitech Homebrew tap, Scoop bucket and winget). Every release lists the SHA-256 checksum
of each file in `SHA256SUMS.txt`. How to check a download, and which file to pick, is on the download page:
[docs/DOWNLOADS.md](docs/DOWNLOADS.md). Copies of the app elsewhere are not ours.

What it runs on: **macOS 13 (Ventura) or newer** on Apple silicon or Intel; Windows x64 with the Microsoft Edge WebView2
runtime (part of Windows 11 and current Windows 10); Linux x64 with GTK 4 and WebKitGTK 6.0 (the .deb installs them).

To build it yourself you need [Go](https://go.dev) 1.26 or newer and
[Node.js](https://nodejs.org); the window is [Wails v3](https://v3.wails.io), which comes in as a Go module (no CLI to
install):

```
scripts/build.sh                 # Linux needs libgtk-4-dev and libwebkitgtk-6.0-dev; Windows: run it in Git Bash
```

The app is in `build/bin`.

## Try it without your files

```
mormicommander --demo
```

shows a made-up home folder (and a folder with 100,000 files). Nothing on your disk is read or changed, and your settings
are not used.

## Privacy: no telemetry, and a test to prove it

mormiCommander makes no network connections of its own: no telemetry, no crash reports, no usage statistics, no update
checks in the background, no account. "Check for updates" in the Help menu opens the release page in your browser, and
only when you choose it. The app goes on the network only when you ask it to: when you connect to an FTP, SFTP or
WebDAV server, or open a network folder. AI search (Find files > AI) runs only when you start it, through the AI
program you installed yourself (the Claude Code command), which sends your prompt and the search results to its
provider; it can use only mormiCommander's own non-destructive tools (see [docs/privacy.md](docs/privacy.md)).

You don't have to take our word for it. The **zero-network test** checks the promise on every build:

- [`privacy_test.go`](privacy_test.go) reads all of the Go code and fails if anything can open a connection, listen on
  a port or look up a host name outside the FTP, SFTP and WebDAV clients (which only the Net menu starts) and two
  listeners bound to 127.0.0.1 (control mode, off unless you start it; the hand-over between windows of one app; and
  the AI tool endpoint, opened by the first AI search you start and answering only searches that are running).
- [`privacy_session_test.go`](privacy_session_test.go) runs a scripted session through the app's features (browse,
  view, search, copy, thumbnails, a Markdown file with remote pictures) and fails on any HTTP request, host-name lookup
  or connection to another machine.
- [`frontend/test/privacy.test.js`](frontend/test/privacy.test.js) checks the window's code: no script fetches anything,
  the pages load nothing from other sites (no CDNs, no web fonts), and the only web addresses are links you click.
- [`frontend/test/viewer-noremote.test.js`](frontend/test/viewer-noremote.test.js) checks that the Lister drops remote
  images, styles and frames from HTML and Markdown files, so viewing a file never contacts a server.

Run them yourself with `go test -run ZeroNetwork .` and
`cd frontend && npx vitest run test/privacy.test.js test/viewer-noremote.test.js`. The whole statement, and what the
tests do not cover yet, is in [docs/privacy.md](docs/privacy.md).

## For developers

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): how the app is built and how to add a feature.
- [docs/control.md](docs/control.md): control mode, which lets a script drive the app (tests, screenshots).
- [docs/keyboard.md](docs/keyboard.md): the keyboard rule on macOS, Windows and Linux.
- [docs/marta.md](docs/marta.md): coming from Marta (importing favourites, the Marta-like preset, where the actions are).
- `scripts/qa_smoke.sh`: builds everything and checks the app end to end.

## Code signing

The releases will not be signed at first. On macOS the Homebrew cask clears the download quarantine flag; a downloaded
zip needs right-click > Open the first time. Windows SmartScreen may warn about an unknown publisher. Check the
checksum (see [docs/DOWNLOADS.md](docs/DOWNLOADS.md)) before you allow it.

## License

MIT, see [LICENSE](LICENSE). Total Commander is a registered trademark of Christian Ghisler; mormiCommander is not
affiliated with Ghisler Software GmbH.
