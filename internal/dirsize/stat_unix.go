//go:build darwin || linux

// On macOS and Linux everything a size calculation needs comes with the lstat call that reading a folder already does:
// the logical size, the number of 512-byte blocks the file occupies (its size on disk, which is smaller for sparse
// files and larger for small files), the device it lives on (for "stay on this volume") and the inode with its link
// count (for counting hard links once). So no extra system calls are made per file here.
package dirsize

import (
	"io/fs"
	"syscall"
)

// statEnv carries per-calculation platform state; Unix needs none.
type statEnv struct{}

func newStatEnv(root string, quick bool) *statEnv { return &statEnv{} }

func fileStat(path string, fi fs.FileInfo, env *statEnv, wantID, allIDs bool) entryStat {
	st := entryStat{size: fi.Size(), alloc: fi.Size()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		st.alloc = int64(sys.Blocks) * 512
		st.links = uint64(sys.Nlink)
		st.id = fileID{dev: uint64(sys.Dev), ino: uint64(sys.Ino)}
		st.hasID = true
	}
	return st
}

func dirDev(path string, fi fs.FileInfo, env *statEnv) (uint64, bool) {
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return canonicalDev(uint64(sys.Dev)), true
}

func dirID(path string, fi fs.FileInfo, env *statEnv) (fileID, bool) {
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}, false
	}
	return fileID{dev: uint64(sys.Dev), ino: uint64(sys.Ino)}, true
}
