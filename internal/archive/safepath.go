// This file is the zip-slip guard. An archive entry name is attacker
// controlled: it can be absolute, contain "..", or (on Windows) contain drive
// letters, reserved device names or characters the file system rejects. Every
// path that extraction writes to goes through SafeJoin, which cleans the
// entry path, makes each element legal on the current platform, joins it to
// the target folder and then proves the result is still inside that folder.
// A second check, insideReal, resolves symbolic links of the parent folder so
// an archive cannot first create a link pointing elsewhere and then write
// through it.
package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SafeJoin returns the local path for entry path rel below dest. The entry
// path is cleaned (see CleanPath) and each element is passed through
// SanitizeName so it is valid on this operating system. The function fails
// with ErrInsecurePath when the result would not be strictly inside dest or
// rel cleans to nothing.
func SafeJoin(dest, rel string) (string, error) {
	clean, _ := CleanPath(rel)
	if clean == "" {
		return "", fmt.Errorf("%w: %q", ErrInsecurePath, rel)
	}
	parts := strings.Split(clean, "/")
	for i, p := range parts {
		parts[i] = SanitizeName(p)
	}
	full := filepath.Join(append([]string{dest}, parts...)...)
	r, err := filepath.Rel(dest, full)
	if err != nil || r == "." || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
		return "", fmt.Errorf("%w: %q", ErrInsecurePath, rel)
	}
	return full, nil
}

// insideReal reports whether dir, after resolving symbolic links, is inside
// (or equal to) realDest, which must itself already be resolved.
func insideReal(realDest, dir string) bool {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	if samePath(real, realDest) {
		return true
	}
	r, err := filepath.Rel(realDest, real)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

// realDir resolves symbolic links in an existing folder path, falling back to
// the absolute path when that fails.
func realDir(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	if a, err := filepath.Abs(dir); err == nil {
		return a
	}
	return dir
}

// linkStaysInside reports whether a symbolic link stored at entry path rel
// with the given target would point inside the extraction root. Absolute
// targets never do.
func linkStaysInside(rel, target string) bool {
	t := strings.ReplaceAll(target, "\\", "/")
	if t == "" || strings.HasPrefix(t, "/") || filepath.IsAbs(target) || (len(t) >= 2 && t[1] == ':') {
		return false
	}
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	depth := 0
	if dir != "." {
		depth = len(strings.Split(dir, "/"))
	}
	for _, p := range strings.Split(t, "/") {
		switch p {
		case "", ".":
		case "..":
			depth--
			if depth < 0 {
				return false
			}
		default:
			depth++
		}
	}
	return true
}

func existsLocal(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
