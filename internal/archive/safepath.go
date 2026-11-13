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
// targets never do. See linkTargetInside for the rules.
func linkStaysInside(rel, target string) bool {
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	depth := 0
	if dir != "." {
		depth = len(strings.Split(dir, "/"))
	}
	return linkTargetInside(depth, target)
}

// linkTargetInside reports whether a link target is safe for a link whose
// folder lies depth levels below the extraction root. Counting ".." alone
// is not enough, because earlier links of the same archive can make a name
// in the middle of the target climb further than its text suggests (with
// x/up -> "..", the target "x/up/.." leaves the root). So, like the tar
// and 7z engines, only leading ".." elements are accepted, at most depth of
// them, and the rest of the target may only descend. A target that only
// descends from inside the root can pass other links only if they were
// checked the same way, so it stays inside.
func linkTargetInside(depth int, target string) bool {
	t := strings.ReplaceAll(target, "\\", "/")
	if t == "" || strings.HasPrefix(t, "/") || filepath.IsAbs(target) || (len(t) >= 2 && t[1] == ':') {
		return false
	}
	up := 0
	leading := true
	for _, p := range strings.Split(t, "/") {
		switch p {
		case "", ".":
		case "..":
			if !leading {
				return false
			}
			up++
		default:
			leading = false
		}
	}
	return up <= depth
}

// realDepth returns how many folders the existing folder dir lies below
// realDest after resolving symbolic links, or -1 when it is not inside.
func realDepth(realDest, dir string) int {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return -1
	}
	if samePath(real, realDest) {
		return 0
	}
	r, err := filepath.Rel(realDest, real)
	if err != nil {
		return -1
	}
	if r == "." {
		return 0
	}
	if r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
		return -1
	}
	return len(strings.Split(filepath.ToSlash(r), "/"))
}

func existsLocal(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
