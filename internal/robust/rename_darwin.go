package robust

import "golang.org/x/sys/unix"

func renameNoReplace(oldpath, newpath string) error {
	err := unix.RenamexNp(oldpath, newpath, unix.RENAME_EXCL)
	if err == unix.ENOTSUP || err == unix.ENOSYS {
		return errNoSupport
	}
	return err
}
