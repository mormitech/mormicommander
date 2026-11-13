# internal/vfs

The file systems behind the panels. A panel never touches the disk or the
network itself; it asks an `FS` (`vfs.go`): `List`, `Stat`, `Clean`,
`Parent`, `Root`, `Join`. `Local` is the disk and `Mock` the demo tree. The
adapter files plug the archive, FTP and SFTP engines in, and `Router` picks
the right one for any path, so the UI needs a single object.

## Router (router.go, router_local.go)

```go
r := vfs.NewRouter()           // LocalRW + ArchiveFS + FTPFS + SFTPFS
defer r.Close()                // disconnects, closes archives, removes temp files
r.FTP.Configure  = func(c *ftp.Config) error  { /* password, TLS, proxy ... */ }
r.SFTP.Configure = func(c *sftp.Config) error { /* host key prompt, keys ... */ }
r.Archives.Password = func(req vfs.PasswordRequest) (string, bool) { /* dialog */ }

path, err := r.Connect(ctx, "ftp://peter:pw@host/pub") // path bar / Ctrl+F; never returns the password
l, err := r.List(ctx, path)
r.CanEnter(dir, entry)         // Enter: folders, links to folders, archive extensions
r.Kind(p)                      // KindLocal, KindArchive, KindFTP, KindSFTP, KindOther, KindUnknown
fsys, clean := r.Resolve(p)    // the owning FS and canonical path
r.Disconnect(p)                // closes the server and forgets archives downloaded from it
r.RegisterSchemes()            // vfs.For("ftp://...") now returns the router
```

Routing rules:

- `ftp://`, `ftps://`, `ftpes://` go to `FTPFS`; `sftp://`, `scp://` to
  `SFTPFS`; other schemes to an FS registered with `Register`, else an FS
  that answers `ErrUnsupported`.
- A local path that exists is local. A missing one whose first existing
  ancestor is an archive file is inside that archive (`C:\x\a.zip\docs`).
- The archive file itself is a file for every call except `List`, which shows
  its root (so Enter and Backspace work without special cases).
- Archives on servers are downloaded to a temp file when entered (only names
  that look like archives) and browsed read-only; entering again re-checks
  the server file's size and date.

`LocalRW` adds the optional operations to `Local` (direct `os` calls; trash
and big copy jobs stay in `internal/fileops` / `internal/trash`).

## Optional operations (adapter_ops.go)

Backends implement what they can; callers use the helpers, which return
`ErrUnsupported` otherwise. `Router` implements all of them by delegation.

| Interface | Methods | Helper |
|---|---|---|
| `Opener` | `Open(ctx, p)` | `OpenFile` |
| `Creator` | `Create(ctx, p)` (commit on `Close`) | `CreateFile` |
| `DirMaker` | `Mkdir(ctx, p)` (parents too; existing = `fs.ErrExist`) | `Mkdir` |
| `Remover` | `Remove` (file / empty folder), `RemoveAll` | `Remove`, `RemoveAll` |
| `Renamer` | `Rename(ctx, from, to)` (existing target = `fs.ErrExist`, other FS = `ErrCrossFS`) | `Rename` |
| `AttrSetter` | `Chmod`, `Chtimes` | `Chmod`, `Chtimes` |
| `Transferer` | `Download(ctx, p, local, opt)`, `Upload(ctx, local, p, opt)` with `Resume`, `PreserveTime`, `Progress` | |
| `Packer` | `AddLocal(ctx, dir, sources)` (one archive rewrite) | |
| `Capper` | `Caps(p)` → `Read Write Mkdir Delete Rename Chmod Chtimes Remote Archive` | `CapsOf` |

`CopyFile(ctx, srcFS, src, dstFS, dst, local, opt)` copies one file between
any two backends: native download/upload when one side is the local disk,
otherwise an Open/Create stream. Errors: `ErrUnsupported`, `ErrReadOnly`,
`ErrCrossFS`, `ErrNotEmpty`, `ErrIsFolder`, `ErrNotFolder`; missing paths
match `fs.ErrNotExist` on every backend.

## ArchiveFS (adapter_archive.go, adapter_archive_backends.go)

- Engines: `ziparc` (zip family, read/write; spanned sets read-only),
  `tarfmt` (tar, tgz, tbz, txz, tzst, tlz read/write; single-file gz/bz2/xz/
  zst/lzma read-only), `sevenrar` (7z and RAR, read-only). The engine is chosen
  by content (`sevenrar.DetectFile`), then by name.
- Nested archives (`a.zip\old\src.tar.gz\main.go`) are unpacked to a temp
  file once and browsed read-only; a change of the outer archive voids them.
- Opened archives are cached (`MaxOpen`, default 8) and reopened when the
  file's size or date changes; readers keep their archive open while in use.
- Writes (`Mkdir`, `Create`, `Upload`, `AddLocal`, `Remove`, `RemoveAll`,
  `Rename`) go through the engine's atomic rewrite (one rewrite per call,
  also for tar), under a per-archive write lock so concurrent writers to the
  same archive never drop each other's changes.
- `IsArchive(name)` (Enter; `DefaultEnterExtensions` or `EnterExtensions`),
  `Split(p)`, `Info(p)` (`Format`, `Root`, `File`, `Inner`, `ReadOnly`,
  `Nested`), `Mount`/`Unmount`/`Mounted`, `Password`, `Encoding`, `TempDir`.
- Test (Alt+Shift+F9), extract with dialogs (Alt+F9) and packing with options
  (Alt+F5) use the engine packages directly with `Info(p).File`.

## FTPFS (adapter_ftp.go) and SFTPFS (adapter_sftp.go)

- One session per scheme+user+host+port, shared by all panels. FTP keeps a
  pool (`MaxConns`, default 2) so listing works while a file is read; SFTP
  multiplexes one SSH connection. Dead connections are redialled and the call
  retried once.
- `Connect(ctx, url)`, `ConnectConfig(ctx, cfg)` (connection manager),
  `Disconnect(p)`, `Sessions()`, `Close()`; FTP also `CloseIdle(d)`.
- `Configure` fills in what a URL cannot carry. Without it, SFTP rejects
  unknown host keys.
- FTP: binary streams for `Open`/`Create`, the session's transfer mode
  (ASCII/auto) for `Download`/`Upload`, `REST` resume, `SITE CHMOD`, `MFMT`
  (`Caps().Chtimes`), link targets checked with CWD (`NoLinkResolve` to skip).
- SFTP: `/~/dir` URLs are home-relative, missing user = local login name,
  links resolved (broken ones flagged), resume both ways.
- Panel URLs are canonical: lower-case scheme and host, default port dropped,
  cleaned raw path (not percent-encoded), never the password.

## Tests

`go test ./internal/vfs/` builds archives with the engines in `t.TempDir()`
and runs FTP and SFTP against in-process servers on 127.0.0.1
(`adapter_ftp_server_test.go`, `adapter_sftp_server_test.go`, copies of the
engine packages' test servers).

## FEATURES.md IDs backed

F-051 and F-078 (Enter / Ctrl+PgDn on archives: `CanEnter`, `List` of an
archive file), F-071 (archive and FTP paths in the path bar), F-482 and F-615
(copy into archive: `Upload`, `AddLocal`, `CopyFile`), F-483 (copy out:
`Download`, `Open`), F-596 (archive as folder), F-597 and F-611 (content
detection: any archive path opens, `.jar`/`.docx` by explicit path), F-600,
F-604, F-605, F-606, F-608 (browse zip/tar family/single-file packers/7z/RAR
through one FS), F-617, F-618, F-619 (delete, rename, make folder in archive),
F-620 (nested archives), F-626 (archive folders for branch view and find:
`List` on archive paths), F-629 (FTP panel operations), F-633 (SFTP chmod,
links, resume), F-636 (quick connect by URL), F-641 (`AddLocal` uploads
trees), F-648 (F3/F4 on remote files: `Open`, `Download`, `Upload`), F-649
(remote `Stat`), F-650 (`Disconnect`, `CloseIdle`, `Close`).
