//go:build !unix && !windows

package fileops

// Systems that are neither Unix-like nor Windows (Plan 9, WebAssembly) get the portable part of a copy only: bytes,
// mode bits and modification date. Owner, access and creation dates, attributes and extended attributes are left out.

import (
	"io/fs"
	"time"
)

func copyOwner(dst string, info fs.FileInfo) error       { return nil }
func setCreationTime(dst string, info fs.FileInfo) error { return nil }
func copyPlatformAttrs(src, dst string) error            { return nil }
func copyXattrs(src, dst string) error                   { return nil }
func atimeOf(info fs.FileInfo) time.Time                 { return info.ModTime() }
