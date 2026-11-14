//go:build !unix && !windows

// Fallback for systems that are neither Unix nor Windows: every file counts as having one name and ownership is not
// touched, so Replace behaves like a plain rename.
package robust

import "io/fs"

func linkCount(string, fs.FileInfo) uint64 { return 1 }

func keepOwner(fs.FileInfo, string) {}
