//go:build !darwin && !linux && !windows

package dirsize

import "errors"

// Space is not supported on this platform.
func Space(path string) (Usage, error) { return Usage{}, errors.New("unsupported platform") }
