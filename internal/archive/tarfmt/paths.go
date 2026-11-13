// This file holds the path rules of the tar engine. Names inside a tar
// archive are whatever the program that wrote it chose: they can start with
// "./" or "/", contain ".." to climb out of the target folder, use
// backslashes, or name Windows devices like "CON". Every name is normalised
// here into the clean slash form the rest of mormiCommander expects, and
// every name and link target is checked before anything touches the disk, so
// a hostile archive can never write outside the folder the user picked.
package tarfmt

import (
	"path"
	"path/filepath"
	"strings"
)

// cleanName normalises an archive member name: forward slashes, no leading
// "./" or "/", no trailing slash, "." and ".." resolved where possible. A
// name that still climbs out (starts with "..") is returned as is so it can
// be listed and then refused at extraction time.
func cleanName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	for strings.HasPrefix(name, "/") {
		name = name[1:]
	}
	if name == "" {
		return ""
	}
	name = path.Clean(name)
	if name == "." {
		return ""
	}
	return name
}

// isLocalName reports whether a cleaned slash name is safe to create under
// the target folder on this operating system. filepath.IsLocal also rejects
// Windows reserved names and drive letters when running on Windows.
func isLocalName(name string) bool {
	if name == "" {
		return false
	}
	if strings.ContainsRune(name, 0) {
		return false
	}
	return filepath.IsLocal(filepath.FromSlash(name))
}

// linkTargetSafe reports whether a symlink stored at linkPath (cleaned, slash
// form, relative to the target folder) pointing at target stays inside the
// target folder. Absolute targets are always refused.
func linkTargetSafe(linkPath, target string) bool {
	if target == "" {
		return false
	}
	t := strings.ReplaceAll(target, "\\", "/")
	if path.IsAbs(t) || filepath.IsAbs(target) || filepath.VolumeName(target) != "" {
		return false
	}
	resolved := path.Join(path.Dir(linkPath), t)
	if resolved == "." {
		// A link to the folder that contains it is harmless.
		return true
	}
	return isLocalName(resolved)
}

// underDir reports whether name equals dir or lies below it. An empty dir
// matches everything (the archive root).
func underDir(name, dir string) bool {
	if dir == "" {
		return true
	}
	return name == dir || strings.HasPrefix(name, dir+"/")
}

// selected reports whether name is picked by the selection list: empty means
// everything, and naming a folder picks everything below it.
func selected(name string, names []string) bool {
	if len(names) == 0 {
		return true
	}
	for _, n := range names {
		if underDir(name, n) {
			return true
		}
	}
	return false
}

// cleanAll normalises a list of selection names.
func cleanAll(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, cleanName(n))
	}
	return out
}
