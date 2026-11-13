# internal/clipfiles

Copy, cut and paste files through the operating system clipboard, and copy file names as text, the way Total
Commander's Ctrl+C / Ctrl+X / Ctrl+V and its "copy names to clipboard" commands work. The package does not use cgo and
has no UI code.

| Platform | Files on the clipboard | Cut mark | Text | Images | How |
|---|---|---|---|---|---|
| macOS | NSURL file URLs (`public.file-url`, the same types Finder writes) plus the bare names as text | private type `org.mormicommander.drop-effect` (Finder has no cut, so it sees a copy) | `public.utf8-plain-text` | `public.png`, falls back to `public.tiff` and converts it | `osascript -l JavaScript` (AppKit bridge), JSON on stdin/stdout |
| Windows | `CF_HDROP` | `Preferred DropEffect` (DROPEFFECT_MOVE = cut) | `CF_UNICODETEXT` | reads `PNG`, then `CF_DIBV5` / `CF_DIB` (converted); writes `PNG` + `CF_DIB` | user32/kernel32 calls via `golang.org/x/sys/windows` |
| Linux | `x-special/gnome-copied-files` (GNOME, Cinnamon, MATE, Xfce, LXDE, Budgie, …) or `text/uri-list` (KDE and others) | first line `cut` in the GNOME format; reads KDE's `application/x-kde-cutselection` | `text/plain;charset=utf-8` / `UTF8_STRING` | reads PNG, BMP, JPEG, GIF (converted to PNG); writes PNG | `wl-copy`/`wl-paste` on Wayland, `xclip` on X11, `xsel` text-only fallback |
| others | — | — | — | — | every call returns `ErrUnsupported` |

Linux limits: the command-line tools offer one data type per copy. A cut is therefore always written as
`x-special/gnome-copied-files`, so KDE's Dolphin does not see it (Dolphin reads `text/uri-list` plus its own cut
marker). Set `LinuxFileFormat` to force one format. When no tool is installed the error wraps `ErrNoTool` and names the
package to install.

## API

Clipboard access (package-level functions use the system clipboard; `New()` returns a `*Clipboard` with the same
methods):

- `WriteFiles(paths []string, op Op) error` — Ctrl+C (`Copy`) / Ctrl+X (`Cut`). Paths must be absolute.
- `ReadFiles() ([]string, Op, error)` — file list and copy/cut; `ErrEmpty` when the clipboard holds no files.
- `WriteText(s) error`, `ReadText() (string, error)`.
- `WriteImage(png) error`, `ReadImage() ([]byte, error)` — PNG bytes in and out.
- `ReadContent() (Content, error)` (method: `Content()`) — which of files / text / image the clipboard holds, plus the
  raw type names. The UI uses it to decide what Ctrl+V does.
- `Clear() error` — call after cut files were moved by a paste, as Explorer does.
- `CopyAsText(kind TextKind, entries []Entry, f Format) error` — format and copy in one step.
- `PlanPaste(dest) (Plan, error)` — read the clipboard and return `{Sources, Dest, Move, SameDir}` for the file
  operations engine; refuses a folder pasted into itself (`ErrIntoItself`). `NewPlan(paths, op, dest)` does the same
  without reading the clipboard. Every source and `dest` must be an absolute path.
- `PasteData(dir, name) (string, error)` — TC 10+: save clipboard image (preferred) or text as a new file; empty name
  means `clipboard.png` / `clipboard.txt` made unique. Never overwrites.

Text formatting (pure):

- `Entry{Path, Name, Size, ModTime, IsDir, Mode, Attr}`; `FromPaths(paths)`, `StatPaths(paths)` (Lstat, TC-style
  `rahs` attributes on Windows, `-rw-r--r--` elsewhere).
- `FormatText(kind, entries, Format) string` with kinds `Names`, `FullNames`, `Details`, `FullDetails` (tab
  separated: name, size or `<DIR>`, date, attributes), `NetNames` (mapped drive → `\\server\share` on Windows),
  `UnixPaths`, `URLs`.
- `Format{Newline, TrailingNewline, DateLayout, DirSuffix, DirSizeText}` — newline defaults to CRLF on Windows, LF
  elsewhere.
- `CommandKind("cm_CopyNamesToClip")` maps TC command names to kinds.
- `FolderPath(dir, trailingSep)` for cm_CopySrcPathToClip / cm_CopyTrgPathToClip.
- `ParseNameList(text)` and `SelectionNames(list, dir)` for cm_LoadSelectionFromClip; `Lines(text)` keeps blank lines
  for the multi-rename tool.
- `SuggestName`, `SaveText`, `SaveImage` for paste-as-file.

Format helpers (pure, exported for reuse and tests): `FileURI`, `PathFromURI`, `EncodeURIList`, `ParseURIList`,
`EncodeGnomeCopied`, `ParseGnomeCopied`, `EncodeHDROP`, `DecodeHDROP`, `EncodeDropEffect`, `DecodeDropEffect`,
`DecodeDIB`, `EncodeDIB`, `DIBToPNG`, `PNGToDIB`.

## FEATURES.md IDs backed

- F-237 Ctrl+C / Ctrl+X / Ctrl+V clipboard copy, cut, paste files (cm_CopyToClipboard, cm_CutToClipboard,
  cm_PasteFromClipboard)
- F-739 Clipboard file copy/paste interop with Finder / Explorer / Nautilus
- F-740 Paste text or image from clipboard as a new file
- F-138 cm_CopyNamesToClip, F-139 cm_CopyFullNamesToClip, F-140 cm_CopyDetailsToClip / cm_CopyFullDetailsToClip,
  F-141 cm_CopySrcPathToClip, F-142 cm_CopyNetNamesToClip and Unix/URL variants, F-796 cm_CopyTrgPathToClip
- F-350 (Files/Mark menu entries for the copy-names commands) and the clipboard half of F-132 / F-349
  (cm_LoadSelectionFromClip)
- Text helpers used by F-835 (multi-rename: names from clipboard) and F-838 (find: copy results to clipboard)

## Tests

`go test ./internal/clipfiles/` runs the format, planning and Linux-backend tests everywhere (the Linux tools are
faked). On macOS it also runs the real osascript backend against a private named pasteboard, so the user's clipboard
is never touched. The Windows backend is covered by its byte-level encoders here and by cross-compiling.
