package robust

import (
	"errors"
	"io/fs"
	"os"
)

// RenameNoReplace renames oldpath to newpath, failing with an error that matches fs.ErrExist when newpath exists,
// even when it appears between a check and the rename. A plain os.Rename replaces whatever is at newpath on POSIX.
// macOS uses renamex_np(RENAME_EXCL), Linux renameat2(RENAME_NOREPLACE), Windows MoveFileEx without
// MOVEFILE_REPLACE_EXISTING. Where the system or the file system has no such call, it checks and renames (the gap
// stays, but only there).
func RenameNoReplace(oldpath, newpath string) error {
	err := renameNoReplace(oldpath, newpath)
	if err == errNoSupport {
		if _, err := os.Lstat(newpath); err == nil {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: fs.ErrExist}
		}
		return os.Rename(oldpath, newpath)
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	return nil
}

// errNoSupport means the no-replace rename is not available here.
var errNoSupport = errors.New("no-replace rename not supported")

// ListsName reports whether the folder dir has an entry spelled exactly name. Two paths can reach one file through
// different entries (hard links) or through one entry (another case or Unicode form on a folding file system); only
// in the second case is a "rename" between them a respelling.
func ListsName(dir, name string) bool {
	des, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, d := range des {
		if d.Name() == name {
			return true
		}
	}
	return false
}
