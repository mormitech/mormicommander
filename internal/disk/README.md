# internal/disk

UI-free engine that lists the drives and volumes for the drive bar and the
drive popup, and tells the size and free space of the volume a path is on.
Standard library plus `golang.org/x/sys` (already a dependency).

## API

| Call | What it does |
|------|--------------|
| `Volumes() []Volume` | The drive list, in display order. macOS: the startup disk at `/` (named after Finder's `/Volumes/<name>` link, via `dirsize.VolumeOf`), then every mount under `/Volumes`. Linux: `/`, the home folder, then real disks and network shares from `/proc/mounts`. Windows: every drive letter, including empty card readers and CD drives (listed without size). |
| `UsageOf(path) (Usage, error)` | Total and user-writable free bytes of the volume holding `path` (cheap: one statfs / GetDiskFreeSpaceEx). |
| `Volume` | `Name`, `Path`, `Label` (Windows volume label), `Kind` (`fixed`, `removable`, `network`, `cdrom`, `home`), `Total`, `Free`. |
| `Usage` | `Total`, `Free`. |

Platform notes:

- Linux sizes use `f_frsize` (the unit of the block counts), not `f_bsize`.
- macOS kinds: no `MNT_LOCAL` is network, cd9660/udf is cdrom, `MNT_REMOVABLE`
  or FAT/exFAT is removable, anything else (internal second disk, other APFS
  volumes) is fixed. Best effort without IOKit.
- Windows: critical-error dialogs are switched off (`SetErrorMode`) so an empty
  drive never shows "No disk", and every drive is queried in parallel with a
  1.5 s budget, so a disconnected network drive cannot stall the list.

## Relation to internal/dirsize

`internal/dirsize.VolumeOf(path)` describes the volume of any path in detail
(mount point, label, file system, device, block size, read-only, remote,
removable). This package enumerates the volumes; use `dirsize.VolumeOf` when
a panel needs the details of the volume it is on. The macOS startup disk entry
is taken from `dirsize.VolumeOf("/")` so the two never disagree.

## FEATURES.md IDs backed

| ID | Feature | API |
|----|---------|-----|
| F-058, F-174, F-175, F-219 | Drive popup (Alt+F1 / Alt+F2) with labels and free space | `Volumes` |
| F-059 | Drive bar buttons, one per drive/volume | `Volumes` (`Name`, `Path`, `Kind` for the icon) |
| F-709 | Drive list with labels and free space | `Volumes` |
| F-714 | Free space indicator in the drive bar | `Volume.Total`, `Volume.Free` |
| F-016 | Drive info line above each panel (sizes) | `UsageOf` (details: `dirsize.VolumeOf`) |
| F-705 | Drive button context menu: eject for removable drives (data source) | `Volume.Kind == "removable"` |
