# internal/sftp — SFTP and SCP client

UI-free engine for Total Commander's SFTP connections (FEATURES.md section 30).
Built on `golang.org/x/crypto/ssh` and `github.com/pkg/sftp`. `*Client`
implements `remotefs.FS` (see `internal/remotefs`), so the VFS and job engine
can treat it like any other remote file system.

## FEATURES.md IDs backed

| ID | What this package provides |
|----|----------------------------|
| F-632 | `Dial` with password, private keys (encrypted keys via `Passphrase` / `PassphraseFunc`), SSH agent (`UseAgent`, `AgentSocket`, `Agent`), keyboard-interactive; known_hosts check with `HostKeyPrompt` (unknown / changed / new key type; reject, accept once, accept and save) |
| F-633 | `Chmod`, `ChmodAll`, `Chown`, `Chtimes`, `Symlink`, `Link`, `ReadLink`, link resolution in `ReadDir`; resume via `TransferOptions.Resume` in `Download` / `Upload` |
| F-636 | `ParseURL("sftp://user:pw@host:port/path")` (also `scp://`, `ssh://`, `/~/` = home-relative) and `Config.String()` for display |
| F-637 | `Config.Proxy`: SOCKS5 (user/password), SOCKS4a, HTTP CONNECT (basic auth) |
| F-638 | `CopyRemote` (copy within the server, streamed), `Rename` / `RenameOverwrite` (move within server) |
| F-641 | `UploadTree` / `DownloadTree` keep the folder structure |
| F-642 | `Config.KeepAlive` (keepalive@openssh.com pings; dead link → `Err() == ErrKeepAliveFailed`) |
| F-648 | `Open` (seekable reader for the Lister), `Download`/`Upload` for edit-and-upload |
| F-649 | `Lstat` / `Stat` (`remotefs.Entry`: size, mode, times, uid/gid, link target), `Space` (statvfs) |
| F-650 | `Config.IdleTimeout` (`Err() == ErrIdleTimeout`), `Close`, `Done()` channel |

SCP-only servers (no SFTP subsystem) still connect: `HasSFTP()` is false,
SFTP methods return `ErrNoSFTP`, and `SCPUpload` / `SCPDownload` work.

## API sketch

```go
cfg := sftp.Config{
    Host: "example.org", Port: 22, User: "bob",
    Password: pw,                       // and/or:
    KeyFiles: []string{"~/.ssh/id_ed25519" /* expanded by caller */},
    PassphraseFunc: askPassphrase,      // UI prompt for encrypted keys
    UseAgent: true,
    KnownHostsFiles: nil,               // default ~/.ssh/known_hosts
    HostKeyPrompt: func(i sftp.HostKeyInfo) sftp.HostKeyDecision {
        // show i.Status, i.KeyType, i.Fingerprint, i.KnownKeys
        return sftp.HostKeyAcceptAndSave
    },
    KeepAlive: 60 * time.Second, IdleTimeout: 15 * time.Minute,
    Proxy: &sftp.Proxy{Type: sftp.ProxySOCKS5, Addr: "proxy:1080"},
}
c, err := sftp.Dial(ctx, cfg)          // ctx cancels connect/handshake
defer c.Close()

entries, _ := c.ReadDir(c.StartDir())  // []remotefs.Entry, links resolved
c.Mkdir(p); c.MkdirAll(p); c.Remove(p); c.RemoveAll(ctx, p, onItem)
c.Rename(a, b)                          // fs.ErrExist if b exists; case-only renames allowed
c.RenameOverwrite(a, b)
c.Chmod(p, 0o644); c.Chtimes(p, at, mt); c.Symlink(target, link)
n, err := c.Download(ctx, remote, local, remotefs.TransferOptions{
    Resume: true, PreserveTimes: true, Progress: func(p remotefs.Progress) {...},
})
c.UploadTree(ctx, localDir, remoteDir, sftp.TreeOptions{OnFile: ..., SkipExisting: true})
c.Walk(ctx, root, fn); c.TreeSize(ctx, root)
res, _ := c.RunCommand(ctx, "uptime")   // stdout, stderr, exit code
c.SCPUpload(ctx, local, remoteTarget, sftp.SCPOptions{Preserve: true})
```

Helpers: `AddKnownHost`, `RemoveKnownHost` (handles hashed entries),
`ListAgentKeys`.

## Behaviour notes for the UI

- Errors keep `errors.Is(err, fs.ErrNotExist / fs.ErrExist / fs.ErrPermission)`.
- A cancelled transfer leaves its partial destination so it can be resumed;
  the job engine deletes it if the user does not want incomplete files.
- `Dial` pauses its handshake timeout while `HostKeyPrompt` waits for the user.
- Without `HostKeyPrompt`, unknown and changed keys are rejected
  (`*HostKeyError` carries the details for a later dialog).
- `@revoked` keys in known_hosts fail with `ErrHostKeyRevoked` and are never
  offered to the prompt.
- SCP download refuses server-sent names containing `/`, `\` or `..`, names
  that are not valid local names on this OS (`CON`, `x:stream` on Windows),
  and any top-level item other than the one requested (CVE-2019-6111).
- `DownloadTree` refuses server names that would not stay inside the local
  folder on this OS (e.g. `..\x` on Windows).
- `Rename` / `RenameOverwrite` allow changing only the letter case on
  case-insensitive servers (Windows OpenSSH, macOS).
- Agent transport: `$SSH_AUTH_SOCK` Unix socket (macOS/Linux), the
  `\\.\pipe\openssh-ssh-agent` named pipe on Windows.

## Tests

`go test ./internal/sftp/` starts a real SSH server on 127.0.0.1 inside the
test (password/key/keyboard-interactive auth, pkg/sftp subsystem over
`t.TempDir()`, a Go implementation of the SCP server side, keep-alive replies)
plus SOCKS4/5 and HTTP CONNECT proxies. No system ssh/scp binary is used.
