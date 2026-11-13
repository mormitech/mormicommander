//go:build unix

// Unix version (macOS, Linux, the BSDs) of the two disk operations that
// differ between platforms. Here a rename atomically replaces the target and
// a file's own permission bits do not stop it from being replaced or
// deleted (only the folder's do), so both are plain os calls.
package syncdirs

import "os"

func replaceFile(oldp, newp string) error { return os.Rename(oldp, newp) }

func removeEntry(p string) error { return os.Remove(p) }
