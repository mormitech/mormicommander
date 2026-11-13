# internal/trash

Moves files, folders and links to the system trash: the macOS Trash, the freedesktop.org trash on Linux, or the
Windows Recycle Bin. Used by `internal/fileops` for Trash jobs (F8/Delete with "move to trash").

## API

- `Supported() bool`: whether this platform has a trash the package can use (false on the BSDs and others).
- `Move(path string) error`: trashes one item. `path` must be absolute and exist (`Lstat`, so a broken link counts);
  empty or relative paths are refused before any system call, because a cleaned `""` is the working folder.
- `MoveAll(paths []string) error`: trashes several items with as few system calls as possible (one Finder call on
  macOS for all ordinary items, one `gio trash` call on Linux). Every path is checked first; a failure may leave some
  items already trashed, so use `Move` when each item needs its own result.

## Platform notes

- macOS: ordinary items go through Finder (so "Put Back" works; the first call triggers the automation prompt).
  Symbolic links go through `NSFileManager trashItemAtURL` via AppleScriptObjC, because Finder needs an alias and an
  alias coercion resolves the link, which would trash its target instead of the link.
- Linux: `gio trash`, else trash-cli's `trash-put`; both trash links as links.
- Windows: PowerShell with `Microsoft.VisualBasic.FileIO.FileSystem` and `SendToRecycleBin`, one call per item.
- Paths travel as arguments or an environment variable, never inside script text.

## Tests

The tests replace the command runner, so they never touch the real trash.

## FEATURES.md IDs

F-165, F-493 (delete to the trash / Recycle Bin), F-501 (macOS "Put Back" via Finder), F-833 (a failed move lets fileops offer a permanent delete).
