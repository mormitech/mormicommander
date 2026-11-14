//go:build !darwin && !linux && !windows

package trash

// Other systems (the BSDs, Plan 9, WebAssembly) have no trash this package knows how to use, so Supported reports
// false and every move fails with a plain message; the file operations engine then offers a permanent delete.

import (
	"context"
	"errors"
)

const supported = false

const directSupported = false

var errUnsupported = errors.New("moving to the trash is not supported on this platform")

func moveOne(context.Context, string) error { return errUnsupported }

func moveDirect(context.Context, string) error { return errUnsupported }

func moveMany(context.Context, []string) error { return errUnsupported }
