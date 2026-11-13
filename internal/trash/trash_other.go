//go:build !darwin && !linux && !windows

package trash

import "errors"

const supported = false

// Move is not available on this platform.
func Move(path string) error {
	return errors.New("moving to the trash is not supported on this platform")
}
