//go:build !darwin && !windows

package cloudfile

import "io/fs"

// Linux and the BSDs record no placeholder state a program can read.
func placeholder(fs.FileInfo) bool { return false }
