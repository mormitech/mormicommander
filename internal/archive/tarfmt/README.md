# internal/archive/tarfmt

UI-free engine for the tar family: `.tar`, `.tar.gz/.tgz/.taz`, `.tar.bz2/.tbz/.tbz2/.tb2`,
`.tar.xz/.txz`, `.tar.zst/.tzst`, `.tar.lzma/.tlz`, and the single-file packers
`.gz`, `.bz2`, `.xz`, `.zst`, `.lzma`. Everything can be read **and** written
(bzip2 writing via `github.com/dsnet/compress`, xz/lzma via `github.com/ulikunitz/xz`,
zstd via `github.com/klauspost/compress`).

`*tarfmt.Reader` implements the shared `archive.Reader` interface from
`internal/archive` (`Entries`, `Extract`, `Close`).

## Backs FEATURES.md

F-596 (enter archive as folder), F-598 (pack dialog: tar/tgz/tbz/txz/gz/bz2/xz/zstd
packers, level, store paths, recurse, move, prefix), F-604 (TAR create/read with
gzip, bzip2, xz, zstd), F-605 (GZ/BZ2/XZ single-file packers), F-612/F-613 (unpack
dialog: target, mask, with/without paths, overwrite), F-614/F-160/F-228/F-320 (test
archive), F-615/F-482 (copy into archive), F-616/F-223/F-278 (move to archive),
F-617/F-498 (delete in archive), F-618 (rename in archive), F-619/F-523 (make folder
in archive), F-483 (copy out of archive), F-592 (put an edited file back), F-620
(nested archives via `NewReader`), F-621 (long/Unicode names via PAX/GNU headers),
F-625 (pack options), F-628 (convert = Extract + Create), F-847 (info: `Stat`/`Header`).

## Reading

```go
r, err := tarfmt.Open(path)                 // format from magic bytes, then name
defer r.Close()
r.Format()                                  // tarfmt.FormatTarGz, ...
es, err := r.Entries(ctx)                   // full flat list, cached; implied folders added
kids, err := r.ReadDir(ctx, "dir/sub")      // children of one folder, sorted ("" = root)
e, err := r.Stat(ctx, "dir/a.txt")
h, err := r.Header(ctx, "dir/link")         // *tar.Header: Linkname, Uname, Uid, PAX...
rc, err := r.OpenEntry(ctx, "dir/a.txt")    // stream one file (viewer, nested archive)
err = r.Test(ctx, progressFn)               // Alt+Shift+F9; also tarfmt.Test(ctx, path, fn)
r2, err := tarfmt.NewReader(readerAt, size, "inner.tgz", modTime) // nested archives
```

Entry paths use `/`, no leading `./` or `/`, no trailing slash. Duplicate members:
the later copy wins (GNU tar semantics). Single-file archives list one entry named
after the gzip header name or the archive name minus its extension; its `Size` is
known for gzip (trailer) and `-1` (unknown) for the others.

Helpers: `DetectFormat(name)`, `IsArchiveName(name)`, `BaseName(name)` ("a.tar.gz" ->
"a", for "unpack to own folder"), `Formats()` (pack dialog list).

## Extracting

```go
err := r.Extract(ctx, names, dest, archive.ExtractOptions{
    Flatten:     false,               // "unpack path names" unchecked -> true
    StripPrefix: "dir/sub",           // copying out of a panel showing archive/dir/sub
    Filter:      func(e archive.Entry) bool { ... }, // file mask
    Conflict:    archive.ConflictAsk, // Ask (default) | Overwrite | Skip | OverwriteOlder | Abort
    OnConflict:  func(c archive.Conflict) archive.ConflictPolicy { ... }, // nil + Ask = Skip
    Progress:    func(p archive.Progress) { ... },
})
```

`names` empty = everything; a folder name selects its subtree. Per-entry problems
(unsafe name, link outside, folder in the way) do not stop the run: they come back
joined as `*archive.EntryError` (check with `errors.As` / `errors.Is(err,
archive.ErrUnsafePath)`). Cancellation returns `ctx.Err()`; corruption returns the
codec/tar error.

Safety: all writes go through `os.Root` on `dest`; names with `..` are refused;
leading `/` is stripped (like GNU tar); symlinks are created only when relative and
resolving inside `dest` (`..` only as a leading run, not deeper than the link's
folder, never through a symlinked parent); hard links only to files unpacked in the
same run (otherwise the data is fetched in a second pass). Files are written to a
temp name and renamed, so cancel never leaves partial files. Modes (minus
setuid/setgid) and mtimes are restored; folder dates are set last. Devices/FIFOs are
skipped. `Progress.Total` is known only if `Entries` was called first;
`ArchiveRead/ArchiveSize` always give a percentage.

## Creating

```go
err := tarfmt.Create(ctx, "/out/x.tar.xz", []string{"/src/a", "/src/b.txt"}, tarfmt.CreateOptions{
    Format:         tarfmt.FormatUnknown, // from the name
    Level:          0,                    // 1..9, 0 = default
    BaseDir:        "/src",               // names relative to this; "" = base names
    Prefix:         "inside/folder",
    NoPaths:        false,                // "pack path names" unchecked -> true
    NoRecurse:      false,                // "recursively pack subdirectories" unchecked -> true
    FollowSymlinks: false,
    RemoveSources:  false,                // move to archive (deletes only after Test passes)
    Progress:       fn,
})
```

Written to a temp file next to the target and renamed at the end; an existing file
of that name is replaced (the UI asks first). Hard links are stored once (Unix).
Single-file formats need exactly one regular file.

## Modifying (rewrite)

```go
err := tarfmt.Modify(ctx, path, tarfmt.Changes{
    Delete: []string{"old.txt", "dir"},
    Rename: []tarfmt.Rename{{From: "a", To: "b/a"}},
    Mkdir:  []string{"new/folder"},
    Add:    []string{"/local/file"}, AddOptions: tarfmt.CreateOptions{Prefix: "dir"}, // replaces same names
    AddTo:  []tarfmt.AddItem{{Source: "/local/x", Dest: "sub/y.txt"}}, // own name per source
    Level:  0, Progress: fn,
})
```

Adding a file under the name of a folder that keeps its contents, or below a
name that stays a file, is refused with an `fs.ErrExist` entry error (as zip
does). An archive path that is a symbolic link is resolved first, so `Modify`
and `Create` update the real archive and keep the link.

One rewrite with the same compression; the original is replaced only on success
(errors and cancel leave it untouched). Single-file formats return
`archive.ErrNotSupported`. Hard-link data survives deleting its first name.
