# internal/archive/sevenrar

Read-only 7z and RAR engine plus the archive format detector. Pure Go, no
external programs. UI-free; everything is plain Go calls the UI agents can wire
to panels, dialogs and the file-operation job engine.

Libraries: `github.com/bodgit/sevenzip` (7z), `github.com/nwaples/rardecode/v2`
(RAR 1.5-7), plus `github.com/ulikunitz/xz` and `github.com/klauspost/compress`
for peeking into .txz/.tzst streams in the detector.

## Backs these FEATURES.md items

| ID | What this package provides |
|---|---|
| F-596, F-597, F-078, F-051 | `Open` + `ReadDir`/`Stat`: 7z/RAR shown as a folder; `DetectFile` decides "archive or plain file" by content |
| F-606 | 7z list + extract (LZMA, LZMA2, PPMd, BZip2, Deflate, BCJ/BCJ2/ARM/…, zstd, brotli, lz4, AES-256, split .7z.001) |
| F-608 | RAR list + extract (RAR 4 and RAR 5, solid, multi-volume, encrypted contents and file names, links) |
| F-611 | `Detect` sniffs content: renamed archives, jar/apk/docx/epub reported as zip |
| F-609, F-610 | detector reports ARJ, LZH, CAB, ISO 9660/UDF (report only, not opened here) |
| F-483, F-612, F-613, F-319, F-159 | `Extract`: selection, BaseDir, file mask, flatten paths, overwrite modes, rename, progress, cancel |
| F-614, F-160, F-228, F-320 | `Test`: per-file CRC/integrity report |
| F-071, F-626, F-528 | `ReadDir`/`Walk` for archive paths in the path bar, branch view and find-in-archives |
| F-621 | Unicode and long names; Windows-illegal names made legal on extraction |
| F-847 | `Entry`: size, packed size (RAR), `Ratio()`, date, CRC (7z), encrypted/solid flags |
| F-620 | nested archives: extract the inner archive to a temp file with `Archive.Open`/`Extract`, then `Open` it |

## Detecting

```go
d, err := sevenrar.DetectFile(path)   // content + volume name analysis
// d.Format: Format7z, FormatRar, FormatZip, FormatTar, FormatGzip, FormatBzip2,
//           FormatXz, FormatZstd, FormatLz4, FormatCompress, FormatCab, FormatISO,
//           FormatAr, FormatCpio, FormatArj, FormatLzh, FormatUnknown
// d.Inner == FormatTar for .tgz/.tbz/.txz/.tzst streams
// d.SFX, d.Offset           self-extracting archives (exe stub + archive)
// d.RarVersion              4 or 5
// d.Volume                  {Multi, Number (0-based, -1 unknown), First}
// d.HeadersEncrypted        RAR "encrypt file names": password needed to list
// d.Solid, d.Locked         RAR main header flags
// d.Supported()             true for 7z and RAR (open with this package)
```

`Detect(io.ReaderAt, size)` is the same without the file-name logic.

## Volumes

```go
sevenrar.FirstVolume("x.part3.rar")  // "x.part1.rar"; x.r05 -> x.rar; x.7z.004 -> x.7z.001; x.z02 -> x.zip
sevenrar.FindVolumes("x.r01")        // ["x.rar","x.r00","x.r01"] (stops at first gap)
sevenrar.ArchiveStem("x.part01.rar") // "x"  (folder name for "unpack to separate subfolders")
```

A missing volume gives an error matching `errors.Is(err, ErrMissingVolume)`;
`errors.As(err, &*MissingVolumeError)` yields the expected file name, so the UI
can say which part to fetch (Total Commander's "insert next disk").

## Opening and browsing

```go
a, err := sevenrar.Open(path, sevenrar.Options{
    Password:     "",                       // optional, tried first
    PasswordFunc: func(r sevenrar.PasswordRequest) (string, bool) {
        // r.HeadersEncrypted: needed to list; r.Entry: first encrypted file;
        // r.Attempt > 1: previous password was wrong. Return ok=false on Cancel.
    },
    MaxPasswordAttempts: 3,
})
defer a.Close()

entries, _ := a.ReadDir("")          // root; sorted; implied folders have Synthetic=true
e, _ := a.Stat("docs/readme.txt")    // clean slash paths, "" = root
rc, _ := a.Open("docs/readme.txt")   // F3 / single copy; final Read reports ErrCRC on mismatch
a.Walk("docs", func(e sevenrar.Entry) error { ... })
a.Info()                             // counts, sizes, volumes, solid, encrypted, unsafe count
a.Entries()                          // raw list in archive order, incl. Unsafe entries
a.SetPassword(pw)                    // if the UI asks itself; returns ErrWrongPassword
```

Password timing follows Total Commander: if the file list is encrypted, the
password is asked in `Open`; if only contents are encrypted, browsing works and
the password is asked on the first `Open`/`Extract`/`Test` that reads encrypted
data, then remembered.

Entries whose names contain `..` are marked `Unsafe`, are not shown by
`ReadDir`/`Walk`, and are never extracted (reported in `Result.Skipped`).
Leading `/`, drive letters and UNC prefixes are stripped; backslashes count as
separators.

## Extracting (Alt+F9 / F5 out of an archive)

```go
res, err := a.Extract(ctx, dest, sevenrar.ExtractOptions{
    Paths:        []string{"docs", "top.bin"}, // empty = everything
    BaseDir:      "",          // archive folder being browsed: paths are made relative to it
    Mask:         "*.txt *.doc | *.bak",       // TC mask; quotes allowed
    FlattenPaths: false,       // TC "unpack path names" unchecked
    Overwrite:    sevenrar.OverwriteAsk,       // All, Skip, Older, Rename, RenameExisting
    OnConflict: func(c sevenrar.Conflict) sevenrar.ConflictDecision {
        return sevenrar.ConflictDecision{Action: sevenrar.ActionOverwrite, ApplyToAll: true}
        // ActionSkip, ActionOverwriteOlder, ActionRename (+NewName), ActionRenameExisting, ActionCancel
    },
    Symlinks:   sevenrar.SymlinksSafe, // or SymlinksSkip, SymlinksAsFiles
    KeepBroken: false,                 // keep files that failed the CRC check
    OnError:    func(e *sevenrar.EntryError) bool { return true }, // false = abort
    Progress:   func(p sevenrar.Progress) { ... },                 // per 256 KiB chunk
})
// res.Files/Dirs/Links/Bytes, res.Skipped (unsafe names, existing files, refused links),
// res.Errors (per entry). err: ctx.Err() on cancel, ErrPasswordCancelled,
// ErrMissingVolume, or the per-entry errors joined (errors.Is(err, ErrCRC) ...).
```

Guarantees:

- nothing is written outside `dest`; nothing is written through an existing
  symbolic link below `dest`; an existing link at a target path is replaced,
  never followed;
- each file is written to a temp file and renamed into place after its
  checksum matched; cancel or failure never leaves partial files and never
  destroys the file it would have replaced;
- stored symlinks are created last, only with relative targets that stay
  inside `dest` (leading `..` only, no foreign links on the way); RAR hard
  links/file copies are linked/copied from files extracted in the same run;
- permissions are restored without setuid/setgid/sticky and without group/other
  write; modification times are restored for files and folders;
- on Windows, names are made legal (`<>:"|?*` -> `_`, trailing dots/spaces
  removed, `CON`/`LPT1`… prefixed with `_`).

Without `OnConflict`, `OverwriteAsk` skips existing files.

## Testing (Alt+Shift+F9)

```go
tr, err := a.Test(ctx, func(p sevenrar.Progress) { ... })
tr.OK(); tr.Files; tr.Bytes; tr.Errors // []*EntryError, e.g. ErrCRC on "f.txt"
```

## Errors

`ErrUnsupportedFormat`, `ErrPasswordRequired`, `ErrWrongPassword`,
`ErrPasswordCancelled`, `ErrUnsafePath`, `ErrCRC` (for encrypted entries usually
a wrong password), `ErrNotFound`, `ErrNotDir`, `ErrIsDir`, `ErrMissingVolume`
(`*MissingVolumeError`), `ErrClosed`, and `*EntryError{Path, Err}`.

## Known limits

- 7z: per-file packed size and RAR compression method are not exposed by the
  libraries (`PackedSize` is -1 for 7z). A 7z symlink's target is only known
  when it is extracted (`LinkTarget` is empty in listings).
- 7z encrypted-contents detection probes the first stream (7-Zip encrypts all
  streams with one password).
- RAR 4 archives with encrypted file names can be detected and refused without
  a password; RAR 4 has no password check value, so a wrong password shows up
  as `ErrWrongPassword`/`ErrCRC` while reading.
- Read-only: creating 7z/RAR needs external tools (F-607/F-608).

Test archives: `writers_test.go` writes stored 7z/RAR4/RAR5 archives (hostile
names, links, encryption, volumes); compressed and AES 7z samples in
`testdata/7z` come from bodgit/sevenzip (BSD-3, `LICENSE.bodgit-sevenzip`).
