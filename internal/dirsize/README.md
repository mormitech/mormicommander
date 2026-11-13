# internal/dirsize

UI-free engine for folder sizes and volume details. Standard library plus
`golang.org/x/sys` (already a dependency). It backs Space on a folder,
Alt+Shift+Enter (count all folders), Ctrl+L (occupied space of the selection),
the folder size column, the drive bar, the drive popup and the drive info line.

## FEATURES.md IDs backed

| ID | Feature | API |
|----|---------|-----|
| F-118, F-797 | Space on a folder calculates its size | `Start` / `Calculate` (the "select only" option is UI wiring) |
| F-183, F-264 | Alt+Shift+Enter: sizes of all folders in the panel | `CalculateAll` with `Options.ItemDone`, or `Start` |
| F-244, F-325 | Ctrl+L: bytes, size on disk, files, folders; cancellable | `CalculateAll` → `Summary.Total`, `SpaceLine` |
| F-091 | Folder size column shows the size after Space/calc | `Cache` (`Put`, `Get`, `Invalidate`, `Clear`) |
| F-093 | Size in bytes / KB / MB / dynamic | `FormatSize`, `Unit`, `Group` |
| F-016 | Drive info line above each panel | `VolumeOf`, `DriveLine` |
| F-058, F-709 | Drive popup / drive list with labels and free space | `VolumeOf` (label, file system, sizes, remote/removable) |
| F-714 | Free space indicator in the drive bar | `VolumeInfo.UsedFraction`, `Used` |
| F-681 | Properties dialog: size and size on disk | `Calculate` (`Bytes`, `Allocated`), `VolumeInfo.BlockSize` |
| F-491 | Free space check before copy (data source) | `VolumeOf(target).Available` + `Calculate(source).Bytes` |

The drive list itself (enumerating volumes) is `internal/disk`; this package
describes the volume of any path in more detail.

## Calculating sizes

- `Calculate(ctx, path, Options) (Result, error)` counts one file or folder.
  The error is set when the path cannot be read at all, or on cancel (wraps
  `ErrCanceled` and `ctx.Err()`; the partial `Result` has `Complete == false`).
- `CalculateAll(ctx, paths, Options) (Summary, error)` counts several paths one
  after another (each walked in parallel). `Summary.Items[i]` matches
  `paths[i]`; `Summary.Total` adds them up, counts the given folders themselves
  in `Dirs` (Ctrl+L wording) and drops data shared by hard links between items.
  Unreadable paths get an error in their own `Result`; the returned error is
  only set on cancel.
- `Start(ctx, paths, Options) *Job` runs `CalculateAll` in the background:
  `Cancel()`, `Done()`, `Wait()`, `Progress()` (latest snapshot, pollable).
- `Options`:
  - `SameVolume` — do not enter other mounted volumes (like `du -x`). On macOS
    the system and data volumes of the startup disk count as one.
  - `FollowSymlinks` — enter links/junctions to folders and count link targets;
    loops are detected, each real folder and file is counted once. The root is
    always followed.
  - `CountHardlinksTwice` — turn off hard link de-duplication (default: data of
    a multi-link file counts once; each name still counts in `Files`).
  - `Quick` — Windows only: skip the per-file open used for exact size on disk
    and hard link identity (size on disk is then rounded up to the cluster).
  - `Workers` (default 16), `ProgressInterval` (default 150 ms),
    `Progress func(Progress)` (serialized; last call has `Done`),
    `ItemDone func(i int, r Result)` (fill the size column folder by folder).
- `Result`: `Path, IsDir, Bytes` (logical), `Allocated` (size on disk),
  `Files, Dirs` (excluding the root), `Symlinks` (links not followed, no size),
  `Others` (sockets, pipes, devices), `HardlinkDupes, HardlinkBytes`,
  `SkippedMounts/SkippedMountCount` (other volumes, link loops),
  `Errors/ErrorCount` (unreadable entries; at most 100 listed, count exact),
  `Complete, Duration`. Entries that vanish while counting are ignored.
- `Progress`: `Current` folder, `Item, ItemPath, ItemsDone, ItemsTotal`, running
  `Bytes, Allocated, Files, Dirs, Errors`, `Elapsed`, `Done`.

Size on disk: Unix uses `st_blocks * 512` (free, from the lstat the walk does
anyway). Windows opens each file for attributes only (shares everything) and
reads `FILE_STANDARD_INFO.AllocationSize` and the link count; compressed and
sparse files use `GetCompressedFileSize`, rounded up to the cluster size.

## Cache

`Cache` (zero value ready, concurrency safe) keeps complete results by path.
`Invalidate(path)` removes the path, all its ancestors and all its descendants,
because any change inside a folder changes every size above it. Keys are cleaned
and case-insensitive on Windows and macOS.

## Volumes

- `VolumeOf(path) (VolumeInfo, error)`: `Root` (mount point / drive root),
  `Label`, `FileSystem`, `Device`, `Serial` (Windows), `Total`, `Free`,
  `Available` (for this user), `BlockSize` (cluster), `ReadOnly`, `Remote`,
  `Removable`. Methods `Used()`, `UsedFraction()`, `DisplayName()`.
  - macOS: `statfs`; paths on the system or data volume of the startup disk
    report root `/`, the data volume's sizes and the startup disk's name (from
    the `/Volumes/<name>` link to `/`).
  - Linux: `statfs` + `/proc/self/mountinfo` (matched by path and device
    numbers) + `/dev/disk/by-label` + `/sys/class/block/*/removable`.
  - Windows: `GetVolumePathName`, `GetVolumeInformation`,
    `GetDiskFreeSpaceEx`, `GetDiskFreeSpace`, `GetDriveType`,
    `GetVolumeNameForVolumeMountPoint`.
- Text: `DriveLine(v, unit)` → `[System]  26,214,400 k of 104,857,600 k free`;
  `SpaceLine(r)` → `1,234,567 bytes (1.18 M) in 1,200 files, 3 folders[, 2 M on disk]`;
  `FormatSize(n, UnitBytes|UnitKB|UnitMB|UnitGB|UnitDynamic)`; `Group(n)`.

## Files

`dirsize.go` types and options · `walk.go` parallel walker, `Calculate`,
`CalculateAll` · `job.go` background jobs · `cache.go` result cache ·
`volume.go` `VolumeInfo` and formatting · `mountinfo.go` Linux mount table
parsing (tested everywhere) · `stat_unix.go` / `stat_windows.go` /
`stat_other.go` per-file details · `devalias_darwin.go` / `devalias_linux.go`
volume identity · `volume_darwin.go` / `volume_linux.go` / `volume_windows.go` /
`volume_other.go` volume details.
