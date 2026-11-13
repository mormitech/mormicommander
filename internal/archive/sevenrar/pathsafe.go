// This file keeps extraction inside the folder the user chose. Archive entry
// names are just strings written by whoever made the archive, and a hostile
// archive can name an entry "../../.bashrc", "/etc/passwd", "C:\Windows\x" or
// "\\server\share\x" (the "zip slip" family of attacks), or plant a symbolic
// link first and then a file "through" it. Total Commander and 7-Zip strip
// leading slashes and drive letters and refuse ".." components; we do the
// same, and in addition never write through a symbolic link that already
// exists below the target folder.
//
// Names are also made legal for the local file system: on Windows the
// characters < > : " | ? * and control characters are replaced by "_",
// trailing dots and spaces are removed and reserved device names such as CON
// or LPT1 get a leading "_", so an archive made on a Mac still unpacks.

package sevenrar

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// cleanEntryName turns a raw archive entry name into a slash-separated
// relative path. Backslashes count as separators (Windows packers store
// them), leading slashes, drive letters and UNC prefixes are dropped, and "."
// components and empty components are removed. unsafe is true when the name
// contains a ".." component or a NUL byte; such entries are never extracted.
// The result is "" for names that reduce to nothing (for example "/").
func cleanEntryName(raw string) (clean string, unsafe bool) {
	if strings.IndexByte(raw, 0) >= 0 {
		return strings.ReplaceAll(raw, "\x00", "_"), true
	}
	s := strings.ReplaceAll(raw, "\\", "/")
	// Drive letter ("C:" or "C:/...").
	if len(s) >= 2 && s[1] == ':' && isLetter(s[0]) {
		s = s[2:]
	}
	parts := strings.Split(s, "/")
	out := parts[:0]
	for _, p := range parts {
		switch p {
		case "", ".":
			continue
		case "..":
			unsafe = true
		}
		out = append(out, p)
	}
	return strings.Join(out, "/"), unsafe
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// windowsReserved lists device names that cannot be used as file names on
// Windows, with or without an extension.
var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// legalComponent makes one path component legal on the target system.
func legalComponent(c string, windows bool) string {
	if !windows {
		return c
	}
	var b strings.Builder
	for _, r := range c {
		switch {
		case r < 32, strings.ContainsRune(`<>:"|?*`, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	s := strings.TrimRight(b.String(), ". ")
	if s == "" {
		s = "_"
	}
	stem := strings.ToUpper(s)
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if windowsReserved[strings.TrimRight(stem, " ")] {
		s = "_" + s
	}
	return s
}

// localPath turns a clean relative entry path into a relative local path
// with legal components and native separators.
func localPath(clean string, windows bool) string {
	parts := strings.Split(clean, "/")
	for i, p := range parts {
		parts[i] = legalComponent(p, windows)
	}
	if windows {
		return strings.Join(parts, `\`)
	}
	return filepath.Join(parts...)
}

// targetPath joins dest and a clean relative entry path, and double-checks
// lexically that the result stays inside dest.
func targetPath(dest, clean string) (string, error) {
	if clean == "" {
		return "", fmt.Errorf("%w: empty name", ErrUnsafePath)
	}
	if _, unsafe := cleanEntryName(clean); unsafe {
		return "", fmt.Errorf("%w: %s", ErrUnsafePath, clean)
	}
	t := filepath.Join(dest, localPath(clean, windowsPaths))
	if !within(dest, t) {
		return "", fmt.Errorf("%w: %s", ErrUnsafePath, clean)
	}
	return t, nil
}

// within reports whether p is dest or below it, lexically.
func within(dest, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(dest), filepath.Clean(p))
	if err != nil {
		return false
	}
	return rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// errThroughSymlink is wrapped into ErrUnsafePath when a parent folder of an
// entry is a symbolic link.
var errThroughSymlink = errors.New("path goes through a symbolic link")

// mkdirNoFollow creates the folder dir (below dest) and its missing parents,
// refusing to pass through any existing symbolic link or non-folder. dest
// itself may be a link (the user chose it); only what is below it counts.
func mkdirNoFollow(dest, dir string, perm fs.FileMode) error {
	rel, err := filepath.Rel(dest, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrUnsafePath, dir)
	}
	if rel == "." {
		return os.MkdirAll(dest, 0o755)
	}
	cur := dest
	for _, c := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, c)
		st, err := os.Lstat(cur)
		switch {
		case err == nil && st.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%w: %s: %v", ErrUnsafePath, cur, errThroughSymlink)
		case err == nil && !st.IsDir():
			return fmt.Errorf("sevenrar: %s exists and is not a folder", cur)
		case err == nil:
			continue
		case errors.Is(err, fs.ErrNotExist):
			if err := os.Mkdir(cur, perm|0o700); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

// symlinkTargetOK decides whether a symbolic link at linkRel (clean relative
// path below dest) pointing to target may be created. The target must be
// relative, may climb with leading ".." components only, must stay inside
// dest, and must not pass through an existing link that this extraction did
// not create (ours is the set of links created so far, as clean paths).
// Together with "never write through a link" this keeps every link created
// by an extraction pointing inside the target folder.
func symlinkTargetOK(dest, linkRel, target string, ours map[string]bool) bool {
	t := strings.ReplaceAll(target, "\\", "/")
	if t == "" || strings.HasPrefix(t, "/") || len(t) >= 2 && t[1] == ':' && isLetter(t[0]) || strings.IndexByte(t, 0) >= 0 {
		return false
	}
	seenName := false
	for _, c := range strings.Split(t, "/") {
		switch c {
		case "..":
			if seenName {
				return false
			}
		case "", ".":
		default:
			seenName = true
		}
	}
	resolved := path.Join(path.Dir(linkRel), t)
	if resolved == ".." || strings.HasPrefix(resolved, "../") || path.IsAbs(resolved) {
		return false
	}
	if resolved == "." {
		return true
	}
	cur := ""
	for _, c := range strings.Split(resolved, "/") {
		cur = path.Join(cur, c)
		st, err := os.Lstat(filepath.Join(dest, localPath(cur, windowsPaths)))
		if err != nil {
			break
		}
		if st.Mode()&fs.ModeSymlink != 0 && !ours[cur] {
			return false
		}
	}
	return true
}
