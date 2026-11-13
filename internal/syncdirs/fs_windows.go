// Windows version of the two disk operations that behave differently there.
// Windows refuses to replace or delete a file that has the read-only
// attribute, while Total Commander's sync happily overwrites such files
// (after its overwrite confirmation), so both helpers clear the attribute
// first. Go's os.Rename already uses MoveFileEx with REPLACE_EXISTING, so a
// plain rename replaces the target once it is writable.
package syncdirs

import (
	"errors"
	"io/fs"
	"os"
)

func clearReadOnly(p string) {
	fi, err := os.Lstat(p)
	if err != nil || fi.Mode()&fs.ModeSymlink != 0 || fi.IsDir() {
		return
	}
	if fi.Mode().Perm()&0o200 == 0 {
		_ = os.Chmod(p, fi.Mode().Perm()|0o200)
	}
}

func replaceFile(oldp, newp string) error {
	clearReadOnly(newp)
	return os.Rename(oldp, newp)
}

func removeEntry(p string) error {
	err := os.Remove(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		clearReadOnly(p)
		err = os.Remove(p)
	}
	return err
}
