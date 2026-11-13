// Split, Combine and Decode all write new files next to the files they read,
// and with "Overwrite" allowed an output is opened with O_TRUNC. If an output
// name turns out to be one of the inputs, the input is emptied before it is
// read and then deleted by the cleanup of the failed job: the user's data is
// gone. Comparing absolute path strings is not enough to prevent this,
// because the same file can be reached under many spellings: through a
// symlinked folder, with different letter case on the case-insensitive
// default file systems of macOS and Windows, or through a hard link. This
// file holds the small helper that asks the file system instead: two paths
// are the same file when os.SameFile says so, with the path comparison kept
// as a fallback for names that do not exist yet.

package fileutil

import (
	"os"
	"path/filepath"
)

// fileSet remembers a group of input files so that output paths can be
// checked against them by identity, not only by spelling.
type fileSet struct {
	abs   map[string]bool
	infos []os.FileInfo
}

// newFileSet records the given paths. Paths that cannot be stat'ed are kept
// by their absolute spelling only.
func newFileSet(paths ...string) *fileSet {
	s := &fileSet{abs: map[string]bool{}}
	for _, p := range paths {
		s.add(p)
	}
	return s
}

func (s *fileSet) add(p string) {
	if a, err := filepath.Abs(p); err == nil {
		s.abs[a] = true
	}
	if fi, err := os.Stat(p); err == nil {
		s.infos = append(s.infos, fi)
	}
}

// contains reports whether p names one of the recorded files. os.Stat
// follows symlinks on purpose: opening p for writing would follow them too.
func (s *fileSet) contains(p string) bool {
	if a, err := filepath.Abs(p); err == nil && s.abs[a] {
		return true
	}
	if len(s.infos) == 0 {
		return false
	}
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	for _, src := range s.infos {
		if os.SameFile(fi, src) {
			return true
		}
	}
	return false
}
