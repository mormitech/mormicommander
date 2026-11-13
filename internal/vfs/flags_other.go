//go:build !windows && !darwin

package vfs

import "io/fs"

// sysFlags: Linux and the BSDs have no hidden flag, only dotfiles (List marks those itself).
func sysFlags(info fs.FileInfo, name string) int { return unixModeFlags(info) }
