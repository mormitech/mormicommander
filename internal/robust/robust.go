// Package robust replaces files in a way that survives Windows' habit of briefly locking files that were just
// written. On Windows, virus scanners and the search indexer open a freshly written file for a moment, and renaming
// over a file that someone has open fails with "Access is denied" or a sharing violation. The archive writers build
// the new archive in a temporary file and rename it over the old one, so without a retry an update can fail at random.
// Go's own toolchain does the same thing (cmd/internal/robustio). On macOS and Linux Rename is a plain os.Rename.
package robust

import (
	"os"
	"time"
)

// retryFor is how long Rename keeps trying on Windows before it gives up and returns the last error.
var retryFor = 2 * time.Second

// Rename renames from to to, replacing to if it exists. On Windows it retries for a short while when the error is
// one that a scanner or indexer holding the file open causes.
func Rename(from, to string) error {
	err := os.Rename(from, to)
	if err == nil || !retryable(err) {
		return err
	}
	deadline := time.Now().Add(retryFor)
	wait := 5 * time.Millisecond
	for time.Now().Before(deadline) {
		time.Sleep(wait)
		if err = os.Rename(from, to); err == nil || !retryable(err) {
			return err
		}
		if wait < 200*time.Millisecond {
			wait *= 2
		}
	}
	return err
}

// retryable reports whether err is a transient lock error (only ever true on Windows).
func retryable(err error) bool { return isTransient(err) }
