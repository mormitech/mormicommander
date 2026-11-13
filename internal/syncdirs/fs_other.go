//go:build !unix && !windows

// Fallback for platforms that are neither Unix nor Windows (wasm, plan9).
// mormiCommander does not ship there, but the package should still build, so
// the helpers simply use the portable os calls.
package syncdirs

import "os"

func replaceFile(oldp, newp string) error { return os.Rename(oldp, newp) }

func removeEntry(p string) error { return os.Remove(p) }
