package robust

import "golang.org/x/sys/unix"

func renameNoReplace(oldpath, newpath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, unix.RENAME_NOREPLACE)
	if err == unix.ENOSYS || err == unix.EINVAL || err == unix.ENOTSUP {
		return errNoSupport // an old kernel, or a file system without the flag
	}
	return err
}
