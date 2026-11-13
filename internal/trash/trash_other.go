//go:build !darwin && !linux && !windows

package trash

// Other systems (the BSDs, Plan 9, WebAssembly) have no trash this package knows how to use, so Supported reports
// false and every move fails with a plain message; the file operations engine then offers a permanent delete.

import "errors"

const supported = false

var errUnsupported = errors.New("moving to the trash is not supported on this platform")

func moveOne(path string) error { return errUnsupported }

func moveMany(paths []string) error { return errUnsupported }
