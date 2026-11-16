//go:build !darwin

package disk

// VolumeRootOf is "" here: Windows drive letters are roots already, and Linux mounts keep their folder above.
func VolumeRootOf(string) string { return "" }
