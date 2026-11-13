//go:build !darwin && !linux && !windows

// Volume details are only implemented for the three platforms mormiCommander ships on. Elsewhere VolumeOf reports an
// error, so the drive info line can simply stay empty, and the package still builds.
package dirsize

import "errors"

// VolumeOf is not supported on this platform.
func VolumeOf(path string) (VolumeInfo, error) {
	return VolumeInfo{Path: path, Root: "/"}, errors.New("dirsize: volume information not supported on this platform")
}
