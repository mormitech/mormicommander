# internal/multirename

The UI-free engine for the Multi-Rename Tool (Ctrl+M). It parses the masks,
builds the preview, finds conflicts, runs the renames (with rollback) and
undoes them. The Wails dialog only collects input and shows `Result`.

Backs FEATURES.md IDs: **F-504, F-505, F-506, F-507, F-508, F-509, F-510,
F-511, F-512, F-513, F-514, F-515, F-516, F-517, F-518, F-519, F-834, F-835,
F-836** (the UI still has to wire up the dialog, the hotkeys and the editor
launch for F-504/F-512/F-517).

## Typical flow

```go
s := multirename.DefaultSettings()          // also the "Reset" button (F-518)
s.NameMask = "[N]_[C]"

r, err := multirename.Compile(s)            // *SettingsError{Field: "nameMask"|"extMask"|"search"}
res := r.Preview(items, multirename.PreviewOptions{})  // call on every keystroke
// show res.Entries[i].Item.Name -> NewName, Problems (red), Warnings (yellow), res.Warnings

log, err := multirename.Execute(res, multirename.ExecOptions{Progress: fn})
multirename.SaveUndoLog(filepath.Join(cfgDir, "multirename-undo.json"), log)

// later, "Undo" (F-514):
log, _ = multirename.LoadUndoLog(path)
err = multirename.Undo(log, multirename.ExecOptions{})
```

`items` is `[]Item{Dir, Name, IsDir, Size, ModTime, Fields}` in list order; the
counter follows that order (F-519). Use `SortItems(items, SortByName|SortByExt|
SortBySize|SortByDate|SortByPath, desc)` for header clicks, or reorder by
hand. For branch view / subdirectories (F-511) and folders (F-516) just pass
items from several folders; folders and their contents may be renamed in one
batch.

## Masks (name and extension fields)

| Placeholder | Meaning |
|---|---|
| `[N]` `[E]` | old name / extension (no dot) |
| `[N2-5]` `[N2,3]` `[N2-]` `[N1]` | chars 2..5, 3 chars from 2, from 2 to end, char 1 |
| `[N-8,5]` `[N-8-5]` `[N-8--5]` `[N2--5]` `[N-5-]` | counted from the end |
| `[P]` `[G]` | parent / grandparent folder name (ranges allowed) |
| `[C]` `[C10+5:3]` `[C+2]` `[C:4]` `[C:a]` `[Ca]` `[CA]` | counter (start+step:digits, auto width, letters) |
| `[Y]` `[y]` `[M]` `[D]` `[h]` `[m]` `[s]` | modification date/time parts, zero padded |
| `[d]` `[t]` | date / time using `Settings.DateFormat`/`TimeFormat` (Go layouts) |
| `[U]` `[L]` `[F]` `[n]` | upper / lower / Each Word / unchanged case from here on |
| `[=field]` | built-in field (`FieldNames()`), `tc.` prefix optional, or `Item.Fields[key]` (EXIF etc.) |
| `[[]` `[]]` | literal `[` and `]` |

Note: `[n]` follows Total Commander (switch case handling back to
"unchanged"); `[=fullname]` gives the original name with extension.

Pipeline per item: masks → search & replace → case drop-down → accent removal.

## Settings (preset-able, JSON)

`NameMask, ExtMask, Search, Replace, CaseSensitive, RegEx, OnlyFirst,
IncludeExt` ("[E]" box), `Case` (`CaseUnchanged|CaseLower|CaseUpper|
CaseFirstUpper|CaseEachWord`), `CounterStart, CounterStep, CounterDigits`
(`AutoDigits` = -1), `CounterResetPerFolder, RemoveAccents, SplitDirExt,
DateFormat, TimeFormat`.

Plain search supports `*` / `?` and `a|b` → `x|y` pairs (applied in one pass;
one replacement is reused for all). RegEx mode: `|` is alternation, `$1`,
`${name}` in the replacement.

## Preview and conflicts (F-515)

`PreviewOptions{Names, TargetOS, CaseRule, FS, SkipDiskCheck}`.
`Entry.Problems[].Kind`: `empty, dot-name, invalid-chars, reserved, trailing,
too-long, duplicate, exists`. `Execute` refuses with `ErrHasConflicts` while
`res.HasErrors()`. After a manual edit of `NewName` in the grid call
`CheckEntries(res.Entries, opts)`. Chains and swaps are fine (temporary
names are used). `ValidateName(name, goos)` is usable on its own (e.g. for
the Shift+F6 inline rename).

## Execute / undo

`Execute` renames deepest level first, uses temporary names when targets
overlap sources, re-checks that targets don't exist, and rolls back all steps
on failure (`*ExecError` with `RollbackErrs`; wraps `ErrTargetExists` or the
OS error). `UndoLog` is JSON (`SaveUndoLog`/`LoadUndoLog`).

## Name lists (F-835, F-512)

`ParseNames([]byte)` reads a file or clipboard text (UTF-8 ± BOM, UTF-16 BOM,
Latin-1 fallback, LF/CRLF) and the result goes into `PreviewOptions.Names`;
a count mismatch adds a `res.Warnings` entry. For "edit names in editor":
`FormatNames(NewNamesOf(res), runtime.GOOS == "windows")` → temp file →
editor → `ParseNames` → `PreviewOptions.Names`.

## Presets (F-513)

`SavePreset(dir, name, s)`, `LoadPreset(dir, name)`, `ListPresets(dir)`,
`DeletePreset(dir, name)`. One JSON file per preset; any name works.

## Other helpers

`RemoveAccents(s)` (F-836), `ApplyCase(s, mode)`, `SplitExt(name)`,
`NaturalCompare(a, b)`, `CompileMask(src)` (returns `*MaskError` with offset).
