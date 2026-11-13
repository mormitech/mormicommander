//go:build unix

// On Unix-like systems a rename is only durable once the folder that holds
// the file has been flushed too, so after an atomic save we fsync the folder.
// Errors are ignored: some file systems (network shares, FUSE) don't support
// syncing a folder, and the rename itself already succeeded.

package config

import "os"

func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	d.Sync()
	d.Close()
}
