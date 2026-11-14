//go:build !darwin && !linux

// On Windows and the other systems extended attributes are absent or behind calls mormiCommander does not use, so
// every file reads as having none and there is nothing to remove.
package xattr

const supported = false

func listNames(string) ([]string, error) { return nil, nil }

func valueSize(string, string) int { return -1 }

func removeName(string, string) error { return nil }

func isAbsent(error) bool { return true }
