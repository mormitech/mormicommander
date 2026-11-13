package fileops

// A move is a rename when source and target are on the same volume, and a copy followed by a delete when they are
// not. SameVolume decides which, before anything is touched, so the progress dialog can say "moving" or "copying" and
// the free-space check knows whether it applies. Paths that don't exist yet (a target folder that the job will make)
// are judged by their nearest existing parent. A link is judged by the folder it sits in, not by what it points to.
// The device lookup itself is per platform (volume_unix.go, volume_windows.go, volume_other.go).

import (
	"path/filepath"
)

// SameVolume says whether a and b are on the same volume. When it can't tell, it says false, which makes a move
// copy and delete: slower, but always correct.
func SameVolume(a, b string) bool {
	va, ok1 := volumeOf(volumeProbe(a))
	vb, ok2 := volumeOf(volumeProbe(b))
	return ok1 && ok2 && va == vb
}

// volumeProbe is the path whose volume stands for p.
func volumeProbe(p string) string {
	p = existingAncestor(p)
	if fi, err := lstat(p); err == nil && isLink(fi) {
		return filepath.Dir(p)
	}
	return p
}
