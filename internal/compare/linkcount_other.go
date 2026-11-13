// This file is the fallback for systems that are neither Unix nor Windows
// (Plan 9, WebAssembly). They offer no portable way to read a hard-link count,
// so every file is treated as having a single name and saved by the usual
// temporary-file-and-rename route.

//go:build !unix && !windows

package compare

import "os"

// linkCount always reports a single link on this platform.
func linkCount(string, os.FileInfo) uint64 { return 1 }
