//go:build !windows

package lister

import "os"

// keepOpen: on Unix an open file never stops other programs from renaming,
// replacing or deleting it, so a File keeps its handle for its lifetime and
// keeps reading the file it opened until Reload notices a replacement.
const keepOpen = true

// openShared opens path for reading. On Unix os.Open is enough.
func openShared(path string) (*os.File, error) { return os.Open(path) }
