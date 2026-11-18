//go:build !windows

package fileops

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// hiddenTarget says whether a file that would be overwritten counts as hidden: on macOS and Linux a name that starts
// with a dot.
func hiddenTarget(path string, _ fs.FileInfo) bool {
	return strings.HasPrefix(filepath.Base(path), ".")
}
