# internal/fileops

The copy / move / delete / trash / wipe / rename engine of mormiCommander: a job queue that works like Total
Commander's background transfer manager. UI-free; the app wires it to Wails events.

## API

```go
q := fileops.NewQueue(emit)         // emit(event, data): "job", "job-ask", "job-done" with a Progress
q.Log = logFn                       // optional: one Entry per file done/skipped/failed (see FileLogger)
q.AskFunc = fn                      // optional: answer questions synchronously instead of via Answer

id, err := q.Start(fileops.Request{Kind: fileops.Copy, Sources: paths, Target: dir, ...})
q.Pause(id) / q.Resume(id) / q.Cancel(id) / q.CancelAll()
q.SetSpeedLimit(id, bytesPerSecond) // 0 = no limit, takes effect at once
q.RunNow(id)                        // take a waiting job out of the queue lane and start it
q.Answer(id, fileops.Answer{Choice: fileops.ChoiceOverwrite})
q.Get(id) / q.List() / q.Wait(id) / q.Forget()
```

Kinds: `Copy`, `Move`, `Delete` (permanent), `Trash`, `Wipe`.

`Request` options (zero value = TC default): `Rename` (new name or `*.bak` mask), `Overwrite` policy (`Ask`,
`Overwrite`, `Skip`, `Older`, `Larger`, `Smaller`, `AutoRenameSource`, `AutoRenameTarget`), `ReadOnly` policy for
read-only targets, `Filter` ("Only files of this type", `*.txt;*.md|old*;vendor/`), `Links` (`LinksCopy`,
`LinksFollow`, `LinksSkip`), `NoDates`, `NoPerms`, `NoAttrs`, `Owner`, `Verify` (SHA-256 re-read), `SpeedLimit`,
`BufferSize` + `NoCache` (big-file mode), `OnError` (`ErrorsSkip`, `ErrorsAsk`, `ErrorsStop`), `Retries`,
`IgnoreSpace`, `Queued`, `ConfirmDirs`, `ConfirmReadOnly`, `ConfirmHidden` (ask before overwriting a hidden or system
target, question type `hidden`), `WipePasses`.

`Start` refuses empty or relative `Sources` and `Target` (a cleaned `""` is the working folder), and a `Rename` of
`""`-like, `.` or `..`; a mask whose result for some item is `.`, `..` or empty fails that item.

Questions (`Progress.Ask`, a `Conflict` with `Type` and the valid `Choices`): target exists (Overwrite, Overwrite all,
Skip, Skip all, Overwrite all older, Rename, Auto-rename, Auto-rename all, Rename target, Rename all targets, Copy all
larger, Copy all smaller, Append, Resume, Compare, Cancel), read-only target, file error (Retry, Skip, Skip all, Cancel,
plus Delete permanently when the trash fails), not enough free space, delete non-empty folder, delete read-only file.

`Progress`: state (`queued`, `running`, `paused`, `asking`, `done`, `failed`, `cancelled`), files/bytes total and done,
current file with its own bytes, speed, ETA, elapsed time (pauses excluded), speed limit, errors, skipped, verified,
and a final `Summary` sentence.

Helpers: `ParseTarget` (split the dialog's target field), `ApplyMask`, `ParseFilter`, `UniqueName` ("file(2).txt"),
`RenameItem` (in-place rename, case-only renames included), `SameVolume`, `LongPath` (`\\?\` on Windows),
`TrashSupported`, `FileLogger`.

Behaviour notes: a move on one volume is a rename; across volumes each file's source is removed right after its own
copy (and verification) succeeded, and source folders that end up empty are removed. A cancel removes the half-written
file. An overwrite writes to a hidden temporary file next to the target and renames it over the target only after
the copy (and verification) succeeded, so a cancel, read error or full disk leaves the old target intact. An
exclude-only filter (`|*.bak`) keeps empty folders; only filters with include patterns prune folders without matching
files. Wipe only unlinks a file that has other hard links (overwriting would destroy the other name's data), and a
folder that can't be removed gets its name back. Links are copied as links unless `LinksFollow`, which detects loops. Dates (modification, access; creation on
Windows and, indirectly, macOS), mode bits, Windows attributes, extended attributes / resource forks (macOS, Linux)
and alternate data streams (Windows) are copied; the owner only with `Owner` and where the system allows it.

## FEATURES.md IDs backed

Section 20 (copy/move engine): F-457 (filter), F-458 (rename target), F-459 (wildcard rename), F-460, F-461, F-462,
F-463 (all overwrite answers; the preview is UI), F-464, F-465, F-466 (compare result; the compare tool is UI), F-467,
F-468, F-469, F-470, F-471, F-472, F-473, F-474, F-475, F-476, F-477, F-478, F-479 (auto-name engine side), F-486,
F-487, F-488, F-490, F-491, F-492 (summary and log), F-831.

Section 21 (delete): F-493 (via internal/trash), F-494, F-495, F-496, F-497, F-500, F-833 (fallback to permanent
delete when the trash fails).

Not here: dialogs, drag and drop and menus (UI), archive and FTP/SFTP targets (F-482–F-484, F-498, F-499: their own
packages), sync (F-485), trash restore (F-501).
