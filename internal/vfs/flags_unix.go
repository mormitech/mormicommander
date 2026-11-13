//go:build !windows

package vfs

import "io/fs"

// unixModeFlags reads the executable and read-only marks from the permission bits.
func unixModeFlags(info fs.FileInfo) int {
	f := 0
	m := info.Mode()
	if m.IsRegular() && m.Perm()&0o111 != 0 {
		f |= FlagExec
	}
	if m.Perm()&0o222 == 0 {
		f |= FlagReadOnly
	}
	return f
}
