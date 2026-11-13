//go:build !windows

package fileops

// Only Windows needs a special form for long paths (see longpath_windows.go); macOS and Linux take paths of any length
// the file system allows, so lp leaves them as they are.

func lp(p string) string { return p }
