# internal/dircompare

Engine for Total Commander's **Compare directories** (Shift+F2, Mark > Compare directories, `cm_CompareDirs`,
`cm_CompareDirsWithSubdirs`). It compares two folders by name, date and size (optionally by content) and returns the
names to select in each panel. No UI code; it cross-compiles and tests on its own.

## Backs FEATURES.md

- F-133 Compare directories: select newer / missing (`cm_CompareDirs`, Shift+F2)
- F-134 Compare directories including subdirectories (`cm_CompareDirsWithSubdirs`)
- F-212, F-351 Shift+F2 / menu entry: the engine the command calls (wiring is UI work)
- F-352 Hide equal files after compare: `Result.LeftEqual` / `Result.RightEqual`
- F-547 (shared rules) ignore date, 2 s FAT tolerance, 1 h time zone offset, by content: `Options`, `CompareTimes`
- F-321, F-466, F-552 (shared helper) byte-for-byte comparison: `SameContent`

## API

```go
res, err := dircompare.Compare(ctx, dircompare.OS{}, leftDir, dircompare.OS{}, rightDir, dircompare.Options{
    Subdirs:          dircompare.SubdirsMissing, // SubdirsIgnore (default) | SubdirsMissing | SubdirsRecursive
    Case:             dircompare.CaseAuto,       // CaseAuto (insensitive on Windows/macOS) | CaseSensitive | CaseInsensitive
    TimeTolerance:    2 * time.Second,           // FAT / network shares
    IgnoreHourOffset: true,                      // DST / time zone shifts of exactly 1 h
    IgnoreDate:       false,                     // size (and content) only
    ByContent:        true,                      // equal-size files are compared byte by byte
    IncludeHidden:    true,                      // follow the panel's "show hidden" switch
    Filter:           func(rel string, e dircompare.Entry) bool { return true },
    Progress:         func(p dircompare.Progress) {},
})
res.LeftSelect, res.RightSelect // names to select in each panel (what Shift+F2 does)
res.LeftEqual, res.RightEqual   // names to hide for "hide equal files"
res.Items                       // per-name verdicts (Status, Left/Right Entry, Children when recursive)
res.Flatten(), res.Counts, res.Errors, res.Differ()
```

- `Lister` (`ReadDir`, `Open`, `Join`) is what is compared; `OS` is the local disk. An optional `RealPather`
  (`OS` has it) lets `SubdirsRecursive` detect symlink loops (`ErrSymlinkLoop`); other Listers get a depth limit
  (`ErrTooDeep`). Archives, FTP and SFTP wrap
  their listing code in it, and the two sides may use different Listers.
- `CompareEntries(left, right []Entry, opt)` compares two listings already in memory (no content, no recursion).
- `CompareTimes(a, b, tol, hourOffset)` and `SameContent(ctx, a, b)` expose the two rules for other tools.

## Rules (TC behaviour)

| Situation | Status | Selected |
|---|---|---|
| Only on one side | `LeftOnly` / `RightOnly` | that side |
| Both, one newer (beyond tolerance) | `LeftNewer` / `RightNewer` | the newer side |
| Both, same time, different size or content | `Different` | both |
| File on one side, folder on the other | `TypeMismatch` | both |
| Same time and size, or identical content with `ByContent` | `Equal` | neither |
| Could not be read | `Error` (see `Item.Err`) | both |

Folders: ignored by default; `SubdirsMissing` selects folders present on one side only; `SubdirsRecursive` also
walks folders present on both sides and selects such a folder on each side that has something selected inside it.
Times are truncated to milliseconds before comparing. Names that fold together on a case-sensitive side are not
merged twice: the extra one is reported as a lone entry. Unicode normalization (NFC vs NFD names) is not applied.
Pipes, sockets and devices (`Entry.Special`) are compared by date and size only and never opened; `OS.Open` refuses
them (`ErrNotRegular`).
