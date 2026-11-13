# internal/attrs

Backend for the **Change attributes** dialog (`cm_SetAttrib`) and the
**Properties** window (`cm_Properties`, Alt+Enter / Cmd+I). UI-free; the UI
reads `Caps()` to decide which controls to enable, builds a `Request`, shows
the `Plan` as a preview and calls `Plan.Apply` when the user confirms.

Backs FEATURES.md: **F-681, F-682, F-683, F-684, F-685, F-686** (section 34),
**F-184, F-194, F-315, F-316, F-338, F-324, F-263** and the attribute parts of
**F-080, F-140, F-674, F-815** (attribute column / attributestr, owner and
permissions on macOS/Linux). F-317/F-340 (plugin/EXIF dates) are covered by
the `TimeChange.Func` hook.

## Reading

| API | Use |
|---|---|
| `Stat(path)` / `StatFollow(path)` | `Info`: kind, mode, size, **size on disk**, modified/accessed/changed/created (+`Has*` flags), UID/GID + owner/group names, link count, inode, Windows attributes (`Win`, native on Windows, derived elsewhere), macOS `HiddenFlag`, `Hidden`, symlink `LinkTarget` / `LinkResolved` / `LinkBroken`. Lstat semantics. |
| `ComputeTotals(ctx, paths, progress)` | Files / folders / links / size / size on disk for a folder tree or selection; hard links counted once; cancellable; progress callback every 256 entries. |
| `DescribeType(info)` | `TypeInfo{Kind, Description, MIME, Executable}`: "PNG image", "Mach-O executable", "Python script", "Application bundle", "Symbolic link to folder", ... (extension first, then magic bytes). |
| `ColumnString(info)` | Attribute column text: `-a--` on Windows, `drwxr-xr-x` elsewhere (F-686). |
| `Caps()` | `Capabilities{SettableAttrs, ReportedAttrs, NativeAttrs, UnixPerms, Owner, CreatedTime, ReadCreated, HiddenFlag}`. |
| `Users()`, `Groups()` | `[]Account{Name, ID}` for the owner/group combo boxes (nil on Windows). |
| `OpenSystemProperties(path)` | Explorer property sheet / Finder Get Info / freedesktop FileManager1 `ShowItemProperties` (F-682). On Windows call from the UI thread. |

## Dialog state

- `Summarize([]Info) Selection` gives three-state checkbox values for a
  multiple selection: `PermBit(bit)`, `Attr(a)`, `HiddenFlag()` return
  `On` / `Off` / `Mixed`; plus common `Owner`, `Group`, dates (with `*Mixed`
  flags), `CommonPerm()` for the octal field and file/folder/link counts.
- `PermBits` (dialog order) + `PermBitName`, `DialogAttrs` (r, a, h, s, c, e).
- `PermChangeFromTri(map)`, `WinAttrChangeFromTri(map)` turn final checkbox
  states into a change (Mixed = leave alone); `ChangedOnly(initial, final)`
  keeps only boxes the user touched.

## Changing

```go
req := attrs.Request{
    Paths:     selected,
    Recursive: true,                         // "Process subfolders"
    ApplyTo:   attrs.FilesAndFolders,        // or FilesOnly / FoldersOnly
    Filter:    attrs.ParseMask("*.sh | .git\\"), // TC mask; Filter.Match hook for search templates
    Attrs:     attrs.WinAttrChange{Set: attrs.AttrReadOnly},
    Perm:      attrs.PermChange{Set: 0o100, Clear: 0o002, Expr: "a+X"},
    Owner:     attrs.OwnerChange{Group: "staff"},
    Times: attrs.TimeChange{
        Modified: attrs.NowTime(),               // TC "Current" button: same instant for all
        Accessed: attrs.CopyTime(attrs.FieldModified),
        Created:  attrs.SetTime(t),              // only where Caps().CreatedTime
    },
}
if err := attrs.Validate(req); err != nil { /* show message */ }
plan, err := attrs.MakePlan(ctx, req)   // walks, changes nothing
for _, it := range plan.Items { it.Summary(); it.Changes; it.Notes; it.Err }
res := plan.Apply(ctx, func(done, total int, it *attrs.Item, err error) {})
// res.Changed, res.Failed []Failure{Path, Err}, res.Canceled
```

- Time modes: `SetTime`, `NowTime`, `ShiftTime(d)`, `CopyTime(field)`, or
  `TimeChange.Func(info) (time.Time, ok)` per file (EXIF / date from name).
- Apply runs **deepest first** (removing a folder's x/r bits never blocks its
  contents), continues after single failures, orders owner → permissions →
  dates/attributes, and on Windows clears read-only before writing dates.
- A recursive change that gives the owner r+x back to a folder we can't list
  yet is planned as that folder alone (with a note); Apply changes the folder
  first, then plans and applies its contents (TC's "process subfolders").
- Symbolic links: by default the link itself is changed (times, owner,
  macOS hidden flag); permissions on links are skipped with a note.
  `FollowLinks` changes the targets. Walks never descend through links.
- Unix mapping: **read-only** set = clear all write bits, cleared = add owner
  write; **hidden** = macOS `UF_HIDDEN` (via setattrlist, works on links).
  System/archive are Windows-only; compressed/encrypted are reported only.

## Permission helpers

`FormatPerm` (`rwsr-xr-t`), `FormatMode` (`drwxr-xr-x`), `FormatOctal`
(`4755`), `ParsePerm`, `ParseOctal`, `ParsePermAny`, `ApplyChmod(mode, "u+x,go-w", isDir)`
(supports `ugoa`, `+-=`, `rwxXst`, copy form `g=u`, octal), `ToUnix`/`FromUnix`.
Windows attributes: `WinAttr.String()` (`rahs`), `LongString()` (`rahsce`),
`Names()`, `ParseWinAttr`, `ParseWinAttrChange("+r -h")`.

## Platform files

`platform_unix.go` (shared Unix), `extras_darwin.go` (birth time, chflags,
setattrlist, dscl), `extras_linux.go` (statx birth time), `extras_bsd.go`,
`platform_windows.go` (attributes, SetFileTime, owner SID names, compressed
size rounded to clusters), `platform_other.go`, `sysprops_*.go`.
