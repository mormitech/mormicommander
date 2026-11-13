# internal/remotefs — shared remote file system types

Tiny, dependency-free package that the remote clients share so the VFS layer
can wire any of them with one adapter:

- `FS` interface: `ReadDir`, `Stat`, `Mkdir`, `Remove`, `Rename`, `Open`,
  `Create`, `Download(ctx, …)`, `Upload(ctx, …)`, `Close`.
- `Entry`: name, path, size, mode, times, uid/gid, `IsDir` (true for links to
  folders), `IsLink`, `LinkTarget`, `LinkBroken`; `Entry.Info()` adapts it to
  `fs.FileInfo`.
- `TransferOptions` (`Resume`, `NoOverwrite`, `PreserveTimes`,
  `PreserveMode`, `Progress`) and `Progress{Path, Done, Total}`.
- `ErrExists`, `ErrResumeUnsupported`.
- `ProgressWriter` / `ProgressReader`: count bytes, report progress, stop on
  context cancellation.
- `Join`: slash-only path join for remote paths.

Implemented by `internal/sftp.Client` and `internal/webdav.Client`; the FTP
client can adopt it too. Backs the "remote panel" side of FEATURES.md
section 30 (F-629…F-650) and F-645.
