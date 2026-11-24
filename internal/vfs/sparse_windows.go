package vfs

import "io/fs"

// sparseDisk: the listing on Windows does not read the size on disk (that needs a call per file), so a sparse file
// shows its length there (see sparse_unix.go).
func sparseDisk(info fs.FileInfo, flags int) int64 { return -1 }
