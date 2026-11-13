# internal/links

UI-free engine for links: creating symbolic links, hard links and (Windows)
junctions, reading and writing shortcut files, resolving link targets through
whole chains, and detecting link loops so recursive operations never run
forever. Standard library plus `golang.org/x/sys/windows` (already a module
dependency) for the Windows calls. All shortcut parsers are pure Go and work
on every platform, so a .lnk on a USB stick can be followed on macOS.

## FEATURES.md IDs backed

| ID | Feature | API |
|----|---------|-----|
| F-337, F-179 | Create symbolic link / hard link (cm_CreateSymbolicLink, cm_CreateHardLink) dialog | `CreateSymlink`, `CreateHardLink`, `Create`, `CreateOptions`, `SupportedKinds`, `DefaultLinkName`, `UniqueName` |
| F-178 | Ctrl+Shift+F5 create shortcut / symlink (cm_CreateShortcut), also for a selection | `CreateLinks` (batch), `CreateShortcut` (.url / .webloc / .desktop) |
| — (Windows) | Create junction | `CreateJunction` |
| F-075, F-076 | Follow symlink / alias target (cm_GoToDir on a link, Ctrl+PgDn) | `FollowLink` → `Action{OpenDir, FocusFile}`, `Resolve` |
| F-738, F-746 | Smart Enter on .lnk / .url / .webloc / .desktop, open URL files in the browser | `FollowLink` → `Action{OpenURL, Run, OpenDir, FocusFile}`, `Inspect` |
| F-722 | Never follow symlink loops infinitely | `LoopDetector`, `Walk`, `HasLoop`, `PointsToAncestor`, `ErrLoop` |
| F-476, F-535 (support) | Copy links as links or follow them; find with "follow symlinks" | `Walk(WalkOptions{FollowLinks, SkipDuplicates})`, `IsLink`, `ReadLink` |
| F-324 (support) | Properties: link target, hard link count | `Inspect` → `Info` |

The UI wiring (dialog, menu entries, hotkeys, Properties page, broken-link
colour) is not part of this package.

## Creating links

- `CreateSymlink(target, linkPath, CreateOptions{Relative, Overwrite, TargetIsDir})`
  - `Relative` stores the target relative to the link's folder (real paths
    are compared, so it works through macOS `/var` → `/private/var`).
  - Windows: uses `CreateSymbolicLink` with the unprivileged flag (Developer
    Mode) and retries without it on older builds; directory vs file link is
    taken from the target, or `TargetIsDir` when the target does not exist yet.
    A missing privilege returns an error wrapping `ErrPrivilege`.
- `CreateHardLink(target, linkPath, opt)` — refuses folders (`ErrIsDir`) and
  cross-volume links (`ErrCrossDevice`); a symlink target is resolved first so
  the real file is linked on every OS.
- `CreateJunction(target, linkPath, opt)` — Windows only (mount-point reparse
  buffer via `FSCTL_SET_REPARSE_POINT`; absolute local folder targets only).
  Other platforms return an error wrapping `ErrUnsupported`.
- `Create(kind, target, linkPath, opt)` dispatches on `Kind`.
- `CreateLinks(ctx, BatchRequest{Kind, Sources, DestDir, Options, Name})` →
  `[]Result{Source, Link, Err}` in order; cancellation fills `ctx.Err()`.
- `SupportedKinds(goos)` — what the dialog should offer, in order.
- `DefaultLinkName(source, kind)` and `UniqueName(dir, name)` ("x(2).txt", TC style like fileops.UniqueName).

Rules: an existing name returns an error wrapping `os.ErrExist` (the dialog
asks "Overwrite?" and retries with `Overwrite`); with `Overwrite` the new link
is made under a temporary name and renamed over the old one; a real folder is
never replaced; an overwrite whose target is the link path itself, or reaches
it through links, is refused with `ErrSelfLink` (it would destroy the file and
leave a link to itself). A hard link onto a name that already is the same file
is a no-op.

## Shortcut files

| Kind | Read | Write | Notes |
|------|------|-------|-------|
| `KindLnk` (.lnk) | `ReadLnk`, `ParseLnk` → `Lnk` | no | MS-SHLLINK: local/UNC target, description, relative path, working dir, arguments, icon, env-var target; shell item lists are not decoded |
| `KindURL` (.url) | `ParseURLShortcut` | `CreateShortcut`, `URLShortcutData` | `[InternetShortcut] URL=` |
| `KindWebloc` (.webloc) | `ReadWebloc`, `ParseWebloc`, `ParsePlistStrings` | `CreateShortcut`, `WeblocData` | XML and binary (`bplist00`) plists |
| `KindDesktop` (.desktop) | `ParseDesktopEntry`, `ExecArgs` | `CreateShortcut`, `DesktopLinkData` | Type=Link (URL), Application (Exec), Directory |
| `KindAlias` (Finder alias) | `ReadAlias`, `ParseAlias` | no | bookmark data path components; found by content sniffing |

`.lnk` writing is deliberately not offered: a correct .lnk needs the shell's
IShellLink, and on Windows a symbolic link or junction is the better choice.
Local targets are stored in shortcuts as file:// URLs (`FileURL`,
`PathFromFileURL`). Helpers: `ShortcutKindByName`, `ShortcutExt`, `IsURL`,
`ExpandWindowsEnv`.

## Inspecting and following

- `Inspect(path) (*Info, error)` — never follows the path first. `Info` has
  `Kind`, raw `Target`, `URL`, `Resolved`, `Chain`, `TargetExists`,
  `TargetIsDir`, `Broken`, `Loop`, `HardLinks`, and shortcut fields `Name`,
  `Arguments`, `WorkingDir`, `Icon`, `Exec`. Broken links and loops are
  reported in fields, not as errors. Regular files with more than one name are
  `KindHardLink`.
- `FollowLink(path) (*Action, error)` — `ActionOpenDir` (Dir),
  `ActionFocusFile` (Dir + Focus), `ActionOpenURL` (URL), `ActionRun` (Args,
  Dir). Errors wrap `ErrNotLink`, `ErrBroken` (also for a path from another
  OS, such as `C:\…` in a .lnk read on macOS) or `ErrLoop`.
- `Resolve(path) (*Resolution{Chain, Final, Exists, IsDir}, error)` — follows
  symlinks and junctions hop by hop; errors wrap `ErrBroken` (Resolution still
  returned) or `ErrLoop` (`*LoopError`). At most `MaxHops` (40) hops.
- `ReadLink`, `IsLink`, `HasLoop`, `PointsToAncestor`, `IdentityOf` → `FileID`.

## Loop-safe walking

- `NewLoopDetector(skipDuplicates)`; `d.Enter(dir) (leave func(), err)` before
  descending — `ErrLoop` when the folder is already an ancestor (by device +
  inode / volume + file index), `ErrAlreadyVisited` for a folder already
  walked through another link when duplicate skipping is on.
- `Walk(ctx, root, WalkOptions{FollowLinks, SkipDuplicates}, fn)` — lexical
  order; loops, duplicates and broken links are passed to `fn` as errors and
  not descended into; `filepath.SkipDir` / `filepath.SkipAll` supported.

## Platform files

`platform_unix.go` (symlink/link, `Stat_t` identity and link count),
`platform_windows.go` (CreateSymbolicLink, junctions, handle info, junction
detection via the reparse tag), `platform_other.go` (builds elsewhere; no
identity, no junctions).
