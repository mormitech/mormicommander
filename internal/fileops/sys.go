package fileops

// Every file system call of the engine goes through these thin wrappers so that long paths work on Windows: lp adds
// the \\?\ prefix to paths of 248 characters and more (longpath_windows.go), which lifts the old 260-character MAX_PATH
// limit, and is a no-op elsewhere. Paths are kept in their normal form everywhere else (in progress, errors and logs),
// and only get the prefix at the moment they are handed to the system.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// LongPath returns p in the form the system accepts for long paths: with the \\?\ prefix on Windows when p is long,
// unchanged on other systems. Code outside the engine that calls the system with paths from a copy can use it too.
func LongPath(p string) string { return lp(p) }

func lstat(p string) (fs.FileInfo, error) { return os.Lstat(lp(p)) }
func stat(p string) (fs.FileInfo, error)  { return os.Stat(lp(p)) }
func open(p string) (*os.File, error)     { return os.Open(lp(p)) }
func openFile(p string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(lp(p), flag, perm)
}
func mkdir(p string, perm fs.FileMode) error      { return os.Mkdir(lp(p), perm) }
func mkdirAll(p string, perm fs.FileMode) error   { return os.MkdirAll(lp(p), perm) }
func remove(p string) error                       { return os.Remove(lp(p)) }
func rename(a, b string) error                    { return os.Rename(lp(a), lp(b)) }
func chmod(p string, m fs.FileMode) error         { return os.Chmod(lp(p), m) }
func readlink(p string) (string, error)           { return os.Readlink(lp(p)) }
func symlink(target, p string) error              { return os.Symlink(target, lp(p)) }
func readDir(p string) ([]os.DirEntry, error)     { return os.ReadDir(lp(p)) }
func truncate(p string, size int64) error         { return os.Truncate(lp(p), size) }
func sameFile(a, b fs.FileInfo) bool              { return a != nil && b != nil && os.SameFile(a, b) }
func isNotExist(err error) bool                   { return errors.Is(err, fs.ErrNotExist) }
func isPerm(err error) bool                       { return errors.Is(err, fs.ErrPermission) }
func readOnly(fi fs.FileInfo) bool                { return fi.Mode().IsRegular() && fi.Mode().Perm()&0o200 == 0 }
func isLink(fi fs.FileInfo) bool                  { return fi.Mode()&fs.ModeSymlink != 0 }
func makeWritable(p string, fi fs.FileInfo) error { return chmod(p, fi.Mode().Perm()|0o200) }

// plainErr makes a system error a short plain phrase for the UI ("permission denied", "the disk is full").
func plainErr(err error) string {
	if err == nil {
		return ""
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		err = le.Err
	}
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	case errors.Is(err, fs.ErrNotExist):
		return "it is not there any more"
	case errors.Is(err, syscall.ENOSPC):
		return "the disk is full"
	case errors.Is(err, errVerify):
		return "the copy differs from the original"
	}
	return err.Error()
}

var errVerify = errors.New("verification failed")

// inside says whether p is the folder dir itself or anything under it.
func inside(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// intoItself says whether p is the folder src (whose Lstat is si) or lies below it. The spelling alone is not enough:
// a target reached through a link to a subfolder of src, or spelled in another case on a case-insensitive disk, is
// below src too. So the part of p that exists is resolved to its real path, and every folder on that path is also
// compared with src by identity.
func intoItself(p, src string, si fs.FileInfo) bool {
	if inside(p, src) {
		return true
	}
	q := existingAncestor(p)
	if real, err := filepath.EvalSymlinks(q); err == nil {
		q = real
	}
	for ; ; q = filepath.Dir(q) {
		if fi, err := stat(q); err == nil && sameFile(fi, si) {
			return true
		}
		if filepath.Dir(q) == q {
			return false
		}
	}
}

// samePath says whether two paths name the same file: equal after cleaning, or the same file on disk (which catches
// case-insensitive systems, so "copy a.txt onto A.TXT" is refused instead of truncating the source, and hard links).
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	ai, err1 := lstat(a)
	bi, err2 := lstat(b)
	return err1 == nil && err2 == nil && sameFile(ai, bi)
}

// existingAncestor is p, or its nearest parent that exists (a target folder may not be made yet).
func existingAncestor(p string) string {
	p = filepath.Clean(p)
	for {
		if _, err := lstat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return p
		}
		p = parent
	}
}
