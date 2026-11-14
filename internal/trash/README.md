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
- `Before(path) (Mark, bool)` and `Locate(Mark) (string, bool)`: where an item is in the trash after a move (for the
  undo journal, board issue #13). `Before` notes the item's identity (device and inode, or volume and file index on
  Windows) just before the move; `Locate` looks through the trash folders of that volume for the same file, checking
  only names with the item's stem (and the Recycle Bin's `$R...` names), and else asks the trash's record of the old
  place (`.trashinfo` on Linux, `$I...` on Windows). The system calls themselves are not changed.
- `Restore(trashed, original string) error`: moves a trashed item back and removes its `.trashinfo` / `$I` record.
  It never overwrites (`ErrOccupied` when something is at the old place) and only takes items that lie directly in a
  trash folder of the old place's volume (`ErrNotInTrash` otherwise).

## Platform notes

- macOS: ordinary items go through Finder (so "Put Back" works; the first call triggers the automation prompt).
  Symbolic links go through `NSFileManager trashItemAtURL` via AppleScriptObjC, because Finder needs an alias and an
  alias coercion resolves the link, which would trash its target instead of the link.
- Linux: `gio trash`, else trash-cli's `trash-put`; both trash links as links.
- Windows: PowerShell with `Microsoft.VisualBasic.FileIO.FileSystem` and `SendToRecycleBin`, one call per item.
- Paths travel as arguments or an environment variable, never inside script text.
- Trash folders (`Locate`, `Restore`): macOS `~/.Trash` for the home volume, `<volume>/.Trashes/<uid>` elsewhere
  (reading them may need the app to have Full Disk Access; without it an item is not found and its delete is not
  undoable); Linux the freedesktop.org home trash (`$XDG_DATA_HOME/Trash/files`) or `<top>/.Trash/<uid>/files` and
  `<top>/.Trash-<uid>/files`; Windows `<drive>:\$Recycle.Bin\<SID>` (none on network shares).

## Tests

The tests replace the command runner, so they never touch the real trash; the locate and restore tests also point
the trash folders (`trashDirs`) at temporary folders, including a temporary `XDG_DATA_HOME` for the `.trashinfo`
records and synthetic `$I` files for the Recycle Bin.

## FEATURES.md IDs

F-165, F-493 (delete to the trash / Recycle Bin), F-501 (macOS "Put Back" via Finder), F-833 (a failed move lets fileops offer a permanent delete), F-859 (undo of a delete to the trash).
