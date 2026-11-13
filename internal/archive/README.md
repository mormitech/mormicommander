# internal/archive

The format-neutral packer layer: what the panel, copy jobs and dialogs call
to treat an archive like a folder. It has no UI code. Concrete formats live in
sub-packages and register themselves; import them for their side effect:

```go
import _ "mormicommander/internal/archive/ziparc"
```

## Interfaces (archive.go)

- `Archive` is the read side every format implements: `Format`, `Path`,
  `Comment`, `Entries`, `ReadDir(dir)`, `Stat(path)`, `Open(entry)`, `Close`.
  Paths are clean slash paths with no leading slash, and `""` is the root.
  `Open` checks the CRC/HMAC when the reader reaches EOF, so a damaged file
  shows up as a `Read` error wrapping `ErrChecksum`.
- `Entry` holds what the panel and the "Info on packed file" dialog show:
  Path/Name/RawName, IsDir/Implied (folder with no record of its own),
  Size/PackedSize/`Ratio()`, ModTime, Mode (including symlink bits), DOSAttrs
  (`AttrReadOnly`...), CRC32/HasCRC, Method, Encrypted/Encryption, Comment, and
  Insecure (the stored name was absolute or had `..` and has been sanitized).
- `Packer.Update(ctx, archivePath, Changes, PackOptions)` applies a batch of
  Delete / Rename / Mkdir / Add / comment changes atomically. It writes a temp
  file and renames it over the original, so a cancel or error never damages
  the archive. Creating an archive is an Update on a path that does not exist.
- `PackOptions` maps onto the Pack dialog: Method/Level, Password/Encryption,
  FlattenPaths ("pack path names" off), NoRecurse, StoreSymlinks, ExcludeMask,
  VolumeSize (multi-volume), MoveSources, Policy/OnConflict, Progress.
- `OpenOptions`: `Password` callback (asked again after a wrong password; the
  first good one is remembered) and `Encoding` for names in old non-UTF-8
  archives (`"auto"`, `"cp852"`, `"cp437"`, `"shift_jis"`...; see
  `Encodings()`).

## Registry (registry.go)

`Register(Format)`, `Formats()`, `Lookup(name)`, `ByExtension(name)`,
`IsArchiveName(name)` (what Enter uses), `Detect(path)` / `DetectReaderAt`
(magic bytes, then a deeper probe, then the extension; this is Ctrl+PgDn), and
`Open(path, opts)`. `Format.Caps` tells the Pack dialog which options to
enable.

## Generic operations

| Function | Use |
|---|---|
| `Extract(ctx, a, ExtractOptions)` | F5 out of an archive, Alt+F9 unpack. Takes selected `Paths` relative to the panel's `Base` folder, `NoPaths`, `Mask`, `Policy`/`OnConflict` (the overwrite dialog), `OnError` (skip or abort), `Symlinks`, `Progress`. Zip-slip safe: `SafeJoin` plus resolving links in parent folders, links created last and only when they point inside the target, temp file + rename for every file. Returns `ExtractResult` (written, skipped, failed). |
| `Verify(ctx, a, VerifyOptions)` | Alt+Shift+F9 test archive. Returns a `VerifyReport` with the failed entries. |
| `OpenNested(ctx, outer, entry, opts, tempDir)` | Enter on an archive inside an archive. Stored zips open in place through `Sectioner`; anything else is copied to a temp file that `Close` removes. |
| `FS(a)` | An `io/fs` view (`fs.WalkDir` for find-in-archive and the branch view). |
| `ItemsFrom(a, entries, base, destDir)` / `Convert(ctx, a, format, dst, opts)` | Copy from one archive panel into another archive, or convert between formats. |
| `Selection(a, paths, base)` | Expand a panel selection (folders bring their subtree). |
| `CollectSources` / `RemoveSources` | Shared by packers: expand local files with the Pack dialog switches, and implement "move to archive". |
| `ParseMask(s).Match(name)` | Total Commander masks: `*.txt *.doc | old*`. |
| `CleanPath`, `SafeJoin`, `SanitizeName`, `FreeName` | Path helpers (`FreeName` gives "name (2).ext"). |
| `NewConflictResolver(policy, fn).Resolve(...)` | Overwrite dialog state that remembers "all" answers, for packers. |
| `NewTracker` / `CopyContext` | Progress and cancel-aware copying. |

Errors to test with `errors.Is`: `ErrFormat`, `ErrPasswordRequired`,
`ErrBadPassword`, `ErrChecksum`, `ErrUnsupportedMethod`, `ErrInsecurePath`,
`ErrReadOnly`, `ErrMissingVolume`, `ErrCanceled`, `ErrNotFound`
(= fs.ErrNotExist), `ErrExist` (= fs.ErrExist). A `PasswordFunc` or
`ConflictFunc` returning or choosing cancel stops the job with `ErrCanceled`.
A cancelled context returns `ctx.Err()`.

Platform splits: `platform_windows.go` / `platform_other.go` (file name
sanitizing, hidden/system attributes), `oemcp_windows.go` (GetOEMCP) and
`oemcp_other.go` (OEM code page guessed from the locale).

## Writing a new format

Implement `Archive`. Building an `Index` with `NewIndex(records)` gives you
`Entries`, `ReadDir` and `Stat`. Fill in `RawName` and the data fields and the
index cleans paths and adds implied folders. If the format can write, implement
`Packer`, reuse `CollectSources`, `NewConflictResolver`, `NewTracker` and
`RemoveSources`, and write atomically. Then call `Register` from `init`.
Extract, Verify, OpenNested, FS and Convert work without further code.

## FEATURES.md IDs backed (section 29)

F-596 (archive as folder: Index/ReadDir/Stat), F-597 and F-611 (Detect by
content), F-598 and F-625 (PackOptions), F-612 and F-613 (Extract; the UI
loops over the selected archives and can use `Path`-derived subfolders),
F-614 (Verify), F-615 (Update with Add or ItemsFrom), F-616 (MoveSources),
F-617/F-618/F-619 (Delete/Rename/Mkdir changes), F-620 (OpenNested), F-621
(Unicode and long paths), F-622 (comments), F-626 (FS), F-628 (Convert),
F-847 (Entry info fields), F-848 (OpenOptions.Encoding). The zip specifics
are in `ziparc/README.md`.
