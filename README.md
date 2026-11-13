<h1 align="center">mormiCommander</h1>

<p align="center">
  <b>Two panels, all the keys you know.</b><br>
  A free, open-source two-panel file manager for macOS, Windows and Linux, in the spirit of Total Commander.
</p>

<p align="center">
  <a href="https://github.com/mormitech/mormicommander/actions/workflows/build.yml"><img alt="build" src="https://github.com/mormitech/mormicommander/actions/workflows/build.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="MIT license" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <img alt="platforms" src="https://img.shields.io/badge/platforms-macOS%20%7C%20Windows%20%7C%20Linux-lightgrey.svg">
  <img alt="status" src="https://img.shields.io/badge/status-early%20work-orange.svg">
</p>

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

mormiCommander is at the start (version 0.1). What works:

- **Two panels** side by side (or on top of each other with Ctrl+Shift+H), with a drive bar, the free space of the
  drive, a path bar you can click into or click a part of, column titles to sort by, and a status line with what is
  selected. Drag the bar between them to share the width.
- **Moving around with the keyboard**: arrows, PgUp/PgDn, Home/End, Enter, Backspace, Ctrl+\\ for the root, Alt+Left and
  Alt+Right through the history, Alt+Down for the list of visited folders, Alt+F1/Alt+F2 for the drive list, Ctrl+Left
  and Ctrl+Right to show a folder in the other panel, Ctrl+U to swap the panels, Ctrl+H for hidden files.
- **Selecting**: Ins, Space, Shift with the arrows, Ctrl+A, Num * (or Shift+8), Num / to bring a selection back.
- **Sorting** by name, extension, date and size (Ctrl+F3 … Ctrl+F7, or click a column title).
- **A command line** under the panels: type and press Enter; `cd` changes folder.
- **Big folders**: 100,000 files scroll as smoothly as ten.
- **Native window, one small binary.** Light and dark theme follow the system. The window comes back where you left it,
  with the same folders.

Copy, move, delete and much more are being built now (the engine for copy, move and delete is in place).

## Install

There are no releases yet. To build it yourself you need [Go](https://go.dev) 1.26 or newer and
[Node.js](https://nodejs.org); the window is [Wails v3](https://v3.wails.io), which comes in as a Go module (no CLI to
install):

```
scripts/build.sh                 # Linux needs libgtk-4-dev and libwebkitgtk-6.0-dev; Windows: run it in Git Bash
```

The app is in `build/bin`. Once releases start, it will install with Homebrew (macOS and Linux), Scoop and winget
(Windows) and a .deb (Debian, Ubuntu), like the other mormi apps.

## Try it without your files

```
mormicommander --demo
```

shows a made-up home folder (and a folder with 100,000 files). Nothing on your disk is read or changed, and your settings
are not used.

## For developers

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): how the app is built and how to add a feature.
- [docs/control.md](docs/control.md): control mode, which lets a script drive the app (tests, screenshots).
- [docs/keyboard.md](docs/keyboard.md): the keyboard rule on macOS, Windows and Linux.
- `scripts/qa_smoke.sh`: builds everything and checks the app end to end.

## Privacy

mormiCommander makes no network connections of its own: no telemetry, no update checks in the background. "Check for
updates" in the Help menu opens the release page in your browser, and only when you choose it.

## Code signing

The releases will not be signed at first. On macOS the Homebrew cask clears the download quarantine flag; a downloaded
zip needs right-click > Open the first time. Windows SmartScreen may warn about an unknown publisher.

## License

MIT, see [LICENSE](LICENSE). Total Commander is a trademark of Christian Ghisler; mormiCommander is not related to it.
