# Engines

This document lists every UI-free engine package under `internal/` so that the UI agents know what exists and where to wire it. Each section names the package's purpose, links its README (the API summary), and lists the `docs/FEATURES.md` IDs it backs (taken from the README). Detailed Total Commander behaviour specs live in `docs/spec/area-1.md` to `area-8.md`.

All packages pass `go vet` and `go test`, and cross-compile with `GOOS=windows` and `GOOS=linux`. Integration status: the engines are wired into the app (Go side through `features.go`, page side through `frontend/src/features/*.js`; see [ARCHITECTURE.md](ARCHITECTURE.md)) and reached through registered cm_* commands. What is still missing, such as the tree views (F-081, F-171), is tracked per item in `docs/FEATURES.md` (`[ ]` todo, `[~]` partly done, `[x]` checked by QA) and on the project board.

## Core (from the work-in-progress snapshot)

### internal/disk
Volume and drive enumeration. No README yet; see the package comment. Backs the drive bar and drive popup (works with `internal/dirsize`).

### internal/trash
Moves files, folders and links to the macOS Trash, the freedesktop.org trash on Linux, or the Windows Recycle Bin. README: [internal/trash/README.md](../internal/trash/README.md). Used by `internal/fileops` for Trash jobs.

### internal/vfs
The file systems behind the panels: `Local`, `Mock`, and adapters that plug the archive, FTP and SFTP engines in, with a `Router` that picks the right file system for any path. README: [internal/vfs/README.md](../internal/vfs/README.md). IDs: F-051, F-071, F-078, F-482, F-483, F-596 to F-620 (archives as folders), F-626, F-629, F-633, F-636, F-641, F-648 to F-650.

### internal/fileops
Copy, move, delete, trash, wipe and rename as a background job queue with ask-on-conflict, progress and logging. README: [internal/fileops/README.md](../internal/fileops/README.md). IDs: F-457 to F-479, F-482 to F-501, F-831, F-833.

## Archives

### internal/archive
Format-neutral packer layer (registry, `Reader`, `Packer`, open options, pack/unpack helpers). README: [internal/archive/README.md](../internal/archive/README.md). IDs: F-596 to F-598, F-611 to F-622, F-625, F-626, F-628, F-847, F-848. Sub-packages are imported for their side effect.

### internal/archive/ziparc
Built-in ZIP: read, write, update, split sets, AES, UTF-8 names. README: [internal/archive/ziparc/README.md](../internal/archive/ziparc/README.md). IDs: F-596, F-598 to F-603, F-611, F-612, F-616 to F-622, F-625, F-627, F-628, F-847, F-848.

### internal/archive/tarfmt
Tar family (tar, tgz, tbz2, txz, tzst, tlz) and single-file packers (gz, bz2, xz, zst, lzma), readable and writable. README: [internal/archive/tarfmt/README.md](../internal/archive/tarfmt/README.md). IDs: F-160, F-223, F-228, F-278, F-320, F-482, F-483, F-498, F-523, F-592, F-596, F-598, F-604, F-605, F-612 to F-621, F-625, F-628, F-847.

### internal/archive/sevenrar
Read-only 7z and RAR, plus the archive format detector. README: [internal/archive/sevenrar/README.md](../internal/archive/sevenrar/README.md). IDs: F-051, F-071, F-078, F-159, F-160, F-228, F-319, F-320, F-483, F-528, F-596, F-597, F-606 to F-614, F-620, F-621, F-626, F-847.

### internal/archive/arcapi
Shared API types for the archive sub-packages (no README, no tests of its own).

## Remote file systems

### internal/remotefs
Shared `FS` interface and `Entry` type that FTP, SFTP and WebDAV implement, so the VFS needs one adapter. README: [internal/remotefs/README.md](../internal/remotefs/README.md). IDs: F-629, F-645, F-650.

### internal/ftp
FTP/FTPS client, connection store and pool. README: [internal/ftp/README.md](../internal/ftp/README.md). IDs: F-629 to F-631, F-634 to F-643, F-648 to F-650.

### internal/sftp
SFTP and SCP client with password, key, agent and keyboard-interactive auth and known_hosts handling. README: [internal/sftp/README.md](../internal/sftp/README.md). IDs: F-632, F-633, F-636 to F-638, F-641, F-642, F-648 to F-650.

### internal/webdav
Standard-library WebDAV client implementing `remotefs.FS`. README: [internal/webdav/README.md](../internal/webdav/README.md). IDs: F-645.

## Panel listing, selection and navigation

### internal/listing
Turns raw folder listings into panel rows: filtering, sorting, formatting, column sets, status line totals, descript.ion comments. README: [internal/listing/README.md](../internal/listing/README.md). IDs: F-079, F-080, F-084 to F-094, F-098, F-102, F-107 to F-117, F-665 to F-668, F-670 to F-674, F-785, F-787, F-788, F-790.

### internal/masks
Wildcard masks and the selection engine (select/unselect groups, invert, restore, compare helpers, clipboard selection). README: [internal/masks/README.md](../internal/masks/README.md). IDs: F-068, F-088, F-089, F-097, F-118 to F-143, F-250, F-428, F-744, F-780, F-781, F-788, F-791 to F-797, F-801.

### internal/dirsize
Folder sizes, count-all-folders, selection size and volume details. README: [internal/dirsize/README.md](../internal/dirsize/README.md). IDs: F-016, F-058, F-091, F-093, F-118, F-183, F-244, F-264, F-325, F-491, F-681, F-709, F-714, F-797.

### internal/fswatch
Automatic panel refresh when files change on disk. README: [internal/fswatch/README.md](../internal/fswatch/README.md). IDs: F-064, F-065, F-147, F-422.

### internal/thumbs
Thumbnail generator, disk cache, worker pool and fallback icons. README: [internal/thumbs/README.md](../internal/thumbs/README.md). IDs: F-675 to F-680.

## File operations around the queue

### internal/multirename
Multi-Rename Tool: masks, preview, conflicts, rename with rollback, undo. README: [internal/multirename/README.md](../internal/multirename/README.md). IDs: F-504 to F-519, F-834 to F-836.

### internal/attrs
Change attributes and Properties backend (attributes, times, owner, permissions) as a plan that is previewed and applied. README: [internal/attrs/README.md](../internal/attrs/README.md). IDs: F-080, F-140, F-184, F-194, F-263, F-315 to F-317, F-324, F-338, F-340, F-674, F-681 to F-686, F-815.

### internal/links
Symbolic links, hard links, junctions, shortcut files (.lnk, .url, .desktop, .webloc), chain resolution and loop detection. README: [internal/links/README.md](../internal/links/README.md). IDs: F-075, F-076, F-178, F-179, F-324, F-337, F-476, F-535, F-722, F-738, F-746.

### internal/fileutil
Checksums (CRC32, MD5, SHA family, BLAKE), verify, split and combine, encode and decode. README: [internal/fileutil/README.md](../internal/fileutil/README.md). IDs: F-328 to F-333, F-687 to F-693, F-763.

### internal/clipfiles
Copy, cut and paste files and names through the OS clipboard. README: [internal/clipfiles/README.md](../internal/clipfiles/README.md). IDs: F-132, F-138 to F-142, F-237, F-349, F-350, F-739, F-740, F-796, F-835, F-838.

## Search, compare, view

### internal/find
Find Files engine (masks, start folders, content search, archives, encodings, results). README: [internal/find/README.md](../internal/find/README.md). IDs: F-524 to F-545, F-837, F-838.

### internal/dircompare
Compare directories (Shift+F2), with and without subfolders. README: [internal/dircompare/README.md](../internal/dircompare/README.md). IDs: F-133, F-134, F-212, F-321, F-351, F-352.

### internal/syncdirs
Synchronize Directories: compare, categorise, edit actions, run with progress. README: [internal/syncdirs/README.md](../internal/syncdirs/README.md). IDs: F-546 to F-555, F-562.

### internal/compare
Compare by content (text and binary diff, merge operations, encoding detection). README: [internal/compare/README.md](../internal/compare/README.md). IDs: F-556 to F-563, F-565, F-570, F-839.

### internal/lister
Lister backend (F3 viewer, Quick View): offset-based text, hex and binary modes, search, code pages. README: [internal/lister/README.md](../internal/lister/README.md). IDs: F-564 to F-573, F-575 to F-584, F-586 to F-590, F-840 to F-842, F-846.

## Launching, commands, configuration

### internal/launch
Open with default app, Open with, run with parameters, external viewer and editor, terminal here, reveal in file manager, associations. README: [internal/launch/README.md](../internal/launch/README.md). IDs: F-051, F-075, F-153, F-196, F-199, F-200, F-214, F-232, F-265, F-292 to F-298, F-301 to F-303, F-310, F-323, F-335, F-363, F-366, F-371, F-420, F-590 to F-593, F-595, F-703, F-708, F-738, F-812 to F-814.

### internal/params
Command line parsing, placeholders (%P, %N, ...), start-up switches, history and completion logic. README: [internal/params/README.md](../internal/params/README.md). IDs: F-292 to F-298, F-301, F-302, F-305, F-307, F-310, F-311, F-448 to F-450, F-728, F-734, F-735, F-812 to F-814, F-852.

### internal/config
wincmd.ini-compatible settings engine with typed models for configuration, hotlist, Start menu, button bar and hotkeys. README: [internal/config/README.md](../internal/config/README.md). IDs: F-8, F-16, F-304, F-306, F-308, F-311, F-313, F-314, F-412, F-440, F-444, F-447 to F-450, F-652 to F-668, F-674, F-814, F-820, F-828, F-849.

## Specs

Detailed Total Commander behaviour specs: `docs/spec/area-1.md` to `docs/spec/area-8.md`.
