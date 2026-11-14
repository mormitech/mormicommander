//go:build !darwin && !linux

// On Windows and the other systems extended attributes are either absent or behind calls mormiCommander does not
// use, so Replace leaves them out there.
package robust

func copyXattrs(src, dst string) {}
