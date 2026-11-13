# internal/find — Find Files engine

UI-free engine behind Total Commander's Find Files dialog (Alt+F7, `cm_SearchFor`).
Standard library only, plus `golang.org/x/text/encoding/charmap` for ANSI/OEM code pages
(already in the main `go.mod` as an indirect dependency; `go mod tidy` makes it direct).

## FEATURES.md items backed

| ID | What the engine provides |
|----|--------------------------|
| F-525 | `Options.SearchFor` masks: `;`/space separated, quotes, `|` exclusions, TC rules (`*.*`, `*.`, no wildcard = `*text*`) — `ParseMask` |
| F-526 | `Options.SearchIn` (several start folders), `ParseSearchIn("a;b;\"c;d\"")` |
| F-527 | `Options.Levels` (`Unlimited` = -1, 0 = current folder only, n levels) |
| F-528 | `Options.SearchArchives` + `Options.Archives` (`ArchiveHandler` interface; adapt internal/archive) |
| F-529 | `Options.Content` (`ContentOptions`): case, whole words, regex, hex, NOT containing, encodings ANSI/OEM/UTF-8/UTF-16LE/BE, `Codepages()` |
| F-530 | `Options.NameRegex` (+ `NameRegexCase`) |
| F-531 | `Options.Date` (`From`/`To`, `NotOlderThan`, `OlderThan` with minutes…years) |
| F-532 | `Options.Size` (`=`, `>`, `<` with B/KB/MB/GB/TB) |
| F-533 | `Options.AttrSet` / `AttrClear` (`Attr`: r a h s d l x c e), `PermMask`/`PermValue` |
| F-534 | `Options.Duplicates` (`DupOptions`: name, size, content, hash md5/sha1/sha256/crc32) |
| F-535 | `Options.Kind` (`KindAll`, `KindFiles`, `KindDirs`), `Options.FollowLinks` (loop-safe) |
| F-537, F-542 | `Store`: named templates (`Save`, `Get`, `Names`, `Rename`, `Delete`) and `SetLast`/`Last` |
| F-538, F-543 | `Start` → `*Job` (`Results()` channel, `Stop`, `Wait`, `Progress`, `Errors`, `Done`) |
| F-544 | `Options.Within`: search only in previously found paths |
| F-545 | `Options.FS`: any `FS` implementation (embed `SlashFS` for remote paths) |
| F-837 | `|name/` or `|name\` in the mask and `Options.ExcludeDirs` |
| F-838 | `WriteResults(w, results, WriteOptions{Details, Newline})` |

Not engine work: F-524 (dialog), F-539/F-540/F-541 (panel actions on results),
F-536 (N/A plugin tab).

## API in short

```go
o := find.DefaultOptions()                 // SearchFor "*", Levels Unlimited
o.SearchFor = "*.go|*_test.go vendor/"
o.SearchIn = find.ParseSearchIn(field)     // or []string{dir}
o.Content = find.ContentOptions{Text: "TODO", WholeWords: true}

job, err := find.Start(ctx, o)             // compile errors come back here
for r := range job.Results() {             // find.Result: Path, Name, Root, Rel (slash), Size,
    show(r)                                //   ModTime, Mode, IsDir, Attr, Archive, Inner, DupGroup, Hash
}
stats, err := job.Wait()                   // err == context.Canceled after job.Stop()
```

Synchronous use: `q, err := find.Compile(o); q.Run(ctx, find.Handler{Result, Error, Progress})`.
Return `find.ErrStop` from `Result` to end early without an error. `find.Search` collects
everything into a slice. `q.Match(ctx, path)` tests one file against all filters (use a
saved template as a "select group" / custom filter).

## Behaviour notes

- Names compare case-insensitively; masks with a path separator in the middle match the
  relative path (`src/*.go`). The start folder itself is never a result; a start path that
  is a file is tested on its own.
- Folder exclusions skip the folder entirely (not listed, not entered); file exclusions
  only drop names from the results.
- Active size or content filters never match folders. `= 3 KB` means 3072–4095 bytes.
- Errors on single paths go to `Handler.Error` / `Job.Errors()` and never stop the search.
- Content is read in 1 MiB chunks; matches across chunk boundaries are found, regex matches
  across a boundary up to 64 KiB long. UTF-16 is also tried at odd byte offsets.
- Regex content search is multi-line (`^`/`$` are line anchors) and case-insensitive unless
  `CaseSensitive`. Whole words means letters, digits, `_` (any script).
- Duplicates are emitted after the walk, grouped (`DupGroup` 1..n, sorted by path). Content
  mode verifies byte by byte; Hash-only mode trusts the digest and fills `Result.Hash`.
  Archive members join name/size comparisons only.
- Dotfiles count as hidden except on local Windows folders (there only the attribute does).
- Archive members: `Result.Path` = archive path joined with the inner path; `Archive` and
  `Inner` (slash path) are set separately.
