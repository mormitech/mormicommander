# internal/lister

UI-free backend for the Lister (F3 viewer, Ctrl+Q quick view). Standard
library plus `golang.org/x/text` (code pages) and `golang.org/x/sys`
(Windows code page query). Everything is addressed by **byte offset**; the UI
keeps the offset of the top row and asks for rows from there, which is how
multi-GB files open and scroll instantly.

## Backs FEATURES.md IDs

| ID | What this package provides |
|----|----------------------------|
| F-564, F-589 | `OpenViewer` / `NewViewer` — the engine for the Lister window and quick view |
| F-565 | `Detect` (BOM, UTF-8, BOM-less UTF-16, ANSI fallback, Cyrillic 1251/KOI8-R/866 scoring) |
| F-566 | `ModeBinary` rows (fixed width, DOS glyphs for control bytes) |
| F-567 | `ModeHex` rows, `FormatHexRow`, `HexDump` |
| F-568 | `ModeText` rows (tabs, CR/LF/CRLF, wide chars) |
| F-569 | `ProbeMedia`, `SuggestView` → `ViewMedia`, `MediaInfo.MIME` for the web view |
| F-570, F-840, F-841, F-587 | `Encodings()` menu list, `LookupEncoding`, `EncodingForKey` (A/S/U/8), `DefaultANSI`/`DefaultOEM` |
| F-571 | `HTMLToText`, `HTMLCharset`, `SuggestView` → `ViewHTML`/`ViewMarkdown` (rendering itself is the web view's job) |
| F-572 | `Options.Wrap` + `Options.Width` (word wrap; off = one row per line, horizontal scroll in the UI) |
| F-575, F-576 | `Compile(Query)` + `Searcher.Find` / `FindAll`, `Viewer.Search` |
| F-577, F-582, F-587 | `Viewer.Text`, `Viewer.HexText` (copy as hex), `Viewer.CopyRange` (save selection / save as) |
| F-578 | `LineIndex.LineOffset` (go to line), `Viewer.RowStart` (go to offset), `OffsetAtPercent` |
| F-579 | `File` (block-cached `ReadAt`), `MaxLineBytes` cutting of endless lines |
| F-583 | `NeighborFile`, `NeighborIn`, `NaturalLess` |
| F-584, F-842 | `Viewer.Reload`, `Viewer.FollowTail`, `Viewer.AtEnd` |
| F-586 | `MediaInfo.Tags` (EXIF: camera, date, exposure, f-number, ISO, focal length, lens, GPS) + `Orientation` |
| F-588 | `Options.TabWidth`, `Options.ANSI` |
| — (RTF) | `RTFToText` |

Not here (UI work): zoom/rotate/fit of images (F-573/574/845), fonts (F-580),
printing (F-581/843), windows (F-846), external viewer (F-590).

## Typical wiring

```go
v, f, err := lister.OpenViewer(path, lister.Options{})   // auto-detect encoding
defer f.Close()

rows, _ := v.Rows(top, visibleRows)    // []Row{Offset, End, Text, Map, LineStart, Hex}
top, _ = v.Forward(top, 1)             // Down; PageDown = Forward(top, visibleRows-1)
top, _ = v.Back(top, 1)                // Up
top, _ = v.LastPage(visibleRows)       // End;  Home = v.DataStart()
top, _ = v.RowStart(v.OffsetAtPercent(50))

v.SetOptions(lister.Options{Mode: lister.ModeHex})        // keys 1/2/3, W, A/S/U/8
top, _ = v.RowStart(top)                                   // re-align after any option change

off, err := v.Lines().LineOffset(ctx, 12345)   // go to line (ErrBeyondEnd past the end)
n, _ := v.Lines().LineAt(ctx, top)             // status bar line number
total, done := v.Lines().Known()               // cheap; Count(ctx) scans to the end

s, _ := lister.Compile(lister.Query{Pattern: "foo", WholeWords: true, Encoding: v.Encoding()})
m, found, _ := s.Find(ctx, v.Source(), cursor, v.DataStart())   // F3; Backward:true for Shift+F3
top, _ = v.RowStart(m.Offset)

go v.FollowTail(ctx, 0, func(ch lister.Change) { /* re-fetch rows; if at end, LastPage */ }, nil)
info, err := lister.ProbeMediaFile(path)      // key 4, image info / EXIF
```

### Notes for the UI

- `Row.Map[i]` is the byte offset (relative to `Row.Offset`) of rune `i` of
  `Row.Text` (Go runes, not JS UTF-16 units). Use it to paint search matches
  (`Match.Offset`/`Length`) and selections, and to turn a click into an offset.
- Text mode replaces control characters with spaces and expands tabs; binary
  mode shows control bytes as DOS glyphs (☺☻♥…); hex mode shows `.`.
- After `Reload` reports `Truncated`, `Replaced` or `Modified`, call
  `v.Lines()` again (a fresh index is built) and re-align `top` with `RowStart`.
- Long-running calls (`LineIndex.*`, `Searcher.Find`, `CopyRange`) take a
  `context.Context`; run them off the UI thread and cancel on Esc.
  `Searcher.Progress` and `LineIndex.Progress` feed progress bars.
- Over-long lines (no line break for 32 KiB) are cut at fixed positions, so
  scrolling up and down always visits the same rows.
- A single regex match is limited to 64 KiB; Go's `\w`/`\b` are ASCII-only,
  so use `\pL` for letters — the `WholeWords` flag itself is Unicode-aware.
