# internal/ftp

The FTP engine behind Total Commander's "FTP connect" (Ctrl+F) and "FTP new
connection" (Ctrl+N). It is UI-free and knows nothing about the panel VFS:
the VFS layer wraps a `*Client`, the connection-manager dialog edits a
`*Store`, and the transfer queue uses a `*Pool`. Standard library only.

## FEATURES.md IDs backed

| ID    | What this package provides |
|-------|----------------------------|
| F-629 | `Dial`, `List`, `Cd`/`CdUp`/`Pwd`, `Download`/`Upload`, `Delete`, `Rmdir`, `RemoveAll`, `Rename`, `Mkdir`/`MkdirAll`, `Chmod` (SITE CHMOD) |
| F-630 | `Config.DataMode` (`Passive`/`Active`), EPSV/PASV/EPRT/PORT, `PassiveHost` NAT fix, `ActiveIP`, active port range |
| F-631 | `Config.Security`: `SecurityExplicitTLS` (AUTH TLS), `SecurityExplicitTLSIfAvailable`, `SecurityImplicitTLS` (port 990); PBSZ/PROT P, TLS session reuse on data connections |
| F-634 | `Store` (folders, sites, named proxies, JSON `LoadStore`/`Save`), `SecretBox` interface (keychain-backed boxes plug in here, `KeyRefPrefix`), `ImportWcxFtpINI` |
| F-635 | `Store.SetMasterPassword`/`Unlock`/`ChangeMasterPassword`, `MasterKey` (PBKDF2-SHA256 + AES-GCM); passwords are never stored in plain text |
| F-636 | `IsURL`, `ParseURL`, `FormatURL` (password masked), `SiteFromURL` (ftp://, ftps://, ftpes://) |
| F-637 | `Proxy`: SOCKS4/4A/5, HTTP CONNECT, and FTP-style firewalls (USER user@host, SITE host, OPEN host) |
| F-638 | `Rename` (server-side move), `ServerCopy` (SITE CPFR/CPTO) |
| F-639 | `Config.ServerLocation` + `Config.TimeOffset`, `ParseOptions` |
| F-640 | `Config.Encoding` (`EncodingAuto` negotiates OPTS UTF8 ON, `EncodingUTF8`, `EncodingLatin1`) |
| F-641 | `UploadTree` / `DownloadTree` / `Walk` (same folder structure) |
| F-642 | `Config.Keepalive`, `StartKeepalive`/`StopKeepalive`, `NoOp` |
| F-643 | `Pool` (bounded parallel connections for the transfer queue) |
| F-649 | `Stat`, `Size`, `ModTime`, `SetModTime`, `Entry` (mode, owner, group, link target, MLSD facts) |
| F-650 | `Close` (QUIT), `IdleFor`, `Pool.CloseIdle` for idle timeouts |

F-648 (edit/view remote file) is built on `Download` + `Upload` by the UI.

## Connecting

```go
cfg, path, err := ftp.ParseURL("ftp://peter@example.com:2121/pub")
cfg.Password = askPassword()
cfg.Security = ftp.SecurityExplicitTLSIfAvailable
cfg.Keepalive = 60 * time.Second
cfg.Logf = logWindow.Printf // protocol log, passwords masked
c, err := ftp.Dial(ctx, cfg)
defer c.Close()
```

Empty `User`, `anonymous` or `ftp` logs in anonymously. `Config.InitialDir`
is entered right after login. `c.Welcome()`, `c.System()`, `c.Features()`
and `c.Secure()` describe the session. Errors with an FTP reply carry it in
`*ftp.Error` (`ftp.ReplyCode(err)`, `Temporary()` for 4xx); missing paths
match `fs.ErrNotExist`. Every method takes a `context.Context`; cancelling
it aborts the pending reply or transfer; a stream closed early (closing the
data connection) leaves the control connection usable.
`c.Broken()` reports a dead connection so the VFS can reconnect.

## Listing

`List(ctx, dir)` uses MLSD when offered (unless `DisableMLSD`) and falls back
to `ListCommand` (default `LIST`). Lines are parsed by `ParseLine`, trying
MLSx, UNIX ls (incl. numeric-uid and no-group variants, device files,
symlinks), DOS/IIS, VMS, EPLF, OS/400 and finally a bare-name fallback.
`Entry.Format` tells which dialect matched, `Entry.Raw` keeps the line.
`ListRaw` and `NameList` (NLST) give the unparsed views.

## Transfers

* `Download(ctx, remote, local, TransferOptions{Resume, PreserveTime, Progress, Mode})`
* `Upload(ctx, local, remote, ...)`, `Append` (APPE)
* `Retrieve(ctx, path, offset, mode)` / `Store(ctx, path, r, offset, mode)`
  for streaming (the VFS reader/writer and the lister).
* Modes: `TransferBinary`, `TransferASCII`, `TransferAuto` (by
  `ASCIIMasks`, default `DefaultASCIIMasks`; `IsASCIIName`,
  `TransferTypeFor`). ASCII downloads convert CRLF to the local newline
  (LF on macOS/Linux, unchanged on Windows). Resume uses REST and is refused
  in ASCII mode (`ErrResumeASCII`).

## Connection manager

`Store` holds `Folders`, `Sites` and `Proxies`. Folders are slash paths
("Work/Clients"). CRUD: `Add`, `Update`, `Remove`, `Duplicate`, `Move`,
`AddFolder`, `RenameFolder`, `RemoveFolder`, `Children`, `Find`, `Get`.
Passwords go through `SetPassword`/`Password` with a `SecretBox`; with a
master password set, `Unlock(password)` returns the `*MasterKey` box and
wrong input gives `ErrWrongMasterPass`; reading a password without a box
gives `ErrLocked`. `Store.Config(id, box)` builds a ready-to-dial `Config`
including the site's named proxy. SFTP sites are stored here too
(`Protocol`, `KeyFile`) but dialled by `internal/sftp`.

## Testing

All tests run against an in-process FTP server on 127.0.0.1
(`server_test.go`), including TLS (self-signed cert generated in the test),
active mode, proxies (in-test SOCKS/HTTP proxies) and resume. Files are
only written below `t.TempDir()`.
