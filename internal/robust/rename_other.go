//go:build !darwin && !linux && !windows

package robust

func renameNoReplace(oldpath, newpath string) error { return errNoSupport }
