# internal/webdav — WebDAV client

Small standard-library WebDAV client (RFC 4918 subset) implementing
`remotefs.FS`, so a WebDAV share can be shown in a panel like an SFTP
connection. Backs F-645 ("WebDAV ... later built-in feature"); in TC this is
the WebDAV file system plugin.

```go
c, err := webdav.New(webdav.Config{
    URL: "https://cloud.example.org/remote.php/dav/files/bob/", // also webdav://, davs://
    User: "bob", Password: pw,     // or user:pw@ in the URL
})
err = c.Ping()                      // checks reachability + credentials
entries, _ := c.ReadDir("/")        // PROPFIND Depth 1 -> []remotefs.Entry
c.Stat(p); c.Mkdir(p); c.MkdirAll(p)
c.Remove(p)                         // file or empty folder
c.RemoveAll(p)                      // recursive DELETE
c.Rename(a, b)                      // MOVE, fs.ErrExist if b exists
c.RenameOverwrite(a, b)
c.Copy(a, b, overwrite)             // server-side COPY (files and folders)
r, _ := c.Open(p); w, _ := c.Create(p)
c.Download(ctx, remote, local, opts) // Range-based resume
c.Upload(ctx, local, remote, opts)   // Resume -> ErrResumeUnsupported unless complete
```

Paths are share-relative (`/` = the configured URL). HTTP errors are
`*StatusError` and match `fs.ErrNotExist` (404/410), `fs.ErrPermission`
(401/403) and `fs.ErrExist` (412/405). Upload cannot preserve times or modes
(WebDAV has no standard way); download preserves the modification time.

Tests run `golang.org/x/net/webdav`'s server in-process on 127.0.0.1.
