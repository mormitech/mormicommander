// This file is the fallback for platforms other than macOS, Linux and Windows (the BSDs, illumos and so on). The
// portable fs.FileInfo already gives name, size, modification time and mode, which is enough for every built-in column
// except the extra dates and the owner, so those simply stay empty rather than pulling in per-OS stat layouts we
// cannot test.

//go:build !darwin && !linux && !windows

package listing

import "io/fs"

func fillPlatform(it *Item, fi fs.FileInfo) {}
