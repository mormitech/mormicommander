# internal/compare — Compare by content engine

UI-free engine for the Total Commander style **Compare by content** window
(FEATURES.md section 26). Standard library plus `golang.org/x/text` for the
single-byte code pages.

Backs: **F-556** (text compare), **F-557** (binary compare), **F-558**
(next/previous difference), **F-559** (edit, copy left/right, save),
**F-560** (ignore case / whitespace / repeated lines), **F-561** (compare
same-name files: `PairByName`), **F-562** (content equality for folder sync:
`FilesEqual`), **F-563** (external diff tool: `ExternalCommand`), **F-839**
(line numbers, synced rows, encoding choice). The encoding detection is also
reusable for the Lister (F-565, F-570).

## Typical wiring

```go
s, err := compare.OpenFiles(left, right, compare.Options{}, compare.DetectOptions{})
if s.Binary() {
    res, _ := compare.CompareBinaryFiles(ctx, left, right, 0) // first 1000 diffs
    // show res.Diffs / res.Runs (offset, byte A, byte B), res.SizeA/SizeB
}
rows := s.Rows()        // one entry per visual row; Left/Right = line index or -1
blocks := s.Blocks()    // difference blocks in row coordinates
i := compare.NextDiff(blocks, curRow) // -1 = none; PrevDiff likewise
l, r := s.InlineSpans(row)            // byte spans to highlight inside the row
s.CopyBlock(i, compare.Left)          // copy block → right side (undoable)
s.SetLine(compare.Right, line, text)  // edit in place
s.Undo(); s.Redo(); s.Modified(compare.Right)
s.Save(compare.Right, "")             // atomic, original encoding/BOM/EOLs
s.ReloadSide(compare.Left, compare.EncWindows1250, compare.DetectOptions{}, false) // encoding menu
```

## API

| Area | Functions / types |
|---|---|
| Options | `Options{IgnoreCase, Whitespace (Exact/IgnoreChange/IgnoreAll), IgnoreRepeatedLines, ResyncLines, Algorithm (Patience/Myers), Inline (Words/Chars/None), MaxCost}`, `NormalizeLine` |
| Line diff | `DiffLines(a, b []string, Options) []Op`. `Op{Kind, A1, A2, B1, B2}` (half-open ranges), kinds `OpEqual/OpDelete/OpInsert/OpChange`. `Summarize(ops) Stats` |
| Side by side | `BuildRows(ops) []Row`, `Blocks(rows) []Block`, `NextDiff`, `PrevDiff`, `BlockAt`, `RowOfLine` |
| Intra-line | `InlineDiff(a, b string, Options) (left, right []Span)`. Spans are contiguous byte ranges with `Changed` |
| Hunks | `Hunks(a, b, ops, ctx) []Hunk`, `Hunk.Header()`, `WriteUnified(w, nameA, nameB, a, b, ops, ctx)` (patch-compatible) |
| Binary | `CompareBinary(ctx, r1, r2, maxDiffs)`, `CompareBinaryFiles`, `FilesEqual`, `ReadersEqual`. Streams in 64 KiB chunks |
| Encoding | `Detect(data, DetectOptions{ANSI, SniffLen}) Detection{Encoding, BOM, Binary, ASCII}`, `Decode`, `Encode` (`ErrUnencodable`), `ParseEncoding`, `Encodings()`, `IsBinary` |
| Documents | `Document{Lines []Line{Text, EOL}, Encoding, BOM, Path}`, `ParseDocument`, `LoadFile`, `NewDocument`, `SplitLines`, `Document.SaveFile`, `WriteFileAtomic` |
| Session | `NewSession`, `OpenFiles`, `SetOptions`, `Rows/Blocks/Ops/Stats/Identical`, `LineText`, `InlineSpans`, `SetLine`, `InsertLines`, `DeleteLines`, `CopyBlock`, `CopyRows`, `CopyAllBlocks`, `Undo/Redo/CanUndo/CanRedo`, `Modified`, `Save`, `ReloadSide` (`ErrModified`) |
| Helpers | `ExternalCommand(template, left, right) []string` (`%1`, `%2`, `%%`, quotes), `PairByName(left, right, caseInsensitive)` |

## Notes

- Line endings are not part of the comparison; each line keeps its own
  terminator and is saved back unchanged. New or copied lines get the
  target file's dominant ending.
- With `IgnoreRepeatedLines` an equal op may cover different line counts on
  the two sides (the shorter side is padded in the rows). Hunks are exact
  patches only when that option is off.
- `ResyncLines` = TC's "lines to resync": matching runs shorter than this
  between two differences are merged into one block.
- `MaxCost` keeps very large, very different inputs fast: the result stays a
  valid diff, just not guaranteed minimal.
- Detection: BOM → BOM-less UTF-16 → binary (NUL / control chars) → ASCII →
  UTF-8 → `DetectOptions.ANSI` (default Windows-1252; Windows-1250 when bytes
  undefined in 1252 occur). Pass `ANSI: EncWindows1250` for a Hungarian locale.
- Bytes a single-byte code page leaves undefined decode to U+F700+byte and
  encode back to the same byte, so such files can still be saved after an edit.
- Saving follows symbolic links (the target is updated, the link stays) and
  rewrites hard-linked files in place so their other names see the change.
- `CompareBinary` measures the tail of a longer seekable input without reading
  it, and reads a non-seekable tail with cancel checks.
