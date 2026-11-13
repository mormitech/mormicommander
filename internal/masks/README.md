# internal/masks

Total Commander style wildcard masks and the panel selection engine built on them. UI-free; the only dependency
outside the standard library is `golang.org/x/text/unicode/norm` (NFC normalisation, so decomposed macOS names
match composed masks).

## Mask syntax

| Syntax | Meaning |
| --- | --- |
| `*` / `?` | any run of characters / exactly one character |
| `[abc]`, `[a-z]`, `[!a-z]`, `[^a-z]` | one character from a set; an unclosed `[` is literal |
| `*.*`, `name.*` | also match names without extension (DOS rule) |
| `*.`, `name.` | match only names without extension |
| `a;b c` | several masks, separated by `;` or spaces; `"my file*"` quotes a mask with spaces |
| `inc|exc` | include the left masks, minus the right ones; `|*.bak` = everything except backups |
| `name\` or `name/` | folder-only mask, e.g. `*|.git\` |
| `>saved` | a saved filter, looked up through `Options.Resolve` |

Case: `CaseDefault` is case-insensitive on Windows and macOS and case-sensitive on Linux/other Unix
(`case_windows.go`, `case_other.go`, `case_unix.go`); `CaseSensitive` / `CaseInsensitive` override it. Extension
rule (`SplitExt`): text after the last dot; a leading dot does not start one (`.bashrc` has none).

## API

- Masks: `CompileMask(mask, caseSensitive) *Mask` with `Match`, `DirOnly`, `HasWildcards`;
  `Parse(expr, Options) (*List, error)`, `MustParse`, `List.Match(name, isDir)`, `List.MatchesAll`,
  `List.ExcludesDir` (prune recursive walks), `Match(expr, name)`; `SplitExt`.
- Selection: `NewSelection(entries, SelectionOptions)`; `SetEntries` (keeps marks by name across refresh), `Toggle`,
  `Set`, `SetRange`, `SelectGroup` / `UnselectGroup(list, Kind)`, `GroupKind(shift)` (the "select folders too"
  option and its Shift swap), `SelectAll`, `ClearAll` (remembers for restore), `Clear`, `Invert(Kind)`,
  `SelectSameExt` / `SelectSameName` / `SelectSameNameExt(cur, on, Kind)`, `SelectNames` (other panel),
  `SetMarks`, `Remember` / `Restore`, `SaveSlot` / `LoadSlot` / `Slot`, `Selected`, `SelectedNames`,
  `SelectedEntries`, `Count`, `Stats` (status bar). `..` can never be selected.
- Selection files and clipboard text: `WriteSelection`, `ReadSelection` (UTF-8, UTF-8 BOM, UTF-16 LE/BE, CRLF),
  `SplitLines`, `Selection.ApplyList(lines, dir, replace)` (bare names, full paths in this folder, or masks).
- Copy names: `FormatEntries(dir, entries, NameFormat, ClipOptions)` with `FormatNames`, `FormatFullNames`,
  `FormatDetails`, `FormatFullDetails`, `FormatUnixPaths`, `FormatURLs`; `ClipOptions.MapNet` for UNC names.
- Quick filter: `NewQuickFilter(text, QuickFilterOptions{Case, AnyPart, KeepFolders})`, `Match`, `Filter`, `Text`,
  `Active`.
- Ignore list: `ParseIgnoreList`, `LoadIgnoreFile` (missing file = empty list), `Ignored(dir, name, isDir)`,
  `Filter`, `Toggle`, `Lines`. Name entries apply everywhere; entries with a path apply to that folder only.
- Compare directories: `CompareLists(left, right, CompareOptions)`, `CompareTrees(lfs, lroot, rfs, rroot, opt)`
  (recursive with `Subdirs`, any `io/fs.FS`), `EntriesFS`, `ContentEqualFS`, `DefaultCompareOptions` (2 s
  tolerance), `IgnoreHourShift`, `IgnoreDate`, `Filter`; `CompareResult.LeftMarks` / `RightMarks` feed
  `Selection.SetMarks`. Missing or newer files are marked; same date with different size or content marks both.

## FEATURES.md IDs backed

- Section 6 (Selection): F-118 (toggle), F-119 (range), F-120, F-121, F-122, F-123, F-124, F-125, F-126, F-127,
  F-128, F-129, F-130, F-131, F-132, F-133, F-134, F-137 (stats), F-138, F-139, F-140, F-142 (formats),
  F-143, F-791, F-792, F-793, F-794, F-795. Mouse modes (F-135, F-136) and F-797 are UI wiring on top of
  `Toggle`/`SetRange`; F-141 and F-796 (copy a folder path) need no engine.
- Elsewhere: F-068, F-780, F-781, F-250, F-801 (quick filter); F-088, F-089 (custom panel filter = `List`);
  F-097 (colour by file type masks); F-428, F-744, F-788 (ignore list).
