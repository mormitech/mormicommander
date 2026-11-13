# internal/syncdirs

This is the engine behind Synchronize Directories (`cm_SyncDirs`, FEATURES.md section 25).
It has no UI. It compares two folder trees, gives every pair of names a category and a default action, lets the user change those actions, and then runs the sync with progress and cancel.
It uses only the standard library.

## Backs these FEATURES.md items

| ID | What the engine provides |
|----|--------------------------|
| F-546 | `Compare` with left/right sides, `Options.Filter` (TC filter syntax), all dialog options |
| F-547 | `Asymmetric`, `ByContent`, `IgnoreDate`, `ToleranceSeconds` (2 = FAT), `IgnoreHours` (DST/time zone) |
| F-548 | `Subdirs`, `IgnoreEmptyDirs` |
| F-549 | `Show` buttons, `Result.Visible(show)`, `Result.Summary()` |
| F-550 | `Item.CanApply`, `Result.SetAction`, `SetActionTree`, `ReverseAction`, `ResetActions` |
| F-551 | `Result.Plan(PlanFilter)` (the dialog checkboxes) + `Execute`; a re-compare after a sync shows `Summary().AllEqual()` |
| F-552 | `Options.ByContent` (byte compare, stops at the first difference) |
| F-553 | `Result.Recompare(ctx, path)` re-reads one pair after the compare tool edited it (the compare tool itself lives elsewhere) |
| F-554 | `Profile`, `LoadProfiles`, `SaveProfiles`, `PutProfile`, `DeleteProfile` (JSON file) |
| F-555 | Sides are `FS` / `WritableFS` interfaces: `DirFS(path)` for local disk, `FromFS(fsys)` for any `io/fs` (zip, ...) as a read-only side; VFS adapters can implement `WritableFS` |
| F-562 | Recursive compare by content = `Subdirs` + `ByContent` |

## API in short

```go
left, right := syncdirs.DirFS("/a"), syncdirs.DirFS("/b")
opts := syncdirs.DefaultOptions()            // Subdirs on, 2 s tolerance
opts.ByContent = true
res, err := syncdirs.Compare(ctx, left, right, opts, func(p syncdirs.CompareProgress) { ... })

rows := res.Visible(syncdirs.DefaultShow())  // rows for the list, in tree order
res.SetAction("docs/a.txt", syncdirs.CopyRightToLeft) // right-click / arrow click
res.SetActionTree("old", syncdirs.DeleteRight)       // action on a folder and its contents

plan := res.Plan(syncdirs.AllOps())          // or PlanFilter{LeftToRight: true, ...}
// plan.FilesLR, BytesLR, FilesRL, BytesRL, DeletesLeft, DeletesRight for the dialog
rep, err := syncdirs.Execute(ctx, plan, left, right, syncdirs.ExecOptions{
    Progress:         func(p syncdirs.Progress) { ... },
    ConfirmOverwrite: func(op *syncdirs.Op) syncdirs.OverwriteDecision { ... },
    OnError:          func(op *syncdirs.Op, err error) syncdirs.ErrorDecision { ... },
})
```

- `Item` rows have `Path`, `LeftPath`/`RightPath` (they can differ in letter case), `Left`/`Right` entries (nil when missing), `IsDir`, `Depth`, `Category`, `Default`, `Action` and `Note`.
- Categories, actions, kinds and case modes marshal to JSON strings (`"leftNewer"`, `"copyLeftToRight"`, ...), so a `Result` can go to the front end as it is. `Action` and `CaseMode` also unmarshal from those strings.
- Default actions follow TC. Symmetric mode: newer or only copy goes to the other side, and "unequal" gets no action. Asymmetric mode: everything is copied left to right and right-only names are marked `deleteRight`.
- `Execute` returns `ctx.Err()` when cancelled, `context.Canceled` when the overwrite hook says `CancelSync`, `ErrAborted` when the error hook says `Abort`, and `ErrReadOnly` (before doing anything) when a side that must change is not a `WritableFS`. Failed operations are listed in `Report.Errors`.

## Safety rules (tested)

- Each copy goes to a temp file in the target folder and is then renamed over the target. A cancelled copy leaves nothing behind.
- Before each operation, both sides are checked against what the compare saw. If something changed in the meantime, the operation is skipped with `ErrChanged`. `ExecOptions.Force` turns this check off.
- Folders are removed only when they are empty. Deletes run deepest first, after all copies.
- Symlinks are not followed by default. A link is compared by its target, copied as a link and deleted as a link.
- With `FollowSymlinks`, loops are found and skipped. Nothing is ever written or deleted inside a followed folder link (`Entry.ViaLink`, `CanApply`).
- `OSFS` refuses any write or delete whose path goes through a symlinked folder (`ErrSymlinkInPath`), and any path with `..`.
- A subfolder that can't be read is skipped and reported in `Result.Errors`. It is never treated as empty, because that would invent copies or deletions.

## Platform notes

- `CaseAuto` matches names case-insensitively on Windows and macOS and case-sensitively elsewhere (`case_*.go`).
- On Windows the read-only attribute is cleared before a replace or delete (`fs_windows.go`), as TC does after its overwrite confirmation.
- Unicode normalisation (NFC vs NFD names between macOS and other systems) is not handled yet.
