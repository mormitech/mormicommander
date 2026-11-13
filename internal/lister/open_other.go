//go:build !windows

package lister

import "os"

// openShared opens path for reading. On Unix an open file never stops other
// programs from renaming, replacing or deleting it, so os.Open is enough.
func openShared(path string) (*os.File, error) { return os.Open(path) }
