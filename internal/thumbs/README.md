# internal/thumbs

Thumbnail generator, disk cache, background worker pool and fallback file-type
icons for the thumbnails view (Total Commander's Ctrl+Shift+F1). UI-free; the
panel code wires it up.

Backs FEATURES.md section 33: **F-675** (thumbnail view data), **F-676**
(JPEG/PNG/GIF/BMP/WebP/TIFF natively, HEIC/PDF/video through QuickLook on
macOS and the freedesktop cache on Linux, icons for the rest, generated in the
background, cached on disk), **F-677** (size 32..512 px), **F-678** ("Delete
thumbnails cache" = `Cache.Clear`), **F-679** (`Meta` gives format and original
dimensions for field lines under the picture), **F-680** (cache persists
across sessions).

## Generating

```go
opt := thumbs.DefaultOptions()          // 96x96 box, no upscale, system thumbnailer on
opt.Width, opt.Height = 160, 160        // clamped to 32..512
cache, _ := thumbs.OpenCache(dir, 0)    // dir from thumbs.DefaultCacheDir(); 0 = 256 MiB budget
tn := thumbs.New(opt, cache)            // cache may be nil

th, err := tn.Thumbnail(ctx, path)      // *Thumb
// th.DataURL()  -> "data:image/jpeg;base64,..." for an <img>
// th.Width/Height, th.MIME, th.Data, th.FromCache
// th.Meta.Format ("jpeg", "png", "gif", "bmp", "webp", "tiff", "system")
// th.Meta.Width/Height (original, upright), th.Meta.Orientation (EXIF 1..8)
```

Errors: `ErrIsDir`, `ErrUnsupported`, `ErrDecode`, `ErrTooLarge` (check with
`errors.Is`); on any of them show `IconFor(name, isDir)`. Broken/huge images and
files the system thumbnailer could not handle are remembered in the cache
(`IsCachedFailure(err)`), so they are not retried until the file changes.

- EXIF orientation is applied for JPEG, PNG (eXIf), WebP (EXIF chunk) and TIFF.
- Opaque thumbnails are JPEG, transparent ones PNG.
- `tn.SetSize(w, h)` / `tn.SetOptions(opt)` for the size slider; other sizes stay cached.
- `thumbs.Generate(ctx, path, opt)` / `GenerateBytes(ctx, data, opt)`: no cache,
  no system fallback (e.g. for images extracted from archives).
- Safety limits: `MaxSourceBytes` (256 MiB) and `MaxPixels` (~100 MP) checked
  before decoding.

## Background pool

```go
pool := thumbs.NewPool(tn, 0)           // 0 = NumCPU/2 workers
b := pool.Submit(ctx, paths, func(r thumbs.Result) {
    // called on worker goroutines: r.Index, r.Path, r.Thumb, r.Err
})
b.Prioritize(visiblePaths...)           // on scroll: move these to the front
b.Cancel()                              // on folder change: drop pending work, no more callbacks
b.Wait() / <-b.Done()
pool.Close()                            // at shutdown
```

## Cache

Key = absolute path + mtime + size + box size + upscale flag. One file per
entry (`<dir>/<xx>/<hash>.mct`), atomic writes, LRU eviction to 90 % of the
budget when it is exceeded.

| Call | Purpose |
|------|---------|
| `Clear()` | "Delete thumbnails cache" |
| `Stats()` | entries / bytes for a settings page |
| `SetMaxBytes(n)` | change budget (evicts at once) |
| `Evict()`, `EvictOlderThan(d)` | manual trimming |
| `PruneStale()` | drop entries whose source vanished or changed |
| `Get/Put/PutFailure/Remove(Key)`, `KeyFor(path, opt)` | low level |

## Icons

`IconFor(name, isDir) Icon{Kind, Label, Color}`; `KindOf(name)` classifies by
extension (folder, parent, image, video, audio, archive, pdf, document,
spreadsheet, presentation, text, code, executable, font, diskimage, generic).
`Icon.SVG(size)` renders a crisp SVG with the extension label; `Icon.Image(size)`
a raster version without text. `HasNativePreview(name)` hints which files
are worth queueing first.

## Platform notes

- macOS: `qlmanage -t` (QuickLook) for non-native formats, 15 s limit per file.
  `MORMI_TEST_QUICKLOOK=1 go test` runs its test.
- Linux: reads `$XDG_CACHE_HOME/thumbnails` (freedesktop spec), never writes it.
- Windows/others: no system thumbnailer yet (`system_other.go`); native formats
  and icons only.
