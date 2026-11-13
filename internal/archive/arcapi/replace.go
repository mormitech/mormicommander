// This file helps the archive writers replace the right file. Every engine
// that changes an archive writes a complete new copy into a temporary file
// and renames it over the old one, because zip and tar cannot be edited in
// place. When the archive path is a symbolic link, renaming over the path
// would replace the link itself with a regular file and leave the real
// archive unchanged, so the user's change would seem to vanish. The writers
// therefore resolve links first and work on the file the link points to,
// the way an ordinary save through a link behaves.
package arcapi

import (
	"os"
	"path/filepath"
)

// ReplaceTarget returns the path a writer should replace for p: the final
// target when p is a symbolic link (or sits below a linked folder), and p
// itself otherwise. A missing or dangling link resolves as far as possible:
// a dangling link yields its target, so creating an archive through it
// creates the file the link names.
func ReplaceTarget(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	// The file may not exist yet: follow a dangling link by hand, a few
	// levels at most.
	cur := p
	for i := 0; i < 16; i++ {
		fi, err := os.Lstat(cur)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			break
		}
		t, err := os.Readlink(cur)
		if err != nil {
			break
		}
		if !filepath.IsAbs(t) {
			t = filepath.Join(filepath.Dir(cur), t)
		}
		cur = t
	}
	return cur
}
