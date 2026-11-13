package fileops

// Windows limits ordinary paths to 260 characters (MAX_PATH) unless they carry the \\?\ prefix, which also turns off
// the system's own path cleaning. lp adds the prefix to absolute paths that come close to the limit (248 characters,
// the limit for folders), in the UNC form \\?\UNC\server\share for network paths, so copies of deep trees work.

import (
	"path/filepath"
	"strings"
)

const longPathLimit = 248

func lp(p string) string {
	if len(p) < longPathLimit || strings.HasPrefix(p, `\\?\`) || strings.HasPrefix(p, `\\.\`) {
		return p
	}
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return p
		}
		p = abs
	}
	p = filepath.Clean(p)
	if strings.HasPrefix(p, `\\`) {
		return `\\?\UNC\` + p[2:]
	}
	return `\\?\` + p
}
