//go:build !unix && !windows

package fileops

// On systems without a device number to compare (Plan 9, WebAssembly) the engine can't tell volumes apart, so it
// reports "not the same volume": a move then copies and deletes, which is slower than a rename but always correct.

func volumeOf(p string) (string, bool) { return "", false }

func isCrossDeviceOS(err error) bool { return false }
