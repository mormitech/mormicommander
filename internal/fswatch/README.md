# internal/fswatch — automatic panel refresh engine

UI-free engine that tells the panels when the folder they show has changed on disk, so they reread it without
Ctrl+R. It works like Total Commander's automatic refresh. Only the standard library and `golang.org/x/sys` are used,
with no cgo, so the package cross-compiles everywhere.

Backs: **F-065** (automatic refresh when files change on disk, within about 1 s) and **F-422** (Configuration page
Refresh: auto refresh on/off, pause while the window is in the background, excluded folders). It also gives
**F-064** / **F-147** (Reread source) the changed names, so the cursor can stay on its file, and it gives the UI a
per-folder on/off switch.

## Backends

| Platform | Native backend | Notes |
|---|---|---|
| macOS, FreeBSD, NetBSD, OpenBSD, DragonFly | kqueue | One watch on the folder, plus per-file watches for up to `FileWatchLimit` regular files (default 256), so growing files are seen too. On macOS the watches use `O_EVTONLY`, so a watched volume can still be ejected. |
| Linux | inotify | One instance per folder. Reports exact names. Queue overflow leads to a full reread. |
| Windows | ReadDirectoryChangesW (overlapped) | Watches names, size, write time, attributes and creation time. Overflow leads to a full reread. |
| other | none | Falls back to polling. |

Network drives use **polling** by default (`Options.Network`), because native notifications miss changes made by
the server or by other machines. These are detected as network drives:

- macOS: smbfs, nfs, afpfs, webdav and FUSE mounts.
- Linux: NFS, SMB/CIFS, AFS, Coda, Ceph, 9P and FUSE mounts.
- Windows: UNC paths and `DRIVE_REMOTE` drives.

If a native watch cannot be set up (for example, the system has run out of descriptors or inotify instances), the
folder is polled instead, and `Status.Reason` explains why.

Watches are **not recursive**, because a panel only shows one folder level.

## API

```go
w := fswatch.New(fswatch.Options{
    // all optional; zero value = TC defaults
    Debounce: 200 * time.Millisecond, // quiet period before an Event
    MaxDelay: time.Second,            // a constant stream still refreshes within 1 s
    PollInterval: 2 * time.Second,
    Local:   fswatch.MethodDefault,   // native
    Network: fswatch.MethodDefault,   // poll (or MethodNative / MethodOff)
    Exclude: []string{"/Volumes/*", `\\nas\backup`},
    PauseWhenInactive: true,
})
defer w.Close()

w.Watch(leftPath)   // reference counted: both panels may watch the same folder
w.Watch(rightPath)
for ev := range w.Events() {
    switch ev.Kind {
    case fswatch.Changed: // reread ev.Path; ev.Names = changed entry names (hint), ev.Full = names unknown
    case fswatch.Gone:    // folder deleted/renamed/unmounted: go to the nearest existing parent, Unwatch
    case fswatch.Error:   // e.g. a share could not be polled once; show in status line
    }
}
w.Unwatch(oldPath)                 // when a panel leaves a folder
w.SetEnabled(false)                // global "refresh automatically" checkbox
w.SetPathDisabled(path, true)      // per-folder switch (remembered; DisabledPaths() to save it)
w.SetExclude(patterns)             // exclusion list from the Refresh page
w.SetMethods(local, network)       // method per drive kind
w.SetActive(windowFocused)         // with PauseWhenInactive, holds Events until active again
st := w.Status(path)               // Watched, Active, Method, Network, Disabled, Excluded, Reason
```

Behaviour details:

- Events are debounced per folder. Undelivered Events for the same folder are merged, so a slow reader never builds
  up a backlog. An Event carries at most 512 names; beyond that it becomes `Full`.
- Re-enabling (globally, per folder, or by removing an exclusion) sends a `Full` Changed Event, because changes
  made while the watch was off were missed.
- When a backend stops by itself, the watcher checks the folder. If the folder still exists (for example, it was
  replaced), the watch restarts and a `Full` change is sent. Otherwise `Gone` is sent.
- Exclusion entries: a plain path excludes that folder and its subtree. Entries with `* ? [` are `filepath.Match`
  patterns, tried against the folder and each of its parents. Case is ignored on Windows.

## Tests

`go test ./internal/fswatch/` runs these tests:

- Watcher logic with a fake backend: debounce, MaxDelay, overflow, reference counting, switches, exclusions,
  fallback, pause, and gone/restart.
- End-to-end checks on a temp folder for the native backend of the host and for polling: create, append, rename,
  mkdir and delete, the non-recursive check, and deleting the watched folder.
