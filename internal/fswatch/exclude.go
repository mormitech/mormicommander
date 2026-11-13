// Exclusion matching for automatic refresh. Total Commander has a field where the user lists drives and folders that
// must never be watched, typically slow removable media, busy log folders or network roots where notifications
// cause trouble. Here a plain entry excludes that folder and everything below it, and an entry containing a wildcard
// is matched with filepath.Match against the folder and each of its parents, so "/Volumes/*" excludes every mounted
// volume on macOS and "*\Temp" excludes every Temp folder on Windows. Windows paths are compared without case.
package fswatch

import (
	"path/filepath"
	"runtime"
	"strings"
)

// excluded reports whether the cleaned absolute path matches any exclusion pattern.
func excluded(abs string, patterns []string) bool {
	for _, p := range patterns {
		if matchExclude(abs, p) {
			return true
		}
	}
	return false
}

// matchExclude matches one pattern. Empty patterns never match.
func matchExclude(abs, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	fold := runtime.GOOS == "windows"
	if fold {
		abs = strings.ToLower(abs)
		pattern = strings.ToLower(pattern)
	}
	if strings.ContainsAny(pattern, "*?[") {
		pattern = filepath.Clean(pattern)
		for p := abs; ; {
			if ok, _ := filepath.Match(pattern, p); ok {
				return true
			}
			parent := filepath.Dir(p)
			if parent == p {
				return false
			}
			p = parent
		}
	}
	pattern = filepath.Clean(pattern)
	if abs == pattern {
		return true
	}
	prefix := pattern
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(abs, prefix)
}
